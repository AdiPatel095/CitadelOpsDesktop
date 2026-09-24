import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';

const locales = ['en','de','fr','pl','ru','it','nl','pt','es','ar','da','no','fi','sv','ja','ko','el','tr','zh-CN','zh-TW','cs','ro','sk','hu','bg','lt'];
const load = async path => JSON.parse(await readFile(path,'utf8'));
const source = await load('src/config/guideLocales/en.json');
const tower = await load('src/config/autoTowerGuide.json');
const bird = await load('src/config/autoBirdGuide.json');
const station = await load('src/config/autoStationGuide.json');
const fortress = await load('src/config/autoFortressGuide.json');
const invasion = await load('src/config/autoInvasionGuide.json');
const countLeaves = value => typeof value === 'string' ? 1 : Object.values(value).reduce((sum, child) => sum + countLeaves(child), 0);
function leaves(candidate, expected, path='') {
  if (typeof expected === 'string') { assert.equal(typeof candidate,'string',path); assert.ok(candidate.trim(),path); return 1; }
  assert.ok(candidate && typeof candidate === 'object' && !Array.isArray(candidate),path);
  assert.deepEqual(Object.keys(candidate).sort(),Object.keys(expected).sort(),path);
  return Object.keys(expected).reduce((count,key)=>count+leaves(candidate[key],expected[key],`${path}.${key}`),0);
}
test('all 25 non-English guide packs have complete stable-ID content', async () => {
  for (const locale of locales) {
    const pack=await load(`src/config/guideLocales/${locale}.json`);
    const expected = source;
    assert.equal(leaves(pack,expected,locale),countLeaves(expected));
    for (const [key,guide] of [['autoTower',tower],['autoBird',bird],['autoStation',station],['autoFortress',fortress],['autoInvasion',invasion]].filter(([key]) => pack[key])) {
      assert.deepEqual(Object.keys(pack[key].steps).sort(),guide.steps.map(step=>step.id).sort());
      for (const step of guide.steps) assert.deepEqual(Object.keys(pack[key].steps[step.id].items).sort(),step.items.map(item=>item.id).sort());
    }
  }
});


test('guide translation provenance pins the exact 26 locale pack bytes', async () => {
  const { createHash } = await import('node:crypto');
  const provenance = await load('src/config/guideLocales/provenance.json');
  assert.equal(Object.keys(provenance.locales).length, 26);
  for (const locale of locales) {
    const bytes = await readFile(`src/config/guideLocales/${locale}.json`);
    assert.equal(createHash('sha256').update(bytes).digest('hex'), provenance.locales[locale], locale);
  }
  assert.equal(provenance.englishSourceSha256, provenance.locales.en);
});


test('canonical English guides match the rendered source pack', () => {
  for (const [key, guide] of [['autoTower', tower], ['autoBird', bird], ['autoStation', station], ['autoFortress', fortress], ['autoInvasion', invasion]]) {
    assert.equal(source[key].recommendationIntro, guide.recommendationIntro);
    for (const step of guide.steps) {
      const translated = source[key].steps[step.id];
      assert.equal(translated.title, step.title);
      for (const item of step.items) {
        assert.deepEqual(translated.items[item.id], Object.fromEntries(['label','description','recommendation'].filter(field => field in item).map(field => [field,item[field]])));
      }
      if (step.image) assert.deepEqual(translated.image, { alt:step.image.alt, caption:step.image.caption });
    }
  }
});


test('Station English photo and gate source are present', async () => {
  const photo = await readFile('public/guide-auto-station-settings.jpg');
  assert.ok(photo.length > 100_000);
  assert.equal(station.steps.find(step => step.id === 'general').image.src, '/guide-auto-station-settings.jpg');
  assert.match(source.autoStation.steps.general.items.open_gate_fallback.description, /any kingdom except Berimond/);
});
