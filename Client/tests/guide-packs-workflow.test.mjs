import assert from 'node:assert/strict';
import { createHash } from 'node:crypto';
import { copyFile, mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import test from 'node:test';
import { exportGuide, importGuide, statusGuide } from '../scripts/localization/guide-packs.mjs';

const bytes = path => readFile(path, 'utf8');
const digest = value => createHash('sha256').update(value).digest('hex');
const root = new URL('..', import.meta.url).pathname;

async function fixture() {
  const target = await mkdtemp(join(tmpdir(), 'guide-pack-test-'));
  await mkdir(join(target, 'src/config/guideLocales'), { recursive: true });
  await mkdir(join(target, 'src/i18n'), { recursive: true });
  for (const file of ['autoFortressGuide.json', 'guideLocales/en.json', 'guideLocales/fr.json']) {
    await copyFile(join(root, 'src/config', file), join(target, 'src/config', file));
  }
  await writeFile(join(target, 'src/i18n/locales.ts'), "export const localeCodes = ['en', 'fr'] as const;\n");
  await writeFile(join(target, 'src/config/guideLocales/provenance.json'), '{"locales":{"en":"","fr":""}}\n');
  return target;
}

test('guide import validates before writing, mirrors safely, and is repeatable', async t => {
  const primary = await fixture(), mirror = await fixture();
  t.after(async () => { await rm(primary, { recursive: true, force: true }); await rm(mirror, { recursive: true, force: true }); });
  const contract = await exportGuide(primary, 'autoFortress');
  assert.ok(Object.keys(contract.entries).length > 50);
  const translation = { ...contract, entries: Object.fromEntries(Object.entries(contract.entries).map(([key, value]) => [key, `${value} traduction`])) };
  const path = join(primary, 'src/config/guideLocales/fr.json');
  const before = await bytes(path);
  await assert.rejects(importGuide(primary, 'autoFortress', 'fr', { ...translation, entries: { ...translation.entries, stray: 'x' } }, mirror), /extra/);
  await assert.rejects(importGuide(primary, 'autoFortress', 'fr', { ...translation, entries: { ...translation.entries, [Object.keys(translation.entries)[0]]: ' ' } }, mirror), /Blank/);
  assert.equal(await bytes(path), before);
  await importGuide(primary, 'autoFortress', 'fr', translation, mirror);
  assert.equal((await statusGuide(primary, 'autoFortress', mirror)).locales[0].state, 'complete');
  assert.equal(await bytes(path), await bytes(join(mirror, 'src/config/guideLocales/fr.json')));
  const first = digest(await bytes(path));
  await importGuide(primary, 'autoFortress', 'fr', { ...translation, entries: Object.fromEntries(Object.entries(translation.entries).reverse()) }, mirror);
  assert.equal(digest(await bytes(path)), first);
  const changedMirror = JSON.parse(await bytes(join(mirror, 'src/config/guideLocales/fr.json')));
  changedMirror.autoBird.recommendationIntro += ' changed';
  await writeFile(join(mirror, 'src/config/guideLocales/fr.json'), JSON.stringify(changedMirror));
  await assert.rejects(importGuide(primary, 'autoFortress', 'fr', translation, mirror), /unrelated guide-pack differences/);
  assert.equal(digest(await bytes(path)), first);
});

test('source edits invalidate exports and imported progress', async t => {
  const primary = await fixture();
  t.after(async () => rm(primary, { recursive: true, force: true }));
  const contract = await exportGuide(primary, 'autoFortress');
  const translation = { ...contract, entries: Object.fromEntries(Object.entries(contract.entries).map(([key, value]) => [key, `${value} traduction`])) };
  await importGuide(primary, 'autoFortress', 'fr', translation);
  const sourcePath = join(primary, 'src/config/autoFortressGuide.json');
  const englishPath = join(primary, 'src/config/guideLocales/en.json');
  const source = JSON.parse(await bytes(sourcePath)), english = JSON.parse(await bytes(englishPath));
  source.recommendationIntro += ' Revised.';
  english.autoFortress.recommendationIntro = source.recommendationIntro;
  await writeFile(sourcePath, JSON.stringify(source));
  await writeFile(englishPath, JSON.stringify(english));
  assert.equal((await statusGuide(primary, 'autoFortress')).locales[0].state, 'stale');
  const before = await bytes(join(primary, 'src/config/guideLocales/fr.json'));
  await assert.rejects(importGuide(primary, 'autoFortress', 'fr', translation), /source hash/);
  assert.equal(await bytes(join(primary, 'src/config/guideLocales/fr.json')), before);
});
