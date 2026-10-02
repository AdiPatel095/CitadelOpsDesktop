import { test, expect } from '@playwright/test';
import { prepare, openView, settle } from './harness';

for (const theme of ['dark', 'light'] as const) for (const locale of ['en', 'de', 'ar']) {
  test(`Feature Stats tabs ${theme} ${locale}`, async ({ page }) => {
    await page.addInitScript(() => localStorage.setItem('citadelops.viewer-locale', 'en'));
    const seal = await prepare(page, theme);
    await openView(page, 'Feature Stats', 'events'); await settle(page);
    await page.evaluate(locale => { localStorage.setItem('citadelops.viewer-locale', locale); window.dispatchEvent(new CustomEvent('citadelops:viewer-locale', { detail: locale })); }, locale);
    await expect.poll(() => page.locator('.ui-tabs').evaluate(element => getComputedStyle(element).direction)).toBe(locale === 'ar' ? 'rtl' : 'ltr');
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    const list = page.locator('.ui-tabs__scroller');
    if (page.viewportSize()!.width < 768) {
      await expect(page.getByRole('tablist')).toHaveCount(0);
      await expect(page.locator('.ui-tabs--select [role="combobox"]')).toBeVisible();
    } else {
      await expect(list).toBeVisible();
      const checkEdges = async () => {
        let anyOverflow = false;
        for (const side of ['left', 'right']) {
          const overflowing = await list.evaluate((element, side) => {
            const rect = element.getBoundingClientRect();
            return [...element.querySelectorAll('[role="tab"]')].some(tab => side === 'left' ? tab.getBoundingClientRect().left < rect.left - 1 : tab.getBoundingClientRect().right > rect.right + 1);
          }, side);
          anyOverflow ||= overflowing;
          await expect(list).toHaveAttribute(`data-fade-${side}`, String(overflowing));
          await expect(page.locator(`.ui-tabs__arrow--${side}`)).toHaveCount(overflowing ? 1 : 0);
          if (overflowing) {
            const arrow = page.locator(`.ui-tabs__arrow--${side}`);
            await expect.poll(() => arrow.evaluate((button, side) => {
              const rect = button.getBoundingClientRect();
              const parentElement = button.parentElement;
              if (!parentElement) return false; // Reacquire after a resize/edge re-render.
              const parent = parentElement.getBoundingClientRect();
              return Math.abs(rect.top - parent.top) <= 1 && (side === 'left' ? Math.abs(rect.left - parent.left) <= 1 : Math.abs(rect.right - parent.right) <= 1);
            }, side)).toBe(true);
          }
        }
        const mask = await list.evaluate(element => getComputedStyle(element).maskImage);
        if (anyOverflow) expect(mask).not.toBe('none'); else expect(mask).toBe('none');
        return anyOverflow;
      };
      if (!await checkEdges()) {
        // The merged type scale can fit all labels at 1440 px. Exercise overflow
        // separately without requiring a fade when there is no overflowing edge.
        await page.locator('.ui-tabs').evaluate(element => { element.style.maxWidth = '480px'; });
        await expect.poll(() => list.evaluate(element => element.scrollWidth > element.clientWidth)).toBe(true);
        await checkEdges();
      }
      const initialScroll = await list.evaluate(element => element.scrollLeft);
      await page.locator(`.ui-tabs__arrow--${locale === 'ar' ? 'left' : 'right'}`).click();
      await expect.poll(() => list.evaluate((element, initial) => Math.abs(element.scrollLeft - initial), initialScroll)).toBeGreaterThan(10);
      await checkEdges();
      const tab = list.getByRole('tab', { selected: true });
      await tab.focus();
      await tab.press('End');
      await expect(list.getByRole('tab').last()).toHaveAttribute('aria-selected', 'true');
      await expect.poll(() => list.evaluate(element => {
        const selected = element.querySelector('[aria-selected="true"]')!;
        const target = selected.getBoundingClientRect(); const rect = element.getBoundingClientRect();
        return target.left >= rect.left - 1 && target.right <= rect.right + 1;
      })).toBe(true);
      await list.getByRole('tab').last().press('Home');
      await expect(list.getByRole('tab').first()).toHaveAttribute('aria-selected', 'true');
      await list.getByRole('tab').first().press(locale === 'ar' ? 'ArrowLeft' : 'ArrowRight');
      await expect(list.getByRole('tab').nth(1)).toHaveAttribute('aria-selected', 'true');
      await checkEdges();
    }
    seal();
  });
}
