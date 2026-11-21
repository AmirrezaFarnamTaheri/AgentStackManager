'use strict';

(() => {
  const AS = window.AgentStack;
  const { $, state, api, escapeHTML, humanize, formatTime, showNotice, toast, runOperation } = AS;
  const MAX_ACTIVITY_ENTRIES = 50;
  let browserActivity = [];
  let activeResultFilter = 'all';
  let activeResultSort = 'status';
  let currentDiagnostics = [];
  let currentCauses = [];
  let historyQuery = '';
  let historyStatus = 'all';

  const phaseLabel = value => ({
    preparing: 'Preparing changes',
    installing: 'Installing tools',
    configuring: 'Configuring environments',
    verifying: 'Verifying results',
    complete: 'Run finished',
    finished: 'Run finished',
  }[value] || 'Applying changes');

  const itemLabel = value => ({
    waiting: 'Waiting',
    running: 'In progress',
    succeeded: 'Succeeded',
    failed: 'Failed',
    skipped: 'Skipped',
    'not-needed': 'Not needed',
  }[value] || humanize(value));

  function addBrowserActivity(title, detail, status = 'neutral') {
    browserActivity.unshift({ title, detail, status, at: new Date().toISOString() });
    browserActivity = browserActivity.slice(0, MAX_ACTIVITY_ENTRIES);
  }

  function beginProgress(plan) {
    const items = (plan.actions || [])
      .filter(action => ['install', 'repair', 'configure'].includes(action.kind))
      .map(action => ({
        id: action.componentId,
        label: action.name || action.componentId,
        action: action.kind,
        status: 'waiting',
        message: 'Waiting to start',
      }));
    resetResultSurface();
    renderProgress({ phase: 'preparing', processed: 0, succeeded: 0, failed: 0, skipped: 0, total: items.length, items });
    addBrowserActivity('Changes started', `${items.length} requested item${items.length === 1 ? '' : 's'} queued.`, 'running');
  }

  function processedCount(progress) {
    if (Number.isFinite(progress?.processed)) return Math.max(0, progress.processed);
    if (Number.isFinite(progress?.completed)) return Math.max(0, progress.completed);
    return (progress?.items || []).filter(item => ['succeeded', 'failed', 'skipped', 'not-needed'].includes(item.status)).length;
  }

  function progressDiagnostics(progress) {
    return (progress.items || []).map(item => ({
      componentId: item.id,
      label: item.label || item.id,
      action: item.action,
      result: item.status,
      summary: item.message || humanize(item.action),
      recommendedAction: item.status === 'failed' ? 'Wait for the run to finish so AgentStack can classify the failure.' : '',
      final: false,
    }));
  }

  function renderProgress(progress) {
    if (!progress) return;
    const tracker = $('installTracker');
    tracker.hidden = false;
    $('progressRegion').hidden = false;
    $('outcomeSummary').hidden = true;
    $('causeSummary').hidden = true;
    $('resultFilters').hidden = true;
    const total = Math.max(0, progress.total || 0);
    const processed = processedCount(progress);
    const succeeded = Math.max(0, progress.succeeded || 0);
    const failed = Math.max(0, progress.failed || 0);
    const skipped = Math.max(0, progress.skipped || 0);
    const percent = total ? Math.min(100, Math.round((processed / total) * 100)) : 0;
    $('installStage').textContent = phaseLabel(progress.phase);
    $('installCount').textContent = `${processed} of ${total} processed. ${succeeded} succeeded, ${failed} failed, ${skipped} skipped${progress.currentLabel ? `. Current: ${progress.currentLabel}` : ''}`;
    $('installProgress').value = percent;
    $('installProgress').textContent = `${percent}%`;
    $('installPercent').textContent = `${percent}%`;
    $('installOutcomeBadge').textContent = 'In progress';
    $('installOutcomeBadge').className = 'state-badge running';
    currentDiagnostics = progressDiagnostics(progress);
    renderResultRows();
    renderTechnicalDiagnostics({ progress });
  }

  function outcomeFromProgress(progress, failure = null) {
    const diagnostics = progressDiagnostics(progress || {});
    const succeeded = Number(progress?.succeeded) || diagnostics.filter(item => item.result === 'succeeded').length;
    const failed = Number(progress?.failed) || diagnostics.filter(item => item.result === 'failed').length;
    const skipped = Number(progress?.skipped) || diagnostics.filter(item => ['skipped', 'not-needed'].includes(item.result)).length;
    const requested = Number(progress?.total) || diagnostics.length;
    const outcome = failed ? (succeeded ? 'partially_failed' : 'failed') : skipped ? 'partially_failed' : 'succeeded';
    return {
      phase: 'finished',
      outcome,
      requested,
      processed: succeeded + failed + skipped,
      succeeded,
      failed,
      skipped,
      unchanged: 0,
      retryable: Boolean(failed),
      summary: failure?.message || (failed ? (succeeded ? 'Some requested changes were applied.' : 'No requested changes were applied.') : 'All requested changes were applied.'),
      detail: failure?.recovery || (failed ? 'Existing verified items were left unchanged. Resolve the cause, then retry failed items.' : 'Every requested change completed and was verified.'),
      diagnostics,
      causes: failed ? [{ category: 'installation_failed', summary: 'One or more changes could not be completed.', recommendedAction: 'Review failed items, then retry them in a fresh plan.', count: failed, componentIds: diagnostics.filter(item => item.result === 'failed').map(item => item.componentId) }] : [],
    };
  }

  function normalizeOutcome(source, fallbackProgress = null, failure = null) {
    return source?.outcome || source?.result?.outcome || outcomeFromProgress(fallbackProgress || source?.progress || {}, failure || source?.failure);
  }

  function outcomeTone(outcome) {
    if (outcome.outcome === 'succeeded') return ['Succeeded', 'success'];
    if (outcome.outcome === 'partially_failed') return ['Completed with issues', 'attention'];
    if (outcome.outcome === 'cancelled') return ['Cancelled', 'attention'];
    return ['Failed', 'error'];
  }

  function renderOutcome(source, fallbackProgress = null, failure = null) {
    const outcome = normalizeOutcome(source, fallbackProgress, failure);
    state.lastApplyOutcome = outcome;
    currentDiagnostics = (outcome.diagnostics || []).map(item => ({ ...item, final: true }));
    currentCauses = outcome.causes || [];
    $('installTracker').hidden = false;
    $('progressRegion').hidden = true;
    $('installStage').textContent = 'Run finished';
    $('installCount').textContent = `${outcome.processed || 0} of ${outcome.requested || 0} processed`;
    const [label, tone] = outcomeTone(outcome);
    $('installOutcomeBadge').textContent = label;
    $('installOutcomeBadge').className = `state-badge ${tone}`;
    $('outcomeSummary').hidden = false;
    $('outcomeTitle').textContent = outcome.summary || 'Run finished';
    $('outcomeDetail').textContent = outcome.detail || 'Review the result and choose the next action.';
    $('outcomeStats').innerHTML = [
      ['Requested', outcome.requested || 0],
      ['Succeeded', outcome.succeeded || 0],
      ['Failed', outcome.failed || 0],
      ['Skipped', outcome.skipped || 0],
      ['Unchanged', outcome.unchanged || 0],
    ].map(([term, value]) => `<div><dt>${term}</dt><dd>${value}</dd></div>`).join('');
    renderCauses(outcome.causes || []);
    $('resultFilters').hidden = currentDiagnostics.length === 0;
    activeResultFilter = 'all';
    updateResultFilterButtons();
    renderResultRows();
    const cancelled = outcome.outcome === 'cancelled';
    const hasRecoverableItems = (outcome.failed || 0) > 0 || (cancelled && (outcome.skipped || 0) > 0);
    $('retryFailedBtn').textContent = cancelled ? 'Retry unfinished items' : 'Retry failed items';
    $('reviewFailedBtn').textContent = cancelled ? 'Review unfinished items' : 'Review failed items';
    $('retryFailedBtn').hidden = !hasRecoverableItems || !outcome.retryable;
    $('reviewFailedBtn').hidden = !hasRecoverableItems;
    $('createFreshPlanBtn').hidden = !hasRecoverableItems;
    renderTechnicalDiagnostics(source?.result || source);
    renderRepairQueue(outcome);
    return outcome;
  }

  function renderCauses(causes) {
    $('causeSummary').hidden = causes.length === 0;
    $('causeGroups').innerHTML = causes.map(cause => {
      const count = cause.count || cause.componentIds?.length || 0;
      const meta = [cause.method, cause.errorCode ? `Error ${cause.errorCode}` : ''].filter(Boolean).join(' · ');
      const affected = (cause.affectedLabels || []).join(', ');
      return `<article class="cause-group">
        <div>
          ${meta ? `<small class="cause-meta">${escapeHTML(meta)}</small>` : ''}
          <strong>${escapeHTML(cause.title || cause.summary || 'This change could not be completed.')}</strong>
          <p>${escapeHTML(cause.summary || 'This change could not be completed.')}</p>
          ${cause.evidence ? `<small class="cause-evidence">What AgentStack observed: ${escapeHTML(cause.evidence)}</small>` : ''}
          ${affected ? `<small class="cause-affected">Affected: ${escapeHTML(affected)}</small>` : ''}
          <p class="cause-recovery"><strong>Fix:</strong> ${escapeHTML(cause.recommendedAction || 'Review the affected items and retry.')}</p>
        </div>
        <span>${escapeHTML(String(count))} ${count === 1 ? 'tool' : 'tools'}</span>
      </article>`;
    }).join('');
  }

  function resultCount(filter) {
    if (filter === 'all') return currentDiagnostics.length;
    return currentDiagnostics.filter(item => item.result === filter || (filter === 'skipped' && item.result === 'not-needed')).length;
  }

  function updateResultFilterCounts() {
    document.querySelectorAll('[data-result-filter]').forEach(button => {
      const label = button.dataset.resultLabel || humanize(button.dataset.resultFilter);
      button.textContent = `${label} ${resultCount(button.dataset.resultFilter)}`;
    });
  }

  function sharedRootCause(item) {
    return currentCauses.find(cause => (cause.count || cause.componentIds?.length || 0) > 1 && cause.componentIds?.includes(item.componentId));
  }

  function resultRank(result) {
    return ({ failed: 0, skipped: 1, 'not-needed': 1, waiting: 2, running: 2, succeeded: 3 }[result] ?? 4);
  }

  function compareDiagnostics(left, right) {
    if (activeResultSort === 'tool') return String(left.label || left.componentId).localeCompare(String(right.label || right.componentId));
    if (activeResultSort === 'action') return String(left.action).localeCompare(String(right.action)) || String(left.label || left.componentId).localeCompare(String(right.label || right.componentId));
    return resultRank(left.result) - resultRank(right.result) || String(left.label || left.componentId).localeCompare(String(right.label || right.componentId));
  }

  function formatDiagnosticCode(item) {
    if (item?.errorCode) return String(item.errorCode);
    if (!Number.isFinite(item?.exitCode)) return '';
    if (item.method === 'WinGet' && item.exitCode > 0) return `0x${(Number(item.exitCode) >>> 0).toString(16).toUpperCase().padStart(8, '0')}`;
    return String(item.exitCode);
  }

  function resultDetails(item) {
    if (!item.final || item.result !== 'failed') return '';
    const code = formatDiagnosticCode(item);
    return `<details class="item-result-details"><summary>Technical details</summary><dl>
      ${item.method ? `<div><dt>Method</dt><dd>${escapeHTML(item.method)}</dd></div>` : ''}
      ${item.category ? `<div><dt>Category</dt><dd>${escapeHTML(humanize(item.category))}</dd></div>` : ''}
      ${code ? `<div><dt>Error code</dt><dd><code>${escapeHTML(code)}</code></dd></div>` : ''}
      <div><dt>Retryable</dt><dd>${item.retryable ? 'Yes' : 'No'}</dd></div>
    </dl></details>`;
  }

  function renderResultRows() {
    const filtered = (activeResultFilter === 'all'
      ? currentDiagnostics
      : currentDiagnostics.filter(item => item.result === activeResultFilter || (activeResultFilter === 'skipped' && item.result === 'not-needed')))
      .slice().sort(compareDiagnostics);
    $('installItems').innerHTML = filtered.length ? filtered.map(item => {
      const shared = sharedRootCause(item);
      const reason = item.cause || item.summary || shared?.summary || (item.result === 'failed' ? 'The installer failed before a final classification was available.' : 'Completed and verified.');
      const nextAction = item.repairHint || item.recommendedAction || shared?.recommendedAction || (item.result === 'failed' ? 'Wait for the run to finish, then review the classified cause.' : 'None required.');
      return `<tr class="result-row ${escapeHTML(item.result || 'waiting')}">
        <th scope="row"><strong>${escapeHTML(item.label || humanize(item.componentId || '') || 'Unidentified component')}</strong></th>
        <td>${escapeHTML(humanize(item.action || 'change'))}</td>
        <td><span class="state-badge ${escapeHTML(item.result || 'neutral')}">${escapeHTML(itemLabel(item.result || 'waiting'))}</span></td>
        <td>${escapeHTML(reason)}</td>
        <td><div class="result-action"><span>${escapeHTML(nextAction)}</span>${resultDetails(item)}</div></td>
      </tr>`;
    }).join('') : '<tr><td colspan="5" class="empty-table">No results match this filter.</td></tr>';
  }

  function updateResultFilterButtons() {
    updateResultFilterCounts();
    document.querySelectorAll('[data-result-filter]').forEach(button => {
      const active = button.dataset.resultFilter === activeResultFilter;
      button.classList.toggle('active', active);
      button.setAttribute('aria-pressed', String(active));
    });
  }

  function technicalDiagnosticCard(item) {
    const code = formatDiagnosticCode(item);
    return `<article class="technical-diagnostic ${escapeHTML(item.result || 'neutral')}">
      <div class="technical-diagnostic-heading"><strong>${escapeHTML(item.label || item.componentId)}</strong><span class="state-badge ${escapeHTML(item.result || 'neutral')}">${escapeHTML(itemLabel(item.result))}</span></div>
      ${(item.cause || item.summary) ? `<p>${escapeHTML(item.cause || item.summary)}</p>` : ''}
      ${item.evidence ? `<small class="diagnostic-evidence">Observed: ${escapeHTML(item.evidence)}</small>` : ''}
      ${item.repairHint ? `<p class="diagnostic-repair"><strong>Fix:</strong> ${escapeHTML(item.repairHint)}</p>` : ''}
      <dl>
        <div><dt>Change</dt><dd>${escapeHTML(humanize(item.action))}</dd></div>
        ${item.method ? `<div><dt>Method</dt><dd>${escapeHTML(item.method)}</dd></div>` : ''}
        ${item.category ? `<div><dt>Category</dt><dd>${escapeHTML(humanize(item.category))}</dd></div>` : ''}
        ${code ? `<div><dt>Error code</dt><dd><code>${escapeHTML(code)}</code></dd></div>` : ''}
      </dl>
    </article>`;
  }

  function renderTechnicalDiagnostics(source) {
    const outcome = source?.outcome || state.lastApplyOutcome || null;
    const report = source?.report || null;
    const transaction = report?.transaction;
    const progress = source?.progress || null;
    const diagnostics = outcome?.diagnostics || currentDiagnostics;
    const failed = diagnostics.filter(item => item.result === 'failed');
    const succeeded = diagnostics.filter(item => item.result === 'succeeded');
    const other = diagnostics.filter(item => !['failed', 'succeeded'].includes(item.result));
    const summary = [];
    if (transaction?.id) summary.push(`Transaction ${transaction.id}`);
    if (transaction?.status) summary.push(`Status: ${humanize(transaction.status)}`);
    if (progress?.phase) summary.push(`Phase: ${phaseLabel(progress.phase)}`);
    if (diagnostics.length) summary.push(`${failed.length} failed, ${succeeded.length} succeeded${other.length ? `, ${other.length} other` : ''}`);
    $('technicalSummary').textContent = summary.length ? `${summary.join('. ')}.` : 'Sanitized diagnostics for this run.';
    const groups = [];
    if (failed.length) groups.push(`<section class="technical-diagnostic-group" aria-label="Failed changes"><h4>Failures (${failed.length})</h4>${failed.map(technicalDiagnosticCard).join('')}</section>`);
    if (other.length) groups.push(`<section class="technical-diagnostic-group" aria-label="Other changes"><h4>Other results (${other.length})</h4>${other.map(technicalDiagnosticCard).join('')}</section>`);
    if (succeeded.length) groups.push(`<details class="technical-diagnostic-group technical-successes"><summary>Successful changes (${succeeded.length})</summary><div>${succeeded.map(technicalDiagnosticCard).join('')}</div></details>`);
    $('technicalDiagnostics').innerHTML = groups.length ? groups.join('') : '<p>No item diagnostics were recorded.</p>';
  }

  function renderRepairQueue(outcome = state.lastApplyOutcome) {
    const failed = Number(outcome?.failed) || currentDiagnostics.filter(item => item.result === 'failed').length;
    const retryable = currentDiagnostics.filter(item => item.result === 'failed' && item.retryable).length;
    const causeCount = currentCauses.length;
    if (!failed) {
      $('repairQueueSummary').innerHTML = '<div><strong>No active repair queue</strong><p>There are no failed items in the current run.</p></div>';
      return;
    }
    $('repairQueueSummary').innerHTML = `<div><strong>${failed} failed item${failed === 1 ? '' : 's'} ready for review</strong><p>${causeCount} root cause${causeCount === 1 ? '' : 's'} classified. Retry only after applying the listed fixes.</p></div><dl class="health-stats"><div><dt>Retryable</dt><dd>${retryable}</dd></div><div><dt>Causes</dt><dd>${causeCount}</dd></div></dl>`;
  }

  function completeProgress(result) {
    const outcome = renderOutcome(result);
    addBrowserActivity('Run finished', `${outcome.succeeded || 0} succeeded, ${outcome.failed || 0} failed, ${outcome.skipped || 0} skipped.`, outcome.outcome === 'succeeded' ? 'success' : 'failed');
    return outcome;
  }

  function renderApplyFailure(operation) {
    const outcome = renderOutcome(operation, operation.progress, operation.failure);
    addBrowserActivity('Run finished with issues', `${outcome.failed || 0} requested change${outcome.failed === 1 ? '' : 's'} failed.`, 'failed');
  }

  function resetResultSurface() {
    state.lastApplyOutcome = null;
    currentDiagnostics = [];
    currentCauses = [];
    $('outcomeSummary').hidden = true;
    $('causeSummary').hidden = true;
    $('resultFilters').hidden = true;
    $('retryFailedBtn').hidden = true;
    $('reviewFailedBtn').hidden = true;
    $('createFreshPlanBtn').hidden = true;
    $('technicalSummary').textContent = 'No technical details yet.';
    $('technicalDiagnostics').innerHTML = '';
    renderRepairQueue(null);
  }

  function transactionMetrics(transaction) {
    const actions = transaction.actions || [];
    const requestedActions = actions.filter(action => ['install', 'repair', 'configure'].includes(action.kind));
    return {
      requested: requestedActions.length,
      succeeded: requestedActions.filter(action => action.verified && !action.error).length,
      failed: requestedActions.filter(action => action.error || !action.verified).length,
      unchanged: actions.filter(action => !['install', 'repair', 'configure'].includes(action.kind) && action.verified && !action.error).length,
    };
  }

  function durationLabel(transaction) {
    const started = new Date(transaction.startedAt || '');
    const finished = new Date(transaction.finishedAt || '');
    if (Number.isNaN(started.getTime()) || Number.isNaN(finished.getTime()) || started.getFullYear() <= 1900 || finished.getFullYear() <= 1900 || finished < started) return '';
    const seconds = Math.max(0, Math.round((finished - started) / 1000));
    if (seconds < 60) return `${seconds} second${seconds === 1 ? '' : 's'}`;
    const minutes = Math.round(seconds / 60);
    if (minutes < 60) return `${minutes} minute${minutes === 1 ? '' : 's'}`;
    const hours = Math.round(minutes / 60);
    return `${hours} hour${hours === 1 ? '' : 's'}`;
  }

  function transactionSummary(transaction) {
    const metrics = transactionMetrics(transaction);
    return `${metrics.requested} requested, ${metrics.succeeded} succeeded, ${metrics.failed} failed, ${metrics.unchanged} unchanged`;
  }

  function transactionRecovery(transaction) {
    if (!['failed', 'partial'].includes(transaction.status)) return '';
    return 'Review current state before creating a fresh plan.';
  }

  function transactionPresentation(transaction) {
    const metrics = transactionMetrics(transaction);
    if (metrics.succeeded > 0 && metrics.failed > 0) return { title: 'Completed with issues', badge: 'Needs attention', tone: 'attention' };
    if (transaction.status === 'succeeded') return { title: 'Run completed', badge: 'Completed', tone: 'success' };
    if (transaction.status === 'partial') return { title: 'Completed with issues', badge: 'Needs attention', tone: 'attention' };
    if (transaction.status === 'interrupted' || transaction.status === 'cancelled') return { title: 'Run cancelled', badge: 'Cancelled', tone: 'attention' };
    return { title: 'Run failed', badge: 'Failed', tone: 'error' };
  }

  function timestampValue(value) {
    const date = new Date(value || '');
    return Number.isNaN(date.getTime()) || date.getFullYear() <= 1900 ? Number.NEGATIVE_INFINITY : date.getTime();
  }

  function filteredTransactions() {
    const query = historyQuery.trim().toLowerCase();
    return (state.transactions || []).filter(transaction => {
      const metrics = transactionMetrics(transaction);
      const presentation = transactionPresentation(transaction);
      const statusMatches = historyStatus === 'all'
        || (historyStatus === 'attention' && (metrics.failed > 0 || ['failed', 'partial'].includes(transaction.status)))
        || (historyStatus === 'succeeded' && transaction.status === 'succeeded' && metrics.failed === 0)
        || (historyStatus === 'cancelled' && ['interrupted', 'cancelled'].includes(transaction.status));
      if (!statusMatches) return false;
      if (!query) return true;
      const tools = (transaction.actions || []).map(action => action.componentId).join(' ');
      return [transaction.id, transaction.status, presentation.title, tools].join(' ').toLowerCase().includes(query);
    });
  }

  function renderTransactions() {
    const transactions = filteredTransactions();
    $('transactionList').innerHTML = transactions.length ? transactions.slice(0, MAX_ACTIVITY_ENTRIES).map(transaction => {
      const presentation = transactionPresentation(transaction);
      const duration = durationLabel(transaction);
      const recovery = transactionRecovery(transaction);
      const timestamp = formatTime(transaction.finishedAt || transaction.startedAt);
      const support = [duration ? `Finished in ${duration}.` : '', recovery].filter(Boolean).join(' ');
      return `<li class="transaction-row">
        <div><strong>${escapeHTML(presentation.title)}</strong><p>${escapeHTML(transactionSummary(transaction))}</p>${support ? `<small>${escapeHTML(support)}</small>` : ''}</div>
        <div class="transaction-meta"><span class="state-badge ${presentation.tone}">${escapeHTML(presentation.badge)}</span>${timestamp ? `<small>${escapeHTML(timestamp)}</small>` : ''}</div>
      </li>`;
    }).join('') : '<li class="empty-state">No installation history yet.</li>';

    const combined = [
      ...browserActivity.map(item => ({ ...item, local: true })),
      ...transactions.map(transaction => ({ title: transactionPresentation(transaction).title, detail: transactionSummary(transaction), status: transaction.status, at: transaction.finishedAt || transaction.startedAt })),
    ].sort((a, b) => timestampValue(b.at) - timestampValue(a.at)).slice(0, 5);
    $('homeActivity').innerHTML = combined.length ? combined.map(item => {
      const timestamp = formatTime(item.at);
      return `<li><strong>${escapeHTML(item.title)}</strong><small>${escapeHTML(item.detail)}${timestamp ? `. ${escapeHTML(timestamp)}` : ''}</small></li>`;
    }).join('') : '<li class="empty-state">No recorded activity yet.</li>';
  }

  async function loadTransactions() {
    state.transactions = await api('transactions?limit=50');
    renderTransactions();
    return state.transactions;
  }

  async function loadRoutines() {
    try {
      const data = await api('routines');
      const routines = data.routines || [];
      $('routineList').innerHTML = routines.length ? routines.map(routine => `<div class="routine-row"><div><strong>${escapeHTML(routine.name || routine.id)}</strong><p>${escapeHTML(routine.description || 'Local maintenance routine')}</p></div><button class="secondary-button" type="button" data-routine-id="${escapeHTML(routine.id)}">Run</button></div>`).join('') : '<p class="empty-state">No maintenance routines configured.</p>';
      $('routineList').querySelectorAll('[data-routine-id]').forEach(button => button.addEventListener('click', () => runRoutine(button.dataset.routineId, button)));
    } catch {
      $('routineList').innerHTML = '<p class="empty-state">Maintenance routines are unavailable in this build.</p>';
    }
  }

  async function runRoutine(id, button) {
    await runOperation(button, 'Run maintenance', () => api('routines', { method: 'POST', body: JSON.stringify({ id, confirmed: true }) }), {
      onSuccess: result => {
        addBrowserActivity('Maintenance complete', result.receipt?.message || id, 'success');
        toast('Maintenance completed.');
        loadTransactions();
      },
    });
  }

  async function loadDiagnostics() {
    try {
      const data = await api('diagnostics/errors');
      const entries = data.errors || [];
      $('diagnosticList').innerHTML = entries.length ? entries.slice(0, 10).map(item => `<div class="diagnostic-row"><div><strong>${escapeHTML(item.component || 'System')}</strong><p>${escapeHTML(item.message || 'No details')}</p></div><span class="state-badge ${item.severity === 'error' ? 'error' : 'neutral'}">${escapeHTML(humanize(item.severity || 'info'))}</span></div>`).join('') : '<p class="empty-state">No recent system notices.</p>';
    } catch {
      $('diagnosticList').innerHTML = '<p class="empty-state">System notices are unavailable.</p>';
    }
  }

  async function runDoctor(button = $('runDoctorBtn')) {
    await runOperation(button, 'Run system check', () => api('mcp/doctor'), {
      onSuccess: result => {
        const count = result.unhealthyCount || 0;
        showNotice(count ? 'System check found issues' : 'System check passed', count ? `${count} MCP server${count === 1 ? '' : 's'} need attention.` : 'Configured MCP servers responded normally.', count ? 'error' : 'success');
        loadDiagnostics();
      },
    });
  }

  async function load() { await Promise.allSettled([loadTransactions(), loadRoutines(), loadDiagnostics()]); }

  function init() {
    $('refreshActivityBtn').addEventListener('click', load);
    $('runDoctorBtn').addEventListener('click', () => runDoctor());
    document.querySelectorAll('[data-result-filter]').forEach(button => button.addEventListener('click', () => {
      activeResultFilter = button.dataset.resultFilter;
      updateResultFilterButtons();
      renderResultRows();
    }));
    $('resultSort').addEventListener('change', event => {
      activeResultSort = event.target.value;
      renderResultRows();
    });
    $('historySearch').addEventListener('input', event => {
      historyQuery = event.target.value;
      renderTransactions();
    });
    $('historyStatusFilter').addEventListener('change', event => {
      historyStatus = event.target.value;
      renderTransactions();
    });
  }

  AS.activity = {
    init,
    load,
    loadTransactions,
    renderProgress,
    beginProgress,
    completeProgress,
    renderApplyFailure,
    renderTransactions,
    renderOutcome,
    renderTechnicalDiagnostics,
  };
})();
