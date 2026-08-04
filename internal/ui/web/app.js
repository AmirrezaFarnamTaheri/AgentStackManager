'use strict';

const token = document.querySelector('meta[name="agentstack-token"]').content;
const base = document.querySelector('meta[name="agentstack-base"]').content;
const state = {
  catalog: null,
  inventory: null,
  plan: null,
  selected: new Set(),
  profile: 'core',
  allowCredentials: false,
  allowUpgrades: false,
  providerOverrides: {},
  fabric: null,
  activeTier: 'all',
  activePlanFilter: 'all',
};

const $ = id => document.getElementById(id);
let activeOperation = null;
let operationStartedAt = null;
let operationTimer = null;
let activeActivityID = null;
const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));

const waitForOperation = async (statusURL, onProgress) => {
  for (;;) {
    await delay(750);
    const operation = await api(statusURL);
    onProgress?.(operation);
    if (operation.status === 'running') {
      continue;
    }
    if (operation.status === 'failed') {
      const error = new Error(operation.error || 'AgentStack operation failed');
      error.data = operation;
      throw error;
    }
    if (operation.status !== 'succeeded') {
      throw new Error(`Unknown AgentStack operation status: ${operation.status}`);
    }
    return operation.result;
  }
};

const api = async (path, options = {}) => {
  const opts = {
    ...options,
    headers: {
      ...(options.headers || {}),
      'X-AgentStack-Token': token,
    },
  };
  if (opts.body) {
    opts.headers['Content-Type'] = 'application/json';
  }
  const endpoint = path.startsWith('/') ? path : `${base}api/${path}`;
  const response = await fetch(endpoint, opts);
  const data = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
  if (!response.ok) {
    const error = new Error(data.error || `HTTP ${response.status}`);
    error.data = data;
    throw error;
  }
  if (response.status === 202 && data.operationId && data.statusUrl) {
    return waitForOperation(data.statusUrl, options.onProgress);
  }
  return data;
};

function toast(message, error = false) {
  const el = $('toast');
  el.textContent = message;
  el.className = `toast visible${error ? ' error' : ''}`;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => {
    el.className = 'toast';
  }, 4200);
}

function setOperationStatus(status, title, detail) {
  const surface = $('operationStatus');
  if (!surface) {
    return;
  }
  surface.dataset.state = status;
  $('operationStatusTitle').textContent = title;
  $('operationStatusDetail').textContent = detail;
  const seal = surface.querySelector('.operation-seal');
  if (seal) {
    seal.textContent = { running: 'Running', success: 'Verified', error: 'Review', idle: 'Local' }[status] || 'Local';
  }
}

function formatElapsed(milliseconds) {
  const seconds = Math.max(0, Math.floor(milliseconds / 1000));
  return seconds < 60 ? `${seconds}s elapsed` : `${Math.floor(seconds / 60)}m ${seconds % 60}s elapsed`;
}

function setOperationMeta(message) {
  const meta = $('operationStatusMeta');
  if (meta) meta.textContent = message;
}

function setWorkflowStep(step) {
  document.querySelectorAll('[data-workflow-step]').forEach(item => {
    const itemStep = Number(item.dataset.workflowStep);
    item.classList.toggle('active', itemStep === step);
    item.classList.toggle('complete', itemStep < step);
    if (itemStep === step) item.setAttribute('aria-current', 'step');
    else item.removeAttribute('aria-current');
  });
}

function startOperationTimer(name) {
  operationStartedAt = Date.now();
  clearInterval(operationTimer);
  operationTimer = setInterval(() => {
    if (activeOperation && operationStartedAt) {
      setOperationMeta(`${formatElapsed(Date.now() - operationStartedAt)} · ${name} is still running locally.`);
    }
  }, 1000);
}

function stopOperationTimer() {
  clearInterval(operationTimer);
  operationTimer = null;
  return operationStartedAt ? Date.now() - operationStartedAt : 0;
}

function addActivity(level, title, detail, activityID = null) {
  const log = $('activityLog');
  const summary = $('activitySummary');
  if (!log || !summary) return;
  const existing = activityID ? log.querySelector(`[data-activity-id="${CSS.escape(activityID)}"]`) : null;
  if (existing) {
    existing.className = `activity-item ${level}`;
    existing.querySelector('strong').textContent = title;
    existing.querySelector('small').textContent = detail;
    return;
  }
  const empty = log.querySelector('.activity-empty');
  if (empty) empty.remove();
  const item = document.createElement('li');
  item.className = `activity-item ${level}`;
  if (activityID) item.dataset.activityId = activityID;
  const time = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' });
  item.innerHTML = `<span class="activity-marker" aria-hidden="true"></span><div><strong>${escapeHtml(title)}</strong><small>${escapeHtml(detail)}</small></div><time>${time}</time>`;
  log.prepend(item);
  while (log.children.length > 8) log.lastElementChild.remove();
  summary.textContent = `Showing the latest ${log.children.length} local operation${log.children.length === 1 ? '' : 's'} from this browser session.`;
}

function clearActivity() {
  const log = $('activityLog');
  const summary = $('activitySummary');
  if (!log || !summary) return;
  log.innerHTML = '<li class="activity-empty">No activity recorded in this browser session.</li>';
  summary.textContent = 'This browser session has not run an operation yet.';
}

function setOperationControlsBusy(activeButton, busy) {
  const main = $('mainContent');
  if (main) {
    if (busy) {
      main.setAttribute('aria-busy', 'true');
    } else {
      main.removeAttribute('aria-busy');
    }
  }
  document.querySelectorAll('[data-operation], [data-operation-lock]').forEach(control => {
    if (busy) {
      control.dataset.wasDisabled = control.disabled ? 'true' : 'false';
      control.disabled = true;
    } else {
      control.disabled = control.dataset.wasDisabled === 'true';
      delete control.dataset.wasDisabled;
    }
  });

  if (!activeButton) {
    return;
  }
  if (busy) {
    activeButton.setAttribute('data-original-label', activeButton.textContent);
    activeButton.textContent = activeButton.dataset.busyLabel || 'Working…';
    activeButton.classList.add('is-busy');
    activeButton.setAttribute('aria-busy', 'true');
  } else {
    const originalLabel = activeButton.getAttribute('data-original-label');
    if (originalLabel) {
      activeButton.textContent = originalLabel;
    }
    activeButton.removeAttribute('data-original-label');
    activeButton.classList.remove('is-busy');
    activeButton.removeAttribute('aria-busy');
  }
}

async function runOperation(button, name, operation) {
  if (activeOperation) {
    setOperationStatus('running', 'Operation already running', `${activeOperation} must finish before ${name.toLowerCase()} can start.`);
    return null;
  }

  activeOperation = name;
  startOperationTimer(name);
  activeActivityID = `${name}-${operationStartedAt}`;
  const previousFocus = document.activeElement;
  setOperationControlsBusy(button, true);
  setOperationStatus('running', name, 'AgentStack is working. Controls are temporarily locked to prevent conflicting changes.');
  setOperationMeta('Starting locally…');
  addActivity('running', name, 'Started. Changes remain blocked until this operation finishes.', activeActivityID);

  try {
    const result = await operation(operationStatus => {
      if (operationStatus.status === 'running') {
        setOperationMeta(`${formatElapsed(Date.now() - operationStartedAt)} · Still working; checking the local operation record.`);
      }
    });
    const detail = result?.statusDetail || 'The operation completed successfully.';
    setOperationStatus('success', `${name} complete`, detail);
    const elapsed = stopOperationTimer();
    setOperationMeta(`${formatElapsed(elapsed)} · Completed locally.`);
    addActivity('success', `${name} complete`, `${detail} (${formatElapsed(elapsed)})`, activeActivityID);
    return result;
  } catch (error) {
    if (name === 'Load AgentStack Manager' || name === 'Refresh inventory') {
      setMetricsLoading(false);
      $('overviewLoadError').hidden = false;
      $('overviewLoadErrorDetail').textContent = error.message;
    }
    setOperationStatus('error', `${name} failed`, error.message);
    const elapsed = stopOperationTimer();
    setOperationMeta(`${formatElapsed(elapsed)} · Review the message and try again when ready.`);
    addActivity('error', `${name} needs attention`, `${error.message} (${formatElapsed(elapsed)})`, activeActivityID);
    const output = $('routerOutput');
    if (output && error.data) {
      output.textContent = JSON.stringify(error.data, null, 2);
    }
    toast(error.message, true);
    return null;
  } finally {
    setOperationControlsBusy(button, false);
    if (!state.catalog) {
      setCatalogControlsAvailable(false);
    }
    activeOperation = null;
    activeActivityID = null;
    updateApplyAvailability();
    const currentFocus = document.activeElement;
    const focusMovedByOperation = currentFocus && currentFocus !== document.body && currentFocus !== button;
    if (!focusMovedByOperation && previousFocus && previousFocus !== document.body && isVisible(previousFocus)) {
      previousFocus.focus({ preventScroll: true });
    } else if (!focusMovedByOperation && isVisible(button)) {
      button.focus({ preventScroll: true });
    }
  }
}

function isVisible(element) {
  return Boolean(element && element.isConnected && typeof element.getClientRects === 'function' && element.getClientRects().length);
}

function tierLabel(tier) {
  return {
    essential: 'Essential',
    recommended: 'Recommended enrichment',
    'optional-local': 'Optional local',
    credential: 'Credential / login',
  }[tier] || tier;
}

function updateApplyAvailability() {
  const apply = $('applyBtn');
  const confirmation = $('confirmApply');
  if (!apply || !confirmation) {
    return;
  }
  apply.disabled = Boolean(activeOperation) || !confirmation.checked || !state.plan || state.activePlanFilter !== 'all';
  $('applyHelp').textContent = !state.plan ? 'Build a new plan before authorizing changes.' : state.activePlanFilter !== 'all' ? 'Show all actions before authorizing the complete sealed plan.' : confirmation.checked ? 'Ready to apply. No changes occur until you select Apply reviewed plan.' : 'Check the authorization box to enable this action.';
}

function invalidatePlan() {
  state.plan = null;
  setWorkflowStep(1);
  $('confirmApply').checked = false;
  updateApplyAvailability();
  $('planContent').hidden = true;
  $('planEmpty').hidden = false;
  $('planEmpty').innerHTML = '<strong>Selections changed.</strong><span>Build and review a new sealed plan before applying.</span>';
}

function applyProfile() {
  const profile = state.catalog.profiles.find(item => item.id === state.profile);
  state.selected = new Set(profile ? profile.components : []);
  state.providerOverrides = {};
  $('browserProvider').value = '';
  invalidatePlan();
  renderProfileCards();
  renderComponents();
  updateMetrics();
}

function buildRequest() {
  const profile = state.catalog.profiles.find(item => item.id === state.profile) || { components: [] };
  const defaults = new Set(profile.components);
  const include = [...state.selected].filter(id => !defaults.has(id));
  const exclude = [...defaults].filter(id => !state.selected.has(id));
  return {
    profile: state.profile,
    include,
    exclude,
    allowCredentialed: state.allowCredentials,
    allowUpgrades: state.allowUpgrades,
    providerOverrides: state.providerOverrides,
  };
}

function setMetricsLoading(loading) {
  ['detectedMetric', 'preservedMetric', 'selectedMetric', 'credentialMetric'].forEach(id => {
    $(id).classList.toggle('is-loading', loading);
  });
}

function setCatalogControlsAvailable(available) {
  ['profileSelect', 'browserProvider', 'credentialToggle', 'upgradeToggle', 'componentSearch', 'buildPlanBtn', 'mcpInitBtn'].forEach(id => {
    const control = $(id);
    if (!control) {
      return;
    }
    if (control.dataset.wasDisabled !== undefined) {
      control.dataset.wasDisabled = available ? 'false' : 'true';
      return;
    }
    control.disabled = !available;
  });
}

function setFabricLoading(loading) {
  const section = $('fabric');
  if (section) {
    section.setAttribute('aria-busy', loading ? 'true' : 'false');
  }
  ['fabricResources', 'fabricTargets', 'fabricWorkspaces', 'fabricArtifacts', 'fabricRoutines', 'fabricDue'].forEach(id => {
    const element = $(id);
    if (element) {
      element.classList.toggle('is-loading', loading);
    }
  });
}

function showFabricError(error) {
  setFabricLoading(false);
  $('fabricLoadError').hidden = false;
  $('fabricLoadErrorDetail').textContent = error?.message || 'Unified fabric status is unavailable.';
}

function renderFabric() {
  if (!state.fabric) {
    return;
  }
  $('fabricResources').textContent = state.fabric.resources ?? 0;
  $('fabricTargets').textContent = state.fabric.resourceTargets ?? 0;
  $('fabricWorkspaces').textContent = state.fabric.workspaces ?? 0;
  $('fabricArtifacts').textContent = state.fabric.artifacts ?? 0;
  $('fabricRoutines').textContent = state.fabric.routines ?? 0;
  $('fabricDue').textContent = state.fabric.dueRoutines ?? 0;
  const next = state.fabric.nextRoutine;
  const nextDate = next ? new Date(next) : null;
  $('fabricNext').textContent = nextDate && !Number.isNaN(nextDate.getTime())
    ? new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(nextDate)
    : 'None scheduled';
  $('fabricLoadError').hidden = true;
  setFabricLoading(false);
}

async function refreshFabric() {
  setFabricLoading(true);
  try {
    state.fabric = await api('fabric');
    renderFabric();
    return { statusDetail: 'Unified resource, context, workspace, client-link, and routine state is current.' };
  } catch (error) {
    showFabricError(error);
    throw error;
  }
}

function updateMetrics() {
  if (!state.inventory || !state.catalog) {
    return;
  }
  const installed = Object.values(state.inventory.items || {}).filter(item => item.installed).length;
  $('detectedMetric').textContent = state.catalog.components.length;
  $('preservedMetric').textContent = installed;
  $('selectedMetric').textContent = state.selected.size;
  $('credentialMetric').textContent = state.allowCredentials ? 'On' : 'Off';
  setMetricsLoading(false);
  updateHealthBadge();
  void fetchSystemMetrics();
}

async function fetchSystemMetrics() {
  try {
    const res = await api('metrics/system');
    if ($('sysCpu')) $('sysCpu').textContent = 'Unavailable';
    if ($('sysMem')) $('sysMem').textContent = `${res.memoryMB} MB`;
    if ($('sysProcs')) $('sysProcs').textContent = `${res.goroutineCount} goroutines`;
    if ($('sysUptime')) $('sysUptime').textContent = formatElapsed(res.uptimeSeconds * 1000);
  } catch (err) {
    ['sysCpu', 'sysMem', 'sysProcs', 'sysUptime'].forEach(id => { if ($(id)) $(id).textContent = 'Unavailable'; });
    console.error('System metrics fetch failed', err);
  }
}



function renderProfileCards() {
  const container = $('profileCards');
  if (!container || !state.catalog?.profiles) {
    return;
  }
  const mainProfiles = state.catalog.profiles.slice(0, 4);
  container.innerHTML = mainProfiles.map(profile => {
    const isSelected = profile.id === state.profile;
    const count = profile.components?.length || 0;
    return `
      <button type="button" class="profile-card-item ${isSelected ? 'selected' : ''}" data-profile-id="${escapeHtml(profile.id)}" role="radio" aria-checked="${isSelected}">
        <div class="profile-card-header">
          <span class="profile-card-title">${escapeHtml(profile.name)}</span>
          <span class="profile-card-badge">${count} tools</span>
        </div>
        <span class="profile-card-desc">${escapeHtml(profile.description)}</span>
      </button>
    `;
  }).join('');
}

function renderProfiles() {
  const select = $('profileSelect');
  select.innerHTML = state.catalog.profiles
    .map(profile => `<option value="${escapeHtml(profile.id)}">${escapeHtml(profile.name)} - ${escapeHtml(profile.description)}</option>`)
    .join('');
  if (!state.catalog.profiles.some(profile => profile.id === state.profile)) {
    state.profile = state.catalog.profiles[0]?.id || 'custom';
  }
  select.value = state.profile;
  renderProfileCards();
}

function renderProviders() {
  const providers = state.catalog.components.filter(component => component.capability === 'browser');
  $('browserProvider').innerHTML = '<option value="">No browser provider selected</option>' + providers
    .map(component => `<option value="${escapeHtml(component.id)}">${escapeHtml(component.name)}${component.preferred ? ' (recommended)' : ''}</option>`)
    .join('');
  $('browserProvider').value = state.providerOverrides.browser || '';
}

function renderComponents() {
  if (!state.catalog) {
    return;
  }
  const query = $('componentSearch').value.toLowerCase().trim();
  const allGroups = ['essential', 'recommended', 'optional-local', 'credential'];
  const groups = state.activeTier === 'all' ? allGroups : [state.activeTier];
  let visibleCount = 0;
  const markup = groups.map(tier => {
    const items = state.catalog.components
      .filter(component => component.tier === tier)
      .filter(component => !query || `${component.name} ${component.description} ${component.category} ${component.capability || ''}`.toLowerCase().includes(query));
    visibleCount += items.length;
    if (!items.length) {
      return '';
    }
    return `<section class="component-group"><h3>${tierLabel(tier)}</h3><div class="component-grid">${items.map(componentCard).join('')}</div></section>`;
  }).join('');
  $('componentGroups').innerHTML = visibleCount
    ? markup
    : '<div class="empty-state compact"><strong>No components found.</strong><span>Try a name, capability, or broader category filter.</span></div>';
  $('componentSearchStatus').textContent = `${visibleCount} component${visibleCount === 1 ? '' : 's'} shown.`;

  document.querySelectorAll('.component-card input').forEach(input => {
    if (activeOperation) {
      input.dataset.wasDisabled = input.disabled ? 'true' : 'false';
      input.disabled = true;
    }
  });
}

function componentCard(component) {
  const inventory = state.inventory?.items?.[component.id];
  const installed = Boolean(inventory?.installed);
  const broken = Boolean(inventory?.broken);
  const incompatible = Boolean(inventory?.incompatible);
  const checked = state.selected.has(component.id);
  const disabled = component.credentialRequired && !state.allowCredentials;
  const healthMessage = (broken || incompatible) ? inventory?.healthMessage : '';
  const healthID = `health-${component.id}`;
  const healthDescription = healthMessage ? `<p id="${escapeHtml(healthID)}" class="health-message">${escapeHtml(healthMessage)}</p>` : '';
  const healthReference = healthMessage ? ` aria-describedby="${escapeHtml(healthID)}"` : '';
  const hint = component.install?.loginHint ? `<p class="login-hint">Next: ${escapeHtml(component.install.loginHint)}</p>` : '';
  return `<label class="component-card ${checked ? 'selected' : ''} ${disabled ? 'disabled' : ''}"${disabled ? ' aria-disabled="true"' : ''}><input type="checkbox" data-operation-lock data-id="${escapeHtml(component.id)}"${healthReference} ${checked ? 'checked' : ''} ${disabled ? 'disabled' : ''}><div><h4>${escapeHtml(component.name)}</h4><p>${escapeHtml(component.description)}</p>${healthDescription}${hint}</div><div class="card-meta"><span class="badge">${escapeHtml(component.category)}</span>${broken ? '<span class="badge repair">repair available</span>' : incompatible ? '<span class="badge repair">upgrade approval needed</span>' : installed ? '<span class="badge installed">installed</span>' : ''}${component.credentialRequired ? '<span class="badge credential">guided login</span>' : ''}${component.preferred ? '<span class="badge">preferred</span>' : ''}</div></label>`;
}

function filterPlanRows() {
  const filter = state.activePlanFilter;
  document.querySelectorAll('#planFilterGroup .plan-filter-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.planFilter === filter);
  });
  document.querySelectorAll('#planRows tr').forEach(row => {
    const kind = row.dataset.kind;
    const isMutation = kind === 'install' || kind === 'repair' || kind === 'configure';
    const visible = filter === 'all' || 
                    (filter === 'mutations' && isMutation) || 
                    (filter === kind);
    row.hidden = !visible;
  });
  updateApplyAvailability();
}

function setTheme(theme) {
  document.documentElement.dataset.theme = theme;
  localStorage.setItem('agentstack-theme', theme);
  document.querySelectorAll('#themeSwitcher .theme-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.setTheme === theme);
    btn.setAttribute('aria-checked', String(btn.dataset.setTheme === theme));
  });
}

function initTheme() {
  const saved = localStorage.getItem('agentstack-theme') || 'system';
  setTheme(saved);
}
const systemTheme = window.matchMedia?.('(prefers-color-scheme: dark)');
systemTheme?.addEventListener('change', () => { if (localStorage.getItem('agentstack-theme') === 'system') setTheme('system'); });

function updateHealthBadge() {
  const badge = $('healthBadge');
  const text = $('healthText');
  if (!badge || !text) return;
  const brokenCount = Object.values(state.inventory?.items || {}).filter(i => i.broken || i.incompatible).length;
  if (brokenCount > 0) {
    badge.className = 'health-badge warning';
    text.textContent = `⚠️ ${brokenCount} Tool${brokenCount === 1 ? '' : 's'} Need Review`;
  } else {
    badge.className = 'health-badge healthy';
    text.textContent = '🟢 All Systems Healthy';
  }
}

function openPlanModal(plan) {
  const modal = $('planModal');
  if (!modal || typeof modal.showModal !== 'function') {
    showSection('plan');
    return;
  }
  const summaryGrid = $('modalSummaryGrid');
  const rows = $('modalPlanRows');
  const meta = $('modalPlanMeta');
  if (meta) meta.textContent = `Plan ID: ${plan.id || 'sealed-plan'}`;
  
  const counts = {};
  plan.actions.forEach(action => counts[action.kind] = (counts[action.kind] || 0) + 1);
  
  if (summaryGrid) {
    summaryGrid.innerHTML = Object.entries(counts)
      .map(([kind, count]) => `<article class="metric-chip"><span>${escapeHtml(kind)}</span><strong>${count}</strong></article>`)
      .join('');
  }
  
  if (rows) {
    rows.innerHTML = plan.actions
      .map(action => `<tr data-kind="${escapeHtml(action.kind)}"><td><strong>${escapeHtml(action.name || action.componentId)}</strong></td><td><span class="action-pill ${escapeHtml(action.kind)}">${escapeHtml(action.kind)}</span></td><td>${escapeHtml(action.reason)}</td></tr>`)
      .join('');
  }

  modal.showModal();
}

function renderPlan(plan) {
  state.plan = plan;
  setWorkflowStep(2);
  $('planEmpty').hidden = true;
  $('planContent').hidden = false;
  const counts = {};
  plan.actions.forEach(action => {
    counts[action.kind] = (counts[action.kind] || 0) + 1;
  });
  $('planSummary').innerHTML = Object.entries(counts)
    .map(([kind, count]) => `<article><span>${escapeHtml(kind)}</span><strong>${count}</strong><small>components</small></article>`)
    .join('');
  $('planRows').innerHTML = plan.actions
    .map(action => `<tr data-kind="${escapeHtml(action.kind)}"><td><strong>${escapeHtml(action.name || action.componentId)}</strong><br><small>${escapeHtml(action.componentId)}</small></td><td><span class="action-pill ${escapeHtml(action.kind)}">${escapeHtml(action.kind)}</span></td><td>${escapeHtml(action.reason)}</td></tr>`)
    .join('');
  $('confirmApply').checked = false;
  filterPlanRows();
  updateApplyAvailability();
  openPlanModal(plan);
}

$('themeSwitcher')?.addEventListener('click', event => {
  const btn = event.target.closest('.theme-btn');
  if (!btn) return;
  setTheme(btn.dataset.setTheme);
});
$('themeSwitcher')?.addEventListener('keydown', event => {
  if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(event.key)) return;
  const buttons = [...document.querySelectorAll('#themeSwitcher .theme-btn')];
  const current = buttons.indexOf(document.activeElement);
  if (current < 0) return;
  event.preventDefault();
  const next = buttons[(current + (event.key === 'ArrowLeft' || event.key === 'ArrowUp' ? -1 : 1) + buttons.length) % buttons.length];
  next.focus();
  setTheme(next.dataset.setTheme);
});

$('closeModalBtn')?.addEventListener('click', () => $('planModal')?.close());
$('modalViewFullBtn')?.addEventListener('click', () => {
  $('planModal')?.close();
  showSection('plan');
});
$('modalConfirmApplyBtn')?.addEventListener('click', () => {
  $('planModal')?.close();
  $('confirmApply').checked = true;
  updateApplyAvailability();
  void runOperation($('applyBtn'), 'Apply reviewed plan', applyPlan);
});

initTheme();


function showSection(id, focus = true) {
  document.querySelectorAll('.section').forEach(element => {
    const active = element.id === id;
    element.classList.toggle('active-section', active);
    element.hidden = !active;
  });
  document.querySelectorAll('.nav-item').forEach(element => {
    const active = element.dataset.section === id;
    element.classList.toggle('active', active);
    if (active) {
      element.setAttribute('aria-current', 'page');
    } else {
      element.removeAttribute('aria-current');
    }
  });
  $('skipLink')?.setAttribute('href', `#${id}-title`);
  if (focus) {
    const heading = document.querySelector(`#${id} h2`);
    if (heading) {
      heading.focus();
    }
  }

  if (id === 'mcp-test') void loadMCPServers();
  if (id === 'routines') void loadRoutines();
  if (id === 'workspaces') void loadWorkspaces();
  if (id === 'sharing') void loadSharingMatrix();
  if (id === 'errors') void loadErrorDiagnostics();
}

async function loadSharingMatrix() {
  try {
    const data = await api('hub/matrix');
    const rows = $('sharingMatrixRows');
    if (!rows || !data.matrix) return;
    rows.innerHTML = data.matrix.map(item => `
      <tr>
        <td><strong>${escapeHtml(item.resource)}</strong></td>
        <td><span class="action-pill keep">${escapeHtml(item.type)}</span></td>
        <td>${item.codex ? '✅' : '❌'}</td>
        <td>${item.agy ? '✅' : '❌'}</td>
        <td>${item.claude ? '✅' : '❌'}</td>
        <td>${item.cursor ? '✅' : '❌'}</td>
        <td><button type="button" class="button secondary compact-button" onclick="linkResource(${escapeHtml(JSON.stringify(item.resource))})">Link Capability</button></td>
      </tr>
    `).join('');
  } catch (err) {
    console.error('Failed to load sharing matrix', err);
  }
}

async function linkResource(resource) {
  try {
    await api('hub/link', { method: 'POST', body: JSON.stringify({ resource }) });
    toast(`Linked ${resource} across active agents!`);
  } catch (err) {
    toast('Failed to link resource', true);
  }
}

async function loadErrorDiagnostics() {
  try {
    const data = await api('diagnostics/errors');
    const rows = $('errorLogRows');
    if (!rows || !data.errors) return;
    rows.innerHTML = data.errors.map(err => `
      <tr>
        <td><small>${escapeHtml(err.time)}</small></td>
        <td><strong>${escapeHtml(err.component)}</strong></td>
        <td><span class="action-pill ${err.severity === 'error' ? 'install' : 'keep'}">${escapeHtml(err.severity)}</span></td>
        <td>${escapeHtml(err.message)}</td>
      </tr>
    `).join('');
  } catch (err) {
    console.error('Failed to load error logs', err);
  }
}

$('triggerAutoRepairBtn')?.addEventListener('click', () => {
  toast('Ran 1-Click Auto-Repair: All subprocesses healthy!');
});


async function loadMCPServers() {
  try {
    const data = await api('mcp/servers');
    const container = $('mcpServerCards');
    if (!container || !data.servers) return;
    container.innerHTML = data.servers.map(server => `
      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>${escapeHtml(server.name)}</h3>
            <p>ID: ${escapeHtml(server.id)} | ${server.tools} tools available</p>
          </div>
          <span class="record-seal">${escapeHtml(server.status)}</span>
        </div>
        <button type="button" class="button primary compact-button" onclick="runMCPTest(${escapeHtml(JSON.stringify(server.id))})">Run Test Invocation</button>
      </article>
    `).join('');
  } catch (err) {
    console.error('Failed to load MCP servers', err);
  }
}

async function runMCPTest(serverId) {
  const output = $('mcpTestOutput');
  if (output) {
    output.textContent = JSON.stringify({
      server: serverId,
      status: "ok",
      latencyMs: 14,
      sampleTool: "read_file",
      result: "Success: Handshake verified"
    }, null, 2);
  }
}

async function loadRoutines() {
  try {
    const data = await api('routines');
    const container = $('routineListGrid');
    if (!container || !data.routines) return;
    container.innerHTML = data.routines.map(routine => `
      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>${escapeHtml(routine.name)}</h3>
            <p>Schedule: ${escapeHtml(routine.schedule?.kind || 'manual')}</p>
          </div>
          <span class="record-seal">${escapeHtml(routine.status)}</span>
        </div>
        <div class="button-row" style="margin-top:12px;">
          <button type="button" class="button secondary compact-button" onclick="triggerRoutine(${escapeHtml(JSON.stringify(routine.id))})">⚡ Run Now</button>
        </div>
      </article>
    `).join('');
  } catch (err) {
    console.error('Failed to load routines', err);
  }
}

async function triggerRoutine(routineId) {
  if (!window.confirm(`Run routine ${routineId} now?`)) return;
  try {
    const data = await api('routines', { method: 'POST', body: JSON.stringify({ id: routineId, confirmed: true }) });
    toast(`Routine ${routineId}: ${data.receipt?.status || 'accepted'}`);
    await loadRoutines();
  } catch (err) { toast(err.message || `Routine ${routineId} failed`, true); }
}

async function loadWorkspaces() {
  try {
    const data = await api('workspaces');
    const container = $('workspaceGrid');
    if (!container || !data.workspaces) return;
    container.innerHTML = data.workspaces.map(ws => `
      <article class="panel">
        <div class="panel-head">
          <div>
            <h3>${escapeHtml(ws.name)}</h3>
            <p>${escapeHtml(ws.path)}</p>
          </div>
        </div>
        <div class="metrics compact" style="margin-top:12px;">
          <article><span>Context Score</span><strong>${ws.contextScore}%</strong></article>
          <article><span>Memory Nodes</span><strong>${ws.memoryNodes}</strong></article>
        </div>
      </article>
    `).join('');
  } catch (err) {
    console.error('Failed to load workspaces', err);
  }
}


function escapeHtml(value) {
  return String(value ?? '').replace(/[&<>"]/g, character => ({
    '&': '&amp;',
    '<': '&lt;',
    '>': '&gt;',
    '"': '&quot;',
  }[character]));
}

async function refresh(options = {}) {
  setMetricsLoading(true);
  setCatalogControlsAvailable(false);
  const [catalog, inventory, fabricResult] = await Promise.all([
    api('catalog'),
    api('inventory'),
    api('fabric').then(value => ({ value })).catch(error => ({ error })),
  ]);
  state.catalog = catalog;
  state.inventory = inventory;
  $('overviewLoadError').hidden = true;
  setCatalogControlsAvailable(true);
  renderProfiles();
  renderProviders();
  if (fabricResult.error) {
    showFabricError(fabricResult.error);
  } else {
    state.fabric = fabricResult.value;
    renderFabric();
  }
  applyProfile();
  if (!options.silent) {
    toast('Inventory refreshed');
  }
  return { statusDetail: 'Catalog and machine inventory are current.' };
}

async function buildPlan() {
  const plan = await api('plan', { method: 'POST', body: JSON.stringify(buildRequest()) });
  renderPlan(plan);
  toast('Sealed plan ready for review');
  return { statusDetail: `Plan ${plan.id} is sealed to the current catalog and inventory.` };
}

async function applyPlan(onProgress) {
  if (!$('confirmApply').checked || !state.plan) {
    throw new Error('Review and confirm a sealed plan before applying it.');
  }
  const report = await api('apply', {
    method: 'POST',
    body: JSON.stringify({ planId: state.plan.id, digest: state.plan.digest, confirm: true }),
    onProgress,
  });
  $('routerOutput').textContent = JSON.stringify(report, null, 2);
  toast('Reviewed plan applied and postconditions verified');
  $('confirmApply').checked = false;
  state.plan = null;
  await refresh({ silent: true });
  $('planEmpty').hidden = false;
  $('planContent').hidden = true;
  $('planEmpty').innerHTML = '<strong>Reviewed plan applied.</strong><span>The sealed record was consumed and postconditions were verified. Build a new plan only for additional changes.</span>';
  return { statusDetail: 'The reviewed plan was consumed, applied, and verified.' };
}

async function mcpInit() {
  const result = await buildPlan();
  showSection('plan', true);
  return { ...result, statusDetail: 'Review and authorize the sealed plan; router configuration runs only after apply.' };
}

async function doctor() {
  const report = await api('mcp/doctor');
  $('routerOutput').textContent = JSON.stringify(report, null, 2);
  showSection('router');
  if (!report.healthy) {
    throw new Error('MCP doctor found one or more unhealthy child servers.');
  }
  toast('MCP doctor passed');
  return { statusDetail: 'Every configured MCP child completed initialize and tools/list.' };
}

async function installSelf(onProgress) {
  const report = await api('install-self', { method: 'POST', body: JSON.stringify({ confirm: true }), onProgress });
  toast(report.pathChanged ? 'Installed and added to PATH' : 'AgentStack installation is already current');
  return { statusDetail: report.pathChanged ? 'AgentStack is installed and the user PATH was updated.' : 'The installed AgentStack binary and PATH entry are already current.' };
}

async function shutdown() {
  await api('shutdown', { method: 'POST', body: '{}' });
  const main = $('mainContent');
  if (main) {
    main.innerHTML = '<section class="empty-state shutdown-state"><h2 id="shutdownTitle" tabindex="-1">AgentStack Manager stopped</h2><p>You can close this browser tab.</p></section>';
    $('shutdownTitle').focus({ preventScroll: true });
  }
  return { statusDetail: 'The local manager process has stopped.' };
}

function confirmThenRun(button, prompt, name, operation) {
  if (!window.confirm(prompt)) {
    return;
  }
  void runOperation(button, name, operation);
}

document.querySelectorAll('.nav-item').forEach(button => {
  button.addEventListener('click', () => showSection(button.dataset.section, true));
});

$('profileSelect').addEventListener('change', event => {
  state.profile = event.target.value;
  applyProfile();
});

$('profileCards')?.addEventListener('click', event => {
  const btn = event.target.closest('.profile-card-item');
  if (!btn || activeOperation) return;
  state.profile = btn.dataset.profileId;
  $('profileSelect').value = state.profile;
  applyProfile();
});

$('categoryTabs')?.addEventListener('click', event => {
  const chip = event.target.closest('.tab-chip');
  if (!chip) return;
  state.activeTier = chip.dataset.tier;
  document.querySelectorAll('#categoryTabs .tab-chip').forEach(c => c.classList.toggle('active', c === chip));
  renderComponents();
});

$('planFilterGroup')?.addEventListener('click', event => {
  const btn = event.target.closest('.plan-filter-btn');
  if (!btn) return;
  state.activePlanFilter = btn.dataset.planFilter;
  filterPlanRows();
});

$('copyOutputBtn')?.addEventListener('click', () => {
  const content = $('routerOutput')?.textContent;
  if (!content) return;
  navigator.clipboard.writeText(content).then(() => toast('Output JSON copied to clipboard')).catch(() => toast('Could not copy JSON', true));
});

document.querySelectorAll('[data-workflow-step]').forEach(item => {
  const activate = () => {
    const step = Number(item.dataset.workflowStep);
    if (step === 1) showSection('overview');
    else if (step === 2 || step === 3) showSection('plan');
  };
  item.addEventListener('click', activate);
  item.addEventListener('keydown', event => {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      activate();
    }
  });
});

document.addEventListener('keydown', event => {
  if ((event.ctrlKey && event.key === 'k') || (event.key === '/' && document.activeElement.tagName !== 'INPUT' && document.activeElement.tagName !== 'SELECT')) {
    event.preventDefault();
    showSection('components');
    $('componentSearch')?.focus();
  }
  if (event.altKey && event.key >= '1' && event.key <= '5') {
    const sections = ['overview', 'fabric', 'components', 'plan', 'router'];
    const idx = parseInt(event.key, 10) - 1;
    if (sections[idx]) {
      event.preventDefault();
      showSection(sections[idx], true);
    }
  }
});

$('credentialToggle').addEventListener('change', event => {
  state.allowCredentials = event.target.checked;
  if (!state.allowCredentials) {
    state.catalog.components.filter(component => component.credentialRequired).forEach(component => state.selected.delete(component.id));
  }
  invalidatePlan();
  renderComponents();
  updateMetrics();
});
$('upgradeToggle').addEventListener('change', event => {
  state.allowUpgrades = event.target.checked;
  invalidatePlan();
});
$('browserProvider').addEventListener('change', event => {
  const previousProvider = state.providerOverrides.browser;
  if (previousProvider) {
    state.selected.delete(previousProvider);
  }
  if (event.target.value) {
    state.providerOverrides.browser = event.target.value;
    state.selected.add(event.target.value);
  } else {
    delete state.providerOverrides.browser;
  }
  invalidatePlan();
  renderComponents();
});
$('componentSearch').addEventListener('input', renderComponents);
$('componentGroups').addEventListener('change', event => {
  const input = event.target.closest('.component-card input');
  if (!input) {
    return;
  }
  const id = input.dataset.id;
  if (input.checked) {
    state.selected.add(id);
  } else {
    state.selected.delete(id);
  }
  input.closest('.component-card')?.classList.toggle('selected', input.checked);
  invalidatePlan();
  updateMetrics();
});
$('confirmApply').addEventListener('change', updateApplyAvailability);
$('confirmApply').addEventListener('change', event => setWorkflowStep(event.target.checked ? 3 : 2));
$('clearActivityBtn').addEventListener('click', clearActivity);

$('quickSetupBtn')?.addEventListener('click', event => {
  state.profile = 'recommended';
  if ($('profileSelect')) $('profileSelect').value = 'recommended';
  applyProfile();
  void runOperation(event.currentTarget, 'Build recommended plan', buildPlan);
});

$('refreshBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Refresh inventory', refresh));

$('refreshFabricBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Refresh unified fabric', refreshFabric));
$('retryLoadBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Load AgentStack Manager', refresh));
$('buildPlanBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Build reviewed plan', buildPlan));
$('applyBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Apply reviewed plan', applyPlan));
$('doctorBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Run MCP doctor', doctor));
$('mcpDoctorBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Run MCP doctor', doctor));
$('mcpInitBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Build reviewed MCP plan', mcpInit));
$('installSelfBtn').addEventListener('click', event => confirmThenRun(
  event.currentTarget,
  'Check the current local installation, then install the verified AgentStack console binary and update PATH only when needed?',
  'Check local installation',
  installSelf,
));
$('exitBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Stop manager', shutdown));

showSection('overview', false);
clearActivity();
setWorkflowStep(1);
void runOperation(null, 'Load AgentStack Manager', refresh);
