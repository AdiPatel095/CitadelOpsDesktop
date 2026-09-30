import { test, expect } from '@playwright/test';
import { cases, themes } from './cases';
import { openSettings, openView, prepare, settle } from './harness';

const internalCopy = /canonical|soft[- ]lock(?:ed)?|authoritative|managed fleet|on cell-|Checkpoint:\s*never|code\s+\d{2,}|tenant|ep-live-[\w-]+|goodgamestudios\.com|collected runs|score rows|leaderboards? cached|Citadel Ops/i;
for (const visualCase of cases) for (const theme of themes) {
  test(`player copy ${visualCase.name} ${theme}`, async ({ page }) => {
    test.skip(page.viewportSize()?.width === 1024, 'copy checks cover 1440 and 390');
    const verifyNetwork = await prepare(page, theme);
    await openView(page, visualCase.label, visualCase.view);
    if ('settings' in visualCase) await openSettings(page);
    await settle(page);
    expect(await page.locator('body').innerText()).not.toMatch(internalCopy);
    verifyNetwork();
  });
}
