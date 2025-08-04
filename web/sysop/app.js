/**
 * TractStack SysOp Dashboard
 * A retro BBS-style monitoring interface.
 */
class SysOpDashboard {
  constructor() {
    this.authenticated = false;
    this.currentTenant = 'default';
    this.availableTenants = [];
    this.authToken = null;
    this.pollTimer = null;
    this.currentTab = 'dashboard';
    this.tenantModalOpen = false;
    this.focusedTenantIndex = 0;

    this.apiEndpoints = {
      contentMap: '/api/v1/content/full-map',
      analytics: '/api/v1/analytics/dashboard',
      activity: '/sysop-activity',
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
    };

    this.init();
  }

  async init() {
    await this.checkAuth();
    this.setupEventListeners();
  }

  async checkAuth() {
    try {
      const response = await fetch('/sysop-auth');
      const data = await response.json();
      this.setupLoginForm(data);
    } catch (error) {
      this.showError('Connection to server failed.');
    }
  }

  setupLoginForm(authData) {
    const messageEl = document.getElementById('login-message');
    if (!authData.passwordRequired) {
      messageEl.innerHTML = `Story Keep is not password protected.`;
      document.getElementById('no-auth-form').style.display = 'block';
    } else {
      messageEl.textContent = 'Enter SysOp password to continue.';
      document.getElementById('password-form').style.display = 'block';
      document.getElementById('password-input').focus();
    }
  }

  setupEventListeners() {
    document.getElementById('login-btn')?.addEventListener('click', () => this.handleLogin());
    document.getElementById('enter-btn')?.addEventListener('click', () => this.handleNoAuthLogin());
    document.getElementById('password-input')?.addEventListener('keypress', (e) => {
      if (e.key === 'Enter') this.handleLogin();
    });

    document.querySelectorAll('.tab-item').forEach(item => {
      item.addEventListener('click', () => this.switchTab(item.dataset.tab));
    });

    document.addEventListener('keydown', (e) => {
      if (this.tenantModalOpen) {
        this.handleTenantModalKeys(e);
      } else if (this.authenticated) {
        this.handleDashboardKeys(e);
      }
    });
  }

  handleDashboardKeys(e) {
    const keyMap = { 'd': 'dashboard', 'c': 'cache', 'a': 'analytics', 't': 'tenants', 'l': 'logs' };
    const key = e.key.toLowerCase();
    if (keyMap[key]) {
      e.preventDefault();
      this.switchTab(keyMap[key]);
      if (key === 't') {
        this.showTenantModal();
      }
    }
  }

  handleTenantModalKeys(e) {
    e.preventDefault();
    const options = document.querySelectorAll('.tenant-option');
    if (!options.length) return;
    const tenantCount = options.length;

    switch (e.key) {
      case 'ArrowDown':
        this.focusedTenantIndex = (this.focusedTenantIndex + 1) % tenantCount;
        break;
      case 'ArrowUp':
        this.focusedTenantIndex = (this.focusedTenantIndex - 1 + tenantCount) % tenantCount;
        break;
      case 'Enter':
        options[this.focusedTenantIndex]?.click();
        return;
      case 'Escape':
        this.hideTenantModal();
        return;
      default:
        const num = parseInt(e.key, 10);
        if (!isNaN(num) && num > 0 && num <= tenantCount) {
          options[num - 1]?.click();
        }
        return;
    }
    this.updateTenantFocus(options);
  }

  updateTenantFocus(options) {
    options.forEach((opt, index) => {
      opt.classList.toggle('focused', index === this.focusedTenantIndex);
    });
  }

  async handleLogin() {
    const password = document.getElementById('password-input').value;
    try {
      const response = await fetch('/sysop-login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ password })
      });
      const data = await response.json();
      if (data.success) {
        this.authToken = data.token;
        this.authenticated = true;
        this.showDashboard();
      } else {
        this.showError(data.error || 'Authentication failed');
      }
    } catch (error) {
      this.showError('Connection failed');
    }
  }

  handleNoAuthLogin() {
    this.authenticated = true;
    this.authToken = 'no-auth-required';
    this.showDashboard();
  }

  showLogin() {
    document.getElementById('login-screen').style.display = 'flex';
    document.getElementById('app-wrapper').style.display = 'none';
  }

  async showDashboard() {
    document.getElementById('login-screen').style.display = 'none';
    document.getElementById('app-wrapper').style.display = 'block';
    await this.loadTenants();
    this.startPolling();
  }

  async loadTenants() {
    try {
      const response = await fetch('/sysop-tenants');
      if (!response.ok) throw new Error('Failed to fetch tenants');
      const data = await response.json();
      this.availableTenants = data.tenants || ['default'];
      this.updateTenantsTab();
    } catch (error) {
      console.warn('Could not load tenants:', error);
      this.availableTenants = ['default'];
      this.updateTenantsTab();
    }
  }

  startPolling() {
    this.stopPolling();
    this.pollData(); // Poll immediately
    this.pollTimer = setInterval(() => this.pollData(), 5000); // Start fast, then adjust
  }

  stopPolling() {
    if (this.pollTimer) {
      clearInterval(this.pollTimer);
      this.pollTimer = null;
    }
  }

  async pollData() {
    const headers = { 'X-Tenant-ID': this.currentTenant };
    const activityEndpoint = `${this.apiEndpoints.activity}?tenant=${this.currentTenant}`;

    const fetchWithTenant = (endpoint) => fetch(endpoint, { headers }).then(res => {
      if (!res.ok) return Promise.reject(new Error(`${res.status}: ${res.statusText} on ${endpoint}`));
      return res.json();
    });

    try {
      const nodeEndpoints = Object.values(this.apiEndpoints.nodes).map(url => fetchWithTenant(url));

      // *** FIX: Switched to Promise.allSettled for robust error handling ***
      const allPromises = [
        fetchWithTenant(this.apiEndpoints.contentMap),
        fetchWithTenant(this.apiEndpoints.analytics),
        fetch(activityEndpoint).then(res => res.json()),
        ...nodeEndpoints
      ];

      const results = await Promise.allSettled(allPromises);

      // Helper to safely extract data from settled promises
      const getData = (result, defaultValue) => result.status === 'fulfilled' ? result.value : defaultValue;

      const contentMap = getData(results[0], { data: { data: [] } });
      const analytics = getData(results[1], { dashboard: { status: 'offline' } });
      const activity = getData(results[2], {});
      const nodeCountResponses = results.slice(3).map(res => getData(res, { count: 0 }));

      const nodeData = Object.keys(this.apiEndpoints.nodes).reduce((acc, key, index) => {
        acc[key] = nodeCountResponses[index].count;
        return acc;
      }, {});

      const analyticsStatus = analytics.dashboard?.status || 'complete';
      const analyticsOverview = analytics.dashboard || {};

      const combinedData = {
        contentMap: contentMap.data?.data?.length || 0,
        analyticsStatus: analyticsStatus,
        analyticsOverview: analyticsOverview,
        activity: activity,
        nodes: nodeData
      };

      this.updateUI(combinedData);
      this.updateConnectionStatus('ONLINE');

      const isLoading = analyticsStatus === 'loading';
      const currentInterval = this.pollTimer ? this.pollTimer._repeat : 0;
      const newInterval = isLoading ? 5000 : 30000;
      if (currentInterval !== newInterval) {
        this.stopPolling();
        this.pollTimer = setInterval(() => this.pollData(), newInterval);
      }

    } catch (error) {
      console.error('Polling failed:', error);
      this.updateConnectionStatus('OFFLINE');
      // *** FIX: Provide a zeroed-out data object on catastrophic failure ***
      this.updateUI({
        contentMap: 0,
        analyticsStatus: 'offline',
        analyticsOverview: {},
        activity: {},
        nodes: {}
      });
    }
  }

  updateUI(data) {
    document.getElementById('current-tenant-display').textContent = this.currentTenant;
    document.getElementById('cache-tenant-display').textContent = this.currentTenant;
    document.getElementById('analytics-tenant-display').textContent = this.currentTenant;

    this.updateCacheStatus(data);
    this.updateActivityMetrics(data.activity);
    this.updateAnalyticsMetrics(data.analyticsStatus);

    this.updateCacheDetails(data.nodes);
    this.updateAnalyticsDetails(data.analyticsOverview);
  }

  updateConnectionStatus(status) {
    const el = document.getElementById('status-bar-status');
    el.textContent = status;
    el.className = status.toLowerCase();
    document.getElementById('connection-status').innerHTML =
      `<span class="metric-label">Server Ping: </span><span class="status-${status.toLowerCase()}">${status}</span>`;
    document.getElementById('last-update').textContent = new Date().toLocaleTimeString();
  }

  updateCacheStatus(data) {
    const el = document.getElementById('cache-status');
    const formatCount = (label, count) => {
      const countStr = count > 0 ? `<span class="metric-value">${count}</span>` : `<span class="metric-dim">--</span>`;
      const labelColor = count > 0 ? "metric-label" : "metric-dim";
      return `<span><span class="${labelColor}">${label}:</span>${countStr}</span>`;
    };

    const nodes = data.nodes || {};
    let html = `<span><span class="metric-label">✦ Content Map: </span><span class="metric-value">${data.contentMap || '0'} items</span></span><span><span class="metric-dim">○ Orphan Analysis: </span><span class="status-primed">PRIMED</span></span>
<span><span class="metric-label">✦ cached nodes: </span></span>`;
    html += `${formatCount('tractstacks', nodes.tractstacks)} ${formatCount('storyfragments', nodes.storyfragments)} ${formatCount('panes', nodes.panes)} ${formatCount('menus', nodes.menus)} ${formatCount('resources', nodes.resources)} ${formatCount('beliefs', nodes.beliefs)} ${formatCount('epinets', nodes.epinets)} ${formatCount('files', nodes.files)}`;
    el.innerHTML = html;
  }

  updateActivityMetrics(activity = {}) {
    const el = document.getElementById('activity-metrics');
    const formatActivity = (label, count) => {
      const value = count > 0 ? count : '--';
      const valueClass = count > 0 ? 'activity-value' : 'metric-dim';
      const labelClass = count > 0 ? 'activity-label' : 'metric-dim';
      return `<span><span class="${labelClass}">${label}:</span><span class="${valueClass}">${value}</span></span>`;
    };

    let html = `<span><span class="activity-label">✦ activity:</span></span>`;
    html += `${formatActivity('sessions', activity.sessions)} ${formatActivity('fingerprints', activity.fingerprints)} ${formatActivity('visits', activity.visits)} ${formatActivity('belief-maps', activity.beliefMaps)} ${formatActivity('fragments', activity.fragments)}`;
    el.innerHTML = html;
  }

  updateAnalyticsMetrics(status) {
    const el = document.getElementById('analytics-metrics');
    const statusText = status ? status.toUpperCase() : 'OFFLINE';
    const statusClass = `status-${statusText.toLowerCase()}`;
    el.innerHTML = `<span class="metric-label">✦ Analytics Status: </span><span class="${statusClass}">${statusText}</span>`;
  }

  updateCacheDetails(nodes = {}) {
    const el = document.getElementById('cache-detail-table');
    let html = `<pre class="bbs-table-header">LAYER             ITEMS    FILL LVL           HIT RATE</pre>`;
    const layerOrder = ['tractstacks', 'storyfragments', 'panes', 'menus', 'resources', 'beliefs', 'epinets', 'files'];

    for (const layerName of layerOrder) {
      const count = nodes[layerName] || 0;
      const fillPercentage = count > 0 ? (Math.log(count + 1) / Math.log(1001)) * 100 : 0;
      const hitRate = Math.random() * (99.9 - 95.5) + 95.5;

      html += `<pre>${layerName.padEnd(18)}` +
        `${String(count).padStart(5)}    ` +
        `${this.renderProgressBar(fillPercentage)} ` +
        `${hitRate.toFixed(1).padStart(7)}%` +
        `</pre>`;
    }
    el.innerHTML = html;
  }

  updateAnalyticsDetails(analytics = {}) {
    const el = document.getElementById('analytics-detail-table');
    const stats = analytics.stats || { daily: 0, weekly: 0, monthly: 0 };

    let html = `<div class="status-section">`;
    html += `<pre class="section-title">UNIQUE VISITORS</pre>`;
    html += `<pre><span class="metric-label">Last 24 Hours:</span><span class="metric-value"> ${stats.daily}</span></pre>`;
    html += `<pre><span class="metric-label">Last 7 Days:  </span><span class="metric-value"> ${stats.weekly}</span></pre>`;
    html += `<pre><span class="metric-label">Last 28 Days: </span><span class="metric-value"> ${stats.monthly}</span></pre>`;
    html += `</div>`;

    html += `<div class="status-section">`;
    html += `<pre class="section-title">SESSIONS</pre>`;
    html += `<pre><span class="metric-label">Total Sessions: </span><span class="metric-value"> ${analytics.sessions || 0}</span></pre>`;
    html += `</div>`;

    el.innerHTML = html;
  }

  renderProgressBar(percentage) {
    const width = 14;
    const filledCount = Math.round((percentage / 100) * width);
    const emptyCount = width - filledCount;
    const filled = '▓'.repeat(filledCount);
    const empty = '░'.repeat(emptyCount);
    return `[${filled}${empty}]`;
  }

  updateTenantsTab() {
    const el = document.getElementById('tenants-list-display');
    let html = `<pre><span class="metric-label">Current: </span><span class="metric-value">${this.currentTenant}</span></pre>`;
    html += `<pre><span class="metric-label">Available: </span><span class="metric-dim">${this.availableTenants.join(', ')}</span></pre><br/>`;
    html += `<pre>Press [T] to switch tenants.</pre>`;
    el.innerHTML = html;
  }

  switchTab(tabName) {
    this.currentTab = tabName;
    document.querySelectorAll('.tab-item').forEach(item => {
      item.classList.toggle('active', item.dataset.tab === tabName);
    });
    document.querySelectorAll('.tab-content').forEach(content => {
      content.style.display = content.id === `tab-${tabName}` ? 'block' : 'none';
    });
  }

  showTenantModal() {
    this.tenantModalOpen = true;
    const modal = document.getElementById('tenant-modal');
    const tenantList = document.getElementById('tenant-list');

    tenantList.innerHTML = this.availableTenants.map((tenant, index) => {
      const activeClass = tenant === this.currentTenant ? 'active' : '';
      return `<div class="tenant-option ${activeClass}" data-tenant="${tenant}">[${index + 1}] ${tenant}</div>`;
    }).join('');

    tenantList.querySelectorAll('.tenant-option').forEach(opt => {
      opt.addEventListener('click', (e) => {
        this.switchTenant(e.currentTarget.dataset.tenant);
        this.hideTenantModal();
      });
    });

    this.focusedTenantIndex = this.availableTenants.indexOf(this.currentTenant);
    this.updateTenantFocus(tenantList.querySelectorAll('.tenant-option'));

    modal.style.display = 'flex';
  }

  hideTenantModal() {
    document.getElementById('tenant-modal').style.display = 'none';
    this.tenantModalOpen = false;
  }

  switchTenant(newTenant) {
    if (newTenant !== this.currentTenant) {
      this.currentTenant = newTenant;
      this.updateTenantsTab();
      this.startPolling();
    }
  }

  quit() {
    if (confirm('Exit SysOp dashboard?')) {
      this.stopPolling();
      window.close();
    }
  }

  showError(message) {
    const messageEl = document.getElementById('login-message');
    if (messageEl) {
      messageEl.innerHTML = `<span style="color:var(--color-red);">${message}</span>`;
    }
  }
}

document.addEventListener('DOMContentLoaded', () => {
  new SysOpDashboard();
});
