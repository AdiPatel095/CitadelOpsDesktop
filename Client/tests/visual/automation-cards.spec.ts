import { expect, test } from '@playwright/test';
import { assertDisabledNeutral } from './rules';
import { openView, prepare, settle } from './harness';

for (const theme of ['dark', 'light'] as const) {
  test(`CIT-72 card layout and keyboard ${theme}`, async ({ page }) => {
    const verify = await prepare(page, theme);
    await openView(page, 'Automation', 'automation');
    await settle(page);

    const cards = page.locator('.automation-function-row');
    expect(await cards.count()).toBeGreaterThan(3);
    for (const card of await cards.all()) {
      const name = await card.locator('h3').innerText();
      const timer = card.getByRole('button', { name: `Run ${name} for a set time`, exact: true });
      await expect(timer).toHaveCount(1);
      const gear = card.getByRole('button', { name: `Open ${name} settings`, exact: true });
      await timer.scrollIntoViewIfNeeded();
      await expect(timer).toBeVisible();
      expect(await timer.evaluate((button) => button.nextElementSibling?.classList.contains('automation-function-settings'))).toBe(true);
      if (page.viewportSize()!.width === 390) {
        for (const control of [timer, gear]) {
          const box = await control.boundingBox();
          expect(box!.width).toBeGreaterThanOrEqual(44);
          expect(box!.height).toBeGreaterThanOrEqual(44);
        }
      }
    }
    const rows = await cards.evaluateAll(elements => elements.map(element => {
      const box = element.getBoundingClientRect(); return { top: box.top, height: box.height };
    }));
    for (const row of rows) for (const peer of rows.filter(peer => Math.abs(peer.top - row.top) < 1)) {
      expect(Math.abs(peer.height - row.height)).toBeLessThanOrEqual(1);
    }
    for (const section of await page.locator('.automation-function-group').all()) {
      await expect(section).not.toContainText('Right-click');
    }
    const timer = page.getByRole('button', { name: 'Run Auto Towers for a set time', exact: true });
    await page.getByRole('switch', { name: 'Toggle Auto Towers', exact: true }).focus();
    await page.keyboard.press('Tab');
    await expect(timer).toBeFocused();
    await page.keyboard.press('Enter');
    await expect(page.getByRole('dialog')).toBeVisible();
    await page.keyboard.press('Escape');
    await expect(page.getByRole('dialog')).toHaveCount(0);
    await expect(timer).toBeFocused();
    // Both idle and active controls must use Button's neutral disabled treatment.
    await timer.evaluate(element => { (element as HTMLButtonElement).disabled = true; });
    await assertDisabledNeutral(page);
    await timer.evaluate(element => { (element as HTMLButtonElement).disabled = false; });
    const tip = page.locator('[data-timer-tip]');
    await expect(tip).toHaveCount(1);
    await tip.getByRole('button', { name: 'Close', exact: true }).click();
    await expect(tip).toHaveCount(0);
    expect(await page.evaluate(() => document.activeElement?.closest('[data-view="automation"]') !== null)).toBe(true);
    await page.reload();
    await openView(page, 'Automation', 'automation');

    await expect(tip).toHaveCount(0);
    verify();
  });
}
for (const operation of ['getItem', 'setItem'] as const) {
  test(`CIT-72 tip survives throwing storage ${operation}`, async ({ page }) => {
    await page.addInitScript(operation => {
      const original = Storage.prototype[operation];
      Object.defineProperty(Storage.prototype, operation, { value: function(key: string, ...args: string[]) {
        if (key === 'citadelops.automation.timerTipDismissed') throw new Error('Storage unavailable');
        return original.apply(this, [key, ...args] as [string, string]);
      } });
    }, operation);
    const theme = 'dark';
    const verify = await prepare(page, theme);
    await openView(page, 'Automation', 'automation');
    await settle(page);

    const tip = page.locator('[data-timer-tip]');
    await expect(tip).toHaveCount(1);
    await tip.getByRole('button', { name: 'Close', exact: true }).click();
    await expect(tip).toHaveCount(0);
    // Remount the view in the same session; failed persistence must not bring the tip back.
    await openView(page, 'Castle', 'castle');
    await openView(page, 'Automation', 'automation');
    await expect(tip).toHaveCount(0);
    verify();
  });
}
