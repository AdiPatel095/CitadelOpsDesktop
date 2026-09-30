import { expect, test } from '@playwright/test';
import { cases, themes } from './cases';
import { openSettings, openView, prepare, settle } from './harness';

for (const entry of cases) {
  for (const theme of themes) {
    test(`${entry.name}-${theme}`, async ({ page }, testInfo) => {
      const verifyNetwork = await prepare(page, theme);
      await openView(page, entry.label, entry.view);
      if ('settings' in entry) await openSettings(page);
      await settle(page);
      verifyNetwork();
      await expect(page).toHaveScreenshot(`${entry.name}-${testInfo.project.name}-${theme}.png`, {
        fullPage: false, animations: 'disabled', caret: 'hide',
      });
      verifyNetwork();
    });
  }
}
