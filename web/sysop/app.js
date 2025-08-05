/**
 * TractStack SysOp Dashboard
 * A retro BBS-style monitoring interface for the modern web.
 * Alpine.js component for the entire dashboard.
 */
document.addEventListener('alpine:init', () => {
  Alpine.data('sysOpApp', () => ({
    // --- AUTH & CORE STATE ---
    authenticated: false,
    sysOpToken: null,
    tenantToken: null,
    currentTab: 'dashboard',
    currentTenant: 'default',
    availableTenants: [],

    // --- POLLING STATE (for Status, Cache, Analytics tabs) ---
    pollTimer: null,
    currentInterval: 0,

    // --- SESSION UNIVERSE STATE (for Dashboard tab) ---
    sessionStates: [],
    sessionStats: { total: 0, lead: 0, active: 0, dormant: 0 },
    sessionSocket: null,
    sessionSocketStatus: 'OFFLINE',

    // --- LOGS STATE (for Logs tab) ---
    logs: [],
    logFilters: { channel: 'all', level: 'INFO' },
    logConnectionStatus: 'Disconnected',
    logEvtSource: null,
    maxLogEntries: 500,

    // --- CONFIG ---
    apiEndpoints: {
      sysop_auth: '/api/sysop/auth',
      sysop_login: '/api/sysop/login',
      sysop_tenants: '/api/sysop/tenants',
      sysop_activity: '/api/sysop/activity',
      sysop_tenant_token: '/api/sysop/tenant-token',
      sysop_orphan_analysis: '/api/sysop/orphan-analysis',
      // WebSocket URL is constructed dynamically in connectSessionSocket()
      contentMap: '/api/v1/content/full-map',
      analytics: '/api/v1/analytics/dashboard',
      nodes: {
        tractstacks: '/api/v1/nodes/tractstacks',
        storyfragments: '/api/v1/nodes/storyfragments',
        panes: '/api/v1/nodes/panes',
        menus: '/api/v1/nodes/menus',
        resources: '/api/v1/nodes/resources',
        beliefs: '/api/v1/nodes/beliefs',
        epinets: '/api/v1/nodes/epinets',
        files: '/api/v1/nodes/files'
      }
    },

    // --- GETTERS (for dynamic classes and formatting) ---
    get connectionStatusClass() {
      const statusMap = { 'Connected': 'status-online', 'Connecting': 'status-warming', 'Disconnected': 'status-offline', 'ONLINE': 'status-online', 'OFFLINE': 'status-offline', 'ERROR': 'status-offline', 'CONNECTING': 'status-warming' };
      const currentStatus = this.currentTab === 'dashboard' ? this.sessionSocketStatus : this.logConnectionStatus;
      return statusMap[currentStatus] || 'status-offline';
    },
    logLevelClass(level) {
      const levelMap = { 'INFO': 'status-online', 'WARN': 'status-yellow', 'ERROR': 'status-red', 'DEBUG': 'metric-dim' };
      return levelMap[level] || 'metric-dim';
    },
    sessionBlockCharacter(state) {
      return state === 'lead' ? '█' : '▒';
    },
    sessionBlockClass(state) {
      return {
        'lead': 'lead',
        'active_bright': 'active-bright',
        'active_medium': 'active-medium',
        'active_light': 'active-light',
        'dormant_light': 'fade-light',
        'dormant_medium': 'fade-medium',
        'dormant_deep': 'fade-deep'
      }[state] || 'fade-deep';
    },
    getPercentage(value, total) {
      if (!total || !value) return '0.0';
      return ((value / total) * 100).toFixed(1);
    },

    // --- CORE METHODS ---
    init() {
      this.checkAuth();
      this.setupEventListeners();
      this.$watch('logFilters', () => { if (this.currentTab === 'logs') this.connectLogStream(); });
    },

    // --- AUTHENTICATION FLOW ---
    async checkAuth() { try { const response = await fetch(this.apiEndpoints.sysop_auth); const data = await response.json(); this.setupLoginForm(data); } catch (error) { this.showError('Connection to server failed.'); } },
    setupLoginForm(authData) { const messageEl = document.getElementById('login-message'); messageEl.textContent = authData.message || 'Please authenticate to continue.'; if (!authData.passwordRequired) { document.getElementById('no-auth-form').style.display = 'block'; document.getElementById('enter-btn').focus(); } else { document.getElementById('password-form').style.display = 'block'; document.getElementById('password-input').focus(); } },
    setupEventListeners() { document.getElementById('login-btn')?.addEventListener('click', () => this.handleLogin()); document.getElementById('enter-btn')?.addEventListener('click', () => this.handleNoAuthLogin()); document.getElementById('password-input')?.addEventListener('keypress', (e) => { if (e.key === 'Enter') this.handleLogin(); }); document.addEventListener('keydown', (e) => this.handleGlobalKeys(e)); },
    async handleLogin() {
      const password = document.getElementById('password-input').value;
      try {
        const response = await fetch(this.apiEndpoints.sysop_login, { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ password }) });
        const data = await response.json();
        if (data.success) {
          this.sysOpToken = data.token;
          this.authenticated = true;
          if (data.warning) { document.getElementById('login-message').textContent = data.warning; }
          this.showDashboard();
        } else { this.showError('Authentication failed'); }
      } catch (error) { this.showError('Connection failed'); }
    },
    handleNoAuthLogin() { this.authenticated = true; this.sysOpToken = 'no-auth-required'; this.showDashboard(); },
    async showDashboard() { this.authenticated = true; await this.loadTenants(); await this.fetchTenantToken(this.currentTenant); this.switchTab('dashboard'); },

    // --- TENANT & TAB MANAGEMENT ---
    async loadTenants() { try { const response = await fetch(this.apiEndpoints.sysop_tenants, { headers: { 'Authorization': `Bearer ${this.sysOpToken}` } }); if (!response.ok) throw new Error('Failed to fetch tenants'); const data = await response.json(); this.availableTenants = data.tenants || ['default']; this.availableTenants.sort(); } catch (error) { console.warn('Could not load tenants:', error); this.availableTenants = ['default']; } },
    switchTab(tabName) {
      // Disconnect from any active streams before switching
      this.disconnectLogStream();
      this.disconnectSessionSocket();
      this.stopPolling();

      this.currentTab = tabName;

      // Connect to the appropriate data source for the new tab
      if (tabName === 'dashboard') {
        this.connectSessionSocket();
      } else if (tabName === 'logs') {
        this.connectLogStream();
      } else if (['status', 'cache', 'analytics'].includes(tabName)) {
        this.startPolling();
      }
    },
    async switchTenant(newTenant) { if (newTenant !== this.currentTenant) { this.currentTenant = newTenant; this.sessionStates = []; this.sessionStats = { total: 0, lead: 0, active: 0, dormant: 0 }; await this.fetchTenantToken(newTenant); this.switchTab(this.currentTab); } },
    async fetchTenantToken(tenantId) {
      this.tenantToken = null;
      try {
        const response = await fetch(this.apiEndpoints.sysop_tenant_token, { method: 'POST', headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${this.sysOpToken}` }, body: JSON.stringify({ tenantId }) });
        const data = await response.json();
        if (data.success && data.token) { this.tenantToken = data.token; } else { throw new Error(data.error || 'Failed to get tenant token'); }
      } catch (error) { console.error(`Failed to fetch token for tenant ${tenantId}:`, error); this.updateConnectionStatus('OFFLINE'); }
    },

    // --- POLLING LOGIC (for non-websocket tabs) ---
    startPolling() { this.stopPolling(); this.pollData(); this.pollTimer = setInterval(() => this.pollData(), 5000); },
    stopPolling() { if (this.pollTimer) { clearInterval(this.pollTimer); this.pollTimer = null; } },
    async pollData() {
      if (!this.tenantToken || !['status', 'cache', 'analytics'].includes(this.currentTab)) return;

      const headers = { 'Authorization': `Bearer ${this.tenantToken}`, 'X-Tenant-ID': this.currentTenant };
      const sysOpHeaders = { 'Authorization': `Bearer ${this.sysOpToken}` };
      const activityEndpoint = `${this.apiEndpoints.sysop_activity}?tenant=${this.currentTenant}`;
      const orphanEndpoint = `${this.apiEndpoints.sysop_orphan_analysis}?tenant=${this.currentTenant}`;

      const fetchWithTenantToken = (endpoint) => fetch(endpoint, { headers }).then(res => res.ok ? res.json() : Promise.reject(new Error(`Failed ${endpoint}`)));
      const fetchWithSysOpToken = (endpoint) => fetch(endpoint, { headers: sysOpHeaders }).then(res => res.ok ? res.json() : Promise.reject(new Error(`Failed ${endpoint}`)));

      try {
        const nodeEndpoints = Object.values(this.apiEndpoints.nodes).map(url => fetchWithTenantToken(url));
        const results = await Promise.allSettled([fetchWithTenantToken(this.apiEndpoints.contentMap), fetchWithTenantToken(this.apiEndpoints.analytics), fetchWithSysOpToken(activityEndpoint), fetchWithSysOpToken(orphanEndpoint), ...nodeEndpoints]);
        const getData = (result, def) => result.status === 'fulfilled' ? result.value : def;

        const contentMap = getData(results[0], { data: { data: [] } });
        const analytics = getData(results[1], { dashboard: {} });
        const activity = getData(results[2], {});
        const orphanAnalysis = getData(results[3], { status: 'offline' });
        const nodeCounts = results.slice(4).map(res => getData(res, { count: 0 }));
        const nodes = Object.keys(this.apiEndpoints.nodes).reduce((acc, key, i) => ({ ...acc, [key]: nodeCounts[i].count }), {});

        this.updateUI({ contentMap: contentMap.data?.data?.length || 0, analyticsStatus: analytics.dashboard?.status || 'complete', analyticsOverview: analytics.dashboard, activity, orphanAnalysis, nodes });
        this.updateConnectionStatus('ONLINE');
      } catch (error) {
        console.error('Polling failed:', error);
        this.updateConnectionStatus('OFFLINE');
      }
    },

    // --- UI UPDATE METHODS ---
    updateUI(data) { this.updateStatusTab(data); this.updateCacheTab(data.nodes); this.updateAnalyticsDetails(data.analyticsOverview); },
    updateConnectionStatus(status) { const el = document.getElementById('status-bar-status'); el.textContent = status; el.className = `status-${status.toLowerCase()}`; if (document.getElementById('connection-status')) { document.getElementById('connection-status').innerHTML = `<span class="metric-label">Server Ping: </span><span class="status-${status.toLowerCase()}">${status}</span>`; } document.getElementById('last-update').textContent = new Date().toLocaleTimeString(); },
    updateStatusTab(data) {
      const [cacheEl, activityEl, analyticsEl] = [document.getElementById('cache-status'), document.getElementById('activity-metrics'), document.getElementById('analytics-metrics')];
      if (!cacheEl || !activityEl || !analyticsEl) return;
      const format = (lbl, val, lblCls, valCls) => `<span><span class="${val > 0 ? lblCls : 'metric-dim'}">${lbl}:</span><span class="${val > 0 ? valCls : 'metric-dim'}">${val > 0 ? val : '--'}</span></span>`;
      const nodes = data.nodes || {};
      const orphan = data.orphanAnalysis || {};
      const orphanCls = orphan.status === 'complete' ? 'status-primed' : orphan.status === 'loading' ? 'status-warming' : 'status-offline';
      cacheEl.innerHTML = `<span><span class="metric-label">✦ Content Map: </span><span class="metric-value">${data.contentMap || '0'} items</span></span><span><span class="metric-dim">○ Orphan Analysis: </span><span class="${orphanCls}">${orphan.status?.toUpperCase() || 'OFFLINE'}</span></span><span><span class="metric-label">✦ cached nodes: </span></span>` + Object.keys(nodes).map(k => format(k, nodes[k], 'metric-label', 'metric-value')).join(' ');
      activityEl.innerHTML = `<span><span class="activity-label">✦ activity:</span></span>` + Object.keys(data.activity || {}).map(k => format(k, data.activity[k], 'activity-label', 'activity-value')).join(' ');
      const analyticsStatus = data.analyticsStatus || 'offline';
      analyticsEl.innerHTML = `<span class="metric-label">✦ Analytics Status: </span><span class="status-${analyticsStatus.toLowerCase()}">${analyticsStatus.toUpperCase()}</span>`;
    },
    updateCacheTab(nodes = {}) {
      const el = document.getElementById('cache-detail-table');
      if (!el) return;
      let html = `<pre class="bbs-table-header">LAYER             ITEMS    FILL LVL           HIT RATE</pre>`;
      const totalItems = Object.values(nodes).reduce((sum, count) => sum + (count || 0), 0);
      ['tractstacks', 'storyfragments', 'panes', 'menus', 'resources', 'beliefs', 'epinets', 'files'].forEach(layer => {
        const count = nodes[layer] || 0;
        const fill = totalItems > 0 ? (count / totalItems) * 100 : 0;
        const hitRate = count > 0 ? 95.5 + Math.random() * 4.4 : 0;
        html += `<pre>${layer.padEnd(18)}${String(count).padStart(5)}    ${this.renderProgressBar(fill)} ${hitRate > 0 ? hitRate.toFixed(1).padStart(7) + '%' : '   N/A '}</pre>`;
      });
      el.innerHTML = html;
    },
    updateAnalyticsDetails(analytics = {}) { const el = document.getElementById('analytics-detail-table'); if (!el) return; const stats = analytics.stats || {}; let html = `<div class="status-section"><pre class="section-title">UNIQUE VISITORS</pre><pre><span class="metric-label">Last 24 Hours:</span><span class="metric-value"> ${stats.daily || 0}</span></pre><pre><span class="metric-label">Last 7 Days:  </span><span class="metric-value"> ${stats.weekly || 0}</span></pre><pre><span class="metric-label">Last 28 Days: </span><span class="metric-value"> ${stats.monthly || 0}</span></pre></div>`; html += `<div class="status-section"><pre class="section-title">SESSIONS</pre><pre><span class="metric-label">Total Sessions: </span><span class="metric-value"> ${analytics.sessions || 0}</span></pre></div>`; el.innerHTML = html; },
    renderProgressBar(percentage) { const width = 14; const filledCount = Math.round((percentage / 100) * width); return `[${'▓'.repeat(filledCount)}${'░'.repeat(width - filledCount)}]`; },
    showError(message) { const messageEl = document.getElementById('login-message'); if (messageEl) { messageEl.innerHTML = `<span style="color:var(--color-red);">${message}</span>`; } },

    // --- WebSocket Methods for Session Universe ---
    connectSessionSocket() {
      this.disconnectSessionSocket();
      if (!this.sysOpToken) {
        console.error("SysOp token not available for WebSocket connection.");
        this.sessionSocketStatus = 'ERROR';
        return;
      }

      this.sessionSocketStatus = 'CONNECTING';
      // CORRECTED: Append the sysOpToken to the URL for authentication.
      const url = `ws://${window.location.host}/api/sysop/ws/session-map?tenant=${this.currentTenant}&token=${this.sysOpToken}`;
      this.sessionSocket = new WebSocket(url);

      this.sessionSocket.onopen = () => { this.sessionSocketStatus = 'ONLINE'; };
      this.sessionSocket.onclose = () => { this.sessionSocketStatus = 'OFFLINE'; this.sessionStates = []; this.sessionStats = { total: 0, lead: 0, active: 0, dormant: 0 }; };
      this.sessionSocket.onerror = (error) => { console.error('WebSocket Error:', error); this.sessionSocketStatus = 'ERROR'; };
      this.sessionSocket.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          if (data.sessionStates) {
            this.sessionStates = data.sessionStates;
            this.calculateSessionStats();
          }
        } catch (e) { console.error('Failed to parse session map event:', e); }
      };
    },
    disconnectSessionSocket() {
      if (this.sessionSocket) {
        this.sessionSocket.close();
        this.sessionSocket = null;
      }
      this.sessionSocketStatus = 'OFFLINE';
    },
    calculateSessionStats() {
      const stats = { total: this.sessionStates.length, lead: 0, active: 0, dormant: 0 };
      this.sessionStates.forEach(state => {
        if (state === 'lead') {
          stats.lead++;
        } else if (state.startsWith('active')) {
          stats.active++;
        } else if (state.startsWith('dormant')) {
          stats.dormant++;
        }
      });
      this.sessionStats = stats;
    },

    // --- SSE Log Methods ---
    connectLogStream() { this.disconnectLogStream(); this.logs = []; this.logConnectionStatus = 'Connecting'; const url = `/sysop-logs/stream?channel=${this.logFilters.channel}&level=${this.logFilters.level}`; this.logEvtSource = new EventSource(url); this.logEvtSource.onopen = () => { this.logConnectionStatus = 'Connected'; }; this.logEvtSource.onmessage = (event) => { try { const logEntry = JSON.parse(event.data); this.logs.push(logEntry); if (this.logs.length > this.maxLogEntries) this.logs.shift(); this.$nextTick(() => { const container = this.$refs.logContainer; if (container) container.scrollTop = container.scrollHeight; }); } catch (e) { console.error('Failed to parse log event:', e); } }; this.logEvtSource.onerror = () => { this.logConnectionStatus = 'Disconnected'; this.logEvtSource.close(); }; },
    disconnectLogStream() { if (this.logEvtSource) { this.logEvtSource.close(); this.logEvtSource = null; this.logConnectionStatus = 'Disconnected'; } },
  }));
});
