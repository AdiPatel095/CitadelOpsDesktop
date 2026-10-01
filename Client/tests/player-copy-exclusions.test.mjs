import test from 'node:test';
import assert from 'node:assert/strict';
import { copyExclusions, internalCopy, nonExcludedPlayerCopy } from './visual/playerCopy.ts';

test('copy exclusions ignore only complete approved strings and retain owner/removal triggers', () => {
  for (const entry of copyExclusions) {
    assert.ok(entry.owner && entry.removeWhen);
    assert.match(entry.text, internalCopy);
    assert.doesNotMatch(nonExcludedPlayerCopy(`Player text\n${entry.text}\nMore player text`), internalCopy);
    assert.match(nonExcludedPlayerCopy(`Unexpected prefix ${entry.text}`), internalCopy);
    assert.match(nonExcludedPlayerCopy(`${entry.text} unexpected suffix`), internalCopy);
  }
});
test('all other internal copy still fails, even next to an excluded string', () => {
  for (const text of ['Canonical effects', 'Authoritative new message', 'soft locked', 'on cell-a', 'Checkpoint: never', 'code 453', 'ep-live-us1', 'goodgamestudios.com', 'tenant', 'score rows', 'collected runs', 'leaderboard cached', 'Citadel Ops']) {
    assert.match(nonExcludedPlayerCopy(`${copyExclusions[0].text}\n${text}`), internalCopy);
  }
});
