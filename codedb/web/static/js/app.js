// app.js — MuninnDB Alpine.js application
// Alpine.js is loaded from vendor (with defer) AFTER this file.
// alpine:init fires before Alpine initializes DOM — correct hook for Alpine.data()

document.addEventListener('alpine:init', () => {
  Alpine.data('muninnApp', () => ({
    // ── State ──────────────────────────────────────────────────────────────
    currentView: 'dashboard',
    vault: localStorage.getItem('muninnVault') || 'default',
    vaults: ['default'],
    vaultStats: {},
    vaultModalOpen: false,
    vaultPickerSearch: '',
    newVaultModal: { show: false, name: '', error: '', loading: false, collision: null },
    isDarkMode: localStorage.getItem('muninnTheme') !== 'light',
    liveConnected: false,
    appVersion: '',

    // Dashboard
    stats: { engramCount: 0, vaultCount: 0, storageBytes: 0, indexSize: 0 },
    workerStats: [],
    cognitiveInfoModal: false,
    liveFeed: [],
    activityDays: 7,
    activityUntil: '',
    activityShowTable: false,
    activityData: [],
    activityLoading: false,
    activityError: '',
    activityPresets: [
      { label: '7d', days: 7 },
      { label: '14d', days: 14 },
      { label: '30d', days: 30 },
      { label: '60d', days: 60 },
      { label: '90d', days: 90 },
      { label: '180d', days: 180 },
    ],
    _activityFetchId: 0,
    _activityResizeObserver: null,
    _prevEngramCount: 0,
    _prevVaultCount: 0,

    // Graph
    graphLoaded: false,
    graphTab: 'memory',
    graphLabelMode: 'full', // 'full' | 'short' | 'none'
    _cy: null,
    entityGraphLoaded: false,
    entityGraphStatus: '',
    entityGraphLabelMode: 'full',
    _entityCy: null,

    // Cluster
    clusterEnabled: null,  // null=unknown, true=enabled, false=disabled
    clusterNodes: [],
    clusterHealth: null,
    clusterCCS: null,
    clusterEvents: [],
    _clusterNodesInterval: null,
    _clusterCCSInterval: null,

    // Enable cluster form
    clusterEnableForm: { role: 'primary', bindAddr: '', clusterSecret: '', cortexAddr: '' },
    clusterEnableLoading: false,
    clusterEnableError: null,
    clusterEnableProgress: [],
    showEnableClusterConfirm: false,

    // Sub-tab nav
    clusterSubTab: 'overview',

    // Security posture
    clusterSecurityPosture: null,

    // Management tab
    showAddNodeModal: false,
    addNodeForm: { addr: '', token: '' },
    addNodeTesting: false,
    addNodeTestResult: null,
    addNodeLoading: false,
    addNodeError: null,
    addNodeProgress: [],
    showRemoveNodeModal: false,
    removeNodeTarget: null,
    removeNodeDrain: true,
    removeNodeLoading: false,
    removeNodeError: null,
    showFailoverModal: false,
    failoverTarget: '',
    failoverLoading: false,
    failoverError: null,
    failoverProgress: [],
    clusterFeed: [],
    clusterFeedPaused: false,
    _clusterFeedSSE: null,

    // Settings tab
    clusterToken: null,
    clusterTokenLoading: false,
    clusterTokenCopied: false,
    showRegenerateTokenConfirm: false,
    clusterSettings: { heartbeat_ms: 1000, sdown_beats: 3, ccs_interval_seconds: 30, reconcile_on_heal: true },
    clusterSettingsSaving: false,
    clusterSettingsSaved: false,

    // Notifications
    notifications: [],
    _notifId: 0,

    // Auth
    isAuthenticated: false,
    showSignOutConfirm: false,
    loginForm: { username: '', password: '' },
    loginError: '',

    // Settings
    settingsTab: 'vault',
    embedStatus: null,
    connectCopied: false,
    cogWorkerStats: null,

    // Plasticity (vault cognitive pipeline config)
    plasticityForm: {
      preset: 'default',
      showAdvanced: false,
      hebbianEnabled: true,
      temporalEnabled: true,
      hopDepth: null,
      semanticWeight: null,
      ftsWeight: null,
      relevanceFloor: null,
      temporalHalflife: null,
      recallMode: 'balanced',
    },
    plasticitySaving: false,
    plasticitySaveOk: false,
    plasticitySaveErr: '',

    // Plugin configuration state (used by embed status UI)
    pluginCfg: {
      embedProvider: 'none',
      embedOllamaModel: 'nomic-embed-text',
      embedApiKey: '',
      embedUrl: '',
      embedShowForm: false,
      embedError: '',
      enrichProvider: 'none',
      enrichOllamaModel: 'llama3.2',
      enrichModel: 'claude-haiku-4-5-20251001',
      enrichApiKey: '',
      enrichShowForm: false,
      enrichError: '',
      ollamaModels: [],
      ollamaEmbedModels: [],
      ollamaDetected: null,
      ollamaChecking: false,
      embedRatePerSec: 0,
      embedETASecs: 0,
      embedHardwareGPU: null,
    },

    // Vault actions
    vaultActionModal: { show: false, action: '', vault: '', confirmText: '', memCount: 0 },
    cloneModal: { show: false, source: '', newName: '' },
    mergeModal: { show: false, source: '', target: '', deleteSource: false },
    importModal: { show: false, vaultName: '', file: null, resetMeta: false },
    activeJob: null,
    jobPollInterval: null,
    vaultExporting: false,
    reindexing: false,

    // Sidebar
    sidebarExpanded: localStorage.getItem('muninnSidebar') === 'expanded',
    showShortcutsHelp: false,

    // Graph
    graphShowOrphans: false,
    graphLimit: 50,

    // SSE
    _es: null,
    _esRetries: 0,

    // ── Lifecycle ──────────────────────────────────────────────────────────
    async init() {
      // Apply theme to html element
      if (!this.isDarkMode) {
        document.documentElement.classList.add('light');
      } else {
        document.documentElement.classList.remove('light');
      }

      // Hash-based routing
      const onHash = () => {
        const hash = location.hash.replace(/^#\/?/, '') || 'dashboard';
        const parts = hash.split('/');
        const raw = parts[0];
        // Only use known views
        const known = ['dashboard', 'graph', 'settings', 'logs', 'cluster'];
        this.currentView = known.includes(raw) ? raw : 'dashboard';

        // Parse settings sub-tab if entering settings view
        if (raw === 'settings' && parts[1]) {
          const validTabs = ['vault'];
          if (validTabs.includes(parts[1])) {
            this.settingsTab = parts[1];
          }
        }

        this._onViewEnter(this.currentView);
      };
      window.addEventListener('hashchange', onHash);
      onHash();

      // Fetch version from public health endpoint
      try {
        const h = await fetch('/api/health').then(r => r.json());
        this.appVersion = h.version || '';
      } catch (err) {
        console.warn('[muninn] health check failed:', err);
      }

      // Load initial data (gated on auth check)
      await this.checkAuth();

      // Fetch vault list and engram counts whenever the vault picker modal opens.
      // loadVaults() was previously missing here — the vault list would only
      // refresh at login/auth-check, so newly created vaults wouldn't appear
      // until page reload.
      this.$watch('vaultModalOpen', (open) => {
        if (open) {
          this.loadVaults();
          this.loadVaultStats();
        }
      });
    },

    // ── Auth ───────────────────────────────────────────────────────────────
    async checkAuth() {
      try {
        await this.apiCall('/api/auth/check');
        this.isAuthenticated = true;
        this.loadVaults();
        this.loadWorkerStats();
        this.connectLive();
      } catch (_) {
        this.isAuthenticated = false;
        history.replaceState(null, '', location.pathname);
      }
    },

    async login() {
      this.loginError = '';
      try {
        await this.apiCall('/api/auth/login', {
          method: 'POST',
          body: JSON.stringify(this.loginForm),
        });
        this.isAuthenticated = true;
        this.loginForm = { username: '', password: '' };
        this.loadVaults();
        this.loadWorkerStats();
        this.connectLive();
        this.navigateTo('dashboard');
      } catch (err) {
        this.loginError = '用户名或密码无效';
      }
    },

    async logout() {
      await this.apiCall('/api/auth/logout', { method: 'POST' }).catch(() => {});
      this.isAuthenticated = false;
      history.replaceState(null, '', location.pathname);
    },

    async _onViewEnter(view) {
      // Clean up activity chart resources when leaving dashboard.
      if (view !== 'dashboard') {
        const canvas = document.getElementById('activityChart');
        if (canvas && canvas._chart) {
          try { canvas._chart.destroy(); } catch (_) {}
          canvas._chart = null;
        }
        if (this._activityResizeObserver) {
          this._activityResizeObserver.disconnect();
          this._activityResizeObserver = null;
        }
      }

      if (view === 'dashboard') {
        this.loadStats();
        // Chart init happens after DOM renders
        this.$nextTick(() => this._initChart());
      } else if (view === 'settings') {
        // Check current hash to determine which sub-tab to activate
        const hash = location.hash.replace(/^#\/?/, '');
        const parts = hash.split('/');
        if (parts[0] === 'settings' && parts[1]) {
          const validTabs = ['vault'];
          if (validTabs.includes(parts[1])) {
            this.settingsTab = parts[1];
          }
        }

        if (this.settingsTab === 'vault') {
          this.loadEmbedStatus();
          this.loadWorkers();
          this.loadPlasticity();
          this.$nextTick(() => this.initPlasticityChart());
        }

        this.loadVaults();
      } else if (view === 'graph') {
        // Clear graph state on vault change so stale nodes from the previous
        // vault are not shown. User must click Load Graph again for the new vault.
        if (this._cy) { this._cy.destroy(); this._cy = null; }
        if (this._entityCy) { this._entityCy.destroy(); this._entityCy = null; }
        this.graphLoaded = false;
        this.entityGraphLoaded = false;
      }
      if (view === 'cluster') {
        this.loadClusterDashboard();
      } else {
        this.stopClusterFeed();
      }
    },

    navigateTo(view) {
      location.hash = '/' + view;
    },

    toggleTheme() {
      this.isDarkMode = !this.isDarkMode;
      if (this.isDarkMode) {
        document.documentElement.classList.remove('light');
        localStorage.setItem('muninnTheme', 'dark');
      } else {
        document.documentElement.classList.add('light');
        localStorage.setItem('muninnTheme', 'light');
      }
    },

    onVaultChange() {
      localStorage.setItem('muninnVault', this.vault);
      this._onViewEnter(this.currentView);
    },

    pickVault(v) {
      this.vault = v;
      this.vaultModalOpen = false;
      this.vaultPickerSearch = '';
      this.onVaultChange();
    },

    toggleSidebar() {
      this.sidebarExpanded = !this.sidebarExpanded;
      localStorage.setItem('muninnSidebar', this.sidebarExpanded ? 'expanded' : 'collapsed');
    },

    // ── Server-Sent Events ─────────────────────────────────────────────────
    connectLive() {
      if (this._es) {
        this._es.close();
        this._es = null;
      }

      window._muninnSSE = new EventSource('/events');
      const es = window._muninnSSE;

      es.onopen = () => {
        this.liveConnected = true;
        this._esRetries = 0;
      };

      es.onerror = () => {
        this.liveConnected = false;
        // EventSource auto-reconnects, but we track state
        es.close();
        this._es = null;
        window._muninnSSE = null;
        const delay = Math.min(500 * Math.pow(1.5, this._esRetries), 30000);
        this._esRetries++;
        setTimeout(() => this.connectLive(), delay);
      };

      es.onmessage = (e) => {
        try {
          const msg = JSON.parse(e.data);
          this._handleLiveMessage(msg);
        } catch (err) {
          console.warn('[muninn] failed to process live event:', err, e.data);
        }
      };

      es.addEventListener('error', (e) => {
        // Connection-level errors have null data; onerror already handles those silently.
        // Only process named 'event: error' server events (which carry non-null data).
        if (!e.data) return;
        let msg = '实时流错误';
        try {
          const data = JSON.parse(e.data);
          if (data.error) msg = '实时流：' + data.error;
        } catch (_) {}
        console.warn('[muninn] SSE error event:', msg);
        this.addNotification('error', msg);
        this.liveConnected = false;
        es.close();
        this._es = null;
        window._muninnSSE = null;
        const delay = Math.min(500 * Math.pow(1.5, this._esRetries), 30000);
        this._esRetries++;
        setTimeout(() => this.connectLive(), delay);
      });

      this._es = es;
    },

    _handleLiveMessage(msg) {
      if (msg.type === 'stats_update') {
        // Vault count-diff: refresh vault list when a vault is added or removed.
        // Guard with > 0 on first message (learn current count without triggering a reload).
        const newVaultCount = msg.data.vaultCount || 0;
        if (this._prevVaultCount > 0 && newVaultCount !== this._prevVaultCount) {
          this.loadVaults();
        }
        this._prevVaultCount = newVaultCount;

        // Re-fetch stats scoped to the selected vault instead of using
        // the global broadcast values.
        this.loadStats();
      } else if (msg.type === 'workers_update') {
        const d = msg.data;
        if (d) {
          const map = { Hebbian: d.hebbian, Contradict: d.contradict, Confidence: d.confidence };
          this.workerStats = Object.entries(map)
            .filter(([, w]) => w != null)
            .map(([name, w]) => ({
              name,
              state:         w.state         ?? 0,
              processed:     w.processed     ?? 0,
              batches:       w.batches       ?? 0,
              errors:        w.errors        ?? 0,
              dropped:       w.dropped       ?? 0,
              lastRun:       w.lastRun       ?? 0,
              effectiveWait: w.effectiveWait ?? 0,
            }));
        }
      } else if (msg.type === 'memory_added') {
        // Guard: skip malformed events missing required fields.
        // A missing or undefined id causes Alpine x-for to use 'undefined' as
        // a key, corrupting DOM anchor tracking and producing the
        // "can't access property 'after', v is undefined" crash that cascades
        // to break the entire Alpine reactivity system.
        if (!msg.data || !msg.data.id) {
          console.warn('[muninn] live feed received memory_added with missing id — skipping', msg.data);
          return;
        }
        // Deduplicate: guard against double-delivery of the same engram ID.
        // Replace the array reference (instead of in-place unshift+pop) so that
        // Alpine.js x-for can perform a clean diff — in-place mutations of both
        // ends of the array confuse Alpine's DOM anchor tracking and produce the
        // "can't access property 'after', v is undefined" crash.
        if (!this.liveFeed.some(item => item.id === msg.data.id)) {
          const next = [msg.data, ...this.liveFeed];
          this.liveFeed = next.length > 20 ? next.slice(0, 20) : next;
        }
      }
    },

    // ── API helpers ────────────────────────────────────────────────────────
    async apiCall(url, opts = {}) {
      const res = await fetch(url, {
        credentials: 'same-origin', // always send session cookie for admin endpoints
        headers: { 'Content-Type': 'application/json', ...(opts.headers || {}) },
        ...opts,
      });
      if (!res.ok) {
        const text = await res.text().catch(() => res.statusText);
        throw new Error(res.status + ': ' + text);
      }
      return res.json();
    },

    // ── Dashboard ──────────────────────────────────────────────────────────
    async loadStats() {
      try {
        const data = await this.apiCall('/api/stats?vault=' + encodeURIComponent(this.vault));
        this.stats = {
          engramCount:  data.engram_count   || data.engramCount  || 0,
          vaultCount:   data.vault_count    || data.vaultCount   || 0,
          storageBytes: data.storage_bytes  || data.storageBytes || 0,
          indexSize:    data.index_size     || data.indexSize    || 0,
        };
      } catch (err) {
        this.addNotification('error', '统计信息：' + err.message);
      }
    },

    async loadWorkerStats() {
      try {
        const data = await this.apiCall('/api/workers');
        const map = { Hebbian: data.hebbian, Contradict: data.contradict, Confidence: data.confidence };
        this.workerStats = Object.entries(map)
          .filter(([, d]) => d != null)
          .map(([name, d]) => ({
            name,
            state:        d.state        ?? 0,
            processed:    d.processed    ?? 0,
            batches:      d.batches      ?? 0,
            errors:       d.errors       ?? 0,
            dropped:      d.dropped      ?? 0,
            lastRun:      d.lastRun      ?? 0,
            effectiveWait: d.effectiveWait ?? 0,
          }));
      } catch (err) {
        console.warn('[muninn] worker stats failed:', err);
      }
    },

    workerStateName(state) {
      return ['活跃', '空闲', '休眠'][state] ?? '未知';
    },

    workerStateBadge(state) {
      const classes = ['badge-active', 'badge-idle', 'badge-dormant'];
      return classes[state] ?? 'badge-idle';
    },

    formatWorkerProcessed(n) {
      if (!n) return '—';
      if (n >= 1_000_000) return (n / 1_000_000).toFixed(1) + 'M';
      if (n >= 1_000)     return (n / 1_000).toFixed(1) + 'k';
      return String(n);
    },

    formatWorkerLastRun(nanos) {
      if (!nanos) return '从未';
      const ago = Date.now() - nanos / 1_000_000;
      if (ago < 5_000)        return '刚刚';
      if (ago < 60_000)       return Math.floor(ago / 1_000) + '秒前';
      if (ago < 3_600_000)    return Math.floor(ago / 60_000) + '分钟前';
      if (ago < 86_400_000)   return Math.floor(ago / 3_600_000) + '小时前';
      return Math.floor(ago / 86_400_000) + '天前';
    },

    workerTooltip(w) {
      const lines = [
        `${w.name}  ·  ${this.workerStateName(w.state)}`,
        `已处理：${w.processed}  ·  批次：${w.batches}`,
        `上次运行：${this.formatWorkerLastRun(w.lastRun)}`,
      ];
      if (w.errors  > 0) lines.push(`错误：${w.errors}`);
      if (w.dropped > 0) lines.push(`丢弃：${w.dropped}`);
      return lines.join('\n');
    },

    workerDotStyle(state) {
      const colors = ['#10b981', '#f59e0b', '#9ca3af']; // active, idle, dormant
      return `background:${colors[state] ?? '#9ca3af'};`;
    },

    workerStateStyle(state) {
      const styles = ['color:#10b981;', 'color:#f59e0b;', 'color:#9ca3af;'];
      return styles[state] ?? 'color:#9ca3af;';
    },

    workersOverallHealthLabel() {
      if (!this.workerStats.length) return '加载中';
      const active = this.workerStats.filter(w => w.state === 0).length;
      const idle   = this.workerStats.filter(w => w.state === 1).length;
      if (active === this.workerStats.length) return '全部活跃';
      if (active > 0) return active + ' 个活跃';
      if (idle   > 0) return idle + ' 个空闲';
      return '休眠';
    },

    workersOverallHealthStyle() {
      const base = 'font-size:0.6875rem;padding:0.1rem 0.5rem;border-radius:9999px;font-weight:600;';
      if (!this.workerStats.length) return base + 'background:#6b728020;color:#6b7280;';
      const active = this.workerStats.filter(w => w.state === 0).length;
      const idle   = this.workerStats.filter(w => w.state === 1).length;
      if (active === this.workerStats.length) return base + 'background:#10b98120;color:#10b981;';
      if (active > 0) return base + 'background:#f59e0b20;color:#f59e0b;';
      if (idle   > 0) return base + 'background:#f59e0b20;color:#f59e0b;';
      return base + 'background:#6b728020;color:#6b7280;';
    },

    workersLastRunSummary() {
      if (!this.workerStats.length) return '—';
      const max = Math.max(...this.workerStats.map(w => w.lastRun || 0));
      return this.formatWorkerLastRun(max);
    },

    async loadVaults() {
      try {
        const data = await this.apiCall('/api/vaults');
        this.vaults = Array.isArray(data) ? data.slice().sort((a, b) => a.localeCompare(b)) : ['default'];
        if (!this.vaults.includes(this.vault)) {
          this.vault = this.vaults[0] || 'default';
          localStorage.setItem('muninnVault', this.vault);
          // Vault changed — refresh current view so charts/lists use the new vault.
          this.$nextTick(() => this._onViewEnter(this.currentView));
        }
      } catch (_) {
        this.vaults = ['default'];
      }
    },

    async loadVaultStats() {
      try {
        const data = await this.apiCall('/api/vaults/stats');
        if (Array.isArray(data)) {
          const map = {};
          data.forEach(s => { map[s.name] = s.engram_count; });
          this.vaultStats = map;
        }
      } catch (_) {
        // Non-critical — swallow silently
      }
    },

    _initChart() {
      const canvas = document.getElementById('activityChart');
      if (!canvas) return;

      // Monotonic fetch ID — stale responses are discarded.
      const fetchId = ++this._activityFetchId;

      const days = Math.max(1, Math.min(180, this.activityDays || 7));
      let url = '/api/activity-counts?vault=' + encodeURIComponent(this.vault) + '&days=' + days;
      if (this.activityUntil) {
        url += '&until=' + encodeURIComponent(this.activityUntil);
      }

      this.activityLoading = true;
      this.activityError = '';
      this.activityData = [];

      this.apiCall(url).then(resp => {
        // Stale response — a newer call superseded this one.
        if (this._activityFetchId !== fetchId) return;

        this.activityLoading = false;
        const counts = resp.counts || [];
        this.activityData = counts;

        const labels = counts.map(c => c.date);
        const data = counts.map(c => c.count);

        // Dynamic tick grouping: estimate how many labels fit based on canvas width.
        const canvasWidth = canvas.parentElement ? canvas.parentElement.clientWidth : canvas.width;
        const maxTicks = Math.max(4, Math.floor(canvasWidth / 60));

        this._renderActivityChart(canvas, labels, data, maxTicks);

        // Set up a resize observer so tick density adapts to layout changes.
        if (!this._activityResizeObserver && canvas.parentElement) {
          this._activityResizeObserver = new ResizeObserver(() => {
            const chart = canvas._chart;
            if (!chart) return;
            const w = canvas.parentElement ? canvas.parentElement.clientWidth : canvas.width;
            const mt = Math.max(4, Math.floor(w / 60));
            try {
              chart.options.scales.x.ticks.maxTicksLimit = mt;
              chart.update('none');
            } catch (_) { /* best effort */ }
          });
          this._activityResizeObserver.observe(canvas.parentElement);
        }
      }).catch(err => {
        if (this._activityFetchId !== fetchId) return;
        this.activityLoading = false;
        let message = '加载活动数据失败';
        if (err && typeof err.message === 'string' && err.message.trim() !== '') {
          message += ': ' + err.message;
        }
        this.activityError = message;
      });
    },

    _renderActivityChart(canvas, labels, data, maxTicks) {
      // Store chart instance on the canvas DOM element — NOT on `this` —
      // because Alpine.js wraps component properties in reactive Proxies
      // which cause infinite recursion when Chart.js traverses its own data.
      const existing = canvas._chart;
      if (existing) {
        try {
          existing.data.labels = labels;
          existing.data.datasets[0].data = data;
          existing.options.scales.x.ticks.maxTicksLimit = maxTicks;
          existing.update();
          return;
        } catch (_) {
          try { existing.destroy(); } catch (_) {}
          canvas._chart = null;
        }
      }

      canvas._chart = new Chart(canvas, {
        type: 'bar',
        data: {
          labels,
          datasets: [{
            label: '写入记忆数',
            data,
            backgroundColor: 'rgba(6,182,212,0.5)',
            borderColor: '#06b6d4',
            borderWidth: 1,
            borderRadius: 4,
          }],
        },
        options: {
          responsive: true,
          maintainAspectRatio: false,
          plugins: { legend: { display: false },
            tooltip: {
              callbacks: {
                title: function(items) { return items[0] ? items[0].label : ''; },
                 label: function(item) { return item.parsed.y + ' 条记忆'; },
              },
            },
          },
          scales: {
            x: {
              grid: { color: 'rgba(42,42,74,0.5)' },
              ticks: {
                color: '#94a3b8',
                maxTicksLimit: maxTicks,
                maxRotation: 45,
                callback: function(value, index) {
                  const label = this.getLabelForValue(value);
                  const parts = label.split('-');
                  if (parts.length === 3) {
                    // Parse as UTC to avoid timezone-induced date shift.
                    const d = new Date(label + 'T00:00:00Z');
                    if (isNaN(d.getTime())) return label;
                    return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', timeZone: 'UTC' });
                  }
                  return label;
                },
              },
            },
            y: {
              grid: { color: 'rgba(42,42,74,0.5)' },
              ticks: { color: '#94a3b8' },
              beginAtZero: true,
            },
          },
        },
      });
    },

    _copyActivityTable() {
      const header = '日期\t数量';
      const rows = this.activityData.map(r => r.date + '\t' + r.count);
      const total = this.activityData.reduce((s, r) => s + r.count, 0);
      rows.push('总计\t' + total);
      const text = header + '\n' + rows.join('\n');

      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(() => {
            this.addNotification('success', '活动数据已复制到剪贴板');
        }).catch(() => {
          this._copyFallback(text);
        });
      } else {
        this._copyFallback(text);
      }
    },

    _copyFallback(text) {
      try {
        const ta = document.createElement('textarea');
        ta.value = text;
        ta.style.position = 'fixed';
        ta.style.opacity = '0';
        document.body.appendChild(ta);
        ta.select();
        const ok = document.execCommand('copy');
        document.body.removeChild(ta);
        this.addNotification(ok ? 'success' : 'error', ok ? '活动数据已复制到剪贴板' : '复制失败——请手动复制');
      } catch (_) {
        this.addNotification('error', '复制失败——请手动复制');
      }
    },

    formatBytes(bytes) {
      if (!bytes) return '0 B';
      const units = ['B', 'KB', 'MB', 'GB'];
      let i = 0, n = +bytes;
      while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
      return n.toFixed(1) + ' ' + units[i];
    },

    formatUptime(seconds) {
      if (!seconds) return '0s';
      const d = Math.floor(seconds / 86400);
      const h = Math.floor((seconds % 86400) / 3600);
      const m = Math.floor((seconds % 3600) / 60);
      if (d > 0) return d + '天 ' + h + '小时';
      if (h > 0) return h + '小时 ' + m + '分';
      return m + '分';
    },

    // ── Graph ──────────────────────────────────────────────────────────────
    async loadGraph() {
      this.addNotification('info', '正在加载图谱…');
      try {
        // Use GET /api/engrams for node listing
        const limit = Math.max(1, Math.min(200, parseInt(this.graphLimit, 10) || 50));
        this.graphLimit = limit;
        const data = await this.apiCall(
          '/api/engrams?vault=' + encodeURIComponent(this.vault) + '&limit=' + limit + '&offset=0'
        );
        const engrams = data.engrams || [];
        if (!engrams.length) {
          this.addNotification('error', '没有可用于绘图的记忆');
          return;
        }

        // Load all engram links in a single batch call (replaces N+1 pattern).
        const nodeIdSet = new Set(engrams.map(e => e.id));
        const batchResp = await this.apiCall('/api/engrams/links/batch?vault=' + encodeURIComponent(this.vault), {
          method: 'POST',
          body: JSON.stringify({
            ids: engrams.map(e => e.id),
          }),
        });
        const linksMap = batchResp.links || {};
        const edgeSet = new Set();
        const edges = [];
        const connectedNodeIds = new Set();
        for (const [srcId, srcLinks] of Object.entries(linksMap)) {
          for (const l of (srcLinks || [])) {
            const edgeId = srcId + '-' + l.target_id;
            if (nodeIdSet.has(l.target_id) && !edgeSet.has(edgeId)) {
              edgeSet.add(edgeId);
              edges.push({
                data: {
                  id: edgeId,
                  source: srcId,
                  target: l.target_id,
                  weight: l.weight || 0.5,
                },
              });
              connectedNodeIds.add(srcId);
              connectedNodeIds.add(l.target_id);
            }
          }
        }

        // Filter orphan nodes (nodes with 0 edges) unless showOrphans is toggled on
        const showOrphans = this.graphShowOrphans;
        const filteredEngrams = showOrphans
          ? engrams
          : engrams.filter(e => connectedNodeIds.has(e.id));

        // If no connected nodes, fall back to all engrams so the graph isn't empty
        const nodesToRender = filteredEngrams.length > 0 ? filteredEngrams : engrams;

        // Build node elements
        const nodeElements = nodesToRender.map(e => {
          const fullLabel = e.concept || e.id.slice(0, 8);
          return {
            data: {
              id: e.id,
              label: fullLabel,
              shortLabel: fullLabel.length > 20 ? fullLabel.slice(0, 18) + '…' : fullLabel,
              size: connectedNodeIds.has(e.id) ? 20 + (e.confidence || 0.5) * 20 : 12,
              color: !connectedNodeIds.has(e.id) ? '#64748b'
                   : (e.confidence || 0) > 0.7 ? '#06b6d4'
                   : (e.confidence || 0) > 0.4 ? '#a855f7' : '#eab308',
              orphan: !connectedNodeIds.has(e.id),
              snippet: (e.content || '').slice(0, 80),
            },
          };
        });

        const elements = [...nodeElements, ...edges];

        // Init or reinit Cytoscape
        if (this._cy) { this._cy.destroy(); this._cy = null; }
        this._cy = cytoscape({
          container: document.getElementById('cy'),
          elements,
          style: [
            {
              selector: 'node',
              style: {
                'background-color': 'data(color)',
                'width': 'data(size)',
                'height': 'data(size)',
                'label': 'data(label)',
                'color': '#e2e8f0',
                'font-size': '11px',
                'text-valign': 'bottom',
                'text-margin-y': '6px',
                'text-wrap': 'wrap',
                'text-max-width': '100px',
                'border-width': 2,
                'border-color': 'rgba(255,255,255,0.1)',
              },
            },
            {
              selector: 'node[?orphan]',
              style: {
                'opacity': 0.45,
                'border-style': 'dashed',
              },
            },
            {
              selector: 'edge',
              style: {
                'line-color': 'rgba(168,85,247,0.4)',
                'width': 2,
                'curve-style': 'bezier',
                'opacity': 0,
              },
            },
            {
              selector: 'node:selected',
              style: { 'border-width': 3, 'border-color': '#06b6d4' },
            },
          ],
          layout: {
            name: 'fcose',
            animate: true,
            animationDuration: 600,
            randomize: true,
            padding: 40,
            idealEdgeLength: 120,
            nodeRepulsion: 6500,
            edgeElasticity: 0.45,
            gravity: 0.2,
            numIter: 2500,
            tile: true,
            tilingPaddingVertical: 30,
            tilingPaddingHorizontal: 30,
          },
          wheelSensitivity: 0.3,
        });

        // Apply current label mode to the freshly initialised graph.
        this._applyGraphLabelStyle();

        // Resize Cytoscape when the container changes size (sidebar collapse,
        // window resize, etc). Only resize() here — fit() is handled by layoutstop
        // to avoid zooming in on pre-layout node positions.
        if (this._cyResizeObserver) this._cyResizeObserver.disconnect();
        const cyContainer = document.getElementById('cy');
        if (cyContainer && typeof ResizeObserver !== 'undefined') {
          let _cyResizeTimer = null;
          this._cyResizeObserver = new ResizeObserver(() => {
            clearTimeout(_cyResizeTimer);
            _cyResizeTimer = setTimeout(() => {
              if (this._cy) this._cy.resize();
            }, 150);
          });
          this._cyResizeObserver.observe(cyContainer);
        }

        // Fade edges in and fit view after nodes settle (fcose layout: 600ms).
        // cy.one() fires once and removes itself — does not re-trigger on re-runs.
        this._cy.one('layoutstop', () => {
          this._cy.fit(undefined, 40);
          this._cy.edges().animate({
            style: { opacity: 0.6 },
            duration: 250,
            easing: 'ease-in-out',
          });
        });

        this._cy.on('tap', 'node', (evt) => {
          const node = evt.target;
          this.addNotification(
            'info',
            node.data('label') + ': ' + (node.data('snippet') || '（无内容）')
          );
        });

        this.graphLoaded = true;
        const orphanCount = engrams.length - connectedNodeIds.size;
        const msg = '图谱已加载（' + nodesToRender.length + ' 个节点' +
          (orphanCount > 0 && !showOrphans ? '，隐藏 ' + orphanCount + ' 个孤立节点' : '') + '）';
        this.addNotification('success', msg);
      } catch (err) {
        this.addNotification('error', '图谱加载失败：' + err.message);
      }
    },

    graphZoomIn() {
      if (this._cy) { this._cy.zoom(this._cy.zoom() * 1.25); this._cy.center(); }
    },
    graphZoomOut() {
      if (this._cy) { this._cy.zoom(this._cy.zoom() * 0.8); this._cy.center(); }
    },
    graphFit() {
      if (this._cy) { this._cy.fit(); }
    },
    graphCycleLabel() {
      const modes = ['full', 'short', 'none'];
      const next = modes[(modes.indexOf(this.graphLabelMode) + 1) % modes.length];
      this.graphLabelMode = next;
      this._applyGraphLabelStyle();
    },
    _applyGraphLabelStyle() {
      if (!this._cy) return;
      const mode = this.graphLabelMode;
      this._cy.nodes().forEach(node => {
        const lbl = mode === 'full' ? node.data('label')
                  : mode === 'short' ? node.data('shortLabel')
                  : node.data('label');
        node.style({
          label: lbl,
          'text-opacity': mode === 'none' ? 0 : 1,
        });
      });
      this._cy.edges().forEach(edge => {
        edge.style({ 'text-opacity': mode === 'none' ? 0 : 1 });
      });
    },

    // ── Entity Graph ───────────────────────────────────────────────────────
    async loadEntityGraph() {
      this.entityGraphStatus = '正在加载实体图谱…';
      try {
        // Call the REST endpoint directly instead of the MCP server.
        // The previous approach fetched from http://127.0.0.1:8750/mcp which
        // is unreachable from remote browsers (127.0.0.1 resolves to the
        // browser's own loopback, not the server's).
        const data = await this.apiCall(
          '/api/admin/entity-graph?vault=' + encodeURIComponent(this.vault) + '&include_engrams=true'
        );

        const nodes = [];
        const edges = [];
        const nodeIdSet = new Set();

        (data.nodes || []).forEach(n => {
          const entityType = (n.type || 'other').toLowerCase();
          nodeIdSet.add(n.id);
          // Cytoscape requires { data: { id, ... } } element format.
          nodes.push({
            data: {
              id: n.id,
              label: n.id,
              shortLabel: n.id.length > 20 ? n.id.slice(0, 18) + '…' : n.id,
              type: entityType,
              size: 16,
              color: this.getEntityTypeColor(entityType),
              borderWidth: 2,
              borderWidthSelected: 3,
              borderColor: 'rgba(255,255,255,0.2)',
            },
          });
        });

        (data.edges || []).forEach(e => {
          if (nodeIdSet.has(e.from) && nodeIdSet.has(e.to)) {
            // Cytoscape uses source/target (not from/to) and requires { data: { ... } }.
            edges.push({
              data: {
                id: e.from + '-' + e.to + '-' + (e.rel_type || ''),
                source: e.from,
                target: e.to,
                label: e.rel_type || '',
                color: 'rgba(168,85,247,0.4)',
                width: Math.max(1, (e.weight || 0.5) * 3),
              },
            });
          }
        });

        if (nodes.length === 0) {
          this.entityGraphStatus = '仓库中未找到实体';
          return;
        }

        // Reinit or destroy existing graph
        if (this._entityCy) { this._entityCy.destroy(); this._entityCy = null; }

        const elements = nodes.concat(edges);

        this._entityCy = cytoscape({
          container: document.getElementById('cy-entity'),
          elements: elements,
          style: [
            {
              selector: 'node',
              style: {
                'background-color': 'data(color)',
                'width': 'data(size)',
                'height': 'data(size)',
                'label': 'data(label)',
                'color': '#e2e8f0',
                'font-size': '10px',
                'text-valign': 'bottom',
                'text-halign': 'center',
                'text-margin-y': 6,
                'border-width': 'data(borderWidth)',
                'border-color': 'data(borderColor)',
                'text-background-opacity': 0.7,
                'text-background-color': 'rgba(0,0,0,0.6)',
                'text-background-padding': '2px',
                'text-background-shape': 'roundrectangle',
                'text-wrap': 'ellipsis',
                'text-max-width': '100px',
                'min-zoomed-font-size': 8
              }
            },
            {
              selector: 'node:selected',
              style: {
                'border-width': 'data(borderWidthSelected)',
                'border-color': '#06b6d4'
              }
            },
            {
              selector: 'edge',
              style: {
                'line-color': 'data(color)',
                'width': 'data(width)',
                'curve-style': 'bezier',
                'opacity': 0.7,
                'target-arrow-shape': 'triangle',
                'target-arrow-color': 'data(color)',
                'arrow-scale': 0.8,
                'label': 'data(label)',
                'color': '#ccc',
                'font-size': '10px',
                'text-background-opacity': 0.6,
                'text-background-color': 'rgba(0,0,0,0.5)',
                'text-background-padding': '2px',
                'text-background-shape': 'roundrectangle'
              }
            }
          ],
          layout: {
            name: 'fcose',
            animate: true,
            animationDuration: 600,
            animationEasing: 'ease-out',
            randomize: true,
            padding: 60,
            idealEdgeLength: 250,
            nodeRepulsion: 50000,
            edgeElasticity: 0.1,
            gravity: 0.02,
            gravityRange: 1.5,
            numIter: 5000,
            tile: true,
            tilingPaddingVertical: 60,
            tilingPaddingHorizontal: 60,
            nodeSeparation: 200
          },
          wheelSensitivity: 0.3
        });

        // Add click handler to show entity info
        this._entityCy.on('tap', 'node', (evt) => {
          const node = evt.target;
          this.addNotification('info', node.data('label') + ' (' + node.data('type') + ')');
        });

        this.entityGraphLoaded = true;
        this._applyEntityGraphLabelStyle();
        this.entityGraphStatus = '已加载 ' + nodes.length + ' 个实体，' + edges.length + ' 条关系';
        this.addNotification('success', this.entityGraphStatus);
      } catch (err) {
        this.entityGraphStatus = '错误：' + err.message;
        this.addNotification('error', '实体图谱加载失败：' + err.message);
      }
    },

    getEntityTypeColor(entityType) {
      const colors = {
        'person': '#3b82f6',
        'organization': '#8b5cf6',
        'technology': '#10b981',
        'project': '#f59e0b',
        'location': '#ec4899',
        'concept': '#6366f1',
        'tool': '#14b8a6',
        'database': '#8b5cf6',
        'service': '#06b6d4',
        'framework': '#10b981',
        'language': '#f59e0b',
        'product': '#ef4444',
        'event': '#84cc16',
        'other': '#64748b'
      };
      return colors[entityType] || colors['other'];
    },

    entityGraphZoomIn() {
      if (this._entityCy) { this._entityCy.zoom(this._entityCy.zoom() * 1.25); this._entityCy.center(); }
    },

    entityGraphZoomOut() {
      if (this._entityCy) { this._entityCy.zoom(this._entityCy.zoom() * 0.8); this._entityCy.center(); }
    },

    entityGraphFit() {
      if (this._entityCy) { this._entityCy.fit(); }
    },
    entityGraphCycleLabel() {
      const modes = ['full', 'short', 'none'];
      const next = modes[(modes.indexOf(this.entityGraphLabelMode) + 1) % modes.length];
      this.entityGraphLabelMode = next;
      this._applyEntityGraphLabelStyle();
    },
    _applyEntityGraphLabelStyle() {
      if (!this._entityCy) return;
      const mode = this.entityGraphLabelMode;
      this._entityCy.nodes().forEach(node => {
        const lbl = mode === 'full' ? node.data('label')
                  : mode === 'short' ? node.data('shortLabel')
                  : node.data('label');
        node.style({
          label: lbl,
          'text-opacity': mode === 'none' ? 0 : 1,
        });
      });
      this._entityCy.edges().forEach(edge => {
        edge.style({ 'text-opacity': mode === 'none' ? 0 : 1 });
      });
    },

    // ── Settings / Vault ───────────────────────────────────────────────────
    async loadEmbedStatus() {
      try {
        const data = await this.apiCall('/api/admin/embed/status');
        this.embedStatus = data;
        // Reflect the active provider in the plugin config UI
        const p = data?.provider;
        if (p && p !== 'none') {
          this.pluginCfg.embedProvider = p;
        }
        this.pluginCfg.embedRatePerSec = data.rate_per_sec ?? 0;
        this.pluginCfg.embedETASecs    = data.eta_seconds ?? 0;
        // hardware_accelerated is absent for cloud providers; present (true/false) for Ollama
        if (Object.prototype.hasOwnProperty.call(data, 'hardware_accelerated')) {
          this.pluginCfg.embedHardwareGPU = data.hardware_accelerated;
        } else {
          this.pluginCfg.embedHardwareGPU = null;
        }
      } catch (_) {
        // Non-fatal: embedStatus stays null, UI shows fallback
        this.embedStatus = null;
      }
    },

    async loadWorkers() {
        try {
            this.cogWorkerStats = await this.apiCall('/api/workers');
        } catch (e) {
            this.cogWorkerStats = null;
        }
    },
    workerRows() {
        const ws = this.cogWorkerStats || {};
        const toRow = (name, stats) => ({
            name,
            active: stats && (stats.processed > 0 || stats.running),
            processed: stats ? (stats.processed ?? '—') : '—',
        });
        return [
            toRow('赫布学习', ws.hebbian),
            toRow('时间评分', ws.decay),
            toRow('矛盾检测', ws.contradict),
            toRow('置信度更新', ws.confidence),
        ];
    },

    async loadPlasticity() {
        if (!this.isAuthenticated) return;
        try {
            this.plasticitySaveErr = '';
            const data = await this.apiCall(
                '/api/admin/vault/' + encodeURIComponent(this.vault) + '/plasticity'
            );
            const cfg = data.config || {};
            this.plasticityForm.preset         = cfg.preset || 'default';
            this.plasticityForm.hebbianEnabled = data.resolved?.hebbian_enabled ?? true;
            this.plasticityForm.temporalEnabled   = data.resolved?.temporal_enabled   ?? true;
            this.plasticityForm.hopDepth       = cfg.hop_depth       ?? null;
            this.plasticityForm.semanticWeight = cfg.semantic_weight ?? null;
            this.plasticityForm.ftsWeight      = cfg.fts_weight      ?? null;
            this.plasticityForm.relevanceFloor     = cfg.relevance_floor     ?? null;
            this.plasticityForm.temporalHalflife = cfg.temporal_halflife ?? null;
            this.plasticityForm.recallMode = cfg.recall_mode || data.resolved?.recall_mode || 'balanced';
            this.$nextTick(() => this._updatePlasticityChart());
        } catch (err) {
            console.error('loadPlasticity error:', err);
            this.plasticitySaveErr = '加载可塑性设置失败';
        }
    },
    onPlasticityPresetChange() {
        this.plasticityForm.hopDepth       = null;
        this.plasticityForm.semanticWeight = null;
        this.plasticityForm.ftsWeight      = null;
        this.plasticityForm.relevanceFloor     = null;
        this.plasticityForm.temporalHalflife = null;
        this.plasticityForm.hebbianEnabled = true;
        this.plasticityForm.temporalEnabled   = true;
        this._updatePlasticityChart();
    },
    _plasticityData: {
        'default':         [0.30, 0.40, 0.50, 0.70, 0.60],
        'reference':       [1.00, 0.35, 0.375, 0.80, 0.70],
        'scratchpad':      [0.05, 0.00, 0.00, 0.70, 0.60],
        'knowledge-graph': [0.70, 1.00, 1.00, 0.75, 0.50],
    },
    _plasticityColors: {
        'default':         { border: '#6366f1', bg: 'rgba(99,102,241,0.35)' },
        'reference':       { border: '#10b981', bg: 'rgba(16,185,129,0.35)' },
        'scratchpad':      { border: '#f59e0b', bg: 'rgba(245,158,11,0.35)' },
        'knowledge-graph': { border: '#ec4899', bg: 'rgba(236,72,153,0.35)' },
    },
    initPlasticityChart() {
        const canvas = document.getElementById('plasticityChart');
        if (!canvas) return;
        const existing = Chart.getChart(canvas);
        if (existing) existing.destroy();
        const p = this.plasticityForm.preset;
        const c = this._plasticityColors[p];
        new Chart(canvas, {
            type: 'radar',
            data: {
                labels: ['记忆寿命', '关联学习', '图深度', '语义匹配', 'FTS 相关性'],
                datasets: [{
                    data: this._plasticityData[p],
                    borderColor: c.border,
                    backgroundColor: c.bg,
                    borderWidth: 2.5,
                    pointRadius: 5,
                    pointBackgroundColor: c.border,
                }],
            },
            options: {
                responsive: true,
                maintainAspectRatio: true,
                animation: { duration: 300 },
                scales: { r: {
                    min: 0, max: 1,
                    ticks: { display: false },
                    grid: { color: this.isDarkMode ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.08)' },
                    angleLines: { color: this.isDarkMode ? 'rgba(255,255,255,0.08)' : 'rgba(0,0,0,0.08)' },
                    pointLabels: { color: this.isDarkMode ? '#9ca3af' : '#6b7280', font: { size: 11 } },
                }},
                plugins: { legend: { display: false } },
            },
        });
    },
    _updatePlasticityChart() {
        const canvas = document.getElementById('plasticityChart');
        if (!canvas) return;
        const chart = Chart.getChart(canvas);
        if (!chart) return;
        const ds = chart.data.datasets[0];

        if (this.plasticityForm.showAdvanced) {
            // Compute chart shape from override values, falling back to base preset
            const p    = this.plasticityForm.preset || 'default';
            const base = this._plasticityData[p] || this._plasticityData['default'];
            const f    = this.plasticityForm;
            // dimensions: [Memory Lifespan, Associative Learning, Graph Depth, Semantic Match, FTS Relevance]
            const lifespan = f.relevanceFloor     != null ? Math.min(1, Math.max(0, f.relevanceFloor))     : base[0];
            const assoc    = f.hebbianEnabled
                ? (f.temporalHalflife != null ? Math.min(1, f.temporalHalflife / 60) : base[1])
                : 0;
            const depth    = f.hopDepth       != null ? Math.min(1, f.hopDepth / 8)                : base[2];
            const semW     = f.semanticWeight != null ? Math.min(1, Math.max(0, f.semanticWeight)) : base[3];
            const ftsW     = f.ftsWeight      != null ? Math.min(1, Math.max(0, f.ftsWeight))      : base[4];
            ds.data             = [lifespan, assoc, depth, semW, ftsW];
            ds.borderColor      = '#94a3b8';
            ds.backgroundColor  = 'rgba(148,163,184,0.35)';
            ds.pointBackgroundColor = '#94a3b8';
        } else {
            const p = this.plasticityForm.preset;
            const c = this._plasticityColors[p];
            ds.data             = this._plasticityData[p];
            ds.borderColor      = c.border;
            ds.backgroundColor  = c.bg;
            ds.pointBackgroundColor = c.border;
        }
        chart.update();
    },
    plasticityPresetDescription(preset) {
        const d = {
            'default':         '通用模式。启用时间评分与赫布学习，2 跳 BFS，权重均衡。',
            'reference':       '文档与事实。关闭时间评分——记忆可长期保留。',
            'scratchpad':      '临时草稿。激进衰减（7 天半衰期，0.01 下限）。不启用赫布学习、无跳数扩展。',
            'knowledge-graph': '高密度互联概念。4 跳 BFS，缓慢衰减（60 天半衰期）。',
        };
        return d[preset] || '';
    },
    async savePlasticity() {
        this.plasticitySaving = true;
        this.plasticitySaveOk = false;
        this.plasticitySaveErr = '';
        try {
            const payload = { version: 1, preset: this.plasticityForm.preset };
            payload.recall_mode = this.plasticityForm.recallMode;
            if (this.plasticityForm.showAdvanced) {
                if (this.plasticityForm.hopDepth       !== null) payload.hop_depth       = this.plasticityForm.hopDepth;
                if (this.plasticityForm.semanticWeight !== null) payload.semantic_weight = this.plasticityForm.semanticWeight;
                if (this.plasticityForm.ftsWeight      !== null) payload.fts_weight      = this.plasticityForm.ftsWeight;
                if (this.plasticityForm.relevanceFloor     !== null) payload.relevance_floor     = this.plasticityForm.relevanceFloor;
                if (this.plasticityForm.temporalHalflife !== null) payload.temporal_halflife = this.plasticityForm.temporalHalflife;
                payload.hebbian_enabled = this.plasticityForm.hebbianEnabled;
                payload.temporal_enabled   = this.plasticityForm.temporalEnabled;
            }
            await this.apiCall(
                '/api/admin/vault/' + encodeURIComponent(this.vault) + '/plasticity',
                { method: 'PUT', body: JSON.stringify(payload) }
            );
            await this.loadPlasticity();
            this.plasticitySaveOk = true;
            setTimeout(() => { this.plasticitySaveOk = false; }, 3000);
        } catch (err) {
            this.plasticitySaveErr = err.message;
            setTimeout(() => { this.plasticitySaveErr = ''; }, 5000);
        } finally {
            this.plasticitySaving = false;
        }
    },

    async copyToClipboard(text) {
      try {
        await navigator.clipboard.writeText(text);
        this.connectCopied = true;
        setTimeout(() => { this.connectCopied = false; }, 2000);
        this.addNotification('success', '已复制到剪贴板');
      } catch (_) {
        this.addNotification('error', '复制失败——请手动选择并复制');
      }
    },

    confLabel(v) {
      const CONFIDENCE_HIGH = 0.7;
      const CONFIDENCE_MED  = 0.4;
      if (v >= CONFIDENCE_HIGH) return '高';
      if (v >= CONFIDENCE_MED)  return '中';
      return '低';
    },

    confLabelClass(v) {
      const CONFIDENCE_HIGH = 0.7;
      const CONFIDENCE_MED  = 0.4;
      if (v >= CONFIDENCE_HIGH) return 'badge-active';
      if (v >= CONFIDENCE_MED)  return 'badge-warning';
      return 'badge-dormant';
    },

    embedProgressPct() {
      if (!this.embedStatus) return 0;
      const total = this.embedStatus.total_count;
      const embedded = this.embedStatus.embedded_count;
      if (total <= 0 || embedded < 0) return 0;
      return Math.min(100, Math.round((embedded / total) * 100));
    },

    embedSecsPerItem() {
      if (this.pluginCfg.embedRatePerSec > 0) {
        return (1 / this.pluginCfg.embedRatePerSec).toFixed(1) + '秒/条';
      }
      return '';
    },

    embedETADisplay() {
      const secs = this.pluginCfg.embedETASecs;
      if (secs <= 0) return '';
      if (secs < 60) return '< 1 分钟';
      const mins = Math.round(secs / 60);
      if (mins < 60) return '约 ' + mins + ' 分钟';
      const hrs = Math.floor(mins / 60);
      const rem = mins % 60;
      return rem > 0 ? '约 ' + hrs + ' 小时 ' + rem + ' 分钟' : '约 ' + hrs + ' 小时';
    },

    get embedIsCPU() {
      return this.pluginCfg.embedHardwareGPU === false;
    },

    async reembedVault() {
      if (!confirm(`要重新嵌入仓库“${this.vault}”吗？\n\n这会清除所有嵌入，并让 RetroactiveProcessor 使用当前模型重新嵌入每条记忆。\n\n迁移期间仓库仍可查询（召回质量会下降）。`)) return;
      try {
        const data = await this.apiCall('/api/admin/vaults/' + encodeURIComponent(this.vault) + '/reembed', { method: 'POST' });
        this.addNotification('success', `重新嵌入已开始（任务 ${data.job_id}）。可在嵌入状态中查看进度。`);
        // Refresh embed status to show progress.
        this.loadEmbedStatus();
      } catch (e) {
        this.addNotification('error', '重新嵌入失败：' + (e?.message || '未知错误'));
      }
    },

    openVaultAction(action) {
      this.vaultActionModal = {
        show: true,
        action,
        vault: this.vault,
        confirmText: '',
        memCount: this.stats?.engramCount || 0,
      };
    },

    async confirmVaultAction() {
      const { action, vault } = this.vaultActionModal;
      this.vaultActionModal.show = false;
      const method = action === 'delete' ? 'DELETE' : 'POST';
      const path = action === 'delete'
        ? '/api/admin/vaults/' + encodeURIComponent(vault)
        : '/api/admin/vaults/' + encodeURIComponent(vault) + '/clear';
      const headers = { 'Content-Type': 'application/json' };
      if (vault === 'default') {
        headers['X-Allow-Default'] = 'true';
      }
      try {
        const r = await fetch(path, { method, headers });
        if (r.ok) {
          if (action === 'delete') {
            await this.loadVaults();
            if (this.vault === vault) {
              this.vault = this.vaults?.[0] || '';
              localStorage.setItem('muninnVault', this.vault);
            }
            this.addNotification('success', '仓库已删除');
          } else {
            this.addNotification('success', '记忆已清除');
          }
        } else if (r.status === 401) {
          this.addNotification('error', '未认证');
        } else if (r.status === 409) {
          this.addNotification('error', '受保护仓库——无法修改 default');
        } else {
          this.addNotification('error', '错误：' + r.status);
        }
      } catch (e) {
        this.addNotification('error', '网络错误');
      }
    },

    openVaultRename() {
      const newName = prompt('请输入仓库“' + this.vault + '”的新名称：');
      if (!newName || newName === this.vault) return;
      this.renameVault(newName);
    },

    async renameVault(newName, force) {
      try {
        const url = '/api/admin/vaults/' + encodeURIComponent(this.vault) + '/rename' + (force ? '?force=true' : '');
        const r = await fetch(url, {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ new_name: newName }),
        });
        if (r.status === 409) {
          const data = await r.json().catch(() => null);
          if (data && data.code === 'VAULT_NAME_COLLISION') {
            const proceed = confirm(
              '名为“' + data.conflict + '”的仓库已存在且名称相似。\n\n仍要创建“' + newName + '”吗？'
            );
            if (proceed) {
              this.renameVault(newName, true);
            }
            return;
          }
        }
        if (!r.ok) {
          const err = await r.json().catch(() => null);
          const msg = err && err.error && err.error.message ? err.error.message : 'HTTP ' + r.status;
          this.addNotification('error', '重命名失败：' + msg);
          return;
        }
        this.vault = newName;
        this.loadVaults();
        this.addNotification('success', '仓库已重命名为“' + newName + '”');
      } catch (e) {
        this.addNotification('error', '重命名失败：' + e.message);
      }
    },

    openVaultClone() {
      if (this.activeJob && this.activeJob.status === 'running') {
        this.addNotification('warning', '克隆或合并任务仍在进行中。');
        return;
      }
      this.cloneModal = { show: true, source: this.vault, newName: '' };
      this.clearActiveJob();
    },

    openVaultMerge() {
      if (this.activeJob && this.activeJob.status === 'running') {
        this.addNotification('warning', '克隆或合并任务仍在进行中。');
        return;
      }
      this.mergeModal = { show: true, source: this.vault, target: '', deleteSource: false };
      this.clearActiveJob();
    },

    async startClone() {
      if (!this.cloneModal.newName) return;
      const r = await fetch(
        '/api/admin/vaults/' + encodeURIComponent(this.cloneModal.source) + '/clone',
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ new_name: this.cloneModal.newName }),
        }
      );
      if (!r.ok) {
        this.addNotification('error', '克隆失败：' + r.status);
        return;
      }
      const { job_id } = await r.json();
      this.startJobPolling(job_id, this.cloneModal.source, () => {
        this.loadVaults();
        this.cloneModal.show = false;
        this.addNotification('success', '仓库克隆成功');
      });
    },

    async startMerge() {
      if (!this.mergeModal.target) return;
      const r = await fetch(
        '/api/admin/vaults/' + encodeURIComponent(this.mergeModal.source) + '/merge-into',
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ target: this.mergeModal.target, delete_source: this.mergeModal.deleteSource }),
        }
      );
      if (!r.ok) {
        this.addNotification('error', '合并失败：' + r.status);
        return;
      }
      const { job_id } = await r.json();
      this.startJobPolling(job_id, this.mergeModal.source, () => {
        this.loadVaults();
        this.mergeModal.show = false;
        this.addNotification('success', '仓库合并成功');
      });
    },

    startJobPolling(jobId, vaultName, onComplete) {
      this.clearActiveJob();
      this.activeJob = { status: 'running', pct: 0, phase: 'copying', copy_current: 0, copy_total: 0 };
      this.jobPollInterval = setInterval(async () => {
        try {
          const s = await fetch(
            '/api/admin/vaults/' + encodeURIComponent(vaultName) + '/job-status?job_id=' + jobId
          );
          if (!s.ok) return;
          const snap = await s.json();
          this.activeJob = snap;
          if (snap.status !== 'running') {
            this.clearActiveJob();
            if (snap.status === 'done') {
              onComplete();
            } else {
               this.addNotification('error', '任务失败：' + (snap.error || '未知错误'));
            }
          }
        } catch (e) {
          // network hiccup — keep polling
        }
      }, 1000);
    },

    clearActiveJob() {
      if (this.jobPollInterval) {
        clearInterval(this.jobPollInterval);
        this.jobPollInterval = null;
      }
      this.activeJob = null;
    },

    async exportVault() {
      this.vaultExporting = true;
      try {
        const res = await fetch('/api/admin/vaults/' + encodeURIComponent(this.vault) + '/export');
        if (!res.ok) {
          const text = await res.text().catch(() => res.statusText);
          throw new Error(res.status + ': ' + text);
        }
        const blob = await res.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = this.vault + '.muninn';
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
        this.addNotification('success', '仓库已导出：' + this.vault + '.muninn');
      } catch (e) {
        this.addNotification('error', '导出失败：' + (e?.message || '未知错误'));
      } finally {
        this.vaultExporting = false;
      }
    },

    openImportModal() {
      this.importModal = { show: true, vaultName: '', file: null, resetMeta: false };
    },

    async startImport() {
      if (!this.importModal.vaultName || !this.importModal.file) return;
      const params = new URLSearchParams({
        vault: this.importModal.vaultName,
        reset_metadata: this.importModal.resetMeta ? 'true' : 'false',
      });
      try {
        const res = await fetch('/api/admin/vaults/import?' + params.toString(), {
          method: 'POST',
          headers: { 'Content-Type': 'application/octet-stream' },
          body: this.importModal.file,
        });
        if (!res.ok) {
          const text = await res.text().catch(() => res.statusText);
          throw new Error(res.status + ': ' + text);
        }
        const data = await res.json();
        const jobId = data.job_id;
        this.startJobPolling(jobId, this.importModal.vaultName, () => {
          this.loadVaults();
          this.importModal.show = false;
          this.addNotification('success', '仓库导入成功');
        });
      } catch (e) {
        this.addNotification('error', '导入失败：' + (e?.message || '未知错误'));
      }
    },

    async reindexFTS() {
      if (!confirm('要为仓库“' + this.vault + '”重建全文搜索索引吗？\n\n这会为所有记忆重建 FTS 索引。重建期间仓库仍可查询。')) return;
      this.reindexing = true;
      try {
        const res = await fetch(
          '/api/admin/vaults/' + encodeURIComponent(this.vault) + '/reindex-fts',
          { method: 'POST' }
        );
        if (!res.ok) {
          const text = await res.text().catch(() => res.statusText);
          throw new Error(res.status + ': ' + text);
        }
        const data = await res.json();
        this.addNotification('success', 'FTS 重建索引完成——已重建 ' + (data.engrams_reindexed || 0) + ' 条记忆');
      } catch (e) {
        this.addNotification('error', '重建索引失败：' + (e?.message || '未知错误'));
      } finally {
        this.reindexing = false;
      }
    },

    createVault() {
      this.newVaultModal = { show: true, name: '', error: '', loading: false, collision: null };
    },

    async submitNewVault(force) {
      const name = this.newVaultModal.name.trim();
      if (!name) return;
      const valid = /^[a-z0-9_-]{1,64}$/.test(name);
      if (!valid) {
        this.newVaultModal.error = '仅允许小写字母、数字、连字符、下划线（1-64 个字符）';
        return;
      }
      this.newVaultModal.loading = true;
      this.newVaultModal.error = '';
      this.newVaultModal.collision = null;
      try {
        const url = force ? '/api/admin/vaults/config?force=true' : '/api/admin/vaults/config';
        const r = await fetch(url, {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name }),
        });
        if (r.status === 409) {
          const data = await r.json().catch(() => null);
          if (data && data.code === 'VAULT_NAME_COLLISION') {
            this.newVaultModal.collision = data;
            this.newVaultModal.loading = false;
            return;
          }
          const text = await r.text().catch(() => r.statusText);
          throw new Error(r.status + ': ' + text);
        }
        if (!r.ok) {
          const text = await r.text().catch(() => r.statusText);
          throw new Error(r.status + ': ' + text);
        }
        await fetch('/api/hello', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ version: '1', vault: name }),
        }).catch(() => {});
        this.vault = name;
        localStorage.setItem('muninnVault', name);
        await this.loadVaults();
        this.newVaultModal.loading = false;
        this.newVaultModal.show = false;
        this.addNotification('success', '仓库“' + name + '”已创建');
      } catch (err) {
        this.newVaultModal.error = err.message;
        this.newVaultModal.loading = false;
      }
    },

    // ── Cluster ────────────────────────────────────────────────────────────
    async loadClusterDashboard() {
      // Clear any existing intervals and SSE feed before setting up new ones
      if (this._clusterNodesInterval) {
        clearInterval(this._clusterNodesInterval);
        this._clusterNodesInterval = null;
      }
      if (this._clusterCCSInterval) {
        clearInterval(this._clusterCCSInterval);
        this._clusterCCSInterval = null;
      }
      this.stopClusterFeed();

      await this._loadClusterInfo();

      if (this.clusterEnabled) {
        this._clusterNodesInterval = setInterval(() => this._loadClusterNodes(), 5000);
        this._clusterCCSInterval = setInterval(() => this._loadClusterCCS(), 30000);
      }
    },

    async _loadClusterInfo() {
      try {
        const info = await this.apiCall('/v1/cluster/info');
        // If cluster is disabled, info has { enabled: false }
        if (info.enabled === false) {
          this.clusterEnabled = false;
          return;
        }
        this.clusterEnabled = true;
        try {
          const secResp = await fetch('/api/admin/cluster/token', { credentials: 'same-origin' });
          if (secResp.ok) this.clusterSecurityPosture = await secResp.json();
        } catch (err) {
          console.warn('[muninn] cluster security posture fetch failed:', err);
        }
        await Promise.all([
          this._loadClusterNodes(),
          this._loadClusterHealth(),
          this._loadClusterCCS(),
        ]);
      } catch (_) {
        this.clusterEnabled = false;
      }
    },

    async _loadClusterNodes() {
      try {
        const data = await this.apiCall('/v1/cluster/nodes');
        const health = await this.apiCall('/v1/cluster/health');
        const cortexId = health.is_leader ? health.role : null;
        const prevEpoch = this.clusterHealth ? this.clusterHealth.epoch : null;
        const newEpoch = health.epoch || 0;

        this.clusterNodes = (data.nodes || []).map(n => ({
          nodeId: n.node_id,
          role: n.role,
          status: this._nodeStatus(n, health),
          epoch: newEpoch,
          lag: n.last_seq,
        }));

        this.clusterHealth = health;

        // Detect epoch change → record failover event
        if (prevEpoch !== null && newEpoch !== prevEpoch && newEpoch > 0) {
          this._recordFailoverEvent(newEpoch, health);
        }
      } catch (err) {
        console.warn('[muninn] cluster nodes fetch failed:', err);
      }
    },

    async _loadClusterHealth() {
      try {
        this.clusterHealth = await this.apiCall('/v1/cluster/health');
      } catch (err) {
        console.warn('[muninn] cluster health fetch failed:', err);
      }
    },

    async _loadClusterCCS() {
      try {
        this.clusterCCS = await this.apiCall('/v1/cluster/cognitive/consistency');
      } catch (err) {
        console.warn('[muninn] cluster CCS fetch failed:', err);
      }
    },

    _nodeStatus(node, health) {
      if (!health) return 'unknown';
      if (health.status === 'down') return 'down';
      if (node.role === 'primary' || node.role === 'cortex') return 'healthy';
      const lag = node.last_seq || 0;
      if (lag >= 10000) return 'down';
      if (lag >= 1000) return 'degraded';
      return 'healthy';
    },

    _recordFailoverEvent(epoch, health) {
      const stored = JSON.parse(localStorage.getItem('muninnClusterEvents') || '[]');
      const ts = new Date().toISOString();
      const cortexId = health.cortex_id || health.node_id || 'unknown';
      const entry = {
        ts,
        epoch,
        cortexId,
        label: '[' + ts + '] 纪元 ' + epoch + '：' + cortexId + ' 成为 Cortex',
      };
      stored.unshift(entry);
      const trimmed = stored.slice(0, 10);
      localStorage.setItem('muninnClusterEvents', JSON.stringify(trimmed));
      this.clusterEvents = trimmed;
    },

    loadClusterEvents() {
      this.clusterEvents = JSON.parse(localStorage.getItem('muninnClusterEvents') || '[]');
    },

    async enableCluster() {
      this.clusterEnableLoading = true;
      this.clusterEnableError = null;
        this.clusterEnableProgress = ['正在校验设置...'];
      try {
        const resp = await fetch('/api/admin/cluster/enable', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'same-origin',
          body: JSON.stringify({
            role: this.clusterEnableForm.role,
            bind_addr: this.clusterEnableForm.bindAddr,
            cluster_secret: this.clusterEnableForm.clusterSecret,
            cortex_addr: this.clusterEnableForm.cortexAddr,
          })
        });
        if (!resp.ok) {
          const err = await resp.json().catch(() => ({ error: {message:'启用解析失败'} }));
          throw new Error(err.error.message || '启用失败');
        }
        this.clusterEnableProgress = ['正在初始化 TLS...', '正在生成加入令牌...', '正在启动心跳...'];
        await this._loadClusterInfo();
        this.clusterEnableProgress = [...this.clusterEnableProgress, '集群已激活 ✓'];
      } catch (e) {
        console.error(e)
        this.clusterEnableError = e.message;
      } finally {
        this.clusterEnableLoading = false;
      }
    },

    async testNodeReachability() {
      this.addNodeTesting = true;
      this.addNodeTestResult = null;
      try {
        const resp = await fetch('/api/admin/cluster/nodes/test', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'same-origin',
          body: JSON.stringify({ addr: this.addNodeForm.addr })
        });
        const data = await resp.json();
        this.addNodeTestResult = data;
      } catch (e) {
        this.addNodeTestResult = { reachable: false, error: e.message };
      } finally {
        this.addNodeTesting = false;
      }
    },

    async addNode() {
      this.addNodeLoading = true;
      this.addNodeError = null;
      this.addNodeProgress = ['正在校验令牌...'];
      try {
        const resp = await fetch('/api/admin/cluster/nodes', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'same-origin',
          body: JSON.stringify({ addr: this.addNodeForm.addr, token: this.addNodeForm.token })
        });
        if (!resp.ok) {
          const err = await resp.json().catch(() => ({ error: {message:'添加节点失败' }}));
          throw new Error(err.error.message || '添加节点失败');
        }
        this.addNodeProgress = ['正在注册对等节点...', '等待加入握手...', '节点已添加 ✓'];
        await new Promise(r => setTimeout(r, 1200));
        this.showAddNodeModal = false;
        this.addNodeForm = { addr: '', token: '' };
        this.addNodeProgress = [];
      } catch (e) {
        this.addNodeError = e.message;
      } finally {
        this.addNodeLoading = false;
      }
    },

    async removeNode() {
      if (!this.removeNodeTarget) return;
      this.removeNodeLoading = true;
      this.removeNodeError = null;
      try {
        const drain = this.removeNodeDrain ? '?drain=true' : '';
        const resp = await fetch(`/api/admin/cluster/nodes/${this.removeNodeTarget.nodeId}${drain}`, {
          method: 'DELETE',
          credentials: 'same-origin',
        });
        if (!resp.ok) {
          const err = await resp.json().catch(() => ({ error: {message:'删除失败' }}));
          throw new Error(err.error.message || '删除失败');
        }
        this.showRemoveNodeModal = false;
        this.removeNodeTarget = null;
        await this._loadClusterNodes();
      } catch (e) {
        this.removeNodeError = e.message;
      } finally {
        this.removeNodeLoading = false;
      }
    },

    async triggerFailover() {
      this.failoverLoading = true;
      this.failoverError = null;
      this.failoverProgress = ['正在发送切换请求...'];
      try {
        const resp = await fetch('/api/admin/cluster/failover', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'same-origin',
          body: JSON.stringify({ target_node_id: this.failoverTarget })
        });
        if (!resp.ok) {
          const err = await resp.json().message(() => ({ error: {message:'故障切换失败' }}));
          throw new Error(err.error.message || '故障切换失败');
        }
        this.failoverProgress = ['正在发送切换请求...', '已选出新 Cortex...', '切换已确认...', '完成 ✓'];
        await new Promise(r => setTimeout(r, 1500));
        this.showFailoverModal = false;
        this.failoverProgress = [];
      } catch (e) {
        this.failoverError = e.message;
      } finally {
        this.failoverLoading = false;
      }
    },

    startClusterFeed() {
      if (this._clusterFeedSSE) return;
      const es = new EventSource('/api/admin/cluster/events');
      es.addEventListener('entry', (e) => {
        if (this.clusterFeedPaused) return;
        try {
          const data = JSON.parse(e.data);
          this.clusterFeed.unshift({ ...data, ts: new Date().toLocaleTimeString() });
          if (this.clusterFeed.length > 200) this.clusterFeed.pop();
        } catch (err) {
          console.warn('[muninn] cluster feed parse error:', err, e.data);
        }
      });
      this._clusterFeedSSE = es;
    },

    stopClusterFeed() {
      if (this._clusterFeedSSE) {
        this._clusterFeedSSE.close();
        this._clusterFeedSSE = null;
      }
    },

    async loadClusterToken() {
      this.clusterTokenLoading = true;
      try {
        const resp = await fetch('/api/admin/cluster/token', { credentials: 'same-origin' });
        if (resp.ok) this.clusterToken = await resp.json();
      } catch (err) {
        console.warn('[muninn] cluster token load failed:', err);
      } finally { this.clusterTokenLoading = false; }
    },

    async regenerateToken() {
      this.showRegenerateTokenConfirm = false;
      try {
        const resp = await fetch('/api/admin/cluster/token/regenerate', {
          method: 'POST',
          credentials: 'same-origin',
        });
        if (resp.ok) this.clusterToken = await resp.json();
      } catch (err) {
        this.addNotification('error', '令牌重新生成失败：' + err.message);
      }
    },

    copyToken() {
      if (!this.clusterToken?.token) return;
      navigator.clipboard.writeText(this.clusterToken.token).catch(() => {});
      this.clusterTokenCopied = true;
      setTimeout(() => { this.clusterTokenCopied = false; }, 2000);
    },

    async loadClusterSettings() {
      try {
        const resp = await fetch('/api/admin/cluster/settings', { credentials: 'same-origin' });
        if (resp.ok) {
          const data = await resp.json();
          this.clusterSettings = {
            heartbeat_ms: data.heartbeat_ms ?? this.clusterSettings.heartbeat_ms,
            sdown_beats: data.sdown_beats ?? this.clusterSettings.sdown_beats,
            ccs_interval_seconds: data.ccs_interval_seconds ?? this.clusterSettings.ccs_interval_seconds,
            reconcile_on_heal: data.reconcile_on_heal ?? this.clusterSettings.reconcile_on_heal,
          };
        }
      } catch (err) {
        // Non-fatal — form shows defaults; user can still save.
      }
    },

    async saveClusterSettings() {
      this.clusterSettingsSaving = true;
      this.clusterSettingsSaved = false;
      try {
        const resp = await fetch('/api/admin/cluster/settings', {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          credentials: 'same-origin',
          body: JSON.stringify(this.clusterSettings)
        });
        if (resp.ok) {
          this.clusterSettingsSaved = true;
          setTimeout(() => { this.clusterSettingsSaved = false; }, 2500);
        }
      } catch (err) {
        this.addNotification('error', '保存集群设置失败：' + err.message);
      } finally { this.clusterSettingsSaving = false; }
    },

    async rotateTLS() {
      try {
        const resp = await fetch('/api/admin/cluster/tls/rotate', {
          method: 'POST',
          credentials: 'same-origin',
        });
        if (!resp.ok) {
          this.addNotification('error', 'TLS 证书轮换失败');
        } else {
          this.addNotification('success', 'TLS 证书已轮换');
        }
      } catch (_) {
        this.addNotification('error', 'TLS 证书轮换失败');
      }
    },

    clusterBannerClass() {
      if (!this.clusterHealth) return 'cluster-banner-unknown';
      const s = this.clusterHealth.status;
      if (s === 'ok') return 'cluster-banner-ok';
      if (s === 'degraded') return 'cluster-banner-degraded';
      return 'cluster-banner-down';
    },

    clusterBannerText() {
      if (!this.clusterHealth) return '集群状态未知';
      const s = this.clusterHealth.status;
      const n = this.clusterNodes.length;
      if (s === 'ok') return '集群健康 — ' + n + ' 个节点';
      if (s === 'degraded') return '集群降级 — 请检查复制延迟';
      return '集群不可用 — 无法形成法定人数';
    },

    ccsScore() {
      if (!this.clusterCCS) return 0;
      return Math.round((this.clusterCCS.score || 0) * 100);
    },

    ccsBarColor() {
      const pct = this.ccsScore();
      if (pct >= 99) return '#22c55e';
      if (pct >= 90) return '#eab308';
      return '#ef4444';
    },

    nodeStatusBadgeClass(status) {
      if (status === 'healthy') return 'badge-active';
      if (status === 'degraded') return 'badge-warning';
      return 'badge-dormant';
    },

    // ── Notifications ──────────────────────────────────────────────────────
    addNotification(type, msg) {
      const id = ++this._notifId;
      this.notifications.push({ id, type, msg });
      const timeout = type === 'error' ? 6000 : 4000;
      setTimeout(() => this.removeNotification(id), timeout);
    },

    removeNotification(id) {
      this.notifications = this.notifications.filter(n => n.id !== id);
    },
  }));
});
