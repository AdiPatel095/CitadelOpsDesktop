import { test, expect } from '@playwright/test';
import { cases, themes } from './cases';
import { openSettings, openView, prepare, settle } from './harness';

import { internalCopy, nonExcludedPlayerCopy } from './playerCopy';
for (const visualCase of cases) for (const theme of themes) {
  test(`player copy ${visualCase.name} ${theme}`, async ({ page }) => {
    test.skip(page.viewportSize()?.width === 1024, 'copy checks cover 1440 and 390');
    const verifyNetwork = await prepare(page, theme, visualCase.states, visualCase.scenario);
    await openView(page, visualCase.label, visualCase.view);
    if (visualCase.name === 'header-panel') await page.locator('.header-status-cluster').click();
    if (visualCase.settings) await openSettings(page);
    await settle(page);
    expect(nonExcludedPlayerCopy(await page.locator('body').innerText())).not.toMatch(internalCopy);
    verifyNetwork();
  });
}
