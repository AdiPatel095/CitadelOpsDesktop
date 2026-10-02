import { test, expect } from '@playwright/test';
import { prepare, settle } from './harness';
import { assertDisabledNeutral, reportAccentUsage } from './rules';
for (const theme of ['dark','light'] as const) test(`CIT-69 header layout and disclosure ${theme}`, async ({ page }) => {
  const verifyNetwork = await prepare(page, theme);

  await settle(page);
  const header = page.locator('.liquid-header'); const width = page.viewportSize()!.width;
  await expect(header).toHaveCSS('height', width < 768 ? '64px' : '72px');
  const geometry = await header.evaluate(element => {
    const box = element.getBoundingClientRect();
    const children = [...element.querySelector('.liquid-header-inner')!.children].filter(child => child.getBoundingClientRect().width > 0);
    return { outside: children.filter(child => { const b = child.getBoundingClientRect(); return b.left < box.left - 1 || b.right > box.right + 1 || b.top < box.top - 1 || b.bottom > box.bottom + 1; }).map(child => child.className), overflow: document.documentElement.scrollWidth > innerWidth };
  });
  expect(geometry).toEqual({ outside: [], overflow: false });
  for (const selector of ['.castle-focus-shell .truncate','.workspace-account-select:visible .truncate','.header-status-word']) {
    for (const item of await page.locator(selector).all()) expect(await item.evaluate(element => element.scrollWidth <= element.clientWidth + 1), `Untruncated ${selector}`).toBe(true);
  }
  const cluster = page.locator('.header-status-cluster');
  await expect(cluster.locator('button, [tabindex]')).toHaveCount(0);
  await expect(page.locator('.header-status-attacks')).toBeVisible({ visible: width >= 1280 });
  await cluster.focus(); await cluster.press('Enter');
  const panel = page.locator('.cit-popover-layer:visible');
  await expect(panel).toHaveAttribute('data-popover-mode', width < 768 ? 'sheet' : 'anchored');
  await expect(panel.getByRole('heading', { name: 'Attacks today', exact: true })).toBeVisible();
  await assertDisabledNeutral(page); await reportAccentUsage(page, 'header-panel');
  for (const button of await page.locator('.liquid-header button:visible, .cit-popover-layer:visible button:visible').all()) {
    const name = await button.getAttribute('aria-label') || await button.innerText(); expect(name.trim()).not.toBe('');
    if (width < 768) { const box = await button.boundingBox(); expect(box!.width, `${name} width`).toBeGreaterThanOrEqual(44); expect(box!.height, `${name} height`).toBeGreaterThanOrEqual(44); }
  }
  await page.keyboard.press('Escape'); await expect(panel).toBeHidden(); await expect(cluster).toBeFocused();
  await cluster.click();
  if (width < 768) await page.locator('.cit-popover-scrim').click({ position: { x: 2, y: 2 } });
  else await page.locator('[data-view]').first().click({ position: { x: 2, y: 2 } });
  await expect(panel).toBeHidden(); await expect(cluster).toBeFocused();
  verifyNetwork();
});
