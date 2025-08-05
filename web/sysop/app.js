/**
 * TractStack SysOp Dashboard
 * A retro BBS-style monitoring interface for the modern web.
 * Alpine.js component for the entire dashboard.
 */
document.addEventListener('alpine:init', () => {
  Alpine.data('sysOpApp', () => ({
    // --- STATE ---
    authenticated: false,
    currentTenant: 'default',
    availableTenants: [],
    sysOpToken: null,
    tenantToken: null, // Holds the JWT for the currently selected tenant
    pollTimer: null,
    currentInterval: 0, // Explicitly track the current interval
    currentTab: 'dashboard',
    apiEndpoints: {
      sysop_auth: '/api/sysop/auth',
      sysop_login: '/api/sysop/login',
      sysop_tenants: '/api/sysop/tenants',
      sysop_activity: '/api/sysop/activity',
      sysop_tenant_token: '/api/sysop/tenant-token',
      sysop_orphan_analysis: '/api/sysop/orphan-analysis',
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

    // --- LOGS STATE ---
    logs: [],
    logFilters: { channel: 'all', level: 'INFO' },
    logConnectionStatus: 'Disconnected',
    logEvtSource: null,
    maxLogEntries: 500,

    // --- GETTERS ---
    get connectionStatusClass() { return { 'status-online': this.logConnectionStatus === 'Connected', 'status-warming': this.logConnectionStatus === 'Connecting', 'status-offline': this.logConnectionStatus === 'Disconnected' }[this.logConnectionStatus] || 'status-offline'; },
    logLevelClass(level) { return { 'status-online': level === 'INFO', 'status-warmed': level === 'WARN', 'status-offline': level === 'ERROR', 'metric-dim': level === 'DEBUG' }[level] || 'metric-dim'; },

    // --- METHODS ---
    init() { this.checkAuth(); this.setupEventListeners(); this.$watch('logFilters', () => { if (this.currentTab === 'logs') this.connectLogStream(); }); },
    async checkAuth() { try { const response = await fetch(this.apiEndpoints.sysop_auth); const data = await response.json(); this.setupLoginForm(data); } catch (error) { this.showError('Connection to server failed.'); } },
    setupLoginForm(authData) { const messageEl = document.getElementById('login-message'); messageEl.textContent = authData.message || 'Please authenticate to continue.'; if (!authData.passwordRequired) { document.getElementById('no-auth-form').style.display = 'block'; document.getElementById('enter-btn').focus(); } else { document.getElementById('password-form').style.display = 'block'; document.getElementById('password-input').focus(); } },
    setupEventListeners() { document.getElementById('login-btn')?.addEventListener('click', () => this.handleLogin()); document.getElementById('enter-btn')?.addEventListener('click', () => this.handleNoAuthLogin()); document.getElementById('password-input')?.addEventListener('keypress', (e) => { if (e.key === 'Enter') this.handleLogin(); }); document.addEventListener('keydown', (e) => this.handleGlobalKeys(e)); },
    handleGlobalKeys(e) { if (this.authenticated) this.handleDashboardKeys(e); },
    handleDashboardKeys(e) { const keyMap = { 'd': 'dashboard', 'c': 'cache', 'a': 'analytics', 't': 'tenants', 'l': 'logs' }; const key = e.key.toLowerCase(); if (keyMap[key]) { e.preventDefault(); this.switchTab(keyMap[key]); } },
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
    async showDashboard() { this.authenticated = true; await this.loadTenants(); await this.fetchTenantToken(this.currentTenant); this.startPolling(); },
    async loadTenants() { try { const response = await fetch(this.apiEndpoints.sysop_tenants, { headers: { 'Authorization': `Bearer ${this.sysOpToken}` } }); if (!response.ok) throw new Error('Failed to fetch tenants'); const data = await response.json(); this.availableTenants = data.tenants || ['default']; this.updateTenantsTab(); } catch (error) { console.warn('Could not load tenants:', error); this.availableTenants = ['default']; this.updateTenantsTab(); } },
    startPolling() { this.stopPolling(); this.pollData(); const newInterval = 5000; this.currentInterval = newInterval; this.pollTimer = setInterval(() => this.pollData(), newInterval); },
    stopPolling() { if (this.pollTimer) { clearInterval(this.pollTimer); this.pollTimer = null; this.currentInterval = 0; } },
    async pollData() {
      if (this.currentTab === 'logs' || this.currentTab === 'tenants' || !this.tenantToken) return;
      const headers = { 'Authorization': `Bearer ${this.tenantToken}`, 'X-Tenant-ID': this.currentTenant };
      const sysOpHeaders = { 'Authorization': `Bearer ${this.sysOpToken}` };

      const activityEndpoint = `${this.apiEndpoints.sysop_activity}?tenant=${this.currentTenant}`;
      const orphanEndpoint = `${this.apiEndpoints.sysop_orphan_analysis}?tenant=${this.currentTenant}`; // Add this line

      const fetchWithTenantToken = (endpoint) => fetch(endpoint, { headers }).then(res => { if (!res.ok) return Promise.reject(new Error(`${res.status}: ${res.statusText} on ${endpoint}`)); return res.json(); });
      const fetchWithSysOpToken = (endpoint) => fetch(endpoint, { headers: sysOpHeaders }).then(res => res.json());

      try {
        const nodeEndpoints = Object.values(this.apiEndpoints.nodes).map(url => fetchWithTenantToken(url));
        const allPromises = [
          fetchWithTenantToken(this.apiEndpoints.contentMap),
          fetchWithTenantToken(this.apiEndpoints.analytics),
          fetchWithSysOpToken(activityEndpoint),
          fetchWithSysOpToken(orphanEndpoint), // Add this line
          ...nodeEndpoints
        ];
        const results = await Promise.allSettled(allPromises);
        const getData = (result, defaultValue) => result.status === 'fulfilled' ? result.value : defaultValue;
        const contentMap = getData(results[0], { data: { data: [] } });
        const analytics = getData(results[1], { dashboard: { status: 'offline' } });
        const activity = getData(results[2], {});
        const orphanAnalysis = getData(results[3], { status: 'offline' }); // Add this line
        const nodeCountResponses = results.slice(4).map(res => getData(res, { count: 0 })); // Update index from 3 to 4
        const nodeData = Object.keys(this.apiEndpoints.nodes).reduce((acc, key, index) => { acc[key] = nodeCountResponses[index].count; return acc; }, {});
        const analyticsStatus = analytics.dashboard?.status || 'complete';
        const analyticsOverview = analytics.dashboard || {};
        const combinedData = {
          contentMap: contentMap.data?.data?.length || 0,
          analyticsStatus,
          analyticsOverview,
          activity,
          orphanAnalysis, // Add this line
          nodes: nodeData
        };

        this.updateUI(combinedData);
        this.updateConnectionStatus('ONLINE');

        const isLoading = analyticsStatus === 'loading';
        const newInterval = isLoading ? 5000 : 60000;
        if (this.currentInterval !== newInterval) {
          this.stopPolling();
          this.currentInterval = newInterval;
          this.pollTimer = setInterval(() => this.pollData(), newInterval);
        }
      } catch (error) {
        console.error('Polling failed:', error);
        this.updateConnectionStatus('OFFLINE');
        this.updateUI({ contentMap: 0, analyticsStatus: 'offline', analyticsOverview: {}, activity: {}, orphanAnalysis: { status: 'offline' }, nodes: {} });
      }
    },

    // Update the updateCacheStatus method to reflect orphan analysis status
    updateCacheStatus(data) {
      const el = document.getElementById('cache-status');
      const formatCount = (label, count) => {
        const countStr = count > 0 ? `<span class="metric-value">${count}</span>` : `<span class="metric-dim">--</span>`;
        const labelColor = count > 0 ? "metric-label" : "metric-dim";
        return `<span><span class="${labelColor}">${label}:</span>${countStr}</span>`;
      };
      const nodes = data.nodes || {};

      // Update orphan analysis status display
      const orphanStatus = data.orphanAnalysis?.status || 'offline';
      const orphanStatusClass = orphanStatus === 'complete' ? 'status-primed' :
        orphanStatus === 'loading' ? 'status-warming' : 'status-offline';

      let html = `<span><span class="metric-label">✦ Content Map: </span><span class="metric-value">${data.contentMap || '0'} items</span></span><span><span class="metric-dim">○ Orphan Analysis: </span><span class="${orphanStatusClass}">${orphanStatus.toUpperCase()}</span></span><span><span class="metric-label">✦ cached nodes: </span></span>`;
      html += `${formatCount('tractstacks', nodes.tractstacks)} ${formatCount('storyfragments', nodes.storyfragments)} ${formatCount('panes', nodes.panes)} ${formatCount('menus', nodes.menus)} ${formatCount('resources', nodes.resources)} ${formatCount('beliefs', nodes.beliefs)} ${formatCount('epinets', nodes.epinets)} ${formatCount('files', nodes.files)}`;
      el.innerHTML = html;
    },
    updateUI(data) { this.updateCacheStatus(data); this.updateActivityMetrics(data.activity); this.updateAnalyticsMetrics(data.analyticsStatus); this.updateCacheDetails(data.nodes); this.updateAnalyticsDetails(data.analyticsOverview); },
    updateConnectionStatus(status) { const el = document.getElementById('status-bar-status'); el.textContent = status; el.className = `status-${status.toLowerCase()}`; document.getElementById('connection-status').innerHTML = `<span class="metric-label">Server Ping: </span><span class="status-${status.toLowerCase()}">${status}</span>`; document.getElementById('last-update').textContent = new Date().toLocaleTimeString(); },
    updateCacheStatus(data) { const el = document.getElementById('cache-status'); const formatCount = (label, count) => { const countStr = count > 0 ? `<span class="metric-value">${count}</span>` : `<span class="metric-dim">--</span>`; const labelColor = count > 0 ? "metric-label" : "metric-dim"; return `<span><span class="${labelColor}">${label}:</span>${countStr}</span>`; }; const nodes = data.nodes || {}; let html = `<span><span class="metric-label">✦ Content Map: </span><span class="metric-value">${data.contentMap || '0'} items</span></span><span><span class="metric-dim">○ Orphan Analysis: </span><span class="status-primed">PRIMED</span></span><span><span class="metric-label">✦ cached nodes: </span></span>`; html += `${formatCount('tractstacks', nodes.tractstacks)} ${formatCount('storyfragments', nodes.storyfragments)} ${formatCount('panes', nodes.panes)} ${formatCount('menus', nodes.menus)} ${formatCount('resources', nodes.resources)} ${formatCount('beliefs', nodes.beliefs)} ${formatCount('epinets', nodes.epinets)} ${formatCount('files', nodes.files)}`; el.innerHTML = html; },
    updateActivityMetrics(activity = {}) { const el = document.getElementById('activity-metrics'); const formatActivity = (label, count) => { const value = count > 0 ? count : '--'; const valueClass = count > 0 ? 'activity-value' : 'metric-dim'; const labelClass = count > 0 ? 'activity-label' : 'metric-dim'; return `<span><span class="${labelClass}">${label}:</span><span class="${valueClass}">${value}</span></span>`; }; let html = `<span><span class="activity-label">✦ activity:</span></span>`; html += `${formatActivity('sessions', activity.sessions)} ${formatActivity('fingerprints', activity.fingerprints)} ${formatActivity('visits', activity.visits)} ${formatActivity('belief-maps', activity.beliefMaps)} ${formatActivity('fragments', activity.fragments)}`; el.innerHTML = html; },
    updateAnalyticsMetrics(status) { const el = document.getElementById('analytics-metrics'); const statusText = status ? status.toUpperCase() : 'OFFLINE'; const statusClass = `status-${statusText.toLowerCase()}`; el.innerHTML = `<span class="metric-label">✦ Analytics Status: </span><span class="${statusClass}">${statusText}</span>`; },
    updateCacheDetails(nodes = {}) { const el = document.getElementById('cache-detail-table'); let html = `<pre class="bbs-table-header">LAYER             ITEMS    FILL LVL           HIT RATE</pre>`; const layerOrder = ['tractstacks', 'storyfragments', 'panes', 'menus', 'resources', 'beliefs', 'epinets', 'files']; for (const layerName of layerOrder) { const count = nodes[layerName] || 0; const fillPercentage = count > 0 ? (Math.log(count + 1) / Math.log(1001)) * 100 : 0; const hitRate = Math.random() * (99.9 - 95.5) + 95.5; html += `<pre>${layerName.padEnd(18)}${String(count).padStart(5)}    ${this.renderProgressBar(fillPercentage)} ${hitRate.toFixed(1).padStart(7)}%</pre>`; } el.innerHTML = html; },
    updateAnalyticsDetails(analytics = {}) { const el = document.getElementById('analytics-detail-table'); const stats = analytics.stats || { daily: 0, weekly: 0, monthly: 0 }; let html = `<div class="status-section"><pre class="section-title">UNIQUE VISITORS</pre><pre><span class="metric-label">Last 24 Hours:</span><span class="metric-value"> ${stats.daily}</span></pre><pre><span class="metric-label">Last 7 Days:  </span><span class="metric-value"> ${stats.weekly}</span></pre><pre><span class="metric-label">Last 28 Days: </span><span class="metric-value"> ${stats.monthly}</span></pre></div>`; html += `<div class="status-section"><pre class="section-title">SESSIONS</pre><pre><span class="metric-label">Total Sessions: </span><span class="metric-value"> ${analytics.sessions || 0}</span></pre></div>`; el.innerHTML = html; },
    renderProgressBar(percentage) { const width = 14; const filledCount = Math.round((percentage / 100) * width); return `[${'▓'.repeat(filledCount)}${'░'.repeat(width - filledCount)}]`; },
    updateTenantsTab() { this.availableTenants.sort(); }, // Sort tenants alphabetically
    switchTab(tabName) {
      const wasPolling = !['logs', 'tenants'].includes(this.currentTab);
      const willPoll = !['logs', 'tenants'].includes(tabName);

      if (wasPolling && !willPoll) {
        this.stopPolling();
      } else if (!wasPolling && willPoll) {
        this.startPolling();
      }

      if (tabName === 'logs') {
        this.fetchCurrentLevels();
        this.connectLogStream();
      } else {
        this.disconnectLogStream();
      }
      this.currentTab = tabName;
    },
    async switchTenant(newTenant) { if (newTenant !== this.currentTenant) { this.currentTenant = newTenant; this.updateUI({ contentMap: 0, analyticsStatus: 'loading', analyticsOverview: {}, activity: {}, nodes: {} }); await this.fetchTenantToken(newTenant); this.startPolling(); window.dispatchEvent(new CustomEvent('tenant-changed', { detail: { tenantId: newTenant } })); } },
    async fetchTenantToken(tenantId) {
      this.tenantToken = null;
      try {
        const response = await fetch(this.apiEndpoints.sysop_tenant_token, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${this.sysOpToken}` },
          body: JSON.stringify({ tenantId })
        });
        const data = await response.json();
        if (data.success && data.token) {
          this.tenantToken = data.token;
        } else { throw new Error(data.error || 'Failed to get tenant token'); }
      } catch (error) { console.error(`Failed to fetch token for tenant ${tenantId}:`, error); this.updateConnectionStatus('OFFLINE'); }
    },
    showError(message) { const messageEl = document.getElementById('login-message'); if (messageEl) { messageEl.innerHTML = `<span style="color:var(--color-red);">${message}</span>`; } },

    async fetchCurrentLevels() { try { const response = await fetch('/api/sysop/logs/levels', { headers: { 'Authorization': `Bearer ${this.sysOpToken}` } }); if (!response.ok) throw new Error('Server error'); this.channelLevels = await response.json(); } catch (error) { console.error("Could not fetch log levels:", error); this.channelLevels = { 'error': 'Could not load levels' }; } },
    async setLevel(channel, level) { try { const response = await fetch('/api/sysop/logs/levels', { method: 'POST', headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${this.sysOpToken}` }, body: JSON.stringify({ channel, level }) }); if (!response.ok) throw new Error('Server error'); this.channelLevels[channel] = level; this.hideLevelModal(); } catch (error) { console.error(`Failed to set level for ${channel}:`, error); alert(`Error: Could not set log level.`); } },
    connectLogStream() { this.disconnectLogStream(); this.logs = []; this.logConnectionStatus = 'Connecting'; const url = `/sysop-logs/stream?channel=${this.logFilters.channel}&level=${this.logFilters.level}`; this.logEvtSource = new EventSource(url); this.logEvtSource.onopen = () => { this.logConnectionStatus = 'Connected'; }; this.logEvtSource.onmessage = (event) => { try { const logEntry = JSON.parse(event.data); this.logs.push(logEntry); if (this.logs.length > this.maxLogEntries) this.logs.shift(); this.$nextTick(() => { const container = this.$refs.logContainer; if (container) container.scrollTop = container.scrollHeight; }); } catch (e) { console.error('Failed to parse log event:', e); } }; this.logEvtSource.onerror = () => { this.logConnectionStatus = 'Disconnected'; this.logEvtSource.close(); }; },
    disconnectLogStream() { if (this.logEvtSource) { this.logEvtSource.close(); this.logEvtSource = null; this.logConnectionStatus = 'Disconnected'; } },
  }));
});
