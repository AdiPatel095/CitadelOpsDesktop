import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const root = fileURLToPath(new URL('..', import.meta.url));
const source = existsSync(`${root}/src/commandCenter`) ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true, hmr: false } });
after(() => vite.close());
const { attentionEntries, prioritySignal } = await vite.ssrLoadModule(`${source}/components/header/headerStatus.ts`);
const { AUTOMATION_FEATURE_ORDER } = await vite.ssrLoadModule(`${source}/settings/automationFeatureNames.ts`);
const { automationPlayerStatus } = await vite.ssrLoadModule(`${source}/settings/readiness/playerStatus.ts`);

const ORDER = [
  'autoTowers', 'autoFortress', 'autoInvasion', 'autoNomad', 'autoAdvisor', 'autoKhan',
  'autoBeriWorld', 'autoStorm', 'autoRecruit', 'autoTool', 'autoSceatRes', 'autoTCI',
  'autoFoodBalance', 'autoBooster', 'autoBuyer', 'autoEquipmentCleanup', 'autoHospital',
  'autoStation', 'autoBird',
];
const STATUSES = ['running', 'waiting', 'done', 'paused', 'blocked', 'needs-attention', 'off', 'unknown'];
const NOW = Date.parse('2026-10-01T12:00:00Z');
const reason = Object.freeze({ key: 'example.reason', params: Object.freeze({ count: 2 }), fallback: 'Example reason' });
const entry = (featureId, status) => ({ featureId, status, reason });
const input = (extra = {}) => ({
  now: NOW,
  station: { enabled: false, threatCount: 0, nextImpactAt: 0, status: 'off' },
  bird: { enabled: false, nextWakeAt: 0, nextCastleName: '' },
  features: [],
  ...extra,
});
const bird = { enabled: true, nextWakeAt: NOW + 90_000, nextCastleName: 'Castle A' };
const station = { enabled: true, threatCount: 3, nextImpactAt: NOW + 45_000, status: 'waiting' };

test('pins the complete Automation page feature order', () => {
  assert.deepEqual(AUTOMATION_FEATURE_ORDER, ORDER);
  assert.equal(new Set(AUTOMATION_FEATURE_ORDER).size, 19);
});
test('empty attention list returns a new empty array', () => {
  const features = [];
  assert.deepEqual(attentionEntries(features), []);
  assert.notEqual(attentionEntries(features), features);
});
for (const status of STATUSES) test(`attention includes only actionable statuses: ${status}`, () => {
  const feature = entry('autoNomad', status);
  assert.deepEqual(attentionEntries([feature]), ['needs-attention', 'blocked', 'paused'].includes(status) ? [feature] : []);
});
for (const status of ['needs-attention', 'blocked', 'paused']) test(`${status} ties follow the entire pinned page order`, () => {
  const features = ORDER.toReversed().map((id) => entry(id, status));
  assert.deepEqual(attentionEntries(features).map((f) => f.featureId), ORDER);
});
test('severity beats feature order and non-attention entries are excluded', () => {
  const features = [entry('autoTowers', 'paused'), entry('autoBird', 'needs-attention'), entry('autoNomad', 'waiting'), entry('autoRecruit', 'blocked'), entry('autoStation', 'needs-attention')];
  assert.deepEqual(attentionEntries(features), [features[4], features[1], features[3], features[0]]);
});
test('unknown ids follow known ids and sort by id, still within severity', () => {
  const features = [entry('futureZ', 'blocked'), entry('autoBird', 'blocked'), entry('futureA', 'blocked'), entry('autoTowers', 'paused'), entry('futureError', 'needs-attention')];
  assert.deepEqual(attentionEntries(features).map((f) => f.featureId), ['futureError', 'autoBird', 'futureA', 'futureZ', 'autoTowers']);
});
test('same-id ties preserve entries, reasons and stable input order without mutation', () => {
  const first = Object.freeze(entry('autoNomad', 'paused'));
  const second = Object.freeze({ ...first, reason: Object.freeze({ key: 'other.reason' }) });
  const features = Object.freeze([second, first]);
  const result = attentionEntries(features);
  assert.deepEqual(result, [second, first]);
  assert.notEqual(result, features);
  assert.equal(result[0], second);
  assert.equal(result[1].reason, reason);
});
test('incoming beats attention and next Bird', () => {
  assert.deepEqual(prioritySignal(input({ station, bird, features: [entry('autoNomad', 'needs-attention')] })), { kind: 'incoming', count: 3, firstImpactInMs: 45_000 });
});
for (const status of STATUSES) test(`Station incoming rule for ${status}`, () => {
  const features = [entry('autoStation', status)];
  const result = prioritySignal(input({ station: { ...station, status }, features, bird }));
  assert.deepEqual(result, ['needs-attention', 'blocked'].includes(status)
    ? { kind: 'attention', count: 1, worst: status }
    : { kind: 'incoming', count: 3, firstImpactInMs: 45_000 });
});
for (const [label, patch] of [['disabled', { enabled: false }], ['zero threats', { threatCount: 0 }], ['negative threats', { threatCount: -1 }]]) {
  test(`Station with ${label} gives way to attention`, () => {
    assert.deepEqual(prioritySignal(input({ station: { ...station, ...patch }, features: [entry('autoBird', 'paused')], bird })), { kind: 'attention', count: 1, worst: 'paused' });
  });
}
for (const [impact, expected] of [[0, null], [NOW + 45_000, 45_000], [NOW, 0], [NOW - 1000, 0]]) {
  test(`incoming impact ${impact} becomes ${expected}`, () => {
    assert.deepEqual(prioritySignal(input({ station: { ...station, nextImpactAt: impact } })), { kind: 'incoming', count: 3, firstImpactInMs: expected });
  });
}
for (const [statuses, worst] of [
  [['paused', 'blocked', 'needs-attention', 'waiting'], 'needs-attention'],
  [['paused', 'blocked', 'off'], 'blocked'],
  [['paused', 'running'], 'paused'],
]) test(`attention beats Bird, counts only actionable entries and selects ${worst}`, () => {
  assert.deepEqual(prioritySignal(input({ bird, features: statuses.map((status, i) => entry(ORDER[i], status)) })), {
    kind: 'attention', count: statuses.filter((status) => ['needs-attention', 'blocked', 'paused'].includes(status)).length, worst,
  });
});
for (const status of ['blocked', 'needs-attention']) test(`unhandled ${status} Station without feature attention gives way to Bird`, () => {
  assert.deepEqual(prioritySignal(input({ station: { ...station, status }, bird })), { kind: 'nextBird', castleName: 'Castle A', dueInMs: 90_000 });
});
for (const [wake, expected] of [[NOW + 90_000, 90_000], [NOW, 0], [NOW - 1000, 0]]) {
  test(`Bird wake ${wake} becomes ${expected}`, () => {
    assert.deepEqual(prioritySignal(input({ bird: { ...bird, nextWakeAt: wake } })), { kind: 'nextBird', castleName: 'Castle A', dueInMs: expected });
  });
}
for (const [label, patch] of [['disabled', { enabled: false }], ['no wake', { nextWakeAt: 0 }], ['invalid negative wake', { nextWakeAt: -1 }]]) {
  test(`Bird with ${label} gives no signal`, () => assert.equal(prioritySignal(input({ bird: { ...bird, ...patch } })), null));
}
test('nothing actionable gives null', () => {
  assert.equal(prioritySignal(input()), null);
  assert.equal(prioritySignal(input({ features: ['running', 'waiting', 'done', 'off', 'unknown'].map((status, i) => entry(ORDER[i], status)) })), null);
});
test('priority calculation leaves frozen inputs and localized reasons untouched', () => {
  const data = Object.freeze(input({ station: Object.freeze({ ...station, enabled: false }), bird: Object.freeze(bird), features: Object.freeze([Object.freeze(entry('autoNomad', 'blocked'))]) }));
  const before = structuredClone(data);
  assert.deepEqual(prioritySignal(data), { kind: 'attention', count: 1, worst: 'blocked' });
  assert.deepEqual(data, before);
  assert.equal(data.features[0].reason, reason);
});
test('PR-B classifies with desktop Lock ignored; Protection Mode and other attention still count', () => {
  const classify = (status, desktopLocked = false) => ({ featureId: 'autoNomad', ...automationPlayerStatus({
    featureId: 'autoNomad', desktopLocked,
    runtime: { id: 'autoNomad', enabled: true, status, updatedAt: new Date(NOW).toISOString() },
    enabled: { configured: true, enabled: true }, context: { connected: true, now: NOW },
  }) });
  assert.equal(classify('waiting', true).status, 'paused');
  assert.equal(prioritySignal(input({ features: [classify('waiting')] })), null);
  for (const [raw, worst] of [['protected', 'paused'], ['blocked', 'blocked'], ['failed', 'needs-attention']]) {
    assert.deepEqual(prioritySignal(input({ features: [classify(raw)] })), { kind: 'attention', count: 1, worst });
  }
});
