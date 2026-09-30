import test from 'node:test';
import assert from 'node:assert/strict';
import { existsSync, readdirSync, readFileSync } from 'node:fs';

const root = new URL(existsSync(new URL('../src/commandCenter/', import.meta.url)) ? '../src/commandCenter/' : '../src/', import.meta.url);
const { serverLabel } = await import(new URL('worldIntelligence/serverLabel.ts', root));
const directory = [
  { code: 'US1', label: 'United States', host: 'ep-live-us1-game.goodgamestudios.com' },
  { code: 'INT1', label: 'International 1', host: 'ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com' },
  { code: 'SK1', label: 'Slovakia', host: 'ep-live-mz-int1-sk1-gb1-game.goodgamestudios.com' },
];
test('single-world hosts and codes use the catalog display name', () => {
  assert.equal(serverLabel('wss://ep-live-us1-game.goodgamestudios.com:443', directory), 'United States (US1)');
  assert.equal(serverLabel('us1', directory), 'United States (US1)');
});
test('shared hosts select the account world and never guess the first world', () => {
  const host = directory[1].host;
  assert.equal(serverLabel(host, directory, 'SK1'), 'Slovakia (SK1)');
  assert.equal(serverLabel(host, directory), '');
});
test('unknown codes and unavailable catalogs fall back to codes, never hostnames', () => {
  assert.equal(serverLabel('ZZ9', directory), 'ZZ9');
  assert.equal(serverLabel(directory[0].host, [], 'US1'), 'US1');
  for (const value of ['ep-live-secret', 'wss://unknown.goodgamestudios.com:443', 'unknown.example.org', '', '<script>']) {
    assert.doesNotMatch(serverLabel(value, directory), /ep-live|goodgamestudios|example\.org|[/:<>]/);
  }
  assert.equal(serverLabel('US1', [{code:'US1',label:'ep-live-us1-game.goodgamestudios.com'}]), 'US1');
});
test('UI sources, titles and catalog values retain the CitadelOps brand', () => {
  const visit = path => {
    for (const entry of readdirSync(path, { withFileTypes: true })) {
      const url = new URL(entry.name + (entry.isDirectory() ? '/' : ''), path);
      if (entry.isDirectory()) visit(url);
      else if (/\.(?:tsx?|html)$/.test(entry.name)) assert.doesNotMatch(readFileSync(url, 'utf8'), /Citadel Ops/, url.pathname);
      else if (entry.name.endsWith('.json') && /\/(?:catalogs|portal)\//.test(url.pathname)) {
        // Keys, package/executable names, paths and audit metadata are M8 exclusions.
        for (const value of Object.values(JSON.parse(readFileSync(url, 'utf8')))) if (typeof value === 'string') assert.doesNotMatch(value, /Citadel Ops/, url.pathname);
      }
    }
  };
  visit(new URL('../src/', import.meta.url));
  assert.doesNotMatch(readFileSync(new URL('../index.html', import.meta.url), 'utf8'), /<title>[^<]*Citadel Ops/);
});
