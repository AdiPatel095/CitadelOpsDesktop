import assert from 'node:assert/strict';
import test from 'node:test';
import { checkBaselineChanges } from '../scripts/visual/check-baseline-changes.mjs';

const first = 'Client/tests/visual/__screenshots__/castle-1440-dark.png';
const second = 'Client/tests/visual/__screenshots__/automation-1024-light.png';
test('baseline guard permits changes without baseline files', () => {
  assert.deepEqual(checkBaselineChanges(['Client/src/index.css'], ''), []);
});
test('baseline guard requires a dedicated section and every changed filename', () => {
  assert.equal(checkBaselineChanges([first], 'castle-1440-dark.png').length, 1);
  assert.equal(checkBaselineChanges([first, second], '## Visual baseline changes\n- castle-1440-dark.png — initial baseline').length, 1);
  assert.deepEqual(checkBaselineChanges([first, second], '## Visual baseline changes\n- castle-1440-dark.png — initial baseline\n- automation-1024-light.png — initial baseline'), []);
});
test('baseline guard rejects filenames outside the section or as substrings', () => {
  assert.equal(checkBaselineChanges([first], '## Visual baseline changes\nNone\n## Checks\ncastle-1440-dark.png').length, 1);
  assert.equal(checkBaselineChanges([first], '## Visual baseline changes\n- not-castle-1440-dark.png — unrelated').length, 1);
});
