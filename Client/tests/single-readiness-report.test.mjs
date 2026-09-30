import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { after, beforeEach, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const readiness = await vite.ssrLoadModule('/src/settings/readiness/featureReadiness.ts');
const latest = await vite.ssrLoadModule('/src/settings/readiness/latestReadiness.ts');
const eventDifficulty = await vite.ssrLoadModule('/src/settings/EventDifficultyOptions.ts');
const storm = await vite.ssrLoadModule('/src/settings/StormCastleOptions.ts');

after(async () => {
  await vite.close();
});
beforeEach(() => {
  latest.resetLatestReadinessForTests();
  eventDifficulty.resetEventDifficultyCacheForTests();
  storm.resetStormOfferCacheForTests();
});

const SESSION = { generation: 25, baselineGeneration: 25, changedAt: '2026-09-29T09:00:00Z' };
const NOW = Date.parse('2026-09-29T12:00:00Z');
const troops = { 1: { id: 1, name: 'Attacker' } };
const castle = (id) => ({ id, kingdomId: 0, slotType: 1, name: `C${id}`, units: { stationed: { 1: 100 } }, unitsObservedAt: '0001-01-01T00:00:00Z', resources: {}, buildings: {} });
const world = { castles: { 5: castle(5) }, commanders: {}, market: {}, kingdomTransport: { unlocks: {} }, player: { achievements: {} }, session: SESSION };
const sections = {
  'automation.autoInvasion': { version: 1, sourceCastleId: 5, presetId: 'gone', foreignLordsDifficultyId: 3, bloodcrowDifficultyId: 4, scoreTarget: 100 },
  'attacks.presets': { version: 1, presets: [] },
};
const base = { sections, state: world, observation: { session: SESSION, connected: true }, troops, tools: {}, metadataReady: true, movement: null, gameLoggedIn: true, now: NOW };
const blocked = (report) => readiness.blockedChecks(report).map((check) => `${check.id}:${check.slot ?? ''}`).sort();
const catalogState = { difficulties: { optionsByEvent: { 71: [{ value: '3' }], 103: [] }, achievementsObserved: true, loading: false } };

/** What the Automation row evaluates, and what Start evaluates: the same function over the same inputs. */
const evaluateWith = (featureId, catalogs) => readiness.evaluateFeatureReadiness(featureId, { ...base, ...readiness.catalogInputsFor(featureId, catalogs) });

test('a catalog only changes a report through catalogInputsFor, and a catalog that did not load contributes nothing', () => {
  assert.deepEqual(readiness.catalogInputsFor('autoInvasion', {}), {});
  assert.deepEqual(readiness.catalogInputsFor('autoNomad', { stormOffer: { loaded: true, offeredIds: [1] } }), {});
  assert.deepEqual(readiness.catalogInputsFor('autoTowers', catalogState), {}, 'features that do not read catalogs are unaffected');
  assert.deepEqual(readiness.catalogInputsFor('autoInvasion', catalogState), { difficulties: catalogState.difficulties });
  assert.deepEqual(readiness.catalogInputsFor('autoStorm', { stormOffer: { loaded: true, offeredIds: [7] } }), { stormUnlockOffer: { loaded: true, offeredIds: [7] } });
  assert.deepEqual(readiness.READINESS_DIFFICULTY_EVENTS.autoNomad, [72, 80]);
  assert.deepEqual(readiness.READINESS_DIFFICULTY_EVENTS.autoInvasion, [71, 103]);
});

test('row and Start report the same blocked checks when the row published its report for the same saved configuration', () => {
  const rowReport = evaluateWith('autoInvasion', catalogState);
  assert.ok(blocked(rowReport).some((entry) => entry.startsWith('difficulty')), 'with the catalog the difficulty is blocked');
  const digest = latest.savedSectionsDigest(sections);
  latest.publishReadiness('autoInvasion', rowReport, digest, NOW);
  const atStart = latest.latestReadiness('autoInvasion', latest.savedSectionsDigest(sections), NOW + 5_000);
  assert.equal(atStart, rowReport, 'Start reuses the row report');
  assert.deepEqual(blocked(atStart), blocked(rowReport));
});

test('when Start has to evaluate itself it uses the same catalog inputs and lands on the same blocked checks as the row', () => {
  const rowReport = evaluateWith('autoInvasion', catalogState);
  const digest = latest.savedSectionsDigest(sections);
  latest.publishReadiness('autoInvasion', rowReport, digest, NOW);
  // Not reusable: too old, or the saved configuration changed since the row read it.
  assert.equal(latest.latestReadiness('autoInvasion', digest, NOW + latest.LATEST_READINESS_MAX_AGE_MS + 1), null);
  assert.equal(latest.latestReadiness('autoInvasion', latest.savedSectionsDigest({ ...sections, extra: 1 }), NOW + 1), null);
  assert.equal(latest.latestReadiness('autoNomad', digest, NOW + 1), null, 'per feature');
  const startReport = evaluateWith('autoInvasion', catalogState);
  assert.deepEqual(blocked(startReport), blocked(rowReport));
  // The old Start path (no catalogs) is exactly the disagreement this prevents.
  const withoutCatalogs = evaluateWith('autoInvasion', {});
  assert.notDeepEqual(blocked(withoutCatalogs), blocked(rowReport), 'the difficulty check is decided at launch without a catalog');
});

test('the digest follows every saved value and ignores key order only where JSON does not', () => {
  const one = latest.savedSectionsDigest({ a: { x: 1 } });
  assert.equal(one, latest.savedSectionsDigest({ a: { x: 1 } }));
  assert.notEqual(one, latest.savedSectionsDigest({ a: { x: 2 } }));
  assert.equal(latest.savedSectionsDigest(undefined), latest.savedSectionsDigest({}));
});

test('catalog loaders are cached so Start does not refetch what the row just loaded, and failures are not cached', async () => {
  let calls = 0;
  let fail = false;
  const getCatalog = async () => { calls += 1; if (fail) throw new Error('offline'); return { items: [] }; };
  const rows = await eventDifficulty.loadEventDifficultyRows(getCatalog, 'v1', NOW);
  const again = await eventDifficulty.loadEventDifficultyRows(getCatalog, 'v1', NOW + 1_000);
  assert.equal(calls, 3, 'the three official catalogs were read once; the second load is a cache hit');
  assert.equal(again, rows);
  eventDifficulty.resetEventDifficultyCacheForTests();
  fail = true;
  await assert.rejects(() => eventDifficulty.loadEventDifficultyRows(getCatalog, 'v1', NOW));
  fail = false;
  await eventDifficulty.loadEventDifficultyRows(getCatalog, 'v1', NOW + 1);
  assert.equal(calls, 9, 'a failure was not remembered: the next load read the catalogs again');
});

test('the row and the Start check share the same loaders and evaluator', async () => {
  const row = await readFile(new URL('../src/components/AutomationReadinessRow.tsx', import.meta.url), 'utf8');
  const auth = await readFile(new URL('../src/context/AuthContext.tsx', import.meta.url), 'utf8');
  for (const [name, text] of [['row', row], ['Start', auth]]) {
    assert.match(text, /catalogInputsFor\(/, `${name} builds catalog inputs with the shared helper`);
    assert.match(text, /loadStormUnlockOffer/, `${name} reads the Storm offer through the cached loader`);
    assert.match(text, /READINESS_DIFFICULTY_EVENTS/, `${name} reads the same difficulty events`);
  }
  assert.match(row, /publishReadiness\(/);
  assert.match(auth, /latestReadiness\(/);
  assert.match(auth, /savedSectionsDigest\(/);
});
