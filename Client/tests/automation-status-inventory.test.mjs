import assert from 'node:assert/strict';
import { test, after } from 'node:test';
import { readdirSync, readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
import { createServer } from 'vite';

const root = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const { KNOWN_RAW_AUTOMATION_STATUSES } = await vite.ssrLoadModule('/src/settings/readiness/playerStatus.ts');
after(() => vite.close());

function inventory(dir) {
  const entries = [];
  for (const item of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, item.name);
    if (item.isDirectory()) entries.push(...inventory(path));
    else if (item.name.endsWith('.go') && !item.name.endsWith('_test.go')) {
      for (const match of readFileSync(path, 'utf8').matchAll(/\bStatus\s*:\s*"([^"]+)"/g)) entries.push({ path, status: match[1] });
    }
  }
  return entries;
}

test('every automation Status literal has an explicit player mapping', () => {
  const entries = inventory(fileURLToPath(new URL('../../Server/Automation', import.meta.url)));
  assert.ok(entries.length > 0, 'the scanner must find real automation emissions');
  const unknown = entries.filter(({ status }) => !KNOWN_RAW_AUTOMATION_STATUSES.includes(status));
  assert.deepEqual(unknown, [], `Add player mappings for new automation statuses: ${JSON.stringify(unknown)}`);
});
