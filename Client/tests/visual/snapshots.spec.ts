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

// CIT-66: verify the full reason is reachable without relying on color or hover.
test('CIT-66 connection status disclosure supports pointer and keyboard', async ({ page }) => {
  const verifyNetwork = await prepare(page, 'dark');
  await settle(page);
  const panel = page.locator('.player-connection-panel:visible').first();
  const summary = panel.locator('summary');
  const full = await summary.locator('[role="status"]').getAttribute('aria-label');
  await summary.click();
  await expect(panel.locator('.player-connection-details')).toBeVisible();
  await expect(panel.locator('.player-connection-details [role="status"]')).toHaveAttribute('aria-label', full!);
  await summary.focus();
  await summary.press('Enter');
  await expect(panel.locator('.player-connection-details')).toBeHidden();
  await summary.press('Enter');
  await expect(panel.locator('.player-connection-details')).toBeVisible();
  verifyNetwork();
});
