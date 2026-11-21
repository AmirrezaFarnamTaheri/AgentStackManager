'use strict';

(() => {
  const AS = window.AgentStack;
  const { $, state, api, escapeHTML, humanize, showSection } = AS;
  let activeKind = 'all';
  let activeEnvironmentID = '';
  const selectedConnectionIDs = new Set();

  const stateLabel = value => ({
    'needs-attention': 'Needs attention',
    'not-supported': 'Not supported here',
    'not-connected': 'Not connected',
    installed: 'Installed', connected: 'Connected', detected: 'Detected', shared: 'Shared', paused: 'Paused', available: 'Available',
  }[value] || humanize(value));

  function environmentByID(id) { return state.environments?.environments?.find(environment => environment.id === id); }
  function candidatesForEnvironment(environment) {
    return (state.targetCandidates || []).filter(candidate => candidate.agent === environment?.id);
  }
  function candidateForEnvironment(environment) {
    return candidatesForEnvironment(environment)[0];
  }
  function displayedEnvironmentState(environment) {
    const candidate = candidateForEnvironment(environment);
    return environment?.state === 'not-connected' && candidate?.detected ? 'detected' : environment?.state;
  }

  function renderEnvironmentList() {
    const environments = (state.environments?.environments || []).filter(environment => activeKind === 'all' || environment.kind === activeKind);
    if (!environments.some(environment => environment.id === activeEnvironmentID)) activeEnvironmentID = environments[0]?.id || '';
    $('environmentList').innerHTML = environments.length ? environments.map(environment => `
      <button class="environment-button${environment.id === activeEnvironmentID ? ' active' : ''}" type="button" data-environment-id="${escapeHTML(environment.id)}">
        <span><strong>${escapeHTML(environment.name)}</strong><small>${escapeHTML(humanize(environment.kind))} · ${environment.resources?.length || 0} resource${environment.resources?.length === 1 ? '' : 's'}</small></span>
        <span class="state-badge ${escapeHTML(displayedEnvironmentState(environment))}">${escapeHTML(stateLabel(displayedEnvironmentState(environment)))}</span>
      </button>`).join('') : '<p class="empty-state">No environments match this filter.</p>';
    $('environmentList').querySelectorAll('[data-environment-id]').forEach(button => button.addEventListener('click', () => {
      activeEnvironmentID = button.dataset.environmentId;
      renderEnvironmentList();
      renderEnvironmentDetail();
    }));
    renderEnvironmentDetail();
  }

  function renderEnvironmentDetail() {
    const environment = environmentByID(activeEnvironmentID);
    if (!environment) {
      $('environmentDetail').innerHTML = '<h3>Select an environment</h3><p>Choose an item to see what is available there.</p>';
      return;
    }
    const resources = environment.resources || [];
    const candidate = candidateForEnvironment(environment);
    const stateValue = displayedEnvironmentState(environment);
    const canConnect = ['ai-app', 'ide'].includes(environment.kind) && candidate;
    const action = canConnect
      ? `<button class="${candidate.enabled ? 'secondary-button' : 'primary-button'}" type="button" data-connect-target="${escapeHTML(candidate.id)}" data-connect-enabled="${candidate.enabled ? 'false' : 'true'}" ${candidate.writable ? '' : 'disabled'}>${candidate.enabled ? 'Pause connection' : candidate.registered ? 'Reconnect' : candidate.writable ? 'Connect' : 'Read only'}</button>`
      : '';
    const connectionCopy = candidate?.message || environment.message || 'Local environment';
    $('environmentDetail').innerHTML = `
      <div class="section-line environment-detail-heading"><div><h3>${escapeHTML(environment.name)}</h3><p>${escapeHTML(connectionCopy)}</p></div><span class="state-badge ${escapeHTML(stateValue)}">${escapeHTML(stateLabel(stateValue))}</span></div>
      ${canConnect ? `<div class="connection-widget"><div><strong>${candidate.detectionState === 'confirmed' ? 'Application identity confirmed' : humanize(candidate.detectionState)}</strong><p>${escapeHTML(candidate.message)} ${candidate.confidence ? `Evidence confidence: ${candidate.confidence}%.` : ''}</p></div>${action}</div>` : ''}
      <ol class="resource-list">
        ${resources.length ? resources.map(resource => `<li class="resource-row"><div><strong>${escapeHTML(resource.name)}</strong><p>${escapeHTML(resource.message || humanize(resource.type))}${resource.version ? ` · ${escapeHTML(resource.version)}` : ''}</p></div><span class="state-badge ${escapeHTML(resource.state)}">${escapeHTML(stateLabel(resource.state))}</span></li>`).join('') : '<li class="empty-state">No AgentStack-managed resources are listed here yet.</li>'}
      </ol>`;
    $('environmentDetail').querySelectorAll('[data-connect-target]').forEach(button => button.addEventListener('click', () => setConnection(button.dataset.connectTarget, button.dataset.connectEnabled === 'true', button)));
  }

  function renderConnections() {
    const candidates = state.targetCandidates || [];
    for (const id of [...selectedConnectionIDs]) if (!candidates.some(item => item.id === id)) selectedConnectionIDs.delete(id);
    $('connectionList').innerHTML = candidates.length ? `<div class="connection-batch-toolbar"><div><strong>${selectedConnectionIDs.size} selected</strong><p>Select several verified targets and update them together.</p></div><div class="connection-actions"><button id="connectSelectedBtn" class="primary-button" type="button" ${selectedConnectionIDs.size ? '' : 'disabled'}>Connect selected</button><button id="pauseSelectedBtn" class="secondary-button" type="button" ${selectedConnectionIDs.size ? '' : 'disabled'}>Pause selected</button></div></div>${candidates.map(candidate => {
      const stateValue = candidate.enabled ? 'connected' : candidate.registered ? 'paused' : candidate.detected ? 'detected' : candidate.detectionState === 'unsupported' ? 'not-supported' : 'not-connected';
      const label = candidate.enabled ? 'Pause' : candidate.registered ? 'Reconnect' : candidate.writable ? 'Connect' : 'Read only';
      const evidence = (candidate.evidence || []).map(item => item.message).join(' ');
      return `<div class="connection-row">
        <label class="connection-select"><input type="checkbox" data-connection-select="${escapeHTML(candidate.id)}" ${selectedConnectionIDs.has(candidate.id) ? 'checked' : ''} ${candidate.writable ? '' : 'disabled'}><span><strong>${escapeHTML(candidate.name)}${candidate.label ? ` — ${escapeHTML(candidate.label)}` : ''}</strong><p>${escapeHTML(candidate.message)}</p><small>${escapeHTML(evidence || `${humanize(candidate.supportLevel)} adapter · ${candidate.confidence || 0}% confidence`)}</small></span></label>
        <div class="connection-actions"><span class="state-badge ${escapeHTML(stateValue)}">${escapeHTML(stateLabel(stateValue))}</span><button class="secondary-button" type="button" data-connect-target="${escapeHTML(candidate.id)}" data-connect-enabled="${candidate.enabled ? 'false' : 'true'}" ${candidate.writable ? '' : 'disabled'}>${label}</button></div>
      </div>`;
    }).join('')}` : '<p class="empty-state">No supported AI app targets were found.</p>';
    $('connectionList').querySelectorAll('[data-connection-select]').forEach(input => input.addEventListener('change', () => {
      if (input.checked) selectedConnectionIDs.add(input.dataset.connectionSelect); else selectedConnectionIDs.delete(input.dataset.connectionSelect);
      renderConnections();
    }));
    $('connectionList').querySelectorAll('[data-connect-target]').forEach(button => button.addEventListener('click', () => setConnection(button.dataset.connectTarget, button.dataset.connectEnabled === 'true', button)));
    $('connectSelectedBtn')?.addEventListener('click', () => batchSetConnection(true, $('connectSelectedBtn')));
    $('pauseSelectedBtn')?.addEventListener('click', () => batchSetConnection(false, $('pauseSelectedBtn')));
  }

  function connectionPayload(candidate, enabled) {
    return { id: candidate.id, agent: candidate.agent, scope: candidate.scope || 'global', label: candidate.label || candidate.name, profile: candidate.profile || '', enabled };
  }

  async function batchSetConnection(enabled, button) {
    const targets = (state.targetCandidates || []).filter(candidate => selectedConnectionIDs.has(candidate.id) && candidate.writable).map(candidate => connectionPayload(candidate, enabled));
    if (!targets.length) return;
    await AS.runOperation(button, enabled ? 'Connect selected environments' : 'Pause selected environments', () => api('environment-targets/batch', { method: 'POST', body: JSON.stringify({ targets }) }), {
      onSuccess: async result => {
        AS.toast(`${result.updated || targets.length} environment target${targets.length === 1 ? '' : 's'} updated.`);
        selectedConnectionIDs.clear();
        await Promise.all([load(), AS.sync?.load?.()]);
      },
    });
  }

  async function setConnection(targetID, enabled, button) {
    const candidate = (state.targetCandidates || []).find(item => item.id === targetID);
    if (!candidate || !candidate.writable) return;
    await AS.runOperation(button, enabled ? 'Connect environment' : 'Pause environment', () => api('environment-targets/batch', { method: 'POST', body: JSON.stringify({ targets: [connectionPayload(candidate, enabled)] }) }), {
      onSuccess: async () => {
        AS.toast(enabled ? 'Environment connected.' : 'Environment connection paused.');
        await Promise.all([load(), AS.sync?.load?.()]);
      },
    });
  }

  function renderEnvironmentHealth() {
    const overview = state.environments || {};
    const score = Number(overview.healthScore) || 0;
    const issues = Number(overview.issueCount) || 0;
    const connected = Number(overview.connectedCount) || 0;
    const tone = issues ? 'attention' : 'success';
    $('environmentHealthSummary').innerHTML = `<div><strong>${score}% environment health</strong><p>${escapeHTML(overview.recommendedAction || 'Review connection and tool state.')}</p></div><dl class="health-stats"><div><dt>Connected</dt><dd>${connected}</dd></div><div><dt>Issues</dt><dd>${issues}</dd></div><div><dt>Status</dt><dd><span class="state-badge ${tone}">${issues ? 'Needs attention' : 'Healthy'}</span></dd></div></dl>`;
  }

  function renderHomeSummary() {
    const environments = state.environments?.environments || [];
    const inventoryItems = Object.values(state.inventory?.items || {});
    const installed = inventoryItems.filter(item => item.installed).length;
    const attention = inventoryItems.filter(item => item.broken || item.incompatible).length;
    const connected = environments.filter(environment => environment.state === 'connected').length;
    $('homeStats').innerHTML = `<div><dt>Environments</dt><dd>${environments.length}</dd></div><div><dt>Installed tools</dt><dd>${installed}</dd></div><div><dt>Needs attention</dt><dd>${attention}</dd></div>`;
    if (attention) {
      $('homeState').textContent = 'Needs attention';
      $('homeState').className = 'state-badge attention';
      $('homeSummary').textContent = `${attention} item${attention === 1 ? ' needs' : 's need'} review`;
      $('homeDetail').textContent = 'AgentStack found installed tools that are broken, incompatible, or incomplete.';
      $('homeNextAction').textContent = 'Review recommended changes';
      $('homeNextAction').dataset.targetSection = 'changes';
      $('homeRecommendation').innerHTML = `<div><strong>Repair tools that need attention</strong><p>Create a current plan, review the affected items, and approve only the repairs you understand.</p></div>`;
    } else if (!connected) {
      $('homeState').textContent = 'Ready';
      $('homeState').className = 'state-badge success';
      $('homeSummary').textContent = 'Your local tools are ready';
      $('homeDetail').textContent = 'Review environments to see where tools and shared resources are available.';
      $('homeNextAction').textContent = 'View environments';
      $('homeNextAction').dataset.targetSection = 'environments';
      $('homeRecommendation').innerHTML = `<div><strong>Connect an AI app or IDE</strong><p>See supported environments and review which resources can be shared with each one.</p></div>`;
    } else {
      $('homeState').textContent = 'Ready';
      $('homeState').className = 'state-badge success';
      $('homeSummary').textContent = `${connected} environment${connected === 1 ? '' : 's'} connected and ready`;
      $('homeDetail').textContent = 'No local tools currently need attention. You can review optional additions at any time.';
      $('homeNextAction').textContent = 'Review optional changes';
      $('homeNextAction').dataset.targetSection = 'changes';
      $('homeRecommendation').innerHTML = `<div><strong>Nothing urgent</strong><p>Existing compatible tools are preserved. Open Changes only when you want to add or update capabilities.</p></div>`;
    }
    AS.renderOperationalStatus?.();
  }

  async function load() {
    const [environments, targetData] = await Promise.all([api('environments'), api('environment-targets')]);
    state.environments = environments;
    state.targetCandidates = targetData.candidates || [];
    renderEnvironmentList();
    renderConnections();
    renderEnvironmentHealth();
    renderHomeSummary();
    return state.environments;
  }

  function init() {
    document.querySelectorAll('[data-environment-kind]').forEach(button => button.addEventListener('click', () => {
      activeKind = button.dataset.environmentKind;
      document.querySelectorAll('[data-environment-kind]').forEach(item => {
        const active = item === button;
        item.classList.toggle('active', active);
        item.setAttribute('aria-pressed', String(active));
      });
      renderEnvironmentList();
    }));
  }

  AS.environments = { init, load, renderHomeSummary };
})();
