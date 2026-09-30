import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const eligibility = await vite.ssrLoadModule('/src/settings/requirements/commanderEligibility.ts');
const movementState = await vite.ssrLoadModule('/src/Movement/types/MovementState.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const NOW = Date.parse('2026-09-29T12:00:00Z');

function commander(id, available = true, equipment = {}) {
  return { id, name: `Commander ${id}`, visiblePosition: id, available, equipment, gems: {} };
}

function gameState({ commanders = [commander(1), commander(2)], snapshot = true, movements = {}, inventory = { equipment: {} } } = {}) {
  return {
    session: { connectionGeneration: 3 },
    player: { id: 9 },
    castles: {},
    commanders: Object.fromEntries(commanders.map((entry) => [String(entry.id), entry])),
    movements,
    movementSnapshot: snapshot
      ? { version: 1, connectionGeneration: 3, observedAt: '2026-09-29T11:59:00Z' }
      : { version: 0, connectionGeneration: 0 },
    inventory,
  };
}

const assignments = (value = {}, requirements = {}) => ({ version: 2, assignments: value, requirements });

function evaluate({ state = gameState(), document = assignments(), loggedIn = true, featureId = 'autoTowers' } = {}) {
  return eligibility.evaluateCommanderEligibility({
    featureId,
    state,
    assignments: document,
    movement: movementState.movementViewFromState(state),
    gameLoggedIn: loggedIn,
    now: NOW,
  });
}

test('no observed commanders is unavailable, never a fabricated choice', () => {
  const report = evaluate({ state: gameState({ commanders: [] }) });
  assert.equal(report.assignment.state, 'unavailable');
  assert.equal(report.activity.state, 'unavailable');
  assert.equal(report.summary.observed, false);
  assert.deepEqual(report.rows, []);
  assert.equal(evaluate({ state: null }).summary.state, 'unavailable');
});

test('the default (no key) allows every commander and free ones are valid', () => {
  const report = evaluate();
  assert.equal(report.implicitAll, true);
  assert.equal(report.assignment.state, 'valid');
  assert.equal(report.assignment.messageKey, 'commanderEligibility.allAllowed');
  assert.equal(report.activity.state, 'valid');
  assert.deepEqual(report.activity.params, { count: 2 });
  assert.equal(report.summary.state, 'valid');
  assert.ok(report.rows.every((row) => row.assigned && row.eligibleNow && row.activity === 'free'));
});

test('an explicit empty assignment blocks with an assignment fix', () => {
  const report = evaluate({ document: assignments({ autoTowers: [] }) });
  assert.equal(report.assignment.state, 'blocked');
  assert.equal(report.assignment.fix, 'assignment');
  assert.equal(report.summary.state, 'blocked');
  assert.ok(report.rows.every((row) => !row.assigned && row.explicitAssignment));
});

test('assigned commanders that are all busy are pending, not blocked', () => {
  const state = gameState({ commanders: [commander(1, false), commander(2)] });
  const report = evaluate({ state, document: assignments({ autoTowers: [1] }) });
  assert.equal(report.assignment.state, 'valid');
  assert.equal(report.activity.state, 'pending');
  assert.equal(report.summary.freeNowCount, 0);
  assert.equal(report.rows.find((row) => row.commanderId === 1).activity, 'busy');
});

test('a stale or missing movement snapshot is unavailable', () => {
  assert.equal(evaluate({ state: gameState({ snapshot: false }) }).activity.state, 'unavailable');
  assert.equal(evaluate({ loggedIn: false }).activity.state, 'unavailable');
  assert.equal(evaluate({ loggedIn: false }).rows[0].activity, 'unknown');
});

test('assigned commanders not meeting the equipment requirement block', () => {
  const document = assignments({ autoTowers: [1] }, { autoTowers: [{ kind: 'equipmentEffect', effectDefinitionId: 77, minimumValue: 10 }] });
  const report = evaluate({ document });
  assert.equal(report.assignment.state, 'blocked');
  assert.equal(report.rows.find((row) => row.commanderId === 1).meetsRequirements, false);
});

test('an unknown requirement kind fails closed with a Commanders fix', () => {
  const document = assignments({}, { autoTowers: [{ kind: 'futureRule' }] });
  const report = evaluate({ document });
  assert.equal(report.requirementsSupported, false);
  assert.equal(report.assignment.state, 'blocked');
  assert.equal(report.assignment.fix, 'assignment');
});

test('assigned ids that are not in this account block (world switch)', () => {
  const report = evaluate({ document: assignments({ autoTowers: [41, 42] }) });
  assert.equal(report.assignment.state, 'blocked');
  assert.equal(report.summary.assignedCount, 0);
});

test('rows disclose other features that list the commander explicitly', () => {
  const report = evaluate({ document: assignments({ autoKhan: [1], autoStorm: [1, 2], autoTowers: [1] }) });
  assert.deepEqual(report.rows.find((row) => row.commanderId === 1).otherFeatures, ['autoKhan', 'autoStorm']);
  assert.deepEqual(report.rows.find((row) => row.commanderId === 2).otherFeatures, ['autoStorm']);
});

test('every check message exists in the catalog', () => {
  for (const report of [evaluate(), evaluate({ state: null }), evaluate({ document: assignments({ autoTowers: [] }) })]) {
    for (const check of [report.assignment, report.activity]) assert.ok(messages[check.messageKey], check.messageKey);
  }
});

test('assignmentImpact lists additions, removals, emptied features and sharing, never removing elsewhere', () => {
  const current = assignments({ autoKhan: [2] });
  const next = assignments({ autoKhan: [2], autoTowers: [2] });
  const impact = eligibility.assignmentImpact(current, next, [1, 2]);
  assert.deepEqual(impact.removed, [{ commanderId: 1, featureId: 'autoTowers' }]);
  assert.deepEqual(impact.added, []);
  const emptied = eligibility.assignmentImpact(assignments({ autoTowers: [1] }), assignments({ autoTowers: [] }), [1, 2]);
  assert.deepEqual(emptied.featuresLeftEmpty, ['autoTowers']);
  const shared = eligibility.assignmentImpact(assignments({ autoTowers: [1], autoKhan: [2] }), assignments({ autoTowers: [1, 2], autoKhan: [2] }), [1, 2]);
  assert.deepEqual(shared.added, [{ commanderId: 2, featureId: 'autoTowers' }]);
  assert.deepEqual(shared.commandersNowShared, [{ commanderId: 2, features: ['autoKhan'] }]);
  assert.equal(eligibility.assignmentImpactIsEmpty(eligibility.assignmentImpact(current, current, [1, 2])), true);
});

for (const activity of ['syncing', 'unknown', 'free', 'outbound', 'busy', 'posted', 'returning']) {
  test(`an assigned commander shows its ${activity} activity`, () => {
    assert.deepEqual(eligibility.commanderRowStatus({ assigned: true, activity }), { kind: 'activity', activity });
  });

  test(`an unassigned commander hides its ${activity} activity`, () => {
    assert.deepEqual(eligibility.commanderRowStatus({ assigned: false, activity }), { kind: 'off' });
  });
}

test('default assignment mode shows activity for every commander', () => {
  const report = evaluate();
  assert.ok(report.rows.length > 0);
  for (const row of report.rows) {
    assert.deepEqual(eligibility.commanderRowStatus(row), { kind: 'activity', activity: row.activity });
  }
});

test('an explicit empty assignment shows every commander as off', () => {
  const report = evaluate({ document: assignments({ autoTowers: [] }) });
  assert.ok(report.rows.length > 0);
  for (const row of report.rows) {
    assert.deepEqual(eligibility.commanderRowStatus(row), { kind: 'off' });
  }
});
