'use strict';

(() => {
  const AS = window.AgentStack;
  const { $, state, api, escapeHTML, humanize, showSection, showNotice, runOperation } = AS;
  let activeTier = 'all';
  let searchTimer = null;

  function inventoryItem(id) { return state.inventory?.items?.[id] || {}; }
  function componentByID(id) { return state.catalog?.components?.find(component => component.id === id); }
  function activeProfile() { return state.catalog?.profiles?.find(profile => profile.id === state.profile) || state.catalog?.profiles?.[0]; }
  function selectedCount() { return state.selected.size; }

  function dependencyClosure(componentID) {
    const result = [];
    const visited = new Set();
    const queue = [componentID];
    while (queue.length) {
      const id = queue.shift();
      if (!id || visited.has(id)) continue;
      visited.add(id);
      const component = componentByID(id);
      if (!component) continue;
      for (const dependency of component.dependsOn || []) {
        if (!visited.has(dependency)) {
          result.push(dependency);
          queue.push(dependency);
        }
      }
    }
    return [...new Set(result)];
  }

  function includeProviderDependencies(providerID) {
    if (!providerID) return [];
    const added = [];
    for (const id of [providerID, ...dependencyClosure(providerID)]) {
      if (!state.selected.has(id)) {
        state.selected.add(id);
        added.push(componentByID(id)?.name || id);
      }
    }
    return added;
  }

  function includeComponentDependencies(componentID) {
    const added = [];
    for (const id of dependencyClosure(componentID)) {
      if (!state.selected.has(id)) {
        state.selected.add(id);
        added.push(componentByID(id)?.name || id);
      }
    }
    return added;
  }

  function validateSelection() {
    const issues = [];
    const providerID = $('browserProvider').value;
    if (providerID && !state.selected.has(providerID)) {
      issues.push(`${componentByID(providerID)?.name || providerID} must be included as the selected browser provider.`);
    }
    for (const id of state.selected) {
      const component = componentByID(id);
      for (const dependency of component?.dependsOn || []) {
        if (!state.selected.has(dependency)) {
          const dependencyName = componentByID(dependency)?.name || dependency;
          issues.push(`${component.name || id} requires ${dependencyName}.`);
        }
      }
    }
    return [...new Set(issues)];
  }

  function renderSelectionIssues() {
    const issues = validateSelection();
    const container = $('selectionIssues');
    container.hidden = issues.length === 0;
    container.innerHTML = issues.length
      ? `<strong>Resolve before creating changes</strong><ul>${issues.map(issue => `<li>${escapeHTML(issue)}</li>`).join('')}</ul>`
      : '';
    return issues;
  }

  function updateCreateAvailability() {
    const issues = renderSelectionIssues();
    $('createPlanBtn').disabled = !state.selected.size || issues.length > 0 || Boolean(state.activeOperation);
  }

  function invalidatePlan(message = 'Your selection changed. Create changes again to review the latest result.') {
    state.plan = null;
    state.planConsumed = false;
    $('confirmApply').checked = false;
    $('confirmApply').disabled = true;
    $('applyBtn').disabled = true;
    $('planState').textContent = 'Not prepared';
    $('planState').className = 'state-badge neutral';
    $('changeList').innerHTML = '<li class="empty-state">No pending changes.</li>';
    $('createPlanBtn').hidden = false;
    $('createPlanBtn').textContent = 'Create changes';
    updateInlinePreview(message);
    updateCreateAvailability();
  }

  function selectProfile(profileID) {
    const profile = state.catalog?.profiles?.find(item => item.id === profileID);
    if (!profile) return;
    state.profile = profile.id;
    state.selected = new Set(profile.components || []);
    includeProviderDependencies($('browserProvider').value);
    renderProfiles();
    renderComponents();
    invalidatePlan('Profile selected. Create changes to compare it with the current system.');
  }

  function renderProviderOptions() {
    const providers = (state.catalog?.components || []).filter(component => component.capability === 'browser');
    const select = $('browserProvider');
    const current = select.value;
    select.innerHTML = '<option value="">Automatic</option>' + providers.map(component => `<option value="${escapeHTML(component.id)}">${escapeHTML(component.name)}</option>`).join('');
    if ([...select.options].some(option => option.value === current)) select.value = current;
  }

  function renderProfiles() {
    const profiles = state.catalog?.profiles || [];
    $('profileList').innerHTML = profiles.map(profile => `
      <label class="profile-option">
        <input type="radio" name="profile" value="${escapeHTML(profile.id)}" ${profile.id === state.profile ? 'checked' : ''}>
        <strong>${escapeHTML(profile.name)}</strong>
        <small>${escapeHTML(profile.description)}</small>
      </label>`).join('');
    $('profileList').querySelectorAll('input').forEach(input => input.addEventListener('change', () => selectProfile(input.value)));
  }

  function componentState(component) {
    const item = inventoryItem(component.id);
    if (item.broken || item.incompatible) return ['Needs attention', 'needs-attention'];
    if (item.installed) return [item.version ? `Installed ${item.version}` : 'Installed', 'installed'];
    return ['Available', 'available'];
  }

  function renderComponents() {
    if (!state.catalog) return;
    const query = $('componentSearch').value.trim().toLowerCase();
    const components = state.catalog.components.filter(component => {
      const tierMatch = activeTier === 'all' || component.tier === activeTier;
      const text = `${component.name} ${component.description} ${component.category}`.toLowerCase();
      return tierMatch && (!query || text.includes(query));
    });
    $('componentList').innerHTML = components.length ? components.map(component => {
      const [label, status] = componentState(component);
      return `<label class="component-row" data-component-id="${escapeHTML(component.id)}">
        <input type="checkbox" value="${escapeHTML(component.id)}" ${state.selected.has(component.id) ? 'checked' : ''}>
        <span class="component-copy"><strong>${escapeHTML(component.name)}</strong><small>${escapeHTML(component.description)}</small></span>
        <span class="component-meta"><span class="state-badge ${status}">${escapeHTML(label)}</span><small>${escapeHTML(humanize(component.tier))}</small></span>
      </label>`;
    }).join('') : '<p class="empty-state">No tools match this filter.</p>';
    $('componentList').querySelectorAll('input').forEach(input => input.addEventListener('change', () => {
      let added = [];
      if (input.checked) {
        state.selected.add(input.value);
        added = includeComponentDependencies(input.value);
      } else {
        state.selected.delete(input.value);
      }
      renderComponents();
      const message = added.length
        ? `Added required ${added.length === 1 ? 'dependency' : 'dependencies'}: ${added.join(', ')}.`
        : 'Your selection changed. Create changes again to review the latest result.';
      invalidatePlan(message);
    }));
    $('selectionSummary').textContent = `${selectedCount()} tool${selectedCount() === 1 ? '' : 's'} selected.`;
    updateCreateAvailability();
  }

  function updateInlinePreview(message = '') {
    if (!state.catalog || !state.inventory) return;
    const selected = [...state.selected];
    const additions = selected.filter(id => !inventoryItem(id).installed).length;
    const repairs = selected.filter(id => inventoryItem(id).broken || inventoryItem(id).incompatible).length;
    const preserved = Object.values(state.inventory.items || {}).filter(item => item.installed).length;
    $('pendingSummary').textContent = message || (selected.length
      ? `${selected.length} selected. About ${additions} additions, ${repairs} repairs, and ${preserved} installed tools preserved.`
      : 'Choose at least one tool before creating changes.');
  }

  function planRequest() {
    const profile = activeProfile() || { components: [] };
    const defaults = new Set(profile.components || []);
    return {
      profile: state.profile,
      include: [...state.selected].filter(id => !defaults.has(id)),
      exclude: [...defaults].filter(id => !state.selected.has(id)),
      allowCredentialed: state.allowCredentialed,
      allowUpgrades: state.allowUpgrades,
      providerOverrides: $('browserProvider').value ? { browser: $('browserProvider').value } : {},
    };
  }

  function renderPlan(plan) {
    state.plan = plan;
    state.planConsumed = false;
    $('createPlanBtn').hidden = true;
    const meaningful = (plan.actions || []).filter(action => !['keep', 'skip', 'skip-dominated', 'preserve-inactive'].includes(action.kind));
    $('changeList').innerHTML = meaningful.length ? meaningful.map(action => `
      <li class="change-row" data-component-id="${escapeHTML(action.componentId)}">
        <div><strong>${escapeHTML(action.name || action.componentId)}</strong><p>${escapeHTML(action.reason || 'Selected change')}</p></div>
        <span class="state-badge ${action.kind === 'install' ? 'available' : action.kind === 'repair' ? 'needs-attention' : 'neutral'} change-kind">${escapeHTML(humanize(action.kind))}</span>
      </li>`).join('') : '<li class="empty-state">Everything selected is already ready. No changes are needed.</li>';
    $('pendingSummary').textContent = meaningful.length ? `${meaningful.length} pending change${meaningful.length === 1 ? '' : 's'}. Review every item before approval.` : 'No changes are needed.';
    $('planState').textContent = meaningful.length ? 'Ready to review' : 'Up to date';
    $('planState').className = `state-badge ${meaningful.length ? 'attention' : 'success'}`;
    $('confirmApply').disabled = meaningful.length === 0;
    $('confirmApply').checked = false;
    updateApplyAvailability();
  }

  async function createPlan(button = $('createPlanBtn')) {
    if (!state.catalog || !state.inventory) return null;
    const issues = validateSelection();
    if (!state.selected.size || issues.length) {
      showNotice(issues.length ? 'Resolve the selection first' : 'Choose at least one tool', issues[0] || 'Select the tools you need before creating changes.');
      renderSelectionIssues();
      return null;
    }
    return runOperation(button, 'Create changes', () => api('plan', { method: 'POST', body: JSON.stringify(planRequest()) }), {
      onSuccess: plan => {
        renderPlan(plan);
        showNotice('Changes are ready to review', 'Nothing has been installed yet. Review the list and approve once when ready.', 'success');
      },
    });
  }

  function updateApplyAvailability() {
    $('applyBtn').disabled = !state.plan || state.planConsumed || !$('confirmApply').checked || Boolean(state.activeOperation);
    updateCreateAvailability();
  }

  function failedIDsFromOperation(operation) {
    const outcome = operation?.result?.outcome || {};
    const diagnostics = outcome.diagnostics || [];
    if (diagnostics.length) {
      const recoverable = outcome.outcome === 'cancelled' ? new Set(['failed', 'skipped', 'not-needed']) : new Set(['failed']);
      return diagnostics.filter(item => recoverable.has(item.result)).map(item => item.componentId).filter(Boolean);
    }
    const actions = operation?.result?.report?.transaction?.actions || [];
    return actions.filter(action => action.error || (action.exitCode && action.exitCode !== 0) || (!action.verified && ['install', 'repair', 'configure'].includes(action.kind))).map(action => action.componentId).filter(Boolean);
  }

  async function handleApplyFailure(error) {
    const operation = error.data || {};
    const failure = operation.failure || {};
    const outcome = operation.result?.outcome || {};
    state.failedComponentIDs = failedIDsFromOperation(operation);
    state.plan = null;
    state.planConsumed = true;
    $('createPlanBtn').hidden = false;
    $('createPlanBtn').textContent = 'Create fresh plan';
    $('createPlanBtn').disabled = false;
    $('confirmApply').checked = false;
    $('confirmApply').disabled = true;
    updateApplyAvailability();
    const title = outcome.summary || failure.message || 'No requested changes were applied.';
    const recovery = failure.recovery || outcome.detail || 'Existing verified items were left unchanged. Retry failed items or review a fresh plan.';
    showNotice(title, recovery, 'error', { label: 'Review results', run: () => showSection('activity') });
    AS.activity.renderApplyFailure(operation);
    await Promise.allSettled([AS.refreshInventory(), AS.environments.load(), AS.activity.loadTransactions()]);
    AS.renderOperationalStatus?.();
    showSection('activity');
  }

  async function applyPlan() {
    if (!state.plan || state.planConsumed || !$('confirmApply').checked) return;
    const plan = state.plan;
    state.planConsumed = true;
    updateApplyAvailability();
    AS.activity.beginProgress(plan);
    await runOperation($('applyBtn'), 'Apply changes', () => api('apply', {
      method: 'POST',
      body: JSON.stringify({ planId: plan.id, digest: plan.digest, confirm: true }),
      onProgress: progress => AS.activity.renderProgress(progress),
    }), {
      handledFailure: true,
      onSuccess: async result => {
        state.plan = null;
        state.planConsumed = false;
        $('createPlanBtn').hidden = false;
        $('createPlanBtn').textContent = 'Create changes';
        $('confirmApply').checked = false;
        $('confirmApply').disabled = true;
        const outcome = AS.activity.completeProgress(result);
        showNotice(outcome?.summary || 'Changes applied', outcome?.detail || 'AgentStack finished the approved changes and verified the current system.', outcome?.outcome === 'succeeded' ? 'success' : 'error');
        await AS.refreshAll();
        showSection('activity');
      },
      onFailure: handleApplyFailure,
    });
  }

  async function prepareFailedSelection() {
    state.plan = null;
    state.planConsumed = false;
    await AS.refreshInventory();
    if (state.failedComponentIDs.length) state.selected = new Set(state.failedComponentIDs);
    for (const id of [...state.selected]) includeComponentDependencies(id);
    includeProviderDependencies($('browserProvider').value);
    activeTier = 'all';
    $('componentSearch').value = '';
    renderComponents();
  }

  async function retryFailedItems() {
    showSection('changes');
    await prepareFailedSelection();
    invalidatePlan('Failed items and their required dependencies are selected. Review the fresh plan before applying again.');
    return createPlan();
  }

  async function createFreshPlan() {
    showSection('changes');
    state.plan = null;
    state.planConsumed = false;
    await AS.refreshInventory();
    renderComponents();
    invalidatePlan('System state refreshed. Create changes to review a fresh plan.');
    return null;
  }

  function reviewFailedItems() {
    showSection('changes');
    if (!state.failedComponentIDs.length) return;
    state.selected = new Set(state.failedComponentIDs);
    for (const id of [...state.selected]) includeComponentDependencies(id);
    activeTier = 'all';
    $('componentSearch').value = '';
    renderComponents();
    invalidatePlan('Failed items are selected. Review the selection and create fresh changes when ready.');
  }

  function handleProviderChange() {
    const providerID = $('browserProvider').value;
    const added = includeProviderDependencies(providerID);
    renderComponents();
    const message = added.length
      ? `Included ${added.join(', ')} because they are required by the selected browser provider.`
      : 'Browser provider updated. Create changes again to review the latest result.';
    invalidatePlan(message);
  }

  function init() {
    $('componentSearch').addEventListener('input', () => {
      clearTimeout(searchTimer);
      searchTimer = setTimeout(renderComponents, 100);
    });
    document.querySelectorAll('[data-tier]').forEach(button => button.addEventListener('click', () => {
      activeTier = button.dataset.tier;
      document.querySelectorAll('[data-tier]').forEach(item => {
        const active = item === button;
        item.classList.toggle('active', active);
        item.setAttribute('aria-pressed', String(active));
      });
      renderComponents();
    }));
    $('allowCredentialed').addEventListener('change', event => { state.allowCredentialed = event.target.checked; invalidatePlan(); });
    $('allowUpgrades').addEventListener('change', event => { state.allowUpgrades = event.target.checked; invalidatePlan(); });
    $('browserProvider').addEventListener('change', handleProviderChange);
    $('createPlanBtn').addEventListener('click', () => state.planConsumed ? createFreshPlan() : createPlan());
    $('confirmApply').addEventListener('change', updateApplyAvailability);
    $('applyBtn').addEventListener('click', applyPlan);
    $('retryFailedBtn').addEventListener('click', retryFailedItems);
    $('reviewFailedBtn').addEventListener('click', reviewFailedItems);
    $('createFreshPlanBtn').addEventListener('click', createFreshPlan);
  }

  function load(catalog, inventory) {
    state.catalog = catalog;
    state.inventory = inventory;
    if (!catalog.profiles?.some(profile => profile.id === state.profile)) state.profile = catalog.profiles?.[0]?.id || '';
    if (!state.selected.size) state.selected = new Set(activeProfile()?.components || []);
    renderProviderOptions();
    includeProviderDependencies($('browserProvider').value);
    renderProfiles();
    renderComponents();
    if (!state.plan) {
      $('createPlanBtn').hidden = false;
      $('createPlanBtn').textContent = state.planConsumed ? 'Create fresh plan' : 'Create changes';
    }
    updateInlinePreview();
    updateCreateAvailability();
  }

  AS.changes = {
    init,
    load,
    renderComponents,
    createPlan,
    createFreshPlan,
    retryFailedItems,
    reviewFailedItems,
    updateApplyAvailability,
    handleApplyFailure,
    validateSelection,
    includeProviderDependencies,
  };
})();
