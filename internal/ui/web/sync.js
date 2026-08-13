'use strict';

(() => {
  const AS = window.AgentStack;
  const { $, state, api, escapeHTML, humanize } = AS;
  const selectedResources = new Set();
  const selectedTargets = new Set();
  let activeFilter = 'all';
  let query = '';

  const installationTone = value => ({ 'in-sync': 'success', drifted: 'attention', conflict: 'error', orphan: 'attention', unmanaged: 'neutral', missing: 'neutral' }[value] || 'neutral');
  const installationLabel = value => ({ 'in-sync': 'In sync', drifted: 'Drifted', conflict: 'Conflict', orphan: 'Orphan', unmanaged: 'Unmanaged', missing: 'Not installed' }[value] || humanize(value));

  function counts() {
    return state.syncInspection?.counts || {};
  }

  function renderStats() {
    const value = counts();
    $('syncStats').innerHTML = [
      ['Managed', value.managed], ['Installed', value.installed], ['Contained', value.contained], ['In sync', value.inSync],
      ['Drifted', value.drifted], ['Duplicates', value.duplicates], ['Conflicts', value.conflicts],
    ].map(([label, number]) => `<div><dt>${label}</dt><dd>${Number(number) || 0}</dd></div>`).join('');
  }

  function candidateLabel(candidate) {
    const suffix = [candidate.label, candidate.scope].filter(Boolean).join(' · ');
    return suffix ? `${candidate.name} — ${suffix}` : candidate.name;
  }

  function renderTargets() {
    const candidates = (state.targetCandidates || []).filter(item => item.enabled && item.registered && item.writable);
    for (const id of [...selectedTargets]) if (!candidates.some(item => item.id === id)) selectedTargets.delete(id);
    $('syncTargetSelector').innerHTML = candidates.length ? `<div class="sync-selector-heading"><strong>Connected targets</strong><span>${selectedTargets.size} selected</span></div>${candidates.map(candidate => `
      <label class="sync-target-option"><input type="checkbox" data-sync-target="${escapeHTML(candidate.id)}" ${selectedTargets.has(candidate.id) ? 'checked' : ''}><span><strong>${escapeHTML(candidateLabel(candidate))}</strong><small>${escapeHTML(candidate.detectionState === 'confirmed' ? `Confirmed with ${candidate.confidence}% confidence` : candidate.message)}</small></span></label>`).join('')}` : '<p class="empty-state">Connect at least one verified writable target in Environments.</p>';
    $('syncTargetSelector').querySelectorAll('[data-sync-target]').forEach(input => input.addEventListener('change', () => {
      if (input.checked) selectedTargets.add(input.dataset.syncTarget); else selectedTargets.delete(input.dataset.syncTarget);
      updatePlanAvailability();
      renderTargets();
    }));
  }

  function resourceMatches(resource) {
    const installations = resource.installations || [];
    const haystack = [resource.name, resource.namespace, resource.kind, resource.version, ...installations.flatMap(item => [item.targetId, item.agent, item.scope, item.state])].join(' ').toLowerCase();
    if (query && !haystack.includes(query)) return false;
    if (activeFilter === 'all') return true;
    if (activeFilter === 'contained') return Boolean(resource.contained);
    if (activeFilter === 'installed') return installations.some(item => item.state !== 'missing');
    return installations.some(item => item.state === activeFilter);
  }

  function renderResources() {
    const resources = (state.syncInspection?.resources || []).filter(resourceMatches);
    $('syncResourceList').innerHTML = resources.length ? resources.map(resource => {
      const resourceID = resource.resourceIds?.[0] || resource.identity;
      const installations = resource.installations || [];
      return `<article class="sync-resource-row">
        <label class="sync-resource-select"><input type="checkbox" data-sync-resource="${escapeHTML(resourceID)}" ${selectedResources.has(resourceID) ? 'checked' : ''}><span><strong>${escapeHTML(resource.name)}</strong><small>${escapeHTML(resource.namespace)} · ${escapeHTML(humanize(resource.kind))}${resource.version ? ` · ${escapeHTML(resource.version)}` : ''}${resource.contained ? ' · Contained in stack' : ''}</small></span></label>
        <div class="sync-installations">${installations.length ? installations.map(item => `<span class="state-badge ${installationTone(item.state)}" title="${escapeHTML(item.message)}">${escapeHTML(item.targetId)}: ${escapeHTML(installationLabel(item.state))}</span>`).join('') : '<span class="state-badge neutral">No connected installation</span>'}</div>
      </article>`;
    }).join('') : '<p class="empty-state">No canonical resources match this view.</p>';
    $('syncResourceList').querySelectorAll('[data-sync-resource]').forEach(input => input.addEventListener('change', () => {
      if (input.checked) selectedResources.add(input.dataset.syncResource); else selectedResources.delete(input.dataset.syncResource);
      updatePlanAvailability();
    }));
  }

  function renderDuplicates() {
    const groups = state.syncInspection?.duplicates || [];
    $('syncDuplicateList').innerHTML = groups.length ? groups.map(group => `<article class="sync-duplicate-row ${group.review ? 'review' : ''}">
      <div><strong>${escapeHTML(humanize(group.class))}</strong><p>${escapeHTML(group.message)}</p><small>${group.resourceIds?.length || 0} resource entries · ${group.targetIds?.length || 0} targets</small></div>
      <span class="state-badge ${group.review ? 'attention' : 'success'}">${group.review ? 'Review required' : 'Canonical fan-out'}</span>
    </article>`).join('') : '<p class="empty-state">No duplicate, collision, or orphan groups were detected.</p>';
  }

  function updatePlanAvailability() {
    $('syncBuildPlanBtn').disabled = selectedTargets.size === 0 || selectedResources.size === 0;
  }

  function resetPlan() {
    state.syncPlan = null;
    $('syncPlanSummary').textContent = 'Select resources and targets, then build a fresh immutable plan.';
    $('syncPlanChildren').innerHTML = '<li class="empty-state">No pending sync plan.</li>';
    $('confirmSyncApply').checked = false;
    $('confirmSyncApply').disabled = true;
    $('syncApplyBtn').disabled = true;
  }

  async function buildPlan(button = $('syncBuildPlanBtn')) {
    await AS.runOperation(button, 'Build sync plan', () => api('sharing-sync/plan', {
      method: 'POST', body: JSON.stringify({ targetIds: [...selectedTargets], resourceIds: [...selectedResources], maxParallel: Number($('syncParallelism').value) || 3 }),
    }), {
      onSuccess: plan => {
        state.syncPlan = plan;
        $('syncPlanSummary').textContent = `${plan.children?.length || 0} target plans are ready. The approved plan can run up to ${plan.maxParallel || 1} independent targets simultaneously.`;
        $('syncPlanChildren').innerHTML = (plan.children || []).map(child => `<li class="change-row"><div><strong>${escapeHTML(child.targetId)}</strong><p>${child.operations?.length || 0} reviewed operations · expires ${escapeHTML(AS.formatTime(child.expiresAt))}</p></div><span class="state-badge neutral">Ready</span></li>`).join('') || '<li class="empty-state">The plan contains no target changes.</li>';
        $('confirmSyncApply').disabled = false;
        $('syncApplyBtn').disabled = true;
        AS.toast('Reviewed sync plan created.');
      },
    });
  }

  async function applyPlan(button = $('syncApplyBtn')) {
    const plan = state.syncPlan;
    if (!plan || !$('confirmSyncApply').checked) return;
    await AS.runOperation(button, 'Apply sync plan', () => api('sharing-sync/apply', { method: 'POST', body: JSON.stringify({ planId: plan.id, digest: plan.digest, confirm: true }) }), {
      onSuccess: async report => {
        AS.toast(`${report.succeeded || 0} target${report.succeeded === 1 ? '' : 's'} synchronized and verified.`);
        resetPlan();
        await Promise.all([load(), AS.environments.load()]);
      },
    });
  }

  async function load() {
    state.syncInspection = await api('sharing-sync');
    renderStats();
    renderTargets();
    renderResources();
    renderDuplicates();
    updatePlanAvailability();
    return state.syncInspection;
  }

  function init() {
    $('syncScanBtn').addEventListener('click', () => AS.runOperation($('syncScanBtn'), 'Scan sharing and sync', load));
    $('syncBuildPlanBtn').addEventListener('click', () => buildPlan());
    $('syncApplyBtn').addEventListener('click', () => applyPlan());
    $('confirmSyncApply').addEventListener('change', event => { $('syncApplyBtn').disabled = !event.target.checked || !state.syncPlan; });
    $('syncSearch').addEventListener('input', event => { query = event.target.value.trim().toLowerCase(); renderResources(); });
    $('syncStateFilter').addEventListener('change', event => { activeFilter = event.target.value; renderResources(); });
  }

  AS.sync = { init, load, resetPlan };
})();
