import { expect, test } from '@playwright/test';
import { prepare, settle } from './harness';

const views = [
  { label: 'Castle', view: 'castle' },
  { label: 'Automation', view: 'automation' },
  { label: 'Feature Stats', view: 'events' },
  { label: 'Attack Presets', view: 'attack-presets' },
  { label: 'Defense Presets', view: 'defense-presets' },
  { label: 'Equipment', view: 'equipment' },
  { label: 'Commanders', view: 'movement' },
  { label: 'Battle Stats', view: 'battle-stats' },
  { label: 'My Stats', view: 'player-tracker' },
  { label: 'Alliance Targets', view: 'alliance-targets' },
  { label: 'Rift Raid', view: 'rift' },
  { label: 'Settings', view: 'settings' },
  { label: 'Patch Notes', view: 'patch-notes' },
  { label: 'Support', view: 'support' },
] as const;

// Browser messages from requests deliberately refused by the visual harness.
const allowedConsolePrefixes = [
  'Failed to load resource: net::ERR_BLOCKED_BY_CLIENT',
  'Failed to load resource: the server responded with a status of 503 (Service Unavailable)',
];

test('every command-center view renders in dark theme', async ({ page }) => {
  test.skip(page.viewportSize()?.width === 1024, 'view coverage uses 1440 and 390');
  const errors: string[] = [];
  const unhandled: string[] = [];
  page.on('console', message => {
    const text = message.text();
    if (text.includes('[mock] unhandled')) unhandled.push(text);
    if (message.type() === 'error' && !allowedConsolePrefixes.some(prefix => text.startsWith(prefix))) errors.push(text);
  });
  page.on('pageerror', error => errors.push(error.message));
  const verifyNetwork = await prepare(page, 'dark');
  for (const { label, view } of views) {
    await test.step(label, async () => {
      if ((page.viewportSize()?.width ?? 0) < 760) {
        await page.getByRole('button', { name: 'Open workspace navigation', exact: true }).click();
      }
      if (['settings', 'patch-notes', 'support'].includes(view)) {
        await page.locator('.liquid-sidebar-system-island').hover();
      }
      await page.locator('#workspace-navigation').getByRole('button', { name: label, exact: true }).click();
      await settle(page);
      await expect(page.locator(`[data-view="${view}"]`)).toBeVisible();
      await expect(page.locator('.command-center-boundary')).toHaveCount(0);
      await expect(page.getByText(/^(Something went wrong|Application error|Unexpected Application Error|The command center hit an error)$/i)).toHaveCount(0);
      expect(errors, `${label}: console and uncaught errors`).toEqual([]);
      expect(unhandled, `${label}: unhandled mock paths`).toEqual([]);
      verifyNetwork();
      const { scrollWidth, innerWidth } = await page.evaluate(() => ({
        scrollWidth: document.documentElement.scrollWidth, innerWidth: window.innerWidth,
      }));
      expect(scrollWidth, `${label}: no horizontal overflow`).toBeLessThanOrEqual(innerWidth);
    });
  }
});
