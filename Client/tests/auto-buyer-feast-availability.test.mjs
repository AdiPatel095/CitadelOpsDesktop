import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { test } from 'node:test';

test('Auto Buyer exposes and enforces feast automatic-purchase capability', async () => {
  const [contracts, modal, select, styles] = await Promise.all([
    readFile(new URL('../src/api/Contracts.ts', import.meta.url), 'utf8'),
    readFile(new URL('../src/settings/components/AutoBuyerSettingsModal.tsx', import.meta.url), 'utf8'),
    readFile(new URL('../src/components/ui/Select.tsx', import.meta.url), 'utf8'),
    readFile(new URL('../src/MaterialExpressive.css', import.meta.url), 'utf8'),
  ]);

  assert.match(contracts, /automaticPurchase\?: AutoBuyerCapabilityV1/);
  assert.match(modal, /firstSupportedFeast = catalog\.feasts\.find/);
  assert.match(modal, /feast\.automaticPurchase\?\.supported === false/);
  assert.match(modal, /Automatic purchase unavailable/);
  assert.match(modal, /savedFeast\.enabled && savedFeast\.feastId === draft\.feast\.feastId/);

  assert.match(select, /disabled\?: boolean/);
  assert.match(select, /aria-disabled=\{opt\.disabled \|\| undefined\}/);
  assert.match(select, /disabled=\{opt\.disabled\}/);
  assert.match(select, /if \(opt\.disabled\) return/);
  assert.match(styles, /\.m3-select-option:not\(:disabled\):hover/);
  assert.match(styles, /\.m3-select-option:disabled[\s\S]*cursor: not-allowed/);
});
