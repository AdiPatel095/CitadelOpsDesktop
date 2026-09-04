import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

async function importTypeScript(relativePath) {
  const sourceUrl = new URL(relativePath, import.meta.url);
  const source = await readFile(sourceUrl, 'utf8');
  const compiled = ts.transpileModule(source, {
    compilerOptions: {
      module: ts.ModuleKind.ES2022,
      target: ts.ScriptTarget.ES2022,
    },
    fileName: sourceUrl.pathname,
  });
  const compiledUrl = `data:text/javascript;base64,${Buffer.from(compiled.outputText).toString('base64')}`;
  return import(compiledUrl);
}

const {
  OperationFailureNotificationCoordinator,
  isOperationFailureStatus,
  operationFailureNotification,
  operationFailureText,
} = await importTypeScript('../src/api/OperationNotifications.ts');
const { operationFailureReceiptFromHTTP } = await importTypeScript('../src/api/OperationHTTPFailure.ts');
const notificationModule = await importTypeScript('../src/components/Notifications.ts');
const {
  NOTIFICATION_DURATION_MS,
  Notifications,
  notificationDurationMs,
} = notificationModule;

const receipt = (overrides = {}) => ({
  id: 'operation-1',
  intent: 'storm.attack',
  actor: 'ui',
  priority: 100,
  status: 'failed',
  submittedAt: '2026-08-31T12:00:00Z',
  plan: { intent: 'storm.attack', effect: 'launch', stateRevision: 1, steps: [], summary: 'Attack Storm fort at 500:500' },
  ...overrides,
});

test('renders official game knowledge as explanation, recovery, and a quiet reference', () => {
  const notification = operationFailureNotification(receipt({
    failure: {
      kind: 'availability',
      message: 'Could not complete “Attack Storm fort at 500:500”.',
      explanation: 'This target is still on cooldown.',
      recovery: 'Wait for the cooldown or refresh the world map.',
      severity: 'warning',
      gameCode: 95,
      knowledge: 'official',
      toast: true,
    },
  }));
  assert.equal(notification.category, 'yellow');
  assert.equal(notification.message, 'Could not complete “Attack Storm fort at 500:500”.');
  assert.deepEqual(notification.lines, [
    'The game says: This target is still on cooldown.',
    'Wait for the cooldown or refresh the world map.',
    'Game error 95.',
  ]);
});

test('keeps expected automation gates on feature lane status', () => {
  const structured = receipt({
    actor: 'automation:autoStorm',
    failure: {
      kind: 'availability', message: 'Attack was not sent.',
      explanation: 'No eligible commander is available right now.',
      severity: 'warning', toast: false,
    },
  });
  assert.equal(operationFailureNotification(structured), null);
  assert.match(operationFailureText(structured), /No eligible commander/);
  assert.match(operationFailureNotification({ ...structured, status: 'partially_succeeded' }).message, /Attack was not sent/);

  assert.equal(operationFailureNotification(receipt({
    actor: 'automation:autoTowers',
    error: 'intent plan became stale: no commander is currently available',
  })), null);
  assert.equal(operationFailureNotification(receipt({
    actor: 'automation:autoStorm',
    error: 'castle 12 has 20 of item 30; 1 commander(s) require 40',
  })), null);
  for (const error of [
    'intent plan became stale: commander 42 is no longer available',
    'no assigned commander supports the required maiden relic',
    'no assigned Auto Towers commander is in the current roster',
  ]) {
    assert.equal(operationFailureNotification(receipt({ actor: 'automation:autoTowers', error })), null);
  }
});

test('still explains an interactive commander gate', () => {
  const notification = operationFailureNotification(receipt({
    error: 'no commander is currently available',
  }));
  assert.equal(notification.category, 'yellow');
  assert.match(notification.lines.join(' '), /No eligible commander/);
  assert.doesNotMatch(notification.lines.join(' '), /opcode|CRA/);
});

test('hedges observed game behavior and suppresses a routine automated CRA 256 rejection', () => {
  const raw = 'Build and launch attack: response code 256 for CRA was not successful: The selected commander is already assigned to an active movement. (inferred from captures)';
  const interactive = operationFailureNotification(receipt({ error: raw }));
  assert.equal(interactive.category, 'yellow');
  assert.match(interactive.lines[0], /^Based on observed game behavior:/);
  assert.match(interactive.lines.join(' '), /Automated combat pauses/);
  assert.match(interactive.lines.at(-1), /Game error 256/);
  assert.doesNotMatch(interactive.message, /CRA|response code/);

  assert.equal(operationFailureNotification(receipt({ actor: 'automation:autoStorm', error: raw })), null);

  const partial = operationFailureNotification(receipt({
    actor: 'automation:autoStorm', status: 'partially_succeeded', error: raw,
  }));
  assert.match(partial.message, /completed only in part/);
});

test('shows shared CRA 91 incompatible-preset guidance for every attack lane', () => {
  const raw = 'Build and launch attack: response code 91 for CRA was not successful: The selected attack preset has incompatible tools assigned for this attack. (inferred from captures)';
  for (const actor of ['automation:autoNomad', 'automation:autoStorm', 'ui']) {
    const notification = operationFailureNotification(receipt({ actor, error: raw }));
    assert.equal(notification.category, 'red');
    assert.match(notification.lines[0], /^Based on observed game behavior:/);
    assert.match(notification.lines.join(' '), /incompatible tools/);
    assert.match(notification.lines.join(' '), /Remove or replace/);
    assert.match(notification.lines.at(-1), /Game error 91/);
  }
});

test('does not invent a cause for an undocumented response', () => {
  const notification = operationFailureNotification(receipt({
    actor: 'automation:autoRecruit',
    error: 'Request alliance help: response code 269 for AHR was not successful: The game does not provide a known description for this response code. (undocumented)',
  }));
  assert.equal(notification.category, 'red');
  assert.match(notification.lines.join(' '), /does not provide a known explanation/);
  assert.match(notification.lines.join(' '), /Game error 269/);
  assert.doesNotMatch(notification.lines.join(' '), /help limit|cooldown|castle focus/);
});

test('translates transport and uncertainty failures without protocol wording', () => {
  const timeout = operationFailureNotification(receipt({ error: 'Build attack: timed out waiting for cra' }));
  assert.equal(timeout.category, 'yellow');
  assert.match(timeout.lines.join(' '), /did not confirm the action in time/);
  assert.doesNotMatch(timeout.lines.join(' '), /cra/i);
	const deadline = operationFailureNotification(receipt({ error: 'context deadline exceeded' }));
	assert.equal(deadline.category, 'yellow');
	assert.match(deadline.lines.join(' '), /did not confirm the action in time/);

  const indeterminate = operationFailureNotification(receipt({
    status: 'indeterminate',
    plan: { intent: 'resource.ship', effect: 'write', stateRevision: 1, steps: [], summary: 'Send food to Tycho' },
    error: 'outbound effect outcome is indeterminate',
  }));
  assert.match(indeterminate.message, /could not confirm whether/);
  assert.match(indeterminate.lines.join(' '), /not duplicated/);
});

test('does not hide a legacy expected game code joined to an internal commit failure', () => {
  const raw = 'response code 256 for CRA was not successful: Commander busy. (inferred from captures)\ncommit earlier acknowledged response: disk unavailable';
  const notification = operationFailureNotification(receipt({ actor: 'automation:autoStorm', error: raw }));
  assert.equal(notification.category, 'red');
  assert.match(notification.lines.join(' '), /could not be applied safely/);
  assert.match(notification.lines.join(' '), /Game error 256/);
});

test('sanitizes internal observer and action-registry failures from legacy runtimes', () => {
  for (const error of [
    'committed wire response observer is unavailable',
    'action "storm.scan.burst" is not registered',
  ]) {
    const notification = operationFailureNotification(receipt({ error }));
    assert.equal(notification.category, 'red');
    assert.match(notification.lines.join(' '), /internal app error/);
    assert.doesNotMatch(notification.lines.join(' '), /observer|registered/);
  }
});

test('recognizes an immediate terminal HTTP 422 as an operation failure receipt', () => {
  const failed = receipt({
    failure: {
      kind: 'internal', message: 'Could not start this action.',
      explanation: 'The app could not reserve the required game controls.',
      severity: 'error', toast: true,
    },
  });
  assert.equal(operationFailureReceiptFromHTTP(422, failed), failed);
  assert.equal(operationFailureReceiptFromHTTP(500, failed), undefined);
  assert.equal(operationFailureReceiptFromHTTP(422, { error: { message: 'bad request' } }), undefined);
});

test('only terminal failure outcomes produce failure notifications', () => {
  for (const status of ['failed', 'partially_succeeded', 'indeterminate']) {
    assert.equal(isOperationFailureStatus(status), true);
  }
  for (const status of ['running', 'succeeded', 'cancelled']) {
    assert.equal(isOperationFailureStatus(status), false);
    assert.equal(operationFailureNotification(receipt({ status })), null);
  }
});

test('publishes one terminal failure and reuses a feature progress toast ID', () => {
  const coordinator = new OperationFailureNotificationCoordinator();
  const failed = receipt({
    failure: {
      kind: 'internal', message: 'Could not complete this action.',
      explanation: 'The action did not complete.', severity: 'error', toast: true,
    },
  });
  assert.equal(coordinator.next(failed, 'defense-preset-apply').id, 'defense-preset-apply');
  assert.equal(coordinator.next(failed, 'defense-preset-apply'), null);
  assert.equal(coordinator.next({ ...failed, id: 'operation-2' }).id, 'operation-operation-2');
});

test('same-ID toast replacements receive a new rendering revision', () => {
  const observed = [];
  const unsubscribe = Notifications.subscribe((notification) => observed.push(notification));
  Notifications.warning('Still running.', 'replace-me');
  Notifications.error('Could not complete.', 'replace-me');
  unsubscribe();
  assert.equal(observed.length, 2);
  assert.equal(observed[0].id, observed[1].id);
  assert.notEqual(observed[0].revision, observed[1].revision);
});

test('all non-persistent toasts remain for 30 seconds', () => {
  assert.equal(NOTIFICATION_DURATION_MS, 30_000);
  assert.equal(notificationDurationMs('yellow'), 30_000);
  assert.equal(notificationDurationMs('red'), 30_000);
  assert.equal(notificationDurationMs('green'), 30_000);
});
