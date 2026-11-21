'use strict';

(() => {
  const AS = window.AgentStack;
  const { $, state, api, showSection, toast, showNotice, runOperation, setTheme, initMenu } = AS;

  function renderOperationalStatus() {
    const items = Object.values(state.inventory?.items || {});
    const attention = items.filter(item => item.broken || item.incompatible).length;
    const connected = (state.environments?.environments || []).filter(environment => environment.state === 'connected').length;
    if (attention) {
      $('systemState').textContent = `${attention} item${attention === 1 ? ' needs' : 's need'} attention`;
      $('systemState').className = 'state-badge attention';
      return;
    }
    $('systemState').textContent = connected ? `${connected} connected environment${connected === 1 ? '' : 's'}` : 'System ready';
    $('systemState').className = 'state-badge success';
  }

  async function refreshInventory() {
    state.inventory = await api('inventory');
    if (state.catalog) AS.changes.load(state.catalog, state.inventory);
    return state.inventory;
  }

  async function refreshAll(button = null) {
    return runOperation(button, 'Refresh system', async () => {
      const [catalog, inventory] = await Promise.all([api('catalog'), api('inventory')]);
      state.catalog = catalog;
      state.inventory = inventory;
      AS.changes.load(catalog, inventory);
      await AS.environments.load();
      await Promise.allSettled([AS.sync.load(), AS.activity.load()]);
      renderOperationalStatus();
      return { catalog, inventory };
    }, {
      onFailure: error => {
        $('systemState').textContent = 'Refresh failed';
        $('systemState').className = 'state-badge error';
        showNotice('Could not refresh the system', error.message);
      },
    });
  }

  function bindNavigation() {
    document.querySelectorAll('[data-section]').forEach(button => button.addEventListener('click', () => showSection(button.dataset.section)));
    document.addEventListener('click', event => {
      const target = event.target.closest('[data-target-section]');
      if (target) showSection(target.dataset.targetSection);
    });
  }

  function bindSettings() {
    document.querySelectorAll('[data-theme-choice]').forEach(button => button.addEventListener('click', () => setTheme(button.dataset.themeChoice)));
    $('installSelfBtn').addEventListener('click', () => runOperation($('installSelfBtn'), 'Install command shortcut', () => api('install-self', { method: 'POST', body: JSON.stringify({ confirm: true }) }), { onSuccess: () => toast('Command shortcut installed.') }));
    $('shutdownBtn').addEventListener('click', async () => {
      if (!window.confirm('Stop AgentStack Manager? Active operations will be cancelled and this window will close.')) return;
      await api('shutdown', { method: 'POST' });
      showNotice('AgentStack Manager is stopping', 'This desktop window will close when shutdown completes.', 'success');
    });
  }

  function init() {
    AS.refreshAll = refreshAll;
    AS.refreshInventory = refreshInventory;
    AS.renderOperationalStatus = renderOperationalStatus;
    bindNavigation();
    bindSettings();
    initMenu();
    AS.changes.init();
    AS.environments.init();
    AS.activity.init();
    AS.sync.init();
    $('refreshBtn').addEventListener('click', () => refreshAll($('refreshBtn')));
    setTheme(localStorage.getItem('agentstack-theme') || 'system');
    const initial = location.hash.slice(1);
    showSection(['home', 'environments', 'sync', 'changes', 'activity'].includes(initial) ? initial : 'home', false);
    refreshAll();
  }

  if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', init, { once: true });
  else init();
})();
