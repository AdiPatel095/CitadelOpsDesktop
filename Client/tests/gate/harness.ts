import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { expect, type Page, type TestInfo } from '@playwright/test';
import visual from '../../playwright.config';
import { prepareCase, AUTOMATION_FEATURE_NAMES } from './adapter';
import type { GateCase } from './views';
import type { Violation } from './checks';
import enforcement from './enforcement.json' with { type: 'json' };
import { areaFor } from '../../scripts/visual/gate-report.mjs';

export type GateOptions = { theme: 'dark' | 'light'; locale: 'en' | 'ar' };
export const defaultOptions: GateOptions = { theme: 'dark', locale: 'en' };
const desktop = Boolean(visual.use?.baseURL);

export async function openCase(page: Page, entry: GateCase, options: GateOptions = defaultOptions) {
  const { locale } = options;
  const verifyNetwork = await prepareCase(page, entry, options);
  if (!desktop) {
    await page.addInitScript(locale => localStorage.setItem('citadelops.viewer-locale', locale), locale);
    const port = (entry.variant === 'public' ? 18461 : 18462) + Number(process.env.CIT_VISUAL_PORT_OFFSET ?? 0);
    await page.goto(`http://127.0.0.1:${port}${entry.path}`);
  }
  await expect(page.locator('html')).toHaveAttribute('lang', locale);
  const unavailable: Violation[] = [];
  if (entry.view) {
    await expect(page.locator('[data-view="castle"]')).toBeVisible();
    // Use the existing app navigation event: names are translated, and compact
    // navigation intentionally omits three views. Gate their rendered layout too.
    await page.evaluate(view => window.dispatchEvent(new CustomEvent('citadelops:open-view', { detail: view })), entry.view);
    await expect(page.locator(`[data-view="${entry.view}"]`)).toBeVisible();
  }
  if (entry.name === 'header-panel') await page.locator('.header-status-cluster').click();
  if (entry.name === 'avatar-menu') await page.locator('.profile-trigger').click();
  if (entry.settings) {
    const feature = Object.entries(AUTOMATION_FEATURE_NAMES).find(([, name]) => name === entry.settings)?.[0];
    const opener = page.locator(`.automation-function-row:has(#automation-switch-${feature}) .automation-function-settings`);
    await expect(opener).toBeVisible();
    if (!await opener.count()) {
      unavailable.push({ rule: 'coverage', element: entry.settings, detail: 'Settings opener is unavailable in this deployment' });
    } else {
      await opener.evaluate(element => element.setAttribute('data-gate-opener', 'true'));
      await opener.focus();
      await opener.click();
    }
  }
  if (entry.dialog) {
    const card = page.locator('.hosted-account-card').first();
    await card.waitFor();
    const selectors = {
      add: '.account-center-actions md-filled-button',
      login: '.hosted-account-card__login button',
      access: '.hosted-account-card__access button',
      delete: '.hosted-account-card button:has(svg.lucide-trash-2)',
    };
    const opener = entry.dialog === 'login' ? card.locator('.hosted-account-card__login button') : page.locator(selectors[entry.dialog]).first();
    if (!await opener.count()) unavailable.push({ rule: 'coverage', element: entry.dialog, detail: 'Account dialog opener is unavailable in the sealed fixture' });
    else {
      await opener.evaluate(element => element.setAttribute('data-gate-opener', 'true'));
      await opener.focus();
      await opener.click();
    }
  }
  if (entry.toast === 'failure') {
    const control = page.locator('#automation-switch-autoTowers').getByRole('switch');
    await control.waitFor();
    // Failure fixture rejects a start, so turn an already enabled fixture off first.
    if (await control.getAttribute('aria-checked') === 'true') {
      await control.click();
      await expect(control).toHaveAttribute('aria-checked', 'false');
    }
    await control.click();
    await expect.poll(async () => (await page.locator('[role="alert"]').count()) + (await page.getByRole('dialog').count())).toBeGreaterThan(0);
    if (await page.getByRole('dialog').count()) await page.getByRole('dialog').locator('[data-variant="secondary"]').click();
  }
  if (!unavailable.length) await expect(page.locator(entry.ready).first()).toBeVisible();
  return { verifyNetwork, unavailable };
}

export async function writeReport(testInfo: TestInfo, suite: string, entry: GateCase, violations: Violation[], options: GateOptions = defaultOptions) {
  const width = Number(testInfo.project.name);
  const report = { schemaVersion: 1, suite, view: entry.name, width, ...options, violations };
  const directory = resolve('test-results/gate');
  await mkdir(directory, { recursive: true });
  const body = `${JSON.stringify(report, null, 2)}\n`;
  await writeFile(resolve(directory, `${suite}-${entry.name}-${width}-${options.locale}-${options.theme}.json`), body);
  await testInfo.attach('gate-findings', { body, contentType: 'application/json' });
  if (enforcement.areas.includes(areaFor(entry.name).slice(0, 1))) expect(violations, `${entry.name}: enforced gate rules`).toEqual([]);
}
