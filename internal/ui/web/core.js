'use strict';

(() => {
  const token = document.querySelector('meta[name="agentstack-token"]').content;
  const base = document.querySelector('meta[name="agentstack-base"]').content;
  const version = document.querySelector('meta[name="agentstack-version"]').content;
  const $ = id => document.getElementById(id);
  const delay = milliseconds => new Promise(resolve => setTimeout(resolve, milliseconds));

  const state = {
    catalog: null,
    inventory: null,
    environments: null,
    targetCandidates: [],
    transactions: [],
    syncInspection: null,
    syncPlan: null,
    plan: null,
    planConsumed: false,
    selected: new Set(),
    profile: 'core',
    allowCredentialed: false,
    allowUpgrades: false,
    providerOverrides: {},
    activeSection: 'home',
    activeOperation: null,
    failedComponentIDs: [],
  };

  const sectionCopy = {
    home: ['Home', 'See what needs attention and continue from one clear next step.'],
    environments: ['Environments', 'See where your tools, skills, prompts, and MCP servers are available.'],
    sync: ['Sharing & Sync', 'Inspect canonical resources, deduplicate safely, and apply reviewed changes across connected targets.'],
    changes: ['Changes', 'Choose what you need, inspect the pending changes, then approve once.'],
    activity: ['Activity', 'Follow installations, review completed work, and troubleshoot from one place.'],
  };

  const escapeHTML = value => String(value ?? '').replace(/[&<>'"]/g, character => ({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[character]));
  const humanize = value => String(value ?? '').replace(/[-_]/g, ' ').replace(/\b\w/g, letter => letter.toUpperCase()).replace(/\bAi\b/g, 'AI').replace(/\bCli\b/g, 'CLI').replace(/\bMcp\b/g, 'MCP').replace(/\bIde\b/g, 'IDE');
  const formatTime = value => {
    if (!value) return '';
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? '' : date.toLocaleString([], { dateStyle: 'medium', timeStyle: 'short' });
  };

  async function waitForOperation(statusURL, onProgress) {
    for (;;) {
      await delay(750);
      const operation = await api(statusURL);
      onProgress?.(operation.progress);
      if (operation.status === 'running') continue;
      if (operation.status === 'failed' || operation.status === 'cancelled') {
        const fallback = operation.status === 'cancelled' ? 'The operation was cancelled.' : 'AgentStack could not complete this operation.';
        const error = new Error(operation.failure?.message || operation.error || fallback);
        error.data = operation;
        throw error;
      }
      if (operation.status !== 'succeeded') throw new Error('AgentStack returned an unknown operation state.');
      return operation.result;
    }
  }

  async function api(path, options = {}) {
    const headers = { ...(options.headers || {}), 'X-AgentStack-Token': token };
    if (options.body) headers['Content-Type'] = 'application/json';
    const endpoint = path.startsWith('/') ? path : `${base}api/${path}`;
    const response = await fetch(endpoint, { ...options, headers });
    const data = await response.json().catch(() => ({ error: `Request failed (${response.status})` }));
    if (!response.ok) {
      const error = new Error(data.error || `Request failed (${response.status})`);
      error.data = data;
      throw error;
    }
    if (response.status === 202 && data.statusUrl) return waitForOperation(data.statusUrl, options.onProgress);
    return data;
  }

  function showSection(name, moveFocus = true) {
    if (!sectionCopy[name]) return;
    state.activeSection = name;
    document.querySelectorAll('[data-workspace-section]').forEach(section => {
      const active = section.dataset.workspaceSection === name;
      section.hidden = !active;
      section.classList.toggle('active', active);
    });
    document.querySelectorAll('[data-section]').forEach(button => {
      if (button.dataset.section === name) button.setAttribute('aria-current', 'page');
      else button.removeAttribute('aria-current');
    });
    $('pageTitle').textContent = sectionCopy[name][0];
    $('pageSubtitle').textContent = sectionCopy[name][1];
    history.replaceState(null, '', `#${name}`);
    if (moveFocus) $('mainContent').focus({ preventScroll: true });
  }

  function toast(message, error = false) {
    const element = $('toast');
    element.textContent = message;
    element.className = `toast visible${error ? ' error' : ''}`;
    clearTimeout(toast.timer);
    toast.timer = setTimeout(() => { element.className = 'toast'; }, 4200);
  }

  function showNotice(title, message, kind = 'error', action = null) {
    const notice = $('operationNotice');
    notice.hidden = false;
    notice.className = `operation-notice${kind === 'success' ? ' success' : ''}`;
    $('operationNoticeTitle').textContent = title;
    $('operationNoticeMessage').textContent = message;
    const button = $('operationNoticeAction');
    button.hidden = !action;
    button.onclick = null;
    if (action) {
      button.textContent = action.label;
      button.onclick = action.run;
    }
  }

  function hideNotice() { $('operationNotice').hidden = true; }

  function setBusy(button, busy) {
    document.querySelectorAll('[data-operation]').forEach(control => {
      if (busy) {
        control.dataset.previouslyDisabled = control.disabled ? 'true' : 'false';
        control.disabled = true;
      } else {
        control.disabled = control.dataset.previouslyDisabled === 'true';
        delete control.dataset.previouslyDisabled;
      }
    });
    if (!button) return;
    if (busy) {
      button.dataset.originalLabel = button.textContent;
      button.textContent = button.dataset.busyLabel || 'Working…';
    } else if (button.dataset.originalLabel) {
      button.textContent = button.dataset.originalLabel;
      delete button.dataset.originalLabel;
    }
  }

  async function runOperation(button, name, work, options = {}) {
    if (state.activeOperation) {
      toast(`${state.activeOperation} is still running.`, true);
      return null;
    }
    state.activeOperation = name;
    setBusy(button, true);
    hideNotice();
    try {
      const result = await work();
      await options.onSuccess?.(result);
      return result;
    } catch (error) {
      await options.onFailure?.(error);
      if (!options.handledFailure) {
        const failure = error.data?.failure || error.data;
        showNotice(`${name} needs attention`, failure?.message || error.message, 'error', failure?.recovery ? { label: 'Review recovery', run: () => showSection('activity') } : null);
        toast(failure?.message || error.message, true);
      }
      return null;
    } finally {
      setBusy(button, false);
      state.activeOperation = null;
      window.AgentStack.changes?.updateApplyAvailability();
    }
  }

  function setTheme(choice) {
    const safe = ['light', 'dark', 'system'].includes(choice) ? choice : 'system';
    document.documentElement.dataset.theme = safe;
    localStorage.setItem('agentstack-theme', safe);
    document.querySelectorAll('[data-theme-choice]').forEach(button => button.classList.toggle('active', button.dataset.themeChoice === safe));
  }

  function initMenu() {
    const menu = document.querySelector('.app-menu');
    const summary = menu?.querySelector('summary');
    if (!menu || !summary) return;
    document.addEventListener('keydown', event => {
      if (event.key === 'Escape' && menu.open) {
        menu.open = false;
        summary.focus();
        return;
      }
      if (event.key === 'Tab' && menu.open) {
        const focusable = [...menu.querySelectorAll('button:not(:disabled), input:not(:disabled), select:not(:disabled), summary')];
        const first = focusable[0];
        const last = focusable[focusable.length - 1];
        if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
        else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
      }
    });
    document.addEventListener('click', event => {
      if (menu.open && !menu.contains(event.target)) menu.open = false;
    });
  }

  window.AgentStack = { $, api, state, version, escapeHTML, humanize, formatTime, showSection, toast, showNotice, hideNotice, runOperation, setTheme, initMenu };
})();
