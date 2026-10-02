import { test } from '@playwright/test';
import { clippedControls, layoutShiftAfterReady, pageOverflow, smallTargets } from './checks';
import { openCase, writeReport } from './harness';
import { gateCases } from './views';

for (const entry of gateCases) {
  test(`layout ${entry.name} dark`, async ({ page }, testInfo) => {
    const { verifyNetwork, unavailable } = await openCase(page, entry);
    const violations = [...unavailable];
    if (!unavailable.length) {
      // openCase resolves at the ready marker; observe before waiting for fonts/data.
      violations.push(...await layoutShiftAfterReady(page));
      await page.waitForLoadState('networkidle');
      violations.push(...await pageOverflow(page), ...await clippedControls(page));
      if (Number(testInfo.project.name) <= 768) violations.push(...await smallTargets(page));
    }
    verifyNetwork();
    await writeReport(testInfo, 'layout', entry, violations);
  });
}
