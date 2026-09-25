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
const nomad = await load('src/config/autoNomadGuide.json');
const advisor = await load('src/config/autoAdvisorGuide.json');
const khan = await load('src/config/autoKhanGuide.json');
const beri = await load('src/config/autoBeriGuide.json');
const storm = await load('src/config/autoStormGuide.json');
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
    for (const [key,guide] of [['autoTower',tower],['autoBird',bird],['autoStation',station],['autoFortress',fortress],['autoInvasion',invasion],['autoNomad',nomad],['autoAdvisor',advisor],['autoKhan',khan],['autoBeri',beri],['autoStorm',storm]].filter(([key]) => pack[key])) {
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
  for (const [key, guide] of [['autoTower', tower], ['autoBird', bird], ['autoStation', station], ['autoFortress', fortress], ['autoInvasion', invasion], ['autoNomad', nomad], ['autoAdvisor', advisor], ['autoKhan', khan], ['autoBeri', beri], ['autoStorm', storm]]) {
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


test('Advisor guidance separates save, activation, and pre-launch limits', () => {
  assert.match(source.autoAdvisor.steps.activation.items.confirm_paid_activation.description, /token/);
  assert.match(source.autoAdvisor.steps.sizing.items.minimum_remaining.recommendation, /not an ongoing cancellation timer/);
  assert.match(source.autoAdvisor.steps.resources.items.coin_cost.recommendation, /only if it covers/);
});


test('Khan English guide distinguishes score stop, rage cap, and main-wall protection', () => {
  assert.match(source.autoKhan.steps.limits.items.nomad_points.description, /recall outgoing Khan attacks and open the main gates/);
  assert.match(source.autoKhan.steps.rage.items.max_rage_chain.description, /before new camp attacks stop/);
  assert.match(source.autoKhan.steps.protection.items.protect_offense.description, /When attacking from the main castle/);
  assert.equal(source.autoKhan.steps.limits.items.event_end.recommendation, '5 minutes.');
});


test('Beri English guide explains proportional transfers and threshold', () => {
  assert.match(source.autoBeri.steps.transfers.items.preset_troop_mix.description, /one underrepresented type at a time/);
  assert.match(source.autoBeri.steps.transfers.items.minimum_free_capacity.description, /individual proportional shipment can be smaller/);
  assert.match(source.autoBeri.steps.transfers.items.partial_donor.description, /waits instead of sending another type/);
  assert.equal(beri.steps.length, 6);
});


test('Storm English guide keeps premium and Luna spending choices explicit', () => {
  assert.match(source.autoStorm.steps.construction.items.premium.recommendation, /Off unless/);
  assert.match(source.autoStorm.steps.luna.items.unlimited.description, /reserve and shop cap/);
  assert.match(source.autoStorm.steps.troops.items.minimum_kept.recommendation, /1,000/);
  assert.equal(storm.steps.length, 6);
});
