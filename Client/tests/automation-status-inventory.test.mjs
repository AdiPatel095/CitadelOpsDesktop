import assert from 'node:assert/strict';
import { test, after } from 'node:test';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const root = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const { KNOWN_RAW_AUTOMATION_STATUSES } = await vite.ssrLoadModule('/src/settings/readiness/playerStatus.ts');
after(() => vite.close());

function checkInventory(statuses) {
  assert.ok(Array.isArray(statuses) && statuses.length > 0, 'the scanner must find real automation emissions');
  const unknown = statuses.filter((status) => !KNOWN_RAW_AUTOMATION_STATUSES.includes(status));
  assert.deepEqual(unknown, [], `Add player mappings for new automation statuses: ${JSON.stringify(unknown)}`);
}

test('every automation status write has an explicit player mapping', () => {
  const { statuses } = JSON.parse(readFileSync(new URL('../../Server/Automation/statusinventory/testdata/automation-statuses.golden.json', import.meta.url), 'utf8'));
  checkInventory(statuses);
});

test('an unknown status in the inventory fails the player mapping guard', () => {
  assert.throws(() => checkInventory(['hibernating']), /Add player mappings for new automation statuses:.*hibernating/);
});
