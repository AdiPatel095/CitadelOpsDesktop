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

test('CIT-66 wrapped reasons omit the separator and start flush in LTR and RTL', async ({ page }) => {
  const verifyNetwork = await prepare(page, 'dark');
  await settle(page);
  const panel = page.locator('.player-connection-panel:visible').first();
  await panel.locator('summary').click();
  const row = panel.locator('.player-connection-details .player-status-row').first();
  const reason = row.locator('.player-status-card-reason');
  const full = await row.getAttribute('aria-label');
  for (const direction of ['ltr', 'rtl']) {
    await row.evaluate((element, direction) => {
      const panel = element.closest('.player-connection-details') as HTMLElement;
      panel.style.width = '1000px'; panel.style.maxWidth = 'none';
      element.style.width = '1000px'; element.style.maxWidth = 'none';
      element.dir = direction;
      (element.querySelector('.player-status-card-reason') as HTMLElement).dir = direction;
    }, direction);
    await expect(reason).toHaveAttribute('data-wrapped', 'false');
    await expect(reason.locator('.player-status-reason-separator')).toBeVisible();
    // Exercise the exact wrap boundary too: removing the visible separator must
    // not make the reason fit again and oscillate between lines.
    await row.evaluate(element => {
      const badge = element.querySelector('.player-status-badge')!;
      const reason = element.querySelector('.player-status-card-reason')!;
      const gap = parseFloat(getComputedStyle(element).columnGap);
      element.style.width = `${badge.getBoundingClientRect().width + reason.getBoundingClientRect().width + gap - 1}px`;
    });
    await expect(reason).toHaveAttribute('data-wrapped', 'true');
    await page.waitForTimeout(300);
    await expect(reason).toHaveAttribute('data-wrapped', 'true');
    await row.evaluate(element => {
      const badge = element.querySelector('.player-status-badge')!;
      element.style.width = `${badge.getBoundingClientRect().width + 20}px`;
    });
    // The boundary was already wrapped, so its previous data attribute is not
    // enough to await this resize. Wait for the new measured geometry too.
    await expect.poll(() => reason.evaluate(element =>
      element.getBoundingClientRect().top - element.previousElementSibling!.getBoundingClientRect().bottom
    )).toBeGreaterThanOrEqual(0);
    await expect(reason).toHaveAttribute('data-wrapped', 'true');
    await expect(reason.locator('.player-status-reason-separator')).toBeHidden();
    // Measure the first visible word, including its bidi run, not the hidden separator.
    const measureGeometry = () => reason.evaluate(element => {
      const range = document.createRange();
      const text = [...element.childNodes].find(node => node.nodeType === Node.TEXT_NODE && node.textContent?.trim());
      if (!text) throw new Error('Card reason must contain visible text');
      range.setStart(text, 0); range.setEnd(text, text.textContent!.indexOf(' ') === -1 ? text.textContent!.length : text.textContent!.indexOf(' '));
      const first = range.getBoundingClientRect();
      const row = element.parentElement!.getBoundingClientRect();
      const badge = element.previousElementSibling!.getBoundingClientRect();
      return { left: first.left - row.left, right: row.right - first.right, top: first.top - badge.bottom };
    });
    // A wrapped attribute/top can belong to the previous width; wait for both
    // axes of the first visible word to reflect the resized row.
    await expect.poll(async () => {
      const geometry = await measureGeometry();
      return { belowBadge: geometry.top >= 0, flush: Math.abs(direction === 'ltr' ? geometry.left : geometry.right) < 1 };
    }).toEqual({ belowBadge: true, flush: true });
    await expect(row).toHaveAttribute('aria-label', full!);
  }
  // Resizing back restores the inline separator without remounting the badge.
  await row.evaluate(element => { element.style.width = '1000px'; });
  await expect(reason).toHaveAttribute('data-wrapped', 'false');
  await expect(reason.locator('.player-status-reason-separator')).toBeVisible();
  verifyNetwork();
});
