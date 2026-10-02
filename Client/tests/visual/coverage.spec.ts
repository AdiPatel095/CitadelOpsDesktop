import { expect, test } from '@playwright/test';
import { cases } from './cases';
import enforcement from '../gate/enforcement.json' with { type: 'json' };
import { areaFor } from '../../scripts/visual/gate-report.mjs';
import { gateCases } from '../gate/views';
import { openCase } from '../gate/harness';
import { scanColours, assertOnePrimaryPerRegion, reportAccentUsage } from './rules';

import { isDesktop as desktop } from '../gate/adapter';
const oldNames = new Set(cases.map(entry => entry.name));
const newViews = gateCases.filter(entry => entry.view && !oldNames.has(entry.name) && !entry.settings && !entry.toast);
const newSettings = desktop ? [] : gateCases.filter(entry => ['Auto Bird', 'Auto Storm'].includes(entry.settings ?? ''));
const compactHidden = new Set(['battle-stats', 'alliance-targets', 'rift']);
const entries = [...gateCases.filter(entry => oldNames.has(entry.name)), ...newViews, ...newSettings];

for (const entry of entries) {
  for (const theme of ['dark', 'light'] as const) {
    test(`coverage ${entry.name} en ${theme}`, async ({ page }, testInfo) => {
      const width = Number(testInfo.project.name);
      const old = oldNames.has(entry.name);
      test.skip(old ? width !== 768 : desktop ? width !== 1440 : ![390, 1440].includes(width) || (width === 390 && compactHidden.has(entry.view ?? '')));
      const { verifyNetwork, unavailable } = await openCase(page, entry, { theme, locale: 'en' });
      expect(unavailable).toEqual([]);
      await page.waitForLoadState('networkidle');
      await page.evaluate(() => document.fonts.ready);
      await page.waitForTimeout(2000);
      // PR-1 reports new-case violations; existing CIT-64/CIT-65 assertions stay enforced.
      const violations = (await scanColours(page, true)).offenders.map(item => ({ rule: 'R3', element: `${item.element}${item.pseudo}`, detail: `${item.property}: ${item.colour}` }));
      try { await assertOnePrimaryPerRegion(page); } catch (error) { violations.push({ rule: 'R11', element: 'page', detail: String(error) }); }
      await testInfo.attach('snapshot-rule-findings', { body: JSON.stringify({ view: entry.name, width, theme, locale: 'en', violations }), contentType: 'application/json' });
      if (enforcement.areas.includes(areaFor(entry.name).slice(0, 1))) expect(violations).toEqual([]);
      await reportAccentUsage(page, entry.name);
      if (entry.name === 'movement') {
        const fonts = await page.locator('.font-mono').evaluateAll(elements => elements.map(element => {
          const style = getComputedStyle(element);
          return { text: element.textContent, family: style.fontFamily, size: style.fontSize, weight: style.fontWeight,
            mono: style.getPropertyValue('--font-mono'), compatibilityMono: style.getPropertyValue('--compat-font-mono') };
        }));
        await testInfo.attach('commander-fonts', { body: JSON.stringify(fonts), contentType: 'application/json' });
      }
      await expect(page).toHaveScreenshot(`${entry.name}-${width}-${theme}.png`, { fullPage: false, animations: 'disabled', caret: 'hide' });
      verifyNetwork();
    });
  }
}

if (!desktop) {
  for (const name of ['landing', 'account-center', 'castle', 'automation']) {
    const entry = gateCases.find(entry => entry.name === name)!;
    test(`coverage ${name} ar dark`, async ({ page }, testInfo) => {
      test.skip(!['390', '1440'].includes(testInfo.project.name));
      const { verifyNetwork, unavailable } = await openCase(page, entry, { theme: 'dark', locale: 'ar' });
      expect(unavailable).toEqual([]);
      await page.waitForLoadState('networkidle');
      await page.evaluate(() => document.fonts.ready);
      await page.waitForTimeout(2000);
      await expect(page).toHaveScreenshot(`${name}-${testInfo.project.name}-ar-dark.png`, { fullPage: false, animations: 'disabled', caret: 'hide' });
      verifyNetwork();
    });
  }
}
