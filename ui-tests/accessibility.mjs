import { spawn } from 'node:child_process';
import { mkdtemp, rm } from 'node:fs/promises';
import os from 'node:os';
import path from 'node:path';
import process from 'node:process';
import { chromium } from 'playwright';

const binary = process.env.AGENTSTACK_UI_BINARY || path.resolve('..', process.platform === 'win32' ? 'agentstack.exe' : 'agentstack');
const isolatedHome = await mkdtemp(path.join(os.tmpdir(), 'agentstack-a11y-'));
const child = spawn(binary, ['ui', '--no-open', '--listen', '127.0.0.1:0'], {
  cwd: path.dirname(binary),
  env: {
    ...process.env,
    HOME: isolatedHome,
    USERPROFILE: isolatedHome,
    LOCALAPPDATA: path.join(isolatedHome, 'AppData', 'Local')
  },
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
      if (match) {
        clearTimeout(timer);
        resolve(match[1]);
      }
    });
    child.once('exit', code => {
      clearTimeout(timer);
      reject(new Error(`manager exited before startup: code=${code}; stderr=${stderr}`));
    });
  });
}

let browser;
try {
  const url = await managerUrl();
  browser = await chromium.launch({ headless: true });
  const context = await browser.newContext({ reducedMotion: 'reduce' });
  const page = await context.newPage();
  await page.addInitScript({ path: path.resolve('node_modules', 'axe-core', 'axe.min.js') });
  await page.goto(url, { waitUntil: 'domcontentloaded', timeout: 60_000 });
  await page.waitForFunction(() => document.querySelectorAll('#profileSelect option').length > 0, null, { timeout: 60_000 });

  const axe = await page.evaluate(async () => globalThis.axe.run(document, {
    runOnly: { type: 'tag', values: ['wcag2a', 'wcag2aa', 'wcag21a', 'wcag21aa', 'wcag22aa'] }
  }));
  const blocking = axe.violations.filter(item => item.impact === 'critical' || item.impact === 'serious');
  if (blocking.length) {
    throw new Error(`blocking accessibility violations:\n${JSON.stringify(blocking, null, 2)}`);
  }

  await page.locator('body').press('Tab');
  const firstFocus = await page.evaluate(() => document.activeElement?.className || '');
  if (!String(firstFocus).includes('skip-link')) {
    throw new Error(`first keyboard focus is not the skip link: ${firstFocus}`);
  }
  await page.keyboard.press('Enter');
  if (await page.evaluate(() => document.activeElement?.id) !== 'overview-title') {
    throw new Error('skip link did not move focus to the main overview heading');
  }

  await page.route('**/api/inventory', async route => {
    await new Promise(resolve => setTimeout(resolve, 300));
    await route.continue();
  }, { times: 1 });
  const refreshButton = page.locator('#refreshBtn');
  await refreshButton.focus();
  await refreshButton.click();
  await page.locator('#mainContent[aria-busy="true"]').waitFor();
  await page.locator('#refreshBtn[aria-busy="true"]:disabled').waitFor();
  await page.locator('#operationStatus[data-state="running"]').waitFor();
  await page.locator('#operationStatus[data-state="success"]').waitFor();
  await page.waitForFunction(() => !document.querySelector('#mainContent')?.hasAttribute('aria-busy'));
  if (await page.evaluate(() => document.activeElement?.id) !== 'refreshBtn') {
    throw new Error('operation completion did not restore focus to the initiating control');
  }

  const componentsNav = page.locator('[data-section="components"]');
  await componentsNav.focus();
  await page.keyboard.press('Enter');
  await page.locator('#components:not([hidden])').waitFor();
  if (await page.evaluate(() => document.activeElement?.id) !== 'components-title') {
    throw new Error('keyboard navigation did not move focus to the selected section heading');
  }

  const reducedMotion = await page.locator('.button').first().evaluate(element => {
    const style = getComputedStyle(element);
    return { duration: style.transitionDuration, animation: style.animationDuration };
  });
  const nonZero = value => value.split(',').some(part => parseFloat(part) > 0);
  if (nonZero(reducedMotion.duration) || nonZero(reducedMotion.animation)) {
    throw new Error(`reduced-motion styles still animate: ${JSON.stringify(reducedMotion)}`);
  }

  const token = await page.locator('meta[name="agentstack-token"]').getAttribute('content');
  const base = await page.locator('meta[name="agentstack-base"]').getAttribute('content');
  const response = await context.request.post(new URL(`${base}api/shutdown`, url).href, {
    headers: { 'X-AgentStack-Token': token, 'Content-Type': 'application/json' },
    data: {}
  });
  if (!response.ok()) {
    throw new Error(`authenticated manager shutdown failed: ${response.status()} ${await response.text()}`);
  }

  console.log(JSON.stringify({
    axeViolations: axe.violations.length,
    blockingViolations: blocking.length,
    keyboardNavigation: 'pass',
    operationFeedback: 'pass',
    reducedMotion: 'pass',
    shutdown: 'pass'
  }, null, 2));
} finally {
  if (browser) await browser.close();
  if (child.exitCode === null) child.kill('SIGTERM');
  if (child.exitCode === null) {
    await new Promise(resolve => child.once('exit', resolve));
  }
  await rm(isolatedHome, { recursive: true, force: true });
}
