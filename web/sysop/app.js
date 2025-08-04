// TractStack SysOp Dashboard - BBS-style monitoring interface

class SysOpDashboard {
  constructor() {
    this.authenticated = false;
    this.currentTenant = 'default';
    this.availableTenants = [];
    this.ws = null;
    this.authToken = null;

    this.init();
  }

  async init() {
    // Check authentication status
    await this.checkAuth();

    // Setup event listeners
    this.setupEventListeners();

    if (this.authenticated) {
      this.showDashboard();
    } else {
      this.showLogin();
    }
  }

  async checkAuth() {
    try {
      const response = await fetch('/sysop-auth');
      const data = await response.json();

      if (!data.passwordRequired) {
        // No password required
        this.authenticated = true;
        this.authToken = 'no-auth-required';
        return;
      }

      // Check if we have a stored token
      const storedToken = sessionStorage.getItem('sysop-token');
      if (storedToken) {
        // Validate stored token
        const validateResponse = await fetch('/sysop-auth', {
          headers: { 'Authorization': `Bearer ${storedToken}` }
        });
        const validateData = await validateResponse.json();

        if (validateData.authenticated) {
          this.authenticated = true;
          this.authToken = storedToken;
          return;
        }
      }

      // Show login form
      this.setupLoginForm(data);

    } catch (error) {
      this.showError('Connection failed');
    }
  }

  setupLoginForm(authData) {
    const messageEl = document.getElementById('login-message');

    if (!authData.passwordRequired) {
      messageEl.innerHTML = `
                Welcome to your story keep. Set SYSOP_PASSWORD to protect the system 
                <a href="https://tractstack.org" target="_blank" class="docs-link">[ℹ]</a>
            `;
      document.getElementById('no-auth-form').style.display = 'block';
    } else {
      messageEl.textContent = 'Enter SysOp credentials to access monitoring dashboard';
      document.getElementById('password-form').style.display = 'block';
    }
  }

  setupEventListeners() {
    // Login button
    const loginBtn = document.getElementById('login-btn');
    const enterBtn = document.getElementById('enter-btn');
    const passwordInput = document.getElementById('password-input');

    if (loginBtn) {
      loginBtn.addEventListener('click', () => this.handleLogin());
    }

    if (enterBtn) {
      enterBtn.addEventListener('click', () => this.handleNoAuthLogin());
    }

    if (passwordInput) {
      passwordInput.addEventListener('keypress', (e) => {
        if (e.key === 'Enter') this.handleLogin();
      });
    }

    // Dashboard keyboard controls
    document.addEventListener('keydown', (e) => {
      if (!this.authenticated) return;

      switch (e.key.toLowerCase()) {
        case 't':
          this.showTenantModal();
          break;
        case 'q':
          this.quit();
          break;
        case 'escape':
          this.hideTenantModal();
          break;
      }
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
        sessionStorage.setItem('sysop-token', this.authToken);
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
    document.getElementById('login-screen').style.display = 'block';
    document.getElementById('dashboard-screen').style.display = 'none';
  }

  showDashboard() {
    document.getElementById('login-screen').style.display = 'none';
    document.getElementById('dashboard-screen').style.display = 'block';

    // Load available tenants
    this.loadTenants();

    // Start WebSocket connection for real-time updates
    this.connectWebSocket();
  }

  async loadTenants() {
    try {
      // Load tenants from proxy endpoint that reads tenants.json
      const response = await fetch(`/sysop-proxy/health?tenant=${this.currentTenant}`, {
        headers: { 'Authorization': `Bearer ${this.authToken}` }
      });

      if (response.ok) {
        // For now, use default tenants - in full implementation, 
        // you'd add a specific endpoint to list all tenants
        this.availableTenants = ['default', 'love'];
      }
    } catch (error) {
      console.warn('Could not load tenants:', error);
      this.availableTenants = ['default'];
    }
  }

  connectWebSocket() {
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${window.location.host}/sysop-ws?tenant=${this.currentTenant}`;

    this.ws = new WebSocket(wsUrl);

    this.ws.onopen = () => {
      console.log('SysOp WebSocket connected');
    };

    this.ws.onmessage = (event) => {
      const data = JSON.parse(event.data);
      this.updateDashboard(data);
    };

    this.ws.onclose = () => {
      console.log('SysOp WebSocket disconnected');
      // Attempt reconnection after 5 seconds
      setTimeout(() => this.connectWebSocket(), 5000);
    };

    this.ws.onerror = (error) => {
      console.error('SysOp WebSocket error:', error);
    };
  }

  updateDashboard(data) {
    // Update timestamp
    const now = new Date();
    document.getElementById('last-update').textContent =
      `Last scan: ${now.toTimeString().split(' ')[0]}`;

    // Update tenant info
    document.getElementById('current-tenant').textContent = data.tenantId || this.currentTenant;

    // Update cache metrics
    this.updateCacheMetrics(data);

    // Update analytics metrics
    this.updateAnalyticsMetrics(data);
  }

  updateCacheMetrics(data) {
    const container = document.getElementById('cache-metrics');
    const health = data.health || {};
    const contentMap = data.contentMap || {};

    let html = '';

    // Connection status
    const timestamp = new Date().toISOString().replace('T', ' ').slice(0, 19) + ' UTC';
    html += `<div class="metric-line"><span class="block-info">▓</span> ${timestamp} | Tenant: <span class="status-online">${data.tenantId || this.currentTenant}</span></div>`;

    if (health.error) {
      html += `<div class="metric-line"><span class="block-error">✖</span> Connection: <span class="status-error">OFFLINE</span></div>`;
    } else {
      const status = (health.status || 'ok').toUpperCase();
      html += `<div class="metric-line"><span class="block-success">✦</span> Connection: <span class="status-online">ONLINE</span>  Status: <span class="status-online">${status}</span></div>`;

      // Content map status
      const contentMapSize = Object.keys(contentMap).length;
      if (contentMapSize > 0) {
        html += `<div class="metric-line"><span class="block-success">✦</span> Content Map: <span class="content-count">${contentMapSize}</span> items  <span class="block-info">○</span> Orphan Analysis: <span class="status-primed">PRIMED</span></div>`;
      } else {
        html += `<div class="metric-line"><span class="block-info">○</span> Content Map: <span class="status-error">NOT LOADED</span></div>`;
      }

      // Cached nodes (mock based on logs)
      html += `<div class="metric-line"><span class="block-info">✦</span> cached nodes: `;
      html += `<span class="content-type">tractstacks</span>:<span class="content-count">1</span> `;
      html += `<span class="content-type">storyfragments</span>:<span class="content-count">4</span> `;
      html += `<span class="content-type">panes</span>:<span class="content-count">39</span> `;
      html += `<span class="content-type">resources</span>:<span class="content-count">496</span> `;
      html += `<span class="content-type">beliefs</span>:<span class="content-count">20</span></div>`;

      // Activity
      html += `<div class="metric-line"><span class="block-purple">✦</span> activity: `;
      html += `<span class="activity-label">sessions</span>:<span class="activity-empty">--</span> `;
      html += `<span class="activity-label">fingerprints</span>:<span class="activity-empty">--</span> `;
      html += `<span class="activity-label">visits</span>:<span class="activity-empty">--</span> `;
      html += `<span class="activity-label">belief-maps</span>:<span class="activity-count">4</span> `;
      html += `<span class="activity-label">fragments</span>:<span class="activity-empty">--</span></div>`;
    }

    container.innerHTML = html;
  }

  updateAnalyticsMetrics(data) {
    const container = document.getElementById('analytics-metrics');
    const analytics = data.analytics || {};
    const dashboard = data.dashboard || {};

    let html = '';

    // Dashboard query status
    const dashboardStatus = (dashboard.status || 'unavailable').toUpperCase();
    const dashboardColor = this.getStatusColor(dashboardStatus);
    html += `<div class="metric-line"><span class="block-info">⚡</span> Dashboard Query: <span class="${dashboardColor}">${dashboardStatus}</span></div>`;

    // Analytics cache status  
    const cacheStatus = (analytics.status || 'cold').toUpperCase();
    const cacheColor = this.getStatusColor(cacheStatus);
    html += `<div class="metric-line"><span class="block-info">⚡</span> Analytics Cache: <span class="${cacheColor}">${cacheStatus}</span></div>`;

    container.innerHTML = html;
  }

  getStatusColor(status) {
    const statusMap = {
      'ONLINE': 'status-online',
      'COMPLETE': 'status-complete',
      'WARMED': 'status-warmed',
      'PRIMED': 'status-primed',
      'LOADING': 'status-loading',
      'OFFLINE': 'status-offline',
      'UNAVAILABLE': 'status-error',
      'COLD': 'status-error'
    };
    return statusMap[status] || 'status-error';
  }

  showTenantModal() {
    const modal = document.getElementById('tenant-modal');
    const tenantList = document.getElementById('tenant-list');

    let html = '';
    this.availableTenants.forEach((tenant, index) => {
      const activeClass = tenant === this.currentTenant ? 'active' : '';
      html += `<div class="tenant-option ${activeClass}" data-tenant="${tenant}">[${index + 1}] ${tenant}</div>`;
    });

    tenantList.innerHTML = html;

    // Add click handlers
    tenantList.addEventListener('click', (e) => {
      if (e.target.classList.contains('tenant-option')) {
        const newTenant = e.target.getAttribute('data-tenant');
        this.switchTenant(newTenant);
        this.hideTenantModal();
      }
    });

    modal.style.display = 'flex';
  }

  hideTenantModal() {
    document.getElementById('tenant-modal').style.display = 'none';
  }

  switchTenant(newTenant) {
    if (newTenant !== this.currentTenant) {
      this.currentTenant = newTenant;

      // Close existing WebSocket
      if (this.ws) {
        this.ws.close();
      }

      // Reconnect with new tenant
      this.connectWebSocket();
    }
  }

  quit() {
    if (confirm('Exit SysOp dashboard?')) {
      if (this.ws) {
        this.ws.close();
      }
      sessionStorage.removeItem('sysop-token');
      window.close();
    }
  }

  showError(message) {
    const messageEl = document.getElementById('login-message');
    if (messageEl) {
      messageEl.innerHTML = `<span class="error-state">Error: ${message}</span>`;
    }
  }
}

// Initialize dashboard when DOM is ready
document.addEventListener('DOMContentLoaded', () => {
  new SysOpDashboard();
});
