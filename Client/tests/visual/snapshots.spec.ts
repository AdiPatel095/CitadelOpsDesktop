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

const prepareStock = async (page: import('@playwright/test').Page, theme: 'dark' | 'light', state: string) => prepare(page, theme, `stock-wording-${state}`);
const openStockSettings = async (page: import('@playwright/test').Page, feature: string) => {
  await openView(page, 'Automation', 'automation');
  await openSettings(page, feature === 'Auto Nomad' ? 'Auto Nomad / Samurai' : feature);
};
const settleStock = settle;

// CIT-138: capture the stock block inside real settings, with synthetic fixture data.
for (const feature of ['Auto Bird', 'Auto Station']) {
  for (const stockState of ['short', 'covered', 'unknown', 'unsynced'] as const) {
    for (const theme of ['dark', 'light'] as const) {
      test(`stock ${feature} ${stockState} ${theme}`, async ({ page }, testInfo) => {
        const verifyNetwork = await prepareStock(page, theme, stockState);
        await openStockSettings(page, feature);
        const dialog = page.getByRole('dialog').locator(feature === 'Auto Bird' ? '#auto-bird-castles' : '#auto-station-castles');
        const text = stockState === 'covered' ? 'Troops in the castle cover every reserve.'
          : stockState === 'unsynced' ? 'Waiting for the game connection to finish its first sync before troop counts are used.'
          : stockState === 'unknown' ? 'Unknown unit' : 'Keep 100,000 · 70,000 in castle';
        const target = stockState === 'unknown' ? dialog.getByLabel(text, { exact: true }).first() : dialog.getByText(text).first();
        await target.locator('xpath=ancestor::div[contains(@class, "border-t")][1]').scrollIntoViewIfNeeded();
        await expect(target).toBeInViewport();
        if (stockState === 'short') {
          const note = dialog.getByText('1 reserve is above current stock; nothing of that unit is sent until stock exceeds the reserve.').first();
          await expect(note).toBeInViewport();
          await expect(dialog.getByLabel('Below the amount', { exact: true }).first()).toBeVisible();
        }
        if (stockState === 'covered') await expect(dialog.getByLabel('Enough in castle', { exact: true }).first()).toBeVisible();
        await settleStock(page);
        await expect(page).toHaveScreenshot(`stock-${feature.toLowerCase().replaceAll(' ', '-')}-${stockState}-${testInfo.project.name}-${theme}.png`, { fullPage: false, animations: 'disabled', caret: 'hide' });
        verifyNetwork();
      });
    }
  }
}

for (const feature of ['Auto Towers', 'Auto Nomad']) for (const theme of ['dark', 'light'] as const) {
  test(`stock ${feature} required ${theme}`, async ({ page }, testInfo) => {
    const verifyNetwork = await prepareStock(page, theme, 'short');
    await openStockSettings(page, feature);
    const target = page.getByRole('dialog').getByText(/needed · 70,000 in castle/).first();
    await target.scrollIntoViewIfNeeded();
    await expect(target).toBeInViewport();
    await settleStock(page);
    await expect(page).toHaveScreenshot(`stock-${feature.toLowerCase().replaceAll(' ', '-')}-required-${testInfo.project.name}-${theme}.png`, { fullPage: false, animations: 'disabled', caret: 'hide' });
    verifyNetwork();
  });
}
