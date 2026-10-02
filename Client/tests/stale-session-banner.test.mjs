import assert from 'node:assert/strict';
import { readFileSync, existsSync } from 'node:fs';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';
const root = fileURLToPath(new URL('..', import.meta.url));
const portal = existsSync(`${root}/src/commandCenter`);
const source = readFileSync(`${root}/${portal ? 'src/commandCenter' : 'src'}/components/StaleSessionBanner.tsx`, 'utf8');
test('stale session uses a warning status Banner with a ghost Start Bot action', () => {
  assert.match(source, /<Banner tone="warning" role="status"/);
  assert.match(source, /<Button variant="ghost" size="sm" onClick=\{\(\) => startGame\(\)\}/);
  assert.doesNotMatch(source, /text-primary|m3-status-banner|<button\b/);
  assert.match(source, /if \(gameLoggedIn\) return null/);
});
test('hosted stale session retains its hosted copy with no action', { skip: !portal }, () => {
  assert.match(source, /const hosted = isHostedDeployment\(\)/);
  assert.match(source, /action=\{!hosted && <Button/);
  assert.match(source, /hosted \? 'ui.components.staleSessionBanner.body.hosted'/);
});
