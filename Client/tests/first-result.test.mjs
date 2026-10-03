import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const first = await vite.ssrLoadModule('/src/settings/readiness/firstResult.ts');
const attribution = await vite.ssrLoadModule('/src/settings/readiness/automationAttribution.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const SINCE = '2026-09-29T10:00:00Z';
const receipt = (id, extra = {}) => ({
  id, intent: 'attack.launch', actor: 'automation:autoNomad', priority: 1, status: 'succeeded', phase: 'completed',
  plan: { intent: 'attack.launch', effect: 'launch', stateRevision: 1, steps: [], summary: 'Attack camp 12' },
  completedStepIndexes: [0], submittedAt: '2026-09-29T10:05:00Z', completedAt: '2026-09-29T10:06:00Z', ...extra,
});
const result = (operations, extra = {}) => first.firstConfirmedResult('autoNomad', { operations: Object.fromEntries(operations.map((operation) => [operation.id, operation])), accountKey: '1:2', enabledSince: SINCE, ...extra });

test('attribution rule: the actor is automation:<feature id> for every automation feature, lanes included', () => {
  for (const featureId of ['autoNomad', 'autoInvasion', 'autoKhan', 'autoBeriWorld', 'autoTowers', 'autoFortress', 'autoStorm', 'autoFoodBalance', 'autoStation', 'autoBird', 'autoRecruit', 'autoTool', 'autoHospital', 'autoTCI', 'autoSceatRes', 'autoBooster', 'autoBuyer', 'autoAdvisor', 'autoEquipmentCleanup']) {
    assert.equal(attribution.automationActor(featureId), `automation:${featureId}`);
  }
  assert.equal(attribution.automationActor('somethingElse'), null);
  const operations = {
    a: receipt('a'),
    ui: receipt('ui', { actor: 'ui' }),
    other: receipt('other', { actor: 'automation:autoTowers' }),
    scheduler: receipt('scheduler', { actor: 'scheduler:rift' }),
  };
  assert.deepEqual(attribution.attributedReceipts('autoNomad', operations).map((entry) => entry.id), ['a']);
});

test('a succeeded, completed, attributed game action after turning on is the first confirmed result', () => {
  const confirmed = result([receipt('a')]);
  assert.equal(confirmed.state, 'confirmed');
  assert.equal(confirmed.receiptId, 'a');
  assert.equal(confirmed.summary, 'Attack camp 12');
  assert.equal(confirmed.sinceKnown, true);
  assert.equal(confirmed.params.what, 'Attack camp 12');
  assert.equal(confirmed.params.completedAt, Date.parse('2026-09-29T10:06:00Z'));
  assert.equal(confirmed.params.scope, 'plain');
  assert.equal(result([receipt('a')], { accountLabel: 'Player One' }).params.scope, 'account', 'hosted names the account');
  const earlier = result([receipt('late', { completedAt: '2026-09-29T11:00:00Z', submittedAt: '2026-09-29T10:59:00Z' }), receipt('early')]);
  assert.equal(earlier.receiptId, 'early', 'the earliest confirmation is the first');
});

test('nothing but a real receipt confirms: counters, ready status, enabling and saving never do', () => {
  const none = result([], {
    launchRates: { launchesByFeature: { autoNomad: 12 } },
    runtime: { id: 'autoNomad', enabled: true, status: 'ready', updatedAt: '2026-09-29T10:10:00Z', lastRunAt: '2026-09-29T10:09:00Z', lastOperationId: 'x' },
  });
  assert.equal(none.state, 'none');
  assert.equal(none.counter, 12, 'the counter is reported as context only');
  assert.equal(none.summaryKey, 'firstResult.none');
  for (const status of ['running', 'ready', 'complete']) {
    assert.equal(result([], { runtime: { id: 'autoNomad', enabled: true, status, updatedAt: SINCE } }).state, 'none', status);
  }
});

test('reads, configuration follow-ups, dry runs and other actors are not the first action', () => {
  assert.equal(result([receipt('read', { plan: { intent: 'map.scan', effect: 'read', stateRevision: 1, steps: [] } })]).state, 'none');
  assert.equal(result([receipt('cfg', { intent: 'config.update' })]).state, 'none');
  assert.equal(result([receipt('noplan', { plan: undefined })]).state, 'none');
  assert.equal(result([receipt('dry', { status: 'planned' })]).state, 'none');
  assert.equal(result([receipt('ui', { actor: 'ui' })]).state, 'none');
  assert.equal(result([receipt('other-feature', { actor: 'automation:autoInvasion' })]).state, 'none');
  assert.equal(result([receipt('cancelled', { status: 'cancelled' })]).state, 'none', 'a cancelled action is neither a result nor a failure of the feature');
  assert.equal(result([receipt('nophase', { phase: 'awaiting_response' })]).state, 'none', 'a success without a completed phase is not confirmed');
  assert.equal(result([receipt('noevidence', { completedStepIndexes: [], evidence: [], exchanges: [] })]).state, 'none', 'no executed-step evidence, no confirmation');
  assert.equal(result([receipt('nodate', { completedAt: undefined })]).state, 'none');
});

test('receipts from before the automation was turned on are ignored; unknown turn-on time is reported', () => {
  const old = receipt('old', { submittedAt: '2026-09-29T09:00:00Z', completedAt: '2026-09-29T09:01:00Z' });
  assert.equal(result([old]).state, 'none');
  const unknown = result([old], { enabledSince: undefined });
  assert.equal(unknown.state, 'confirmed');
  assert.equal(unknown.sinceKnown, false, 'the card says the turn-on time is not known');
});

test('in progress and failed states come from attributed receipts and reuse the failure presentation', () => {
  const running = result([receipt('run', { status: 'running', phase: 'sent', completedAt: undefined })]);
  assert.equal(running.state, 'in-progress');
  assert.equal(running.receiptId, 'run');
  const failed = result([receipt('bad', { status: 'failed', phase: 'completed', error: 'boom', failure: { kind: 'game_rejected', message: 'The game rejected it', explanation: '', severity: 'error', toast: false, messageDescriptor: { key: 'x', fallback: 'The game rejected it' } } })]);
  assert.equal(failed.state, 'failed');
  assert.equal(failed.outcome, 'failed');
  assert.equal(failed.failure.text, 'The game rejected it');
  assert.equal(failed.summaryKey, 'firstResult.failed');
  assert.equal(result([receipt('bad2', { status: 'failed', failure: undefined, error: undefined })]).summaryKey, 'firstResult.failedNoReason');
  assert.equal(result([receipt('p', { status: 'partially_succeeded' })]).outcome, 'partial');
  assert.equal(result([receipt('i', { status: 'indeterminate' })]).outcome, 'unconfirmed');
  const both = result([receipt('bad', { status: 'failed' }), receipt('good', { submittedAt: '2026-09-29T10:20:00Z', completedAt: '2026-09-29T10:21:00Z' })]);
  assert.equal(both.state, 'confirmed', 'a later confirmed action wins over an earlier failed attempt');
});

test('a feature that cannot be attributed says so and never reads as success', () => {
  const unknown = first.firstConfirmedResult('notAFeature', { operations: { a: receipt('a', { actor: 'automation:notAFeature' }) }, accountKey: '1:2', enabledSince: SINCE });
  assert.equal(unknown.state, 'unattributable');
  assert.equal(unknown.summaryKey, 'firstResult.unattributable');
});

test('turn-on times are stored per account and never leak to another account', () => {
  const values = new Map();
  const storage = { getItem: (key) => values.get(key) ?? null, setItem: (key, value) => values.set(key, value) };
  first.recordEnabledSince(storage, '1:2', 'auto_nomad', '2026-09-29T10:00:00Z');
  first.recordEnabledSince(storage, '3:4', 'auto_nomad', '2026-09-29T11:00:00Z');
  assert.deepEqual(first.readEnabledSince(storage, '1:2'), { auto_nomad: '2026-09-29T10:00:00Z' });
  assert.deepEqual(first.readEnabledSince(storage, '3:4'), { auto_nomad: '2026-09-29T11:00:00Z' });
  assert.deepEqual(first.readEnabledSince(storage, '9:9'), {});
  assert.deepEqual(first.readEnabledSince(storage, ''), {}, 'no account, no times');
  assert.deepEqual(first.clearEnabledSince(storage, '1:2', 'auto_nomad'), {});
  assert.deepEqual(first.readEnabledSince(storage, '3:4'), { auto_nomad: '2026-09-29T11:00:00Z' });
  assert.deepEqual(first.readEnabledSince({ getItem: () => { throw new Error('blocked'); }, setItem: () => { throw new Error('blocked'); } }, '1:2'), {}, 'unavailable storage falls back to nothing');
  assert.doesNotThrow(() => first.recordEnabledSince({ getItem: () => null, setItem: () => { throw new Error('quota'); } }, '1:2', 'auto_nomad', SINCE));
});

test('every first-result message exists', () => {
  const source = ['firstResult.none', 'firstResult.confirmed', 'firstResult.inProgress', 'firstResult.failed', 'firstResult.failedNoReason', 'firstResult.unattributable', 'firstResult.counterOnly', 'firstResult.sinceUnknown'];
  for (const key of source) assert.ok(messages[key], key);
  assert.doesNotMatch(messages['firstResult.none'], /success|ready to go|all set/i, 'the empty state claims nothing');
});
