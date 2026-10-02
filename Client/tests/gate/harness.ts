import { mkdir, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';
import { expect, type Page, type TestInfo } from '@playwright/test';
import visual from '../../playwright.config';
import { prepare } from '../visual/harness';
import type { GateCase } from './views';
import type { Violation } from './checks';

const desktop = Boolean(visual.use?.baseURL);

export async function openCase(page: Page, entry: GateCase) {
  const verifyNetwork = await prepare(page, 'dark');
  if (!desktop) {
    const port = (entry.variant === 'public' ? 18461 : 18462) + Number(process.env.CIT_VISUAL_PORT_OFFSET ?? 0);
    await page.goto(`http://127.0.0.1:${port}${entry.path}`);
  }
  if (entry.label) {
    await expect(page.locator('[data-view="castle"]')).toBeVisible();
    const navigation = page.locator('#workspace-navigation');
    if ((page.viewportSize()?.width ?? 0) < 760) {
      await page.getByRole('button', { name: 'Open workspace navigation', exact: true }).click();
    }
    if (['Settings', 'Patch Notes', 'Support'].includes(entry.label)) {
      await page.locator('.liquid-sidebar-system-island').hover();
    }
    const button = navigation.getByRole('button', { name: entry.label, exact: true, includeHidden: true });
    if (!(await button.count()) || !(await button.isVisible())) {
      verifyNetwork();
      return { verifyNetwork, unavailable: [{ rule: 'coverage', element: entry.label, detail: 'View is unavailable in the current deployment/layout' }] as Violation[] };
    }
    await button.click();
  }
  if (entry.settings) {
    await page.getByRole('button', { name: `Open ${entry.settings} settings`, exact: true }).click();
  }
  await expect(page.locator(entry.ready).first()).toBeVisible();
  return { verifyNetwork, unavailable: [] as Violation[] };
}

export async function writeReport(testInfo: TestInfo, suite: string, entry: GateCase, violations: Violation[]) {
  const width = Number(testInfo.project.name);
  const report = { schemaVersion: 1, suite, view: entry.name, width, theme: 'dark', locale: 'en', violations };
  const directory = resolve('test-results/gate');
  await mkdir(directory, { recursive: true });
  const body = `${JSON.stringify(report, null, 2)}\n`;
  await writeFile(resolve(directory, `${suite}-${entry.name}-${width}.json`), body);
  await testInfo.attach('gate-findings', { body, contentType: 'application/json' });
}
