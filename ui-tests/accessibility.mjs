import { spawn } from 'node:child_process';
import { mkdtemp, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { chromium } from 'playwright';

const binary = process.env.AGENTSTACK_UI_BINARY || path.resolve('..', process.platform === 'win32' ? 'agentstack.exe' : 'agentstack');
const isolatedHome = await mkdtemp(path.join(os.tmpdir(), 'agentstack-lifecycle-ui-'));
const child = spawn(binary, ['ui', '--no-open', '--listen', '127.0.0.1:0'], {
  cwd: path.dirname(binary),
  env: { ...process.env, HOME: isolatedHome, USERPROFILE: isolatedHome, LOCALAPPDATA: path.join(isolatedHome, 'AppData', 'Local') },
  stdio: ['ignore', 'pipe', 'pipe'],
  windowsHide: true
});

let stderr = '';
child.stderr.setEncoding('utf8');
child.stderr.on('data', chunk => { stderr += chunk; });

function managerUrl() {
  return new Promise((resolve, reject) => {
    let output = '';
    const timer = setTimeout(() => reject(new Error(`manager URL timeout; stderr=${stderr}`)), 60_000);
    child.stdout.setEncoding('utf8');
    child.stdout.on('data', chunk => {
      output += chunk;
      const match = output.match(/AgentStack Manager:\s+(http:\/\/\S+)/);
      if (match) { clearTimeout(timer); resolve(match[1]); }
    });
    child.once('exit', code => { clearTimeout(timer); reject(new Error(`manager exited before startup: code=${code}; stderr=${stderr}`)); });
  });
}

const blockingAxe = result => result.violations.filter(item => item.impact === 'critical' || item.impact === 'serious');
let browser;
try {
  const url = await managerUrl();
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ reducedMotion: 'reduce', viewport: { width: 1440, height: 1000 } });
  const page = await context.newPage();
  const consoleErrors = [];
  page.on('console', message => { if (message.type() === 'error') consoleErrors.push(message.text()); });
  page.on('pageerror', error => consoleErrors.push(error.message));
  await page.addInitScript({ path: path.resolve('node_modules', 'axe-core', 'axe.min.js') });
  await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 60_000 });
  await page.waitForFunction(() => document.querySelectorAll('#profileList input').length > 0, null, { timeout: 60_000 });
  await page.waitForFunction(() => !document.querySelector('#systemState')?.textContent.includes('Checking'));

  const axe = await page.evaluate(async () => globalThis.axe.run(document, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'] } }));
  if (blockingAxe(axe).length) throw new Error(`blocking accessibility violations:\n${JSON.stringify(blockingAxe(axe), null, 2)}`);

  await page.locator('body').press('Tab');
  if (!String(await page.evaluate(() => document.activeElement?.className || '')).includes('skip-link')) throw new Error('first keyboard focus is not the skip link');
  await page.keyboard.press('Enter');
  if (await page.evaluate(() => document.activeElement?.id) !== 'mainContent') throw new Error('skip link did not move focus to main content');

  const liveRegionsAreAtomic = await page.evaluate(() => [...document.querySelectorAll('[aria-live]')].every(element => element.getAttribute('aria-atomic') === 'true'));
  if (!liveRegionsAreAtomic) throw new Error('one or more live regions are not atomic');

  const tooSmallText = await page.evaluate(() => [...document.querySelectorAll('body *')].filter(element => {
    if (element.hidden || !element.getClientRects().length) return false;
    const ownText = [...element.childNodes].some(node => node.nodeType === Node.TEXT_NODE && node.textContent.trim());
    return ownText && parseFloat(getComputedStyle(element).fontSize) < 12;
  }).map(element => ({ tag: element.tagName, id: element.id, text: element.textContent.trim().slice(0, 80), size: getComputedStyle(element).fontSize })));
  if (tooSmallText.length) throw new Error(`text below 12px: ${JSON.stringify(tooSmallText.slice(0, 10))}`);

  await page.locator('[data-section="environments"]').click();
  await page.locator('#environments:not([hidden])').waitFor();
  await page.locator('#environmentList .environment-button').first().waitFor();
  if (await page.locator('#connectionList').textContent() === '') throw new Error('sharing and sync state did not render');

  await page.locator('[data-section="changes"]').click();
  const providerOption = page.locator('#browserProvider option[value="chrome-devtools-mcp"]');
  if (await providerOption.count()) {
    await page.locator('#browserProvider').selectOption('chrome-devtools-mcp');
    const selectedRequirements = await page.evaluate(() => ({
      provider: window.AgentStack.state.selected.has('chrome-devtools-mcp'),
      node: window.AgentStack.state.selected.has('node'),
      issuesHidden: document.getElementById('selectionIssues').hidden,
    }));
    if (!selectedRequirements.provider || !selectedRequirements.node || !selectedRequirements.issuesHidden) throw new Error(`provider requirements were not resolved: ${JSON.stringify(selectedRequirements)}`);
  }
  await page.locator('#componentSearch').fill('git');
  await page.waitForTimeout(150);
  if (await page.evaluate(() => document.activeElement?.id) !== 'componentSearch') throw new Error('tool filtering lost keyboard focus');
  await page.locator('#componentSearch').fill('');
  await page.waitForTimeout(150);
  await page.locator('#createPlanBtn').click();
  await page.waitForFunction(() => document.querySelector('#planState')?.textContent !== 'Not prepared', null, { timeout: 60_000 });
  if (!(await page.locator('#changes').isVisible())) throw new Error('creating changes redirected away from the workspace');
  if (await page.locator('#applyBtn').isEnabled()) throw new Error('Apply enabled before explicit approval');

  const menu = page.locator('.app-menu');
  const menuTrigger = menu.locator('summary');
  await menuTrigger.click();
  const menuButtons = menu.locator('.app-menu-popover button');
  await menuButtons.last().focus();
  await page.keyboard.press('Tab');
  if (!(await menuTrigger.evaluate(element => element === document.activeElement))) throw new Error('Settings did not contain forward focus');
  await page.keyboard.press('Escape');
  if (await menu.evaluate(element => element.open)) throw new Error('Escape did not close Settings');
  if (!(await menuTrigger.evaluate(element => element === document.activeElement))) throw new Error('Escape did not restore Settings focus');

  const basePath = new URL(url).pathname;
  await page.route('**/api/apply', route => route.fulfill({
    status: 202,
    contentType: 'application/json',
    body: JSON.stringify({ operationId: 'test-partial', status: 'running', statusUrl: `${basePath}api/operations/test-partial` })
  }), { times: 1 });
  await page.route('**/api/operations/test-partial', route => route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify({
      operationId: 'test-partial', kind: 'apply', status: 'failed',
      progress: { phase: 'verifying', completed: 2, processed: 2, succeeded: 1, failed: 1, skipped: 0, total: 2, currentId: 'tool-b', currentLabel: 'Tool B', items: [
        { id: 'tool-a', label: 'Tool A', action: 'install', status: 'succeeded' },
        { id: 'tool-b', label: 'Tool B', action: 'install', status: 'failed', message: 'Could not verify this item.' }
      ] },
      result: {
        report: { transaction: { id: 'tx-test', status: 'partial', actions: [
          { componentId: 'tool-a', kind: 'install', verified: true },
          { componentId: 'tool-b', kind: 'install', verified: false, error: 'This item could not be completed.' }
        ] } },
        outcome: {
          phase: 'finished', outcome: 'partially_failed', requested: 2, processed: 2, succeeded: 1, failed: 1, skipped: 0, unchanged: 3, retryable: true,
          summary: 'Some requested changes were applied.',
          detail: '1 requested change succeeded and 1 failed. 3 existing verified items were left unchanged.',
          causes: [{ category: 'verification_failed', summary: 'The change ran, but AgentStack could not verify the result.', recommendedAction: 'Refresh the system and review the detected version before retrying.', count: 1, componentIds: ['tool-b'] }],
          diagnostics: [
            { componentId: 'tool-a', label: 'Tool A', action: 'install', result: 'succeeded', summary: 'Completed and verified.', retryable: false },
            { componentId: 'tool-b', label: 'Tool B', action: 'install', result: 'failed', category: 'verification_failed', method: 'WinGet', exitCode: 1, summary: 'The change ran, but AgentStack could not verify the result.', retryable: true, recommendedAction: 'Refresh the system and review the detected version before retrying.' }
          ]
        }
      },
      failure: { code: 'installation_failed', message: 'Some requested changes were applied.', recovery: '1 requested change succeeded and 1 failed. 3 existing verified items were left unchanged. Retry failed items or review a fresh plan.', retryable: true },
      error: 'Some requested changes were applied.' 
    })
  }), { times: 1 });
  await page.locator('#confirmApply').check();
  await page.locator('#applyBtn').click();
  await page.locator('#activity:not([hidden])').waitFor({ timeout: 10_000 });
  await page.locator('#createFreshPlanBtn:visible').waitFor();
  const failureText = `${await page.locator('#operationNotice').textContent()} ${await page.locator('#technicalDiagnostics').textContent()}`.toLowerCase();
  if (!failureText.includes('existing verified items were left unchanged')) throw new Error('partial failure did not distinguish unchanged verified items');
  if (failureText.includes('successful changes were kept')) throw new Error('partial failure retained misleading successful-change copy');
  if (failureText.includes('appdata') || failureText.includes('c:\\users') || failureText.includes('/home/')) throw new Error('failure recovery leaked a private path');
  if (await page.locator('#installStage').textContent() !== 'Run finished') throw new Error('terminal phase was not rendered');
  if (await page.locator('#installOutcomeBadge').textContent() !== 'Partially completed') throw new Error('terminal outcome was not rendered independently');
  if (!(await page.locator('#progressRegion').isHidden())) throw new Error('terminal result retained a success-like progress track');
  if (await page.locator('#installItems tr.result-row').count() !== 2) throw new Error('structured result rows were not rendered');
  await page.locator('[data-result-filter="succeeded"]').click();
  if (await page.locator('#installItems tr.result-row').count() !== 1) throw new Error('result filters did not isolate succeeded items');
  await page.locator('[data-result-filter="failed"]').click();
  if (await page.locator('#installItems tr.result-row').count() !== 1) throw new Error('result filters did not isolate failed items');
  if (!(await page.locator('#retryFailedBtn').isVisible())) throw new Error('retry failed items was not offered');

  await page.evaluate(() => window.AgentStack.activity.renderOutcome({ outcome: {
    phase: 'finished', outcome: 'cancelled', requested: 2, processed: 2, succeeded: 1, failed: 0, skipped: 1, unchanged: 1, retryable: true,
    summary: 'The run was cancelled after some changes were applied.',
    detail: '1 requested change succeeded and 1 was not completed. 1 existing verified item was left unchanged. Refresh the current state before retrying.',
    diagnostics: [
      { componentId: 'tool-a', label: 'Tool A', action: 'install', result: 'succeeded', summary: 'Completed and verified.', retryable: false },
      { componentId: 'tool-b', label: 'Tool B', action: 'install', result: 'skipped', summary: 'This item was not attempted.', retryable: true, recommendedAction: 'Refresh the system state and review this item in a fresh plan.' }
    ],
    causes: []
  } }));
  if (await page.locator('#installOutcomeBadge').textContent() !== 'Cancelled') throw new Error('cancelled outcome was not rendered independently');
  if (await page.locator('#retryFailedBtn').textContent() !== 'Retry unfinished items') throw new Error('cancelled outcome did not offer unfinished-item recovery');

  for (const width of [390, 768, 1280]) {
    await page.setViewportSize({ width, height: 844 });
    const overflow = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
    if (overflow > 1) throw new Error(`horizontal overflow at ${width}px: ${overflow}px`);
    if (width === 390) {
      const undersized = await page.evaluate(() => [...document.querySelectorAll('button, summary, select')].map(element => {
        const rect = element.getBoundingClientRect();
        return { tag: element.tagName, id: element.id, width: rect.width, height: rect.height };
      }).filter(item => item.width < 44 || item.height < 44));
      if (undersized.length) throw new Error(`undersized mobile targets: ${JSON.stringify(undersized.slice(0, 10))}`);
    }
  }

  await page.setViewportSize({ width: 1440, height: 1000 });
  await page.evaluate(() => window.AgentStack.setTheme('dark'));
  const darkAxe = await page.evaluate(async () => globalThis.axe.run(document, { runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'] } }));
  if (blockingAxe(darkAxe).length) throw new Error(`blocking dark-theme violations:\n${JSON.stringify(blockingAxe(darkAxe), null, 2)}`);
  if (consoleErrors.length) throw new Error(`browser console errors:\n${consoleErrors.join('\n')}`);

  console.log(JSON.stringify({
    axeBlocking: 0,
    plainLifecycleNavigation: 'pass',
    environmentInventory: 'pass',
    inlinePendingChanges: 'pass',
    explicitApproval: 'pass',
    truthfulOutcomeRecovery: 'pass',
    providerDependencyResolution: 'pass',
    privatePathRedaction: 'pass',
    installationTracker: 'pass',
    keyboardFocus: 'pass',
    mobileTargets: 'pass',
    responsiveOverflow: 'pass',
    darkThemeAxe: 'pass',
    consoleErrors: 0
  }, null, 2));
} finally {
  if (browser) await browser.close();
  if (child.exitCode === null) child.kill('SIGTERM');
  if (child.exitCode === null) await new Promise(resolve => child.once('exit', resolve));
  await rm(isolatedHome, { recursive: true, force: true });
}
