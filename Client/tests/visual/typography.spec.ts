import { test } from '@playwright/test';
import { cases, themes } from './cases';
import { openSettings, openView, prepare, settle } from './harness';
import { scanTypography, checkDialogWrapping, checkLocaleLeading } from './typographyScan';

for (const entry of cases) for (const theme of themes) {
  test(`type scale: ${entry.name}-${theme}`, async ({page})=>{
    const verifyNetwork=await prepare(page,theme,entry.states,entry.scenario);
    await openView(page,entry.label,entry.view);
    if (entry.name === 'header-panel') await page.locator('.header-status-cluster').click();
    if ('settings' in entry) await openSettings(page);
    await settle(page);await scanTypography(page);verifyNetwork();
  });
}
test('all shared dialog titles wrap; short titles stay on one line',async({page})=>{
  const verifyNetwork=await prepare(page,'dark');await settle(page);
  await checkDialogWrapping(page);verifyNetwork();
});
for(const locale of ['ar','ja','ko','zh-CN']) test(`type scale locale rules: ${locale}`,async({page})=>{
  const verifyNetwork=await prepare(page,'dark');await settle(page);
  await page.evaluate(locale=>document.documentElement.lang=locale,locale);
  await scanTypography(page);await checkLocaleLeading(page,locale);verifyNetwork();
});
