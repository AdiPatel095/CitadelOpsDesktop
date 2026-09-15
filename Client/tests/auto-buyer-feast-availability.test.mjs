import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { test } from 'node:test';

test('Auto Buyer exposes and enforces feast automatic-purchase capability', async () => {
  const [contracts, clientState, modal, select, styles] = await Promise.all([
    readFile(new URL('../src/api/Contracts.ts', import.meta.url), 'utf8'),
    readFile(new URL('../src/settings/AutoBuyerClientState.ts', import.meta.url), 'utf8'),
    readFile(new URL('../src/settings/components/AutoBuyerSettingsModal.tsx', import.meta.url), 'utf8'),
    readFile(new URL('../src/components/ui/Select.tsx', import.meta.url), 'utf8'),
    readFile(new URL('../src/MaterialExpressive.css', import.meta.url), 'utf8'),
  ]);

  assert.match(contracts, /automaticPurchase\?: AutoBuyerCapabilityV1/);
  assert.match(contracts, /feastAutomaticSource\?: AutoBuyerCapabilityV1/);
  assert.match(contracts, /latestFeastPurchase\?: \{/);
  assert.match(clientState, /feastId: 8/);
  assert.match(modal, /firstSupportedFeast = catalog\.feasts\.find/);
  assert.match(modal, /feast\.automaticPurchase\?\.supported === false/);
  assert.match(modal, /Automatic purchase unavailable/);
  assert.match(modal, /savedFeast\.enabled && savedFeast\.feastId === draft\.feast\.feastId/);
  assert.match(modal, /projection\?\.feastAutomaticSource\?\.supported === true/);
  assert.match(modal, /owned positive-net castle with the most food stored/);
  assert.match(modal, /Enter a whole number from 1 to 720 hours/);
  assert.match(modal, /latestFeastPurchase\.activationConfirmed/);
  assert.match(modal, /\(latestFeastPurchase\.foodBefore \?\? 0\)\.toLocaleString\(\)/);
  assert.match(modal, /\(latestFeastPurchase\.foodAfter \?\? 0\)\.toLocaleString\(\)/);
  assert.match(modal, /only positive-net castles qualify/);
  assert.doesNotMatch(modal, /Pay from castle/);

  assert.match(select, /disabled\?: boolean/);
  assert.match(select, /aria-disabled=\{opt\.disabled \|\| undefined\}/);
  assert.match(select, /disabled=\{opt\.disabled\}/);
  assert.match(select, /if \(opt\.disabled\) return/);
  assert.match(styles, /\.m3-select-option:not\(:disabled\):hover/);
  assert.match(styles, /\.m3-select-option:disabled[\s\S]*cursor: not-allowed/);
});
