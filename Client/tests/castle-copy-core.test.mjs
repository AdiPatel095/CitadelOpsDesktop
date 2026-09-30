import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const core = await vite.ssrLoadModule('/src/settings/copy/castleCopy.ts');

after(async () => {
  await vite.close();
});

/** A synthetic descriptor: a scalar `radius`, a list `items`, and a consequential `enabled`. */
const scalar = (id, extra = {}) => ({
  id, labelKey: 'castleCopy.field.radius', read: (record) => record[id], write: (record, value) => ({ ...record, [id]: value }),
  describe: (value) => ({ messageKey: 'castleCopy.value.onOff', params: { on: value ? 'on' : 'off' } }), ...extra,
});
const descriptor = {
  featureId: 'autoTowers', sectionId: 'castles', primaryFieldId: 'items', notCopiedKey: 'castleCopy.notCopied.towers',
  fields: [
    scalar('radius'),
    { ...scalar('items'), entryId: (entry) => entry.id },
    scalar('enabled', { consequential: true }),
  ],
  defaultRecord: () => ({ radius: 10, items: [], enabled: false }),
  recordFor: (draft, key) => draft[key],
  isConfigured: (record) => record.radius !== 10 || record.items.length > 0 || record.enabled,
  validate: (_source, destination) => destination.stub ?? [],
  apply: (draft, key, next) => ({ ...draft, [key]: next }),
};
const castle = (key, stub) => ({ key, liveId: Number(key), name: `C${key}`, kingdomId: 0, castle: {}, ...(stub ? { stub } : {}) });
const context = { candidates: [castle('1'), castle('2'), castle('3'), castle('4')] };
const item = (id) => ({ id });

test('changes: same, set (destination at default) and kept-difference (custom destination) per field', () => {
  const draft = {
    1: { radius: 20, items: [item(7)], enabled: true },
    2: { radius: 20, items: [item(7)], enabled: false },        // radius/items same
    3: { radius: 10, items: [], enabled: false },                // at defaults: set
    4: { radius: 33, items: [item(9)], enabled: false },        // custom: kept differences
  };
  const preview = core.previewCastleCopy(descriptor, draft, '1', ['2', '3', '4'], context);
  const by = (key) => Object.fromEntries(preview.destinations.find((entry) => entry.key === key).changes.map((change) => [change.fieldId, change.kind]));
  assert.deepEqual(by('2'), { radius: 'same', items: 'same', enabled: 'set' });
  assert.deepEqual(by('3'), { radius: 'set', items: 'set', enabled: 'set' });
  assert.deepEqual(by('4'), { radius: 'kept-difference', items: 'kept-difference', enabled: 'set' });
  assert.equal(preview.sourceConfigured, true);
  assert.equal(preview.unsupported, 'castleCopy.notCopied.towers');
});

test('a consequential field is never included by default, even when the destination is at its default', () => {
  const draft = { 1: { radius: 20, items: [item(7)], enabled: true }, 3: { radius: 10, items: [], enabled: false } };
  const preview = core.previewCastleCopy(descriptor, draft, '1', ['3'], context);
  const enabled = preview.destinations[0].changes.find((change) => change.fieldId === 'enabled');
  assert.equal(enabled.kind, 'set');
  assert.equal(enabled.includedByDefault, false);
  const input = core.defaultCopyInput(descriptor, preview);
  assert.equal(input.fields.has('enabled'), false);
  const next = core.applyCastleCopy(descriptor, draft, preview, core.buildCopySelection(preview, input));
  assert.equal(next[3].enabled, false, 'the default copy never widens the running scope');
  assert.equal(next[3].radius, 20);
  const explicit = core.applyCastleCopy(descriptor, draft, preview, core.buildCopySelection(preview, core.withField(input, 'enabled', true)));
  assert.equal(explicit[3].enabled, true, 'only an explicit include copies it');
});

test('a custom destination value is kept unless explicitly included, per destination and field', () => {
  const draft = { 1: { radius: 20, items: [item(7)], enabled: false }, 4: { radius: 33, items: [item(9)], enabled: false } };
  const preview = core.previewCastleCopy(descriptor, draft, '1', ['4'], context);
  let input = core.withDestination(core.defaultCopyInput(descriptor, preview), '4', true);
  let next = core.applyCastleCopy(descriptor, draft, preview, core.buildCopySelection(preview, input));
  assert.equal(next, draft, 'nothing differs by default, so nothing is written (same reference)');
  input = core.withKeptInclude(input, '4', 'radius', true);
  next = core.applyCastleCopy(descriptor, draft, preview, core.buildCopySelection(preview, input));
  assert.equal(next[4].radius, 20);
  assert.deepEqual(next[4].items, [item(9)], 'the other custom value is still kept');
});

test('cancel and an empty selection change nothing; apply writes only the reviewed pairs and leaves the rest by reference', () => {
  const draft = { 1: { radius: 20, items: [item(7)], enabled: false }, 2: { radius: 10, items: [], enabled: false }, 3: { radius: 10, items: [], enabled: false } };
  const before = JSON.stringify(draft);
  const preview = core.previewCastleCopy(descriptor, draft, '1', ['2', '3'], context);
  assert.equal(core.applyCastleCopy(descriptor, draft, preview, {}), draft);
  assert.equal(core.applyCastleCopy(descriptor, draft, preview, { 2: new Set() }), draft);
  const next = core.applyCastleCopy(descriptor, draft, preview, { 2: new Set(['radius']) });
  assert.equal(next[2].radius, 20);
  assert.deepEqual(next[2].items, [], 'a field outside the selection is untouched');
  assert.equal(next[3], draft[3], 'a destination outside the selection is the same object');
  assert.equal(next[1], draft[1]);
  assert.equal(JSON.stringify(draft), before, 'the input draft is never mutated');
});

test('apply is idempotent: a retry after a failed Save re-applies nothing new', () => {
  const draft = { 1: { radius: 20, items: [item(7)], enabled: false }, 2: { radius: 10, items: [], enabled: false } };
  const preview = core.previewCastleCopy(descriptor, draft, '1', ['2'], context);
  const selection = core.buildCopySelection(preview, core.withDestination(core.defaultCopyInput(descriptor, preview), '2', true));
  const once = core.applyCastleCopy(descriptor, draft, preview, selection);
  const twice = core.applyCastleCopy(descriptor, once, core.previewCastleCopy(descriptor, once, '1', ['2'], context), selection);
  assert.deepEqual(twice, once);
  assert.equal(core.applyCastleCopy(descriptor, once, preview, selection)[2].items.length, 1, 'even a stale preview writes values, not increments');
});

test('destination states: unknown and unavailable need an explicit include; incompatible can never be applied', () => {
  const reason = (state, extra = {}) => ({ id: state, state, messageKey: 'castleCopy.reason.unobserved', ...extra });
  const draft = { 1: { radius: 20, items: [item(7)], enabled: false } };
  const ctx = { candidates: [castle('1'), castle('2', [reason('unknown')]), castle('3', [reason('unavailable')]), castle('4', [reason('incompatible')]), castle('5')] };
  const preview = core.previewCastleCopy(descriptor, draft, '1', ['2', '3', '4', '5'], ctx);
  assert.deepEqual(preview.destinations.map((entry) => entry.state), ['unknown', 'unavailable', 'incompatible', 'compatible']);
  const input = core.defaultCopyInput(descriptor, preview);
  assert.deepEqual([...input.destinations], ['5'], 'only compatible destinations are ticked by default');
  assert.deepEqual([...core.withAllCompatible(core.withDestination(input, '2', false), preview).destinations], ['5'], 'Select all compatible never selects the others');
  const forced = core.buildCopySelection(preview, { ...input, destinations: new Set(['2', '3', '4', '5']) });
  assert.deepEqual(Object.keys(forced).sort(), ['2', '3', '5'], 'an incompatible destination is never selected');
  const next = core.applyCastleCopy(descriptor, draft, preview, { 4: new Set(['radius']), 2: new Set(['radius']) });
  assert.equal(next[4], undefined, 'incompatible: refused even if forced into the selection');
  assert.equal(next[2].radius, 20, 'unknown: allowed when explicitly included');
  assert.equal(core.canIncludeDestination(preview.destinations[2]), false);
});

test('entries the destination cannot take are dropped and named, never substituted; all dropped is incompatible', () => {
  const partial = (ids) => ({ id: 'drop', state: 'incompatible', messageKey: 'castleCopy.reason.unknownItem', fieldId: 'items', dropEntryIds: ids });
  const draft = { 1: { radius: 20, items: [item(7), item(8)], enabled: false } };
  const ctx = { candidates: [castle('1'), castle('2', [partial([8])]), castle('3', [partial([7, 8])])] };
  const preview = core.previewCastleCopy(descriptor, draft, '1', ['2', '3'], ctx);
  const [some, all] = preview.destinations;
  assert.equal(some.state, 'unavailable', 'a reduced list is includable with an explicit include');
  assert.deepEqual(some.changes.find((change) => change.fieldId === 'items').to, [item(7)]);
  assert.deepEqual(some.changes.find((change) => change.fieldId === 'items').dropped, [8]);
  assert.equal(all.state, 'incompatible', 'nothing is left to copy');
  const next = core.applyCastleCopy(descriptor, draft, preview, { 2: new Set(['items']) });
  assert.deepEqual(next[2].items, [item(7)]);
  for (const entry of next[2].items) assert.ok([7, 8].includes(entry.id), 'no unit the source did not name');
});

test('a source without setup has nothing to copy', () => {
  const draft = { 1: { radius: 10, items: [], enabled: false } };
  const preview = core.previewCastleCopy(descriptor, draft, '1', ['2'], context);
  assert.equal(preview.sourceConfigured, false);
  assert.equal(preview.destinations[0].changes.length, 0);
  assert.equal(core.applyCastleCopy(descriptor, draft, preview, core.buildCopySelection(preview, core.defaultCopyInput(descriptor, preview))), draft);
  assert.deepEqual(core.configuredSources(descriptor, { 1: { radius: 20, items: [], enabled: false }, 2: { radius: 10, items: [], enabled: false } }, context).map((entry) => entry.key), ['1']);
});

test('selection edits are pure and reason states map to readiness states', () => {
  const input = { destinations: new Set(['1']), fields: new Set(['radius']), keptIncludes: {} };
  const next = core.withKeptInclude(core.withField(core.withDestination(input, '2', true), 'items', true), '2', 'radius', true);
  assert.deepEqual([...input.destinations], ['1'], 'the original is untouched');
  assert.deepEqual([...next.destinations].sort(), ['1', '2']);
  assert.ok(next.fields.has('items') && next.keptIncludes[2].has('radius'));
  assert.equal(core.reasonAsCheckState('incompatible'), 'blocked');
  assert.equal(core.reasonAsCheckState('unavailable'), 'pending');
  assert.equal(core.reasonAsCheckState('unknown'), 'unavailable');
});
