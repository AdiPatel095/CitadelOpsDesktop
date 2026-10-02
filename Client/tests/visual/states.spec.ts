import { test, expect } from '@playwright/test';
import { cases } from './cases';
import { prepare, openView, settle } from './harness';

for (const visualCase of cases.filter(item => item.states)) for (const theme of ['dark', 'light'] as const) {
  test(`single view state ${visualCase.name} ${theme}`, async ({ page }) => {
    const seal = await prepare(page, theme, visualCase.states);
    await openView(page, visualCase.label, visualCase.view);
    await settle(page);
    const state = Object.values(visualCase.states!)[0];
    const region = page.locator(`.ui-view-state[data-state="${state}"]`).first();
    await expect(region).toBeVisible();
    for (const view of await page.locator('.ui-view-state:not([data-state="content"])').all()) {
      await expect(view.locator(':scope > .ui-empty-state, :scope > .ui-loading-state')).toHaveCount(1);
    }
    expect(await page.locator('.ui-view-state[data-state="error"]').count() > 0 && await page.locator('.ui-view-state[data-state="empty"]').count() > 0).toBe(false);
    if (state === 'error') {
      await expect(region.getByRole('button', { name: 'Retry', exact: true })).toBeEnabled();
      await region.getByRole('button', { name: 'Retry', exact: true }).click();
      await expect(region).toHaveAttribute('data-state', 'error');
    }
    if (state === 'loading') await expect(region.getByRole('status')).toHaveAttribute('aria-busy', 'true');
    seal();
  });
}
