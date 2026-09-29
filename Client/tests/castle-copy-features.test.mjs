import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const core = await vite.ssrLoadModule('/src/settings/copy/castleCopy.ts');
const { castleCandidates } = await vite.ssrLoadModule('/src/settings/copy/candidates.ts');
const { towersCopyDescriptor } = await vite.ssrLoadModule('/src/settings/copy/features/towers.ts');
const { stationCopyDescriptor } = await vite.ssrLoadModule('/src/settings/copy/features/station.ts');
const { birdCopyDescriptor, birdCandidateFlags } = await vite.ssrLoadModule('/src/settings/copy/features/bird.ts');
const { recruitCopyDescriptor, toolCopyDescriptor } = await vite.ssrLoadModule('/src/settings/copy/features/queueProduction.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const ZERO = '0001-01-01T00:00:00Z';
const SESSION = { generation: 5, baselineGeneration: 5, changedAt: '2026-09-29T09:00:00Z' };
const LIVE = { session: SESSION, connected: true };
const troops = { 1: { id: 1, name: 'Spearmen' }, 2: { id: 2, name: 'Crossbowmen' }, 277: { id: 277, name: 'Direwolves' } };
const tools = { 500: { id: 500, name: 'Ballistae' } };
const castle = (id, name, kingdomId, slotType, stationed = {}) => ({ id, name, kingdomId, slotType, x: 0, y: 0, units: { stationed }, unitsObservedAt: ZERO, resources: {}, buildings: {} });
const world = (...castles) => ({ castles: Object.fromEntries(castles.map((entry) => [entry.id, entry])) });

function context(castles, extra = {}, flagsFor) {
  const state = world(...castles);
  return {
    state, troops, tools, metadataReady: true, observation: LIVE,
    candidates: castleCandidates(castles.map((entry) => ({ id: entry.id, name: entry.name, kingdomId: entry.kingdomId })), state, flagsFor ? { flagsFor } : {}),
    ...extra,
  };
}
const preview = (descriptor, draft, source, ctx, destinations = ctx.candidates.map((entry) => entry.key)) => core.previewCastleCopy(descriptor, draft, source, destinations, ctx);
const byKey = (result, key) => result.destinations.find((entry) => entry.key === key);
const reasonIds = (destination) => destination.reasons.map((reason) => reason.id);
const allKeysExist = (result) => result.destinations.forEach((destination) => destination.reasons.forEach((reason) => assert.ok(messages[reason.messageKey], reason.messageKey)));

test('Towers: compatible, stock short, unobserved, unknown troop and not-in-world destinations', () => {
  const castles = [castle(1, 'Sunhold', 0, 1, { 2: 50 }), castle(2, 'Frostkeep', 0, 4, { 2: 10 }), castle(3, 'Ashvale', 0, 4, {}), castle(4, 'Mirefen', 0, 4, { 2: 1 })];
  const draft = { 1: { enabled: true, radius: 15, unitId: 2, maidenOnly: true } };
  const ctx = context(castles);
  const result = preview(towersCopyDescriptor, draft, '1', ctx);
  assert.equal(byKey(result, '2').state, 'compatible');
  const short = byKey(result, '3');
  assert.equal(short.state, 'unavailable');
  assert.deepEqual(short.reasons[0].params, { castle: 'Ashvale', stationed: 0, required: 1, unit: 'Crossbowmen' });
  assert.equal(short.reasons[0].messageKey, 'castleCopy.reason.stockShort');
  assert.equal(messages['castleCopy.reason.stockShort'], '{castle} has {stationed, number} of {required, number} {unit} stationed.');
  // Not observed on this connection (D1): unknown, with the connection fix, never blocked.
  const awaiting = context(castles, { observation: { session: { ...SESSION, baselineGeneration: 4 }, connected: true } });
  const unknown = byKey(preview(towersCopyDescriptor, draft, '1', awaiting), '2');
  assert.equal(unknown.state, 'unknown');
  assert.equal(unknown.reasons[0].fix, 'connection');
  assert.equal(messages[unknown.reasons[0].messageKey], 'Troop counts for {castle} have not been observed in this connection yet.');
  // Zero observed castles is waiting for data, not a configuration error.
  const none = context([castle(1, 'Sunhold', 0, 1)], { state: { castles: {} } });
  none.candidates[1] = { key: '2', liveId: 2, name: 'Ghost', kingdomId: 0, castle: null };
  assert.equal(byKey(preview(towersCopyDescriptor, draft, '1', none), '2').state, 'incompatible', 'a castle that is not in this world cannot be copied to');
  // A troop the game does not know makes the whole destination incompatible; nothing is substituted.
  const unknownTroop = preview(towersCopyDescriptor, { 1: { ...draft[1], unitId: 999 } }, '1', ctx);
  assert.equal(byKey(unknownTroop, '2').state, 'incompatible');
  assert.equal(byKey(unknownTroop, '2').reasons[0].messageKey, 'castleCopy.reason.unknownItem');
  assert.equal(core.applyCastleCopy(towersCopyDescriptor, draft, unknownTroop, { 2: new Set(['unitId']) })[2], undefined);
  [result, unknownTroop, preview(towersCopyDescriptor, draft, '1', awaiting)].forEach(allKeysExist);
});

test('Towers: enabled is only copied by an explicit include, radius and maiden preferences copy to a default castle', () => {
  const castles = [castle(1, 'Sunhold', 0, 1, { 2: 50 }), castle(2, 'Frostkeep', 0, 4, { 2: 10 })];
  const draft = { 1: { enabled: true, radius: 15, unitId: 2, maidenOnly: true } };
  const ctx = context(castles);
  const result = preview(towersCopyDescriptor, draft, '1', ctx);
  const input = core.withDestination(core.defaultCopyInput(towersCopyDescriptor, result), '2', true);
  const next = core.applyCastleCopy(towersCopyDescriptor, draft, result, core.buildCopySelection(result, input));
  assert.deepEqual(next[2], { enabled: false, radius: 15, unitId: 2, maidenOnly: true });
  const included = core.applyCastleCopy(towersCopyDescriptor, draft, result, core.buildCopySelection(result, core.withField(input, 'enabled', true)));
  assert.equal(included[2].enabled, true);
  assert.equal(towersCopyDescriptor.enabledIncludeKey, 'castleCopy.include.feature');
  assert.equal(messages['castleCopy.include.feature'], 'Also include the selected castles in {feature}');
  assert.equal(messages['castleCopy.include.helper'], 'Castles you do not include stay as they are. This never starts the automation.');
});

test('Station: reserves are compared as a set; short stock is unavailable per unit; unknown units are dropped, not replaced', () => {
  const castles = [castle(1, 'Sunhold', 0, 1, { 1: 500, 2: 500 }), castle(2, 'Frostkeep', 0, 4, { 1: 500, 2: 0 }), castle(3, 'Ashvale', 0, 4, { 1: 500, 2: 500 })];
  const draft = { 1: [{ id: 1, amount: 100 }, { id: 2, amount: 120 }], 3: [{ id: 2, amount: 120 }, { id: 1, amount: 100 }] };
  const ctx = context(castles);
  const result = preview(stationCopyDescriptor, draft, '1', ctx);
  assert.equal(byKey(result, '3').changes[0].kind, 'same', 'the same reserves in another order are not a difference');
  const short = byKey(result, '2');
  assert.equal(short.state, 'unavailable');
  assert.deepEqual(short.reasons.map((reason) => reason.params), [{ castle: 'Frostkeep', stationed: 0, required: 120, unit: 'Crossbowmen' }]);
  const unknown = preview(stationCopyDescriptor, { 1: [{ id: 1, amount: 100 }, { id: 999, amount: 5 }] }, '1', ctx);
  const dropped = byKey(unknown, '2');
  assert.equal(dropped.state, 'unavailable');
  assert.deepEqual(dropped.changes[0].dropped, [999]);
  assert.deepEqual(dropped.changes[0].to, [{ id: 1, amount: 100 }], 'the unknown unit is dropped and named, never replaced');
  assert.equal(dropped.reasons.find((reason) => reason.id === 'unknown-item:999').messageKey, 'castleCopy.reason.unknownItem');
  const allUnknown = preview(stationCopyDescriptor, { 1: [{ id: 999, amount: 5 }] }, '1', ctx);
  assert.equal(byKey(allUnknown, '2').state, 'incompatible');
  const next = core.applyCastleCopy(stationCopyDescriptor, draft, result, { 2: new Set(['reserves']) });
  assert.deepEqual(next[2], draft[1], 'an explicitly included unavailable destination gets exactly the source reserves');
  [result, unknown, allUnknown].forEach(allKeysExist);
});

test('Bird: Direwolves are dropped at an outer main castle Auto Fortress reserves; the rest copies and the destination Direwolf row is kept', () => {
  const castles = [castle(1, 'Sunhold', 0, 1, { 1: 500, 277: 900 }), castle(2, 'Frostkeep', 1, 12, { 1: 500, 277: 900 }), castle(3, 'Ashvale', 2, 12, { 1: 500, 277: 900 })];
  const fortress = { enabled: true, section: { version: 1, kingdoms: { 1: { enabled: true } } } };
  const ctx = context(castles, { fortress }, (entry) => birdCandidateFlags(entry, { fortress }));
  const draft = { 1: [{ id: 1, amount: 100 }, { id: 277, amount: 200 }], 2: [{ id: 277, amount: 50 }] };
  const result = preview(birdCopyDescriptor, draft, '1', ctx);
  const reserved = byKey(result, '2');
  assert.equal(reserved.state, 'unavailable', 'the rest of the list stays copyable');
  assert.equal(reserved.reasons.find((reason) => reason.id === 'direwolves-reserved').messageKey, 'castleCopy.reason.direwolves');
  assert.equal(messages['castleCopy.reason.direwolves'], 'Direwolves at {castle} are reserved for Auto Fortress.');
  assert.deepEqual(reserved.changes[0].to.map((entry) => entry.id), [1]);
  const next = core.applyCastleCopy(birdCopyDescriptor, draft, result, { 2: new Set(['keep']) });
  assert.deepEqual(next[2], [{ id: 1, amount: 100 }, { id: 277, amount: 50 }], "the destination's own Direwolf row is preserved");
  assert.equal(byKey(result, '3').state, 'compatible', 'kingdom 2 is not reserved: Direwolves copy as any unit');
  const off = context(castles, { fortress: { enabled: false, section: fortress.section } }, (entry) => birdCandidateFlags(entry, { fortress: { enabled: false, section: fortress.section } }));
  assert.equal(byKey(preview(birdCopyDescriptor, draft, '1', off), '2').state, 'compatible', 'with Auto Fortress off nothing is reserved');
  allKeysExist(result);
});

test('Queue Production: items the game does not offer at a castle are dropped and named; cursor restarts; enabled is explicit; scheduled castles refuse', () => {
  const castles = [castle(1, 'Sunhold', 0, 1), castle(2, 'Frostkeep', 0, 4), castle(3, 'Stormhold', 4, 1), castle(4, 'Ashvale', 0, 4)];
  const key = (entry) => (entry.id === 3 ? '777' : String(entry.id));
  const state = world(...castles);
  const ctx = {
    state, troops, tools, metadataReady: true, observation: LIVE,
    candidates: castleCandidates(castles, state, { keyFor: key }),
    allowedItemIds: (entry) => (entry.liveId === 2 ? [1] : [1, 2]),
    usesScheduledItems: (entry) => entry.liveId === 4,
  };
  const base = { version: 1, mode: 'perCastle', checkIntervalSec: 300, globalItems: [], castles: {
    1: { enabled: true, items: [{ id: 1, amount: 5 }, { id: 2, amount: 5 }], cursor: 3 },
    2: { enabled: false, items: [], cursor: 0, kingdomId: 0 },
    777: { enabled: false, items: [{ id: 1, amount: 1 }], cursor: 2, kingdomId: 4 },
  } };
  const result = preview(recruitCopyDescriptor, base, '1', ctx);
  assert.deepEqual(result.destinations.map((entry) => entry.key), ['2', '777', '4'], 'the Storm castle keeps its stable key');
  const partial = byKey(result, '2');
  assert.equal(partial.state, 'unavailable');
  assert.deepEqual(partial.changes.find((change) => change.fieldId === 'items').to, [{ id: 1, amount: 5 }]);
  assert.equal(partial.reasons[0].messageKey, 'castleCopy.reason.notOffered');
  assert.deepEqual(partial.reasons[0].params, { item: 'Crossbowmen', castle: 'Frostkeep' });
  assert.equal(messages['castleCopy.reason.notOffered'], 'The game does not offer {item} at {castle}.');
  assert.equal(byKey(result, '777').state, 'compatible');
  assert.equal(byKey(result, '4').state, 'incompatible', 'a castle that takes its list from calendar slots refuses');
  const chosen = core.withDestination(core.withDestination(core.defaultCopyInput(recruitCopyDescriptor, result), '777', true), '2', true);
  assert.deepEqual(Object.keys(core.buildCopySelection(result, chosen)), ['2'], 'the Storm castle holds a custom list: kept unless explicitly included');
  const selection = core.buildCopySelection(result, core.withKeptInclude(chosen, '777', 'items', true));
  assert.deepEqual(Object.keys(selection).sort(), ['2', '777']);
  const next = core.applyCastleCopy(recruitCopyDescriptor, base, result, selection);
  assert.equal(next.castles[2].enabled, false, 'the default copy never includes the castle in the plan');
  assert.equal(next.castles[777].cursor, 0, 'the rotation restarts when the list changes');
  assert.equal(next.castles[777].kingdomId, 4, 'the destination kingdom is set from the destination');
  assert.deepEqual(next.castles[777].items, [{ id: 1, amount: 5 }, { id: 2, amount: 5 }]);
  assert.equal(next.mode, 'perCastle');
  assert.equal(next.checkIntervalSec, 300);
  const included = core.applyCastleCopy(recruitCopyDescriptor, base, result, core.buildCopySelection(result, core.withField(core.withKeptInclude(core.withDestination(core.defaultCopyInput(recruitCopyDescriptor, result), '777', true), '777', 'items', true), 'enabled', true)));
  assert.equal(included.castles[777].enabled, true);
  assert.equal(recruitCopyDescriptor.enabledIncludeKey, 'castleCopy.include.queue');
  assert.equal(messages['castleCopy.include.queue'], 'Also include the selected castles in the per-castle plan');
  const loading = preview(toolCopyDescriptor, { ...base, castles: { 1: { enabled: true, items: [{ id: 500, amount: 1 }], cursor: 0 } } }, '1', { ...ctx, allowedItemIds: () => undefined });
  assert.equal(byKey(loading, '2').state, 'unknown', 'catalog rows not loaded: unknown');
  assert.equal(toolCopyDescriptor.featureId, 'autoTool');
  [result, loading].forEach(allKeysExist);
});

test('unsupported fields are reported per feature with Maya\'s footer pattern', () => {
  assert.equal(messages['castleCopy.notCopied.towers'], 'Not copied: check timing, map scan, Advisor, travel, daily limit. These settings apply to all castles.');
  for (const descriptor of [towersCopyDescriptor, stationCopyDescriptor, birdCopyDescriptor, recruitCopyDescriptor, toolCopyDescriptor]) {
    assert.match(messages[descriptor.notCopiedKey], /^Not copied: .+\. These settings apply to all castles\.$/, descriptor.featureId);
  }
});
