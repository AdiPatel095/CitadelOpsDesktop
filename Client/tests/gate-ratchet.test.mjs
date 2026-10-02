import assert from 'node:assert/strict';
import { test } from 'node:test';
import { applyGateBaseline } from '../scripts/visual/gate-ratchet.mjs';
import baseline from './gate/system-baseline.json' with { type: 'json' };
const finding = { rule: 'axe:button-name', element: '.castle-focus-shell .m3-select-trigger', detail: 'critical: missing name' };
const context = { area: 'd', suite: 'a11y' };
test('the assigned shared exception consumes at most one finding per case', () => {
  const result = applyGateBaseline([finding, finding], baseline, context);
  assert.deepEqual(result.remaining, [finding]);
  assert.equal(result.excluded.length, 1);
  assert.equal(result.excluded[0].owner, 'CIT-74 area (b)');
});
test('other areas, suites, controls and rules remain enforced', () => {
  for (const other of [{ ...finding, element: '.settings-select' }, { ...finding, rule: 'axe:color-contrast' }]) {
    assert.deepEqual(applyGateBaseline([other], baseline, context).remaining, [other]);
  }
  for (const other of [{ area: 'b', suite: 'a11y' }, { area: 'd', suite: 'keyboard' }]) {
    assert.deepEqual(applyGateBaseline([finding], baseline, other).remaining, [finding]);
  }
  assert.deepEqual(applyGateBaseline([], baseline, context), { remaining: [], excluded: [] });
});
