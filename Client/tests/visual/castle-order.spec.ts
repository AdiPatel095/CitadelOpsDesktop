import { expect, test } from '@playwright/test';
import { prepare } from './harness';

for (const theme of ['dark', 'light'] as const) {
  test(`CIT-71 castle order, disclosure and navigation ${theme}`, async ({ page }) => {
    const crashes: string[] = [];
    page.on('pageerror', error => crashes.push(error.message));
    const verifyNetwork = await prepare(page, theme);
    const overview = page.locator('[data-castle-overview]');
    const decorations = page.locator('[data-castle-decorations]');
    const disclosure = decorations.getByRole('button', { name: 'Decorations', exact: true });
    await expect(overview).toBeVisible();
    expect(await overview.evaluate(element => element.getBoundingClientRect().top)).toBeLessThan(await page.evaluate(() => innerHeight));
    for (const selector of ['[data-castle-queues]', '[data-castle-troops]', '[data-castle-decorations]']) {
      expect(await overview.evaluate((element, selector) => Boolean(element.compareDocumentPosition(document.querySelector(selector)!) & Node.DOCUMENT_POSITION_FOLLOWING), selector)).toBe(true);
    }
    await expect(disclosure).toHaveAttribute('aria-expanded', 'false');
    await disclosure.focus();
    await disclosure.press('Enter');
    await expect(disclosure).toHaveAttribute('aria-expanded', 'true');
    expect(await page.evaluate(() => localStorage.getItem('citadelops.castle.decorations.expanded.v1'))).toBe('true');
    await page.reload();
    await expect(page.locator('[data-castle-decorations]').getByRole('button', { name: 'Decorations', exact: true })).toHaveAttribute('aria-expanded', 'true');
    await page.getByRole('button', { name: 'See all on Automation', exact: true }).click();
    await expect(page.locator('[data-view="automation"]')).toBeVisible();
    expect(crashes).toEqual([]);
    verifyNetwork();
  });
}
