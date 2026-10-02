import { annotateSystemSources } from './systemSources';
import { areaFor } from '../../scripts/visual/gate-report.mjs';
import AxeBuilder from '@axe-core/playwright';
import { test } from '@playwright/test';
import { openCase, writeReport } from './harness';
import { gateCases } from './views';

for (const locale of ['en', 'ar'] as const) {
  for (const theme of ['dark', 'light'] as const) {
    for (const entry of gateCases) {
      test(`axe ${entry.name} ${locale} ${theme}`, async ({ page }, testInfo) => {
        test.skip(!['390', '1440'].includes(testInfo.project.name), 'axe coverage uses 390 and 1440');
        const options = { locale, theme };
        const { verifyNetwork, unavailable } = await openCase(page, entry, options);
        const violations = [...unavailable];
        if (!unavailable.length) {
          await page.waitForLoadState('networkidle');
          await page.evaluate(() => document.fonts.ready);
          const result = await new AxeBuilder({ page }).withTags(['wcag2a', 'wcag2aa', 'wcag21aa', 'wcag22aa']).analyze();
          await testInfo.attach('axe-results', { body: JSON.stringify(result), contentType: 'application/json' });
          for (const finding of result.violations.filter(item => item.impact === 'serious' || item.impact === 'critical')) {
            for (const node of finding.nodes) {
              const target = node.target.join(' > ');
              // React IDs vary between cases; identify only the assigned global selector.
              const castleSelector = finding.id === 'button-name' && await page.locator(target).evaluate(element => element.matches('.castle-focus-shell .m3-select-trigger'));
              violations.push({ rule: `axe:${finding.id}`, element: castleSelector ? '.castle-focus-shell .m3-select-trigger' : target, detail: `${finding.impact}: ${node.failureSummary ?? finding.description}` });
            }
          }
        }
        verifyNetwork();
        await writeReport(testInfo, 'a11y', entry, areaFor(entry.name).startsWith('d') ? await annotateSystemSources(page, violations) : violations, options);
      });
    }
  }
}
