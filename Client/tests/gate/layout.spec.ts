import { annotateSystemSources } from './systemSources';
import { areaFor } from '../../scripts/visual/gate-report.mjs';
import { designRules } from './designScan';
import { test } from '@playwright/test';
import { clippedControls, layoutShiftAfterReady, pageOverflow, smallTargets } from './checks';
import { openCase, writeReport } from './harness';
import { gateCases } from './views';

for (const locale of ['en', 'ar'] as const) {
for (const entry of gateCases) {
  test(`layout ${entry.name} ${locale} dark`, async ({ page }, testInfo) => {
    const { verifyNetwork, unavailable } = await openCase(page, entry, { theme: 'dark', locale });
    const violations = [...unavailable];
    if (!unavailable.length) {
      // openCase resolves at the ready marker; observe before waiting for fonts/data.
      violations.push(...await layoutShiftAfterReady(page));
      await page.waitForLoadState('networkidle');
      violations.push(...await pageOverflow(page), ...await clippedControls(page));
      violations.push(...await designRules(page, locale));
      if (Number(testInfo.project.name) <= 768) violations.push(...await smallTargets(page));
    }
    verifyNetwork();
    await writeReport(testInfo, 'layout', entry, areaFor(entry.name).startsWith('d') ? await annotateSystemSources(page, violations) : violations, { theme: 'dark', locale });
  });
}

}
