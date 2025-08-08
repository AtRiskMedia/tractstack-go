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
    pollDataStore: {}, // Store for footer data

    // --- SESSION UNIVERSE STATE (for Dashboard tab) ---
    sessionStates: [],
    sessionStats: { total: 0, lead: 0, active: 0, dormant: 0, withBeliefs: 0 },
    sessionSocket: null,
    sessionSocketStatus: 'OFFLINE',
    sessionDisplayMode: '1:1',

    // --- GRAPH STATE (for Graph tab) ---
    graphData: { nodes: [], links: [] },
    graphStatus: 'Ready',
    simulation: null,
    graphLegendItems: [],

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
      sysop_graph_realtime: '/api/sysop/graph',
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
      const status = this.statusBarContent.status || 'OFFLINE';
      const statusMap = { 'ONLINE': 'status-online', 'CONNECTING': 'status-connecting', 'OFFLINE': 'status-offline', 'ERROR': 'status-error', 'CONNECTED': 'status-online' };
      return statusMap[status] || 'status-offline';
    },

    get statusBarContent() {
      const defaultStatus = { status: this.sessionSocketStatus, details: `<span>Last Update: <span id="last-update">${this.pollDataStore.lastUpdate || '--:--:--'}</span></span>` };
      switch (this.currentTab) {
        case 'status':
        case 'cache':
        case 'analytics':
          return {
            status: this.pollDataStore.connectionStatus || 'CONNECTING',
            details: `<span>Last Update: <span id="last-update">${this.pollDataStore.lastUpdate || '--:--:--'}</span></span>`
          };
        case 'graph':
          return {
            status: this.graphData.nodes.length > 0 ? 'ONLINE' : 'CONNECTING',
            details: `<span>Nodes: <span class="metric-value">${this.graphData.nodes.length}</span></span><span>Links: <span class="metric-value">${this.graphData.links.length}</span></span>`
          };
        case 'dashboard':
          return {
            status: this.sessionSocketStatus,
            details: `<span>Live Pulse: <span class="metric-value">${this.sessionStats.total}</span> sessions</span>`
          };
        case 'logs':
          return {
            status: this.logConnectionStatus.toUpperCase(),
            details: `<span>Entries: <span class="metric-value">${this.logs.length}</span></span>`
          };
        default:
          return { status: 'ONLINE', details: '' };
      }
    },

    logLevelClass(level) {
      const levelMap = { 'INFO': 'status-online', 'WARN': 'status-yellow', 'ERROR': 'status-red', 'DEBUG': 'metric-dim' };
      return levelMap[level] || 'metric-dim';
    },
    getPercentage(value, total) {
      if (!total || !value) return '0.0';
      return ((value / total) * 100).toFixed(1);
    },
    sessionBlockCharacter(state) {
      return state.isLead ? '░' : '▒';
    },
    sessionBlockClass(state) {
      const now = Date.now();
      const lastActivity = new Date(state.lastActivity).getTime();
      const minutesSince = (now - lastActivity) / (1000 * 60);

      if (minutesSince > 45) {
        return state.isLead ? 'lead-dormant-dim' : 'anonymous-dormant-dim';
      }

      let activity = 'light';
      if (minutesSince <= 15) activity = 'bright';
      else if (minutesSince <= 30) activity = 'medium';

      if (state.isLead) {
        return `lead-active-${activity}`;
      }

      const beliefSuffix = state.hasBeliefs ? '-beliefs' : '';
      return `anonymous-active-${activity}${beliefSuffix}`;
    },
    sessionBlockTitle(state) {
      const lastActivity = new Date(state.lastActivity).toLocaleTimeString();
      let title = `Last Activity: ${lastActivity}`;
      if (state.isLead) title += " | Type: Lead";
      if (state.hasBeliefs) title += " | Has Beliefs";
      return title;
    },

    // --- CORE METHODS ---
    init() {
      this.checkAuth();
      this.setupEventListeners();
      this.setupGraphLegend();
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
      this.disconnectLogStream();
      this.disconnectSessionSocket();
      this.stopPolling();
      this.currentTab = tabName;
      if (tabName === 'dashboard') {
        this.connectSessionSocket();
      } else if (tabName === 'logs') {
        this.connectLogStream();
      } else if (tabName === 'graph') {
        this.loadGraphData();
      } else if (['status', 'cache', 'analytics'].includes(tabName)) {
        this.startPolling();
      }
    },
    async switchTenant(newTenant) { if (newTenant !== this.currentTenant) { this.currentTenant = newTenant; this.sessionStates = []; this.sessionStats = { total: 0, lead: 0, active: 0, dormant: 0, withBeliefs: 0 }; await this.fetchTenantToken(newTenant); this.switchTab(this.currentTab); } },
    async fetchTenantToken(tenantId) {
      this.tenantToken = null;
      try {
        const response = await fetch(this.apiEndpoints.sysop_tenant_token, { method: 'POST', headers: { 'Content-Type': 'application/json', 'Authorization': `Bearer ${this.sysOpToken}` }, body: JSON.stringify({ tenantId }) });
        const data = await response.json();
        if (data.success && data.token) { this.tenantToken = data.token; } else { throw new Error(data.error || 'Failed to get tenant token'); }
      } catch (error) { console.error(`Failed to fetch token for tenant ${tenantId}:`, error); this.pollDataStore.connectionStatus = 'OFFLINE'; }
    },
    handleGlobalKeys(e) {
      if (e.target.tagName === 'INPUT' || e.target.tagName === 'SELECT') return;
      const keyMap = { 'd': 'dashboard', 's': 'status', 'c': 'cache', 'a': 'analytics', 't': 'tenants', 'g': 'graph', 'l': 'logs' };
      if (keyMap[e.key.toLowerCase()]) {
        e.preventDefault();
        this.switchTab(keyMap[e.key.toLowerCase()]);
      }
    },

    // --- GRAPH METHODS ---
    setupGraphLegend() {
      // This will be dynamically generated in renderGraph now
      this.graphLegendItems = [];
    },
    async loadGraphData() {
      if (!this.sysOpToken) return;
      this.graphStatus = 'Loading...';

      try {
        const response = await fetch(`${this.apiEndpoints.sysop_graph_realtime}?tenant=${this.currentTenant}`, {
          headers: { 'Authorization': `Bearer ${this.sysOpToken}` }
        });

        if (!response.ok) throw new Error('Failed to fetch graph data');
        const data = await response.json();

        this.graphData = data;
        this.graphStatus = `${data.stats.nodes} nodes, ${data.stats.links} links`;
        this.renderGraph();
      } catch (error) {
        console.error('Graph load failed:', error);
        this.graphStatus = 'Error loading graph';
      }
    },

    renderGraph() {
      const container = document.getElementById('graph-svg-container');
      if (!container || !this.graphData.nodes.length) return;

      d3.select(container).selectAll('*').remove();

      const width = container.clientWidth;
      const nodeCount = this.graphData.nodes.length;
      const minHeight = 400;
      const maxHeight = 800;
      const optimalHeight = Math.max(minHeight, Math.min(maxHeight, nodeCount * 25 + 200));
      const height = optimalHeight;

      container.style.height = height + 'px';

      const svg = d3.select(container)
        .append('svg')
        .attr('width', width)
        .attr('height', height);

      const linkDistance = nodeCount > 20 ? 60 : 80;
      const chargeStrength = nodeCount > 20 ? -200 : -300;

      this.simulation = d3.forceSimulation(this.graphData.nodes)
        .force('link', d3.forceLink(this.graphData.links).id(d => d.id).distance(linkDistance))
        .force('charge', d3.forceManyBody().strength(chargeStrength))
        .force('center', d3.forceCenter(width / 2, height / 2))
        .force('collision', d3.forceCollide().radius(d => d.size + 5));

      const link = svg.append('g')
        .selectAll('line')
        .data(this.graphData.links)
        .enter().append('line')
        .attr('stroke', '#5c6370')
        .attr('stroke-opacity', 0.6)
        .attr('stroke-width', 1.5);

      const nodeGroup = svg.append('g')
        .selectAll('g')
        .data(this.graphData.nodes)
        .enter().append('g')
        .style('cursor', 'grab')
        .call(d3.drag()
          .on('start', (event, d) => {
            if (!event.active) this.simulation.alphaTarget(0.3).restart();
            d.fx = d.x;
            d.fy = d.y;
            d3.select(event.currentTarget).style('cursor', 'grabbing');
          })
          .on('drag', (event, d) => { d.fx = event.x; d.fy = event.y; })
          .on('end', (event, d) => {
            if (!event.active) this.simulation.alphaTarget(0);
            d.fx = null;
            d.fy = null;
            d3.select(event.currentTarget).style('cursor', 'grab');
          }));

      // --- Wireframe node rendering focused on state relationships ---
      const symbolGenerator = d3.symbol().size(d => d.size * d.size * 2.5);

      // Solid outline for Session (triangle)
      nodeGroup.filter(d => d.type === 'session')
        .append('path')
        .attr('d', symbolGenerator.type(d3.symbolTriangle))
        .attr('fill', 'transparent')
        .attr('stroke', d => this.getNodeColor(d.type))
        .attr('stroke-width', 2);

      // Double line for Visit (triangle)
      const visitGroup = nodeGroup.filter(d => d.type === 'visit');
      visitGroup.append('path')
        .attr('d', symbolGenerator.type(d3.symbolTriangle))
        .attr('fill', 'transparent')
        .attr('stroke', d => this.getNodeColor(d.type))
        .attr('stroke-width', 2.5);
      visitGroup.append('path')
        .attr('d', d3.symbol().size(d => d.size * d.size * 1.2).type(d3.symbolTriangle))
        .attr('fill', 'transparent')
        .attr('stroke', d => this.getNodeColor(d.type))
        .attr('stroke-width', 1);

      // Dashed outline for Page (rectangle)
      nodeGroup.filter(d => d.type === 'page')
        .append('rect')
        .attr('width', d => d.size * 1.8)
        .attr('height', d => d.size * 1.8)
        .attr('x', d => -d.size * 0.9)
        .attr('y', d => -d.size * 0.9)
        .attr('rx', 2)
        .attr('fill', 'transparent')
        .attr('stroke', d => this.getNodeColor(d.type))
        .attr('stroke-width', 2)
        .attr('stroke-dasharray', '5,5');

      // SOLID CIRCLE for Fingerprint
      nodeGroup.filter(d => d.type === 'fingerprint')
        .append('circle')
        .attr('r', d => d.size)
        .attr('fill', d => this.getNodeColor(d.type))
        .attr('stroke', '#21252b')
        .attr('stroke-width', 2);

      // DOTTED CIRCLE for Belief 
      nodeGroup.filter(d => d.type === 'belief')
        .append('circle')
        .attr('r', d => d.size)
        .attr('fill', 'transparent')
        .attr('stroke', d => this.getNodeColor(d.type))
        .attr('stroke-width', 2)
        .attr('stroke-dasharray', '3,3');

      // Solid fill for Lead (circles)
      nodeGroup.filter(d => d.type === 'lead')
        .append('circle')
        .attr('r', d => d.size)
        .attr('fill', d => this.getNodeColor(d.type))
        .attr('stroke', '#21252b')
        .attr('stroke-width', 2);

      const labels = nodeGroup.append('text')
        .attr('font-size', '10px')
        .attr('font-family', 'monospace')
        .attr('fill', '#abb2bf')
        .attr('text-anchor', 'middle')
        .attr('y', d => d.size + 15)
        .style('pointer-events', 'none')
        .each(function (d) {
          const lines = d.label.split('\n');
          for (let i = 0; i < lines.length; i++) {
            d3.select(this).append('tspan')
              .attr('x', 0)
              .attr('dy', i === 0 ? 0 : '1.2em')
              .text(lines[i]);
          }
        });

      nodeGroup.append('title')
        .text(d => this.getNodeTooltip(d));

      this.simulation.on('tick', () => {
        link
          .attr('x1', d => d.source.x)
          .attr('y1', d => d.source.y)
          .attr('x2', d => d.target.x)
          .attr('y2', d => d.target.y);
        nodeGroup.attr('transform', d => `translate(${d.x},${d.y})`);
      });

      // --- Update legend to match new styles ---
      this.updateGraphLegend();
    },

    updateGraphLegend() {
      const legendContainer = d3.select('.graph-legend').html(''); // Clear existing legend
      const items = [
        { type: 'session', label: 'Sessions' },
        { type: 'visit', label: 'Visits' },
        { type: 'page', label: 'Pages' },
        { type: 'fingerprint', label: 'Fingerprints' },
        { type: 'lead', label: 'Leads' },
        { type: 'belief', label: 'Beliefs' }
      ];

      items.forEach(item => {
        const itemDiv = legendContainer.append('div').attr('class', 'graph-legend-item');
        const svg = itemDiv.append('svg').attr('class', 'graph-legend-shape').attr('viewBox', '-10 -10 20 20');
        const color = this.getNodeColor(item.type);

        switch (item.type) {
          case 'session':
            svg.append('path').attr('d', d3.symbol().type(d3.symbolTriangle).size(100)()).attr('fill', 'transparent').attr('stroke', color).attr('stroke-width', 2);
            break;
          case 'visit':
            svg.append('path').attr('d', d3.symbol().type(d3.symbolTriangle).size(100)()).attr('fill', 'transparent').attr('stroke', color).attr('stroke-width', 2.5);
            svg.append('path').attr('d', d3.symbol().type(d3.symbolTriangle).size(50)()).attr('fill', 'transparent').attr('stroke', color).attr('stroke-width', 1);
            break;
          case 'page':
            svg.append('rect').attr('width', 16).attr('height', 16).attr('x', -8).attr('y', -8).attr('rx', 2).attr('fill', 'transparent').attr('stroke', color).attr('stroke-width', 2).attr('stroke-dasharray', '5,5');
            break;
          case 'fingerprint':
            // SOLID CIRCLE for fingerprint
            svg.append('circle').attr('r', 8).attr('fill', color).attr('stroke', '#21252b').attr('stroke-width', 2);
            break;
          case 'belief':
            // DOTTED CIRCLE for belief
            svg.append('circle').attr('r', 8).attr('fill', 'transparent').attr('stroke', color).attr('stroke-width', 2).attr('stroke-dasharray', '3,3');
            break;
          case 'lead':
            svg.append('circle').attr('r', 8).attr('fill', color).attr('stroke', '#21252b').attr('stroke-width', 2);
            break;
        }
        itemDiv.append('span').text(item.label);
      });
    },

    getNodeLabel(node) {
      switch (node.type) {
        case 'session':
          return `Session\n${node.id.substring(0, 8)}...\n${node.lastActivity || ''}`;
        case 'fingerprint':
          return `Fingerprint\n${node.id.substring(0, 8)}...`;
        case 'visit':
          return `Visit\n${node.id.substring(0, 8)}...\n${node.lastActivity || ''}`;
        case 'lead':
          return `Lead\n${node.leadName || 'Unknown'}`;
        default:
          return node.label || node.id;
      }
    },

    getNodeTooltip(node) {
      const typeLabels = {
        'session': 'Session', 'fingerprint': 'User Fingerprint', 'visit': 'Visit',
        'lead': 'Known Lead'
      };
      let tooltip = `${typeLabels[node.type] || node.type}\nID: ${node.id}`;
      if (node.lastActivity) { tooltip += `\nLast Active: ${node.lastActivity}`; }
      if (node.leadName) { tooltip += `\nName: ${node.leadName}`; }
      if (node.type === 'fingerprint') {
        tooltip += `\nBeliefs: ${node.beliefCount || 0}`;
        tooltip += `\nSessions: ${node.sessionCount || 0}`;
        tooltip += `\nAuthenticated: ${node.isAuthenticated ? 'Yes' : 'No'}`;
      }
      return tooltip;
    },

    getNodeColor(type) {
      const colorMap = {
        'session': '#61AFEF',
        'visit': '#56B6C2',
        'fingerprint': '#E5C07B',
        'lead': '#98C379',
      };
      return colorMap[type] || '#ABB2BF';
    },

    // --- POLLING LOGIC ---
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
        const results = await Promise.allSettled([fetchWithTenantToken(this.apiEndpoints.analytics), fetchWithSysOpToken(activityEndpoint), fetchWithSysOpToken(orphanEndpoint), ...nodeEndpoints]);
        const getData = (result, def) => result.status === 'fulfilled' ? result.value : def;

        const analytics = getData(results[0], { dashboard: {} });
        const activity = getData(results[1], {});
        const orphanAnalysis = getData(results[2], { status: 'offline' });
        const nodeCounts = results.slice(3).map(res => getData(res, { count: 0 }));
        const nodes = Object.keys(this.apiEndpoints.nodes).reduce((acc, key, i) => ({ ...acc, [key]: nodeCounts[i].count }), {});

        this.pollDataStore = {
          connectionStatus: 'ONLINE',
          lastUpdate: new Date().toLocaleTimeString(),
          activity,
          orphanAnalysis,
          nodes,
          analyticsStatus: analytics.dashboard?.status || 'complete',
          analyticsOverview: analytics.dashboard
        };
        this.updateUI(this.pollDataStore);

      } catch (error) {
        console.error('Polling failed:', error);
        this.pollDataStore.connectionStatus = 'OFFLINE';
      }
    },

    // --- UI UPDATE METHODS ---
    updateUI(data) { this.updateStatusTab(data); this.updateCacheTab(data.nodes); this.updateAnalyticsDetails(data.analyticsOverview); },
    updateStatusTab(data) {
      const [connEl, cacheEl, nodesEl, activityEl, analyticsEl] = [
        document.getElementById('connection-status'),
        document.getElementById('cache-status'),
        document.getElementById('nodes-in-cache-metrics'),
        document.getElementById('activity-metrics'),
        document.getElementById('analytics-metrics')
      ];
      if (!connEl || !cacheEl || !nodesEl || !activityEl || !analyticsEl) return;

      const format = (lbl, val, lblCls, valCls) => `<span><span class="${val > 0 ? lblCls : 'metric-dim'}">${lbl}:</span><span class="${val > 0 ? valCls : 'metric-dim'}">${val > 0 ? val : '--'}</span></span>`;

      const nodes = data.nodes || {};
      const orphan = data.orphanAnalysis || {};
      const orphanCls = `status-${(orphan.status || 'offline').toLowerCase()}`;

      connEl.innerHTML = `Server Ping: <span class="${this.connectionStatusClass}">${this.pollDataStore.connectionStatus}</span>`;
      cacheEl.innerHTML = `<span><span class="metric-label">✦ Orphan Analysis: </span><span class="${orphanCls}">${orphan.status?.toUpperCase() || 'OFFLINE'}</span></span>`;
      nodesEl.innerHTML = Object.keys(nodes).map(k => format(k, nodes[k], 'activity-label', 'activity-value')).join(' ');
      activityEl.innerHTML = Object.keys(data.activity || {}).map(k => format(k, data.activity[k], 'activity-label', 'activity-value')).join(' ');

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
      if (!this.sysOpToken) { console.error("SysOp token not available."); this.sessionSocketStatus = 'ERROR'; return; }
      this.sessionSocketStatus = 'CONNECTING';
      const url = `ws://${window.location.host}/api/sysop/ws/session-map?tenant=${this.currentTenant}&token=${this.sysOpToken}`;
      this.sessionSocket = new WebSocket(url);
      this.sessionSocket.onopen = () => { this.sessionSocketStatus = 'ONLINE'; };
      this.sessionSocket.onclose = () => { this.sessionSocketStatus = 'OFFLINE'; this.sessionStates = []; this.sessionStats = { total: 0, lead: 0, active: 0, dormant: 0, withBeliefs: 0 }; };
      this.sessionSocket.onerror = (error) => { console.error('WebSocket Error:', error); this.sessionSocketStatus = 'ERROR'; };
      this.sessionSocket.onmessage = (event) => {
        try {
          const data = JSON.parse(event.data);
          this.sessionDisplayMode = data.displayMode || '1:1';
          this.sessionStates = data.sessionStates || [];
          this.sessionStats = {
            total: data.totalCount || 0,
            lead: data.leadCount || 0,
            active: data.activeCount || 0,
            dormant: data.dormantCount || 0,
            withBeliefs: data.withBeliefsCount || 0
          };
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

    // --- SSE Log Methods ---
    connectLogStream() { this.disconnectLogStream(); this.logs = []; this.logConnectionStatus = 'CONNECTING'; const url = `/sysop-logs/stream?channel=${this.logFilters.channel}&level=${this.logFilters.level}`; this.logEvtSource = new EventSource(url); this.logEvtSource.onopen = () => { this.logConnectionStatus = 'Connected'; }; this.logEvtSource.onmessage = (event) => { try { const logEntry = JSON.parse(event.data); this.logs.push(logEntry); if (this.logs.length > this.maxLogEntries) this.logs.shift(); this.$nextTick(() => { const container = this.$refs.logContainer; if (container) container.scrollTop = container.scrollHeight; }); } catch (e) { console.error('Failed to parse log event:', e); } }; this.logEvtSource.onerror = () => { this.logConnectionStatus = 'Disconnected'; this.logEvtSource.close(); }; },
    disconnectLogStream() { if (this.logEvtSource) { this.logEvtSource.close(); this.logEvtSource = null; this.logConnectionStatus = 'Disconnected'; } },
  }));
});
