import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const notes = await vite.ssrLoadModule('/src/config/PatchNotes.ts');
after(() => vite.close());

test('release metadata agrees across runtime, packages and the current notes', async () => {
  const expected = notes.APP_VERSION_CURRENT;
  assert.equal(expected, '2.4.1');
  const version = await readFile(new URL('../../Server/App/Version.go', import.meta.url), 'utf8');
  assert.ok(version.includes(`const Version = "${expected}"`));
  for (const path of ['../package.json', '../package-lock.json', '../../package.json', '../../package-lock.json']) {
    const manifest = JSON.parse(await readFile(new URL(path, import.meta.url), 'utf8'));
    assert.equal(manifest.version, expected, path);
    if (manifest.packages) assert.equal(manifest.packages[''].version, expected, path);
  }
});

test('release notes show 2.4.1 before 2.4.0 with canonical change groups', () => {
  assert.deepEqual(notes.PATCH_NOTES_RELEASES.slice(0, 3).map(({ version }) => version), ['2.4.1', '2.4.0', '2.3.6']);
  assert.equal(notes.PATCH_NOTES_RELEASES.filter(({ version }) => version === '2.4.1').length, 1);
  assert.equal(notes.PATCH_NOTES_RELEASES.filter(({ version }) => version === '2.4.0').length, 1);
  for (const release of notes.PATCH_NOTES_RELEASES) {
    const ranks = release.items.map(({ kind }) => notes.PATCH_NOTE_KIND_ORDER.indexOf(kind));
    assert.ok(ranks.every((rank) => rank >= 0));
    assert.deepEqual(ranks, [...ranks].sort((a, b) => a - b), release.version);
  }
});
