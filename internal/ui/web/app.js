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
};

const $ = id => document.getElementById(id);
let activeOperation = null;
const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));

const waitForOperation = async statusURL => {
  for (;;) {
    await delay(750);
    const operation = await api(statusURL);
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
    return waitForOperation(data.statusUrl);
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
  const previousFocus = document.activeElement;
  setOperationControlsBusy(button, true);
  setOperationStatus('running', name, 'AgentStack is working. Controls are temporarily locked to prevent conflicting changes.');

  try {
    const result = await operation();
    const detail = result?.statusDetail || 'The operation completed successfully.';
    setOperationStatus('success', `${name} complete`, detail);
    return result;
  } catch (error) {
    setOperationStatus('error', `${name} failed`, error.message);
    const output = $('routerOutput');
    if (output && error.data) {
      output.textContent = JSON.stringify(error.data, null, 2);
    }
    toast(error.message, true);
    return null;
  } finally {
    setOperationControlsBusy(button, false);
    activeOperation = null;
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
  apply.disabled = Boolean(activeOperation) || !confirmation.checked || !state.plan;
}

function invalidatePlan() {
  state.plan = null;
  $('confirmApply').checked = false;
  updateApplyAvailability();
  $('planContent').hidden = true;
  $('planEmpty').hidden = false;
  $('planEmpty').textContent = 'Selections changed. Build and review a new plan before applying.';
}

function applyProfile() {
  const profile = state.catalog.profiles.find(item => item.id === state.profile);
  state.selected = new Set(profile ? profile.components : []);
  state.providerOverrides = {};
  $('browserProvider').value = '';
  invalidatePlan();
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

function updateMetrics() {
  if (!state.inventory || !state.catalog) {
    return;
  }
  const installed = Object.values(state.inventory.items || {}).filter(item => item.installed).length;
  $('detectedMetric').textContent = installed;
  $('preservedMetric').textContent = installed;
  $('selectedMetric').textContent = state.selected.size;
  $('credentialMetric').textContent = state.allowCredentials ? 'On' : 'Off';
}

function renderProfiles() {
  const select = $('profileSelect');
  select.innerHTML = state.catalog.profiles
    .map(profile => `<option value="${escapeHtml(profile.id)}">${escapeHtml(profile.name)} — ${escapeHtml(profile.description)}</option>`)
    .join('');
  if (!state.catalog.profiles.some(profile => profile.id === state.profile)) {
    state.profile = state.catalog.profiles[0]?.id || 'custom';
  }
  select.value = state.profile;
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
  const groups = ['essential', 'recommended', 'optional-local', 'credential'];
  $('componentGroups').innerHTML = groups.map(tier => {
    const items = state.catalog.components
      .filter(component => component.tier === tier)
      .filter(component => !query || `${component.name} ${component.description} ${component.category} ${component.capability || ''}`.toLowerCase().includes(query));
    if (!items.length) {
      return '';
    }
    return `<section class="component-group"><h3>${tierLabel(tier)}</h3><div class="component-grid">${items.map(componentCard).join('')}</div></section>`;
  }).join('');

  document.querySelectorAll('.component-card input').forEach(input => {
    if (activeOperation) {
      input.dataset.wasDisabled = input.disabled ? 'true' : 'false';
      input.disabled = true;
    }
    input.addEventListener('change', event => {
      const id = event.target.dataset.id;
      if (event.target.checked) {
        state.selected.add(id);
      } else {
        state.selected.delete(id);
      }
      invalidatePlan();
      renderComponents();
      updateMetrics();
    });
  });
}

function componentCard(component) {
  const inventory = state.inventory?.items?.[component.id];
  const installed = Boolean(inventory?.installed);
  const broken = Boolean(inventory?.broken);
  const incompatible = Boolean(inventory?.incompatible);
  const checked = state.selected.has(component.id);
  const disabled = component.credentialRequired && !state.allowCredentials;
  const health = (broken || incompatible) && inventory?.healthMessage ? ` title="${escapeHtml(inventory.healthMessage)}"` : '';
  const hint = component.install?.loginHint ? `<p class="login-hint">Next: ${escapeHtml(component.install.loginHint)}</p>` : '';
  return `<label class="component-card ${checked ? 'selected' : ''} ${disabled ? 'disabled' : ''}"${health}><input type="checkbox" data-operation-lock data-id="${escapeHtml(component.id)}" ${checked ? 'checked' : ''} ${disabled ? 'disabled' : ''}><div><h4>${escapeHtml(component.name)}</h4><p>${escapeHtml(component.description)}</p>${hint}<div class="card-meta"><span class="badge">${escapeHtml(component.category)}</span>${broken ? '<span class="badge repair">repair available</span>' : incompatible ? '<span class="badge repair">upgrade approval needed</span>' : installed ? '<span class="badge installed">installed</span>' : ''}${component.credentialRequired ? '<span class="badge credential">guided login</span>' : ''}${component.preferred ? '<span class="badge">preferred</span>' : ''}</div></div></label>`;
}

function renderPlan(plan) {
  state.plan = plan;
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
    .map(action => `<tr><td><strong>${escapeHtml(action.name || action.componentId)}</strong><br><small>${escapeHtml(action.componentId)}</small></td><td><span class="action-pill ${escapeHtml(action.kind)}">${escapeHtml(action.kind)}</span></td><td>${escapeHtml(action.reason)}</td></tr>`)
    .join('');
  $('confirmApply').checked = false;
  updateApplyAvailability();
  showSection('plan');
}

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
  if (focus) {
    const heading = document.querySelector(`#${id} h2`);
    if (heading) {
      heading.focus();
    }
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
  [state.catalog, state.inventory] = await Promise.all([api('catalog'), api('inventory')]);
  renderProfiles();
  renderProviders();
  applyProfile();
  renderComponents();
  if (!options.silent) {
    toast('Inventory refreshed');
  }
  return { statusDetail: 'Catalog and machine inventory are current.' };
}

async function buildPlan() {
  const plan = await api('plan', { method: 'POST', body: JSON.stringify(buildRequest()) });
  renderPlan(plan);
  toast('Reviewed plan generated');
  return { statusDetail: `Plan ${plan.id} is sealed to the current catalog and inventory.` };
}

async function applyPlan() {
  if (!$('confirmApply').checked || !state.plan) {
    throw new Error('Review and confirm a sealed plan before applying it.');
  }
  const report = await api('apply', {
    method: 'POST',
    body: JSON.stringify({ planId: state.plan.id, digest: state.plan.digest, confirm: true }),
  });
  $('routerOutput').textContent = JSON.stringify(report, null, 2);
  toast('Reviewed plan applied and postconditions verified');
  $('confirmApply').checked = false;
  state.plan = null;
  await refresh({ silent: true });
  return { statusDetail: 'The reviewed plan was consumed, applied, and verified.' };
}

async function mcpInit() {
  const report = await api('mcp/init', {
    method: 'POST',
    body: JSON.stringify({ request: buildRequest(), registerClients: true, warm: true, confirm: true }),
  });
  $('routerOutput').textContent = JSON.stringify(report, null, 2);
  showSection('router');
  toast('MCP stack initialized');
  return { statusDetail: 'Router configuration is current and selected child servers were checked.' };
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

async function installSelf() {
  const report = await api('install-self', { method: 'POST', body: JSON.stringify({ confirm: true }) });
  toast(report.pathChanged ? 'Installed and added to PATH' : 'AgentStack installation is already current');
  return { statusDetail: report.pathChanged ? 'AgentStack is installed and the user PATH was updated.' : 'The installed AgentStack binary and PATH entry are already current.' };
}

async function shutdown() {
  await api('shutdown', { method: 'POST', body: '{}' });
  const main = $('mainContent');
  if (main) {
    main.innerHTML = '<section class="empty-state shutdown-state"><h2>AgentStack Manager stopped</h2><p>You can close this browser tab.</p></section>';
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
$('confirmApply').addEventListener('change', updateApplyAvailability);

$('refreshBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Refresh inventory', refresh));
$('buildPlanBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Build reviewed plan', buildPlan));
$('applyBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Apply reviewed plan', applyPlan));
$('doctorBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Run MCP doctor', doctor));
$('mcpDoctorBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Run MCP doctor', doctor));
$('mcpInitBtn').addEventListener('click', event => confirmThenRun(
  event.currentTarget,
  'Initialize the selected MCP profile, write AgentStack-managed router configuration, and repair owned client registrations?',
  'Initialize MCP stack',
  mcpInit,
));
$('installSelfBtn').addEventListener('click', event => confirmThenRun(
  event.currentTarget,
  'Install the verified AgentStack console binary into the user application directory and update the user PATH?',
  'Install AgentStack',
  installSelf,
));
$('exitBtn').addEventListener('click', event => void runOperation(event.currentTarget, 'Stop manager', shutdown));

showSection('overview', false);
void runOperation(null, 'Load AgentStack Manager', refresh);
