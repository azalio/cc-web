// Claude Code Mobile Terminal - PWA Frontend
(function () {
  'use strict';

  // --- State ---
  let authToken = localStorage.getItem('cc_auth_token') || '';
  let sessions = [];
  let currentSessionId = null;
  let currentView = 'login'; // login | sessions | session
  let filterStatus = 'all'; // all | running | exited

  // --- API ---
  const api = {
    async fetch(path, opts = {}) {
      const resp = await fetch(path, {
        ...opts,
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${authToken}`,
          ...(opts.headers || {}),
        },
      });
      if (resp.status === 401) {
        logout();
        throw new Error('Unauthorized');
      }
      return resp;
    },

    async listSessions() {
      const resp = await this.fetch('/api/sessions');
      return resp.json();
    },

    async createSession(name, cwd, startCmd) {
      const resp = await this.fetch('/api/sessions', {
        method: 'POST',
        body: JSON.stringify({ name, cwd, start_cmd: startCmd }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || 'Failed to create session');
      return data;
    },

    async getSession(id) {
      const resp = await this.fetch(`/api/sessions/${id}`);
      return resp.json();
    },

    async killSession(id) {
      const resp = await this.fetch(`/api/sessions/${id}/kill`, { method: 'POST' });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || 'Failed to kill session');
      return data;
    },

    async sendText(id, text) {
      const resp = await this.fetch(`/api/sessions/${id}/send`, {
        method: 'POST',
        body: JSON.stringify({ text }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || 'Failed to send text');
      return data;
    },

    async interrupt(id) {
      const resp = await this.fetch(`/api/sessions/${id}/interrupt`, { method: 'POST' });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || 'Failed to interrupt');
      return data;
    },

    async sendKeys(id, keys) {
      const resp = await this.fetch(`/api/sessions/${id}/keys`, {
        method: 'POST',
        body: JSON.stringify({ keys }),
      });
      const data = await resp.json();
      if (!resp.ok) throw new Error(data.error || 'Failed to send keys');
      return data;
    },
  };

  // --- DOM refs ---
  const $ = (sel) => document.querySelector(sel);
  const $$ = (sel) => document.querySelectorAll(sel);

  // --- Toast ---
  function toast(msg, type = 'info') {
    const container = $('.toast-container');
    const el = document.createElement('div');
    el.className = `toast toast-${type}`;
    el.textContent = msg;
    container.appendChild(el);
    setTimeout(() => el.remove(), 3000);
  }

  // --- Auth ---
  function logout() {
    authToken = '';
    localStorage.removeItem('cc_auth_token');
    showView('login');
  }

  function login(token) {
    authToken = token;
    localStorage.setItem('cc_auth_token', token);
    // Set cookie for iframe auth
    document.cookie = `auth_token=${token};path=/;SameSite=Strict`;
    showView('sessions');
    refreshSessions();
  }

  // --- Views ---
  function showView(view) {
    currentView = view;
    document.body.className = `view-${view}`;

    $('#login-screen').style.display = view === 'login' ? 'flex' : 'none';
    $('#sessions-screen').style.display = view === 'sessions' ? 'flex' : 'none';
    $('#session-screen').style.display = view === 'session' ? 'flex' : 'none';

    // Show/hide back button
    if ($('#back-btn')) {
      $('#back-btn').style.display = view === 'session' ? 'block' : 'none';
    }
  }

  // --- Sessions List ---
  async function refreshSessions() {
    try {
      sessions = await api.listSessions();
      renderSessions();
    } catch (e) {
      if (e.message !== 'Unauthorized') {
        toast('Failed to load sessions', 'error');
      }
    }
  }

  function renderSessions() {
    const list = $('#sessions-list');
    const search = $('#search-input').value.toLowerCase();

    const filtered = sessions.filter(s => {
      if (filterStatus !== 'all' && s.status !== filterStatus) return false;
      if (search && !s.name.toLowerCase().includes(search) && !s.id.toLowerCase().includes(search)) return false;
      return true;
    });

    if (filtered.length === 0) {
      list.innerHTML = `
        <div class="empty-state">
          <h2>No sessions</h2>
          <p>${sessions.length === 0 ? 'Create your first Claude Code session' : 'No sessions match your filter'}</p>
        </div>`;
      return;
    }

    list.innerHTML = filtered.map(s => `
      <div class="session-card" data-id="${s.id}">
        <div class="session-card-header">
          <h3>${escapeHtml(s.name || s.id)}</h3>
          <span class="status-badge status-${s.status}">${s.status}</span>
        </div>
        ${s.cwd ? `<div class="cwd">${escapeHtml(s.cwd)}</div>` : ''}
        <div class="session-card-actions">
          <button class="btn btn-primary btn-sm" onclick="app.openSession('${s.id}')">Open</button>
          ${s.status === 'running' ? `<button class="btn btn-ghost btn-sm" onclick="app.interruptSession('${s.id}')">Interrupt</button>` : ''}
          <button class="btn btn-danger btn-sm" onclick="app.killSession('${s.id}')">Kill</button>
        </div>
      </div>
    `).join('');
  }

  // --- Open Session ---
  function openSession(id) {
    currentSessionId = id;
    const session = sessions.find(s => s.id === id);

    // Update header
    $('#session-name').textContent = session ? (session.name || session.id) : id;
    const badge = $('#session-status');
    if (session) {
      badge.textContent = session.status;
      badge.className = `status-badge status-${session.status}`;
    }

    // Load terminal iframe
    const container = $('#terminal-container');
    if (session && session.terminal_url) {
      const termUrl = `${session.terminal_url}?token=${encodeURIComponent(authToken)}`;
      container.innerHTML = `<iframe src="${termUrl}" allow="fullscreen"></iframe>`;
    } else {
      container.innerHTML = `
        <div class="terminal-placeholder">
          <p>Terminal not available</p>
          <p>ttyd may not be installed. You can still use the intervention panel below.</p>
        </div>`;
    }

    showView('session');
  }

  // --- Session actions ---
  async function interruptSession(id) {
    try {
      await api.interrupt(id);
      toast('Interrupted', 'success');
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function killSession(id) {
    if (!confirm('Kill this session?')) return;
    try {
      await api.killSession(id);
      toast('Session killed', 'success');
      if (currentSessionId === id) {
        showView('sessions');
        currentSessionId = null;
      }
      refreshSessions();
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function sendText(text) {
    if (!currentSessionId || !text) return;
    try {
      await api.sendText(currentSessionId, text);
      toast('Sent', 'success');
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  async function sendKeyAction(keys) {
    if (!currentSessionId) return;
    try {
      await api.sendKeys(currentSessionId, keys);
    } catch (e) {
      toast(e.message, 'error');
    }
  }

  // --- Create Session ---
  function showCreateModal() {
    $('#create-modal').classList.add('active');
    $('#create-name').value = '';
    $('#create-cwd').value = '';
    $('#create-cmd').value = 'claude';
    $('#create-error').style.display = 'none';
    $('#create-name').focus();
  }

  function hideCreateModal() {
    $('#create-modal').classList.remove('active');
  }

  async function createSession() {
    const name = $('#create-name').value.trim();
    const cwd = $('#create-cwd').value.trim();
    const cmd = $('#create-cmd').value.trim() || 'claude';

    if (!name) {
      showFormError('Name is required');
      return;
    }
    if (!cwd) {
      showFormError('Working directory is required');
      return;
    }

    const btn = $('#create-submit');
    btn.disabled = true;
    btn.innerHTML = '<span class="spinner"></span> Creating...';

    try {
      const session = await api.createSession(name, cwd, cmd);
      hideCreateModal();
      toast('Session created', 'success');
      await refreshSessions();
      openSession(session.id);
    } catch (e) {
      showFormError(e.message);
    } finally {
      btn.disabled = false;
      btn.textContent = 'Create';
    }
  }

  function showFormError(msg) {
    const el = $('#create-error');
    el.textContent = msg;
    el.style.display = 'block';
  }

  // --- Intervene Bottom Sheet ---
  function showIntervene() {
    $('#intervene-overlay').classList.add('active');
    $('#intervene-sheet').classList.add('active');
    $('#intervene-text').value = '';
    $('#intervene-text').focus();
  }

  function hideIntervene() {
    $('#intervene-overlay').classList.remove('active');
    $('#intervene-sheet').classList.remove('active');
  }

  function intervene(text, append) {
    const textarea = $('#intervene-text');
    const message = append ? `${textarea.value.trim()}\n${append}`.trim() : textarea.value.trim();
    if (!message) return;
    sendText(message);
    hideIntervene();
  }

  function sendMacro(text) {
    sendText(text);
    hideIntervene();
  }

  // --- Utility ---
  function escapeHtml(str) {
    const div = document.createElement('div');
    div.textContent = str;
    return div.innerHTML;
  }

  // --- Init ---
  function init() {
    // Login form
    $('#login-form').addEventListener('submit', (e) => {
      e.preventDefault();
      const token = $('#token-input').value.trim();
      if (token) login(token);
    });

    // Search
    $('#search-input').addEventListener('input', renderSessions);

    // Filter chips
    $$('.chip[data-filter]').forEach(chip => {
      chip.addEventListener('click', () => {
        filterStatus = chip.dataset.filter;
        $$('.chip[data-filter]').forEach(c => c.classList.remove('active'));
        chip.classList.add('active');
        renderSessions();
      });
    });

    // Create modal
    $('#new-session-btn').addEventListener('click', showCreateModal);
    $('#create-cancel').addEventListener('click', hideCreateModal);
    $('#create-submit').addEventListener('click', createSession);
    $('#create-modal').addEventListener('click', (e) => {
      if (e.target === e.currentTarget) hideCreateModal();
    });

    // Back button
    $('#back-btn').addEventListener('click', () => {
      showView('sessions');
      currentSessionId = null;
      $('#terminal-container').innerHTML = '';
      refreshSessions();
    });

    // Header interrupt
    $('#header-interrupt').addEventListener('click', () => {
      if (currentSessionId) interruptSession(currentSessionId);
    });

    // Keys bar
    $$('.key-btn[data-key]').forEach(btn => {
      btn.addEventListener('click', () => {
        const key = btn.dataset.key;
        sendKeyAction([key]);
      });
    });

    // Intervene button
    $('#intervene-open').addEventListener('click', showIntervene);
    $('#intervene-overlay').addEventListener('click', hideIntervene);
    $('#intervene-close').addEventListener('click', hideIntervene);

    // Intervene send actions
    $('#intervene-send').addEventListener('click', () => intervene());
    $('#intervene-send-no-refactor').addEventListener('click', () => intervene(null, "Don't refactor, focus only on the task."));
    $('#intervene-send-summarize').addEventListener('click', () => intervene(null, 'Summarize your progress so far.'));

    // Macros
    $$('.macro-chip[data-text]').forEach(chip => {
      chip.addEventListener('click', () => sendMacro(chip.dataset.text));
    });

    // Logout
    $('#logout-btn').addEventListener('click', logout);

    // Auto-refresh sessions every 5s when on sessions view
    setInterval(() => {
      if (currentView === 'sessions' && authToken) {
        refreshSessions();
      }
    }, 5000);

    // Check if already logged in
    if (authToken) {
      showView('sessions');
      // Set cookie for iframe auth
      document.cookie = `auth_token=${authToken};path=/;SameSite=Strict`;
      refreshSessions();
    } else {
      showView('login');
    }
  }

  // Expose functions for inline onclick handlers
  window.app = {
    openSession,
    interruptSession,
    killSession,
  };

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
  } else {
    init();
  }
})();
