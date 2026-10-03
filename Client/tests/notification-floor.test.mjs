// CIT-58. Shared byte-for-byte between the hosted portal and the desktop client;
// only the LAYOUT constant below differs.
import assert from 'node:assert/strict';
import {existsSync, readFileSync} from 'node:fs';
import {after, mock, test} from 'node:test';
import {fileURLToPath} from 'node:url';
import {createServer} from 'vite';

const LAYOUT = {components: '../src/components/', operations: '../src/api/OperationNotifications.ts', peerEnv: 'CIT_HOSTED_ROOT', peerComponents: 'src/commandCenter/components/'};

const vite = await createServer({root: fileURLToPath(new URL('..', import.meta.url)), appType: 'custom', logLevel: 'silent', server: {middlewareMode: true}});
after(() => vite.close());
const load = (relativePath) => vite.ssrLoadModule(fileURLToPath(new URL(relativePath, import.meta.url)));

const {
  NOTIFICATION_DURATION_MS,
  NOTIFICATION_EXIT_MS,
  NOTIFICATION_MINIMUM_VISIBLE_MS,
  NotificationCenter,
  Notifications,
} = await load(`${LAYOUT.components}Notifications.ts`);
const {OperationFailureNotificationCoordinator} = await load(LAYOUT.operations);

const SECOND = 1000;

/** Deterministic clock and timer set, injected into the center. */
function fakeClock() {
  let now = 1_000_000;
  let sequence = 0;
  const timers = new Map();
  return {
    now: () => now,
    setTimeout: (callback, delayMs) => { const id = ++sequence; timers.set(id, {at: now + delayMs, callback}); return id; },
    clearTimeout: (id) => { timers.delete(id); },
    advance(ms) {
      const target = now + ms;
      for (;;) {
        let next = null;
        for (const [id, timer] of timers) if (timer.at <= target && (!next || timer.at < next.timer.at)) next = {id, timer};
        if (!next) break;
        timers.delete(next.id);
        now = next.timer.at;
        next.timer.callback();
      }
      now = target;
    },
    pending: () => timers.size,
  };
}

function setup(options = {}) {
  const clock = fakeClock();
  const center = new NotificationCenter({...clock, ...options});
  center.attach();
  const live = () => center.visible().filter((item) => !item.exiting);
  return {clock, center, live};
}

test('constants: 30 s default, 10 s floor, floor never above the default', () => {
  assert.equal(NOTIFICATION_DURATION_MS, 30_000);
  assert.equal(NOTIFICATION_MINIMUM_VISIBLE_MS, 10_000);
  assert.ok(NOTIFICATION_MINIMUM_VISIBLE_MS >= 5_000 && NOTIFICATION_MINIMUM_VISIBLE_MS <= NOTIFICATION_DURATION_MS);
});

test('a notification stays for the full duration and is dismissed at 30 s', () => {
  const {clock, center, live} = setup();
  center.warning('Still running.', 'a');
  clock.advance(29_900);
  assert.equal(live().length, 1);
  clock.advance(100);
  assert.equal(live().length, 0);
  assert.equal(center.visible()[0].exiting, true, 'auto dismissal starts the exit animation');
  center.remove('a');
  assert.equal(center.visible().length, 0);
});

test('the minimum visible time wins over a shorter configured duration', () => {
  const {clock, center, live} = setup({durationMs: () => 1_000});
  center.error('Short.', 'a');
  clock.advance(NOTIFICATION_MINIMUM_VISIBLE_MS - 1);
  assert.equal(live().length, 1, 'still visible just before the floor');
  clock.advance(1);
  assert.equal(live().length, 0);
});

test('the default singleton works on the real timer functions', () => {
  mock.timers.enable({apis: ['setTimeout', 'Date']});
  try {
    Notifications.attach();
    const id = Notifications.success('Saved.', 'singleton');
    mock.timers.tick(29_999);
    assert.equal(Notifications.visible().find((item) => item.id === id)?.exiting, false);
    mock.timers.tick(1);
    assert.equal(Notifications.visible().find((item) => item.id === id)?.exiting, true);
    Notifications.remove(id);
    Notifications.detach();
  } finally {
    mock.timers.reset();
  }
});

test('a same-id republish updates in place and extends, never shortens', () => {
  const {clock, center, live} = setup();
  center.warning('Running.', 'operation');
  const first = center.visible()[0];
  clock.advance(1_500);
  center.error('Failed.', 'operation');
  assert.equal(center.visible().length, 1);
  const second = center.visible()[0];
  assert.equal(second.id, 'operation');
  assert.equal(second.message, 'Failed.');
  assert.equal(second.category, 'red');
  assert.ok(second.revision > first.revision);
  assert.equal(second.shownAt, first.shownAt, 'shownAt is kept');
  assert.equal(second.exiting, false);
  clock.advance(29_999);
  assert.equal(live().length, 1, 'still visible 31.5 s after the first publish minus 1 ms');
  clock.advance(1);
  assert.equal(live().length, 0, 'removed 30 s after the update');
});

test('a republish keeps its list position and does not add entries', () => {
  const {center} = setup();
  center.success('one', 'a');
  center.success('two', 'b');
  center.success('one again', 'a');
  assert.deepEqual(center.visible().map((item) => [item.id, item.message]), [['a', 'one again'], ['b', 'two']]);
});

test('an update never shortens a longer remaining countdown', () => {
  const {clock, center, live} = setup({durationMs: (category) => (category === 'red' ? 12_000 : 40_000)});
  center.warning('Long.', 'a');
  clock.advance(1_000);
  center.error('Shorter.', 'a');
  clock.advance(38_999);
  assert.equal(live().length, 1, 'the original 40 s deadline stands');
  clock.advance(1);
  assert.equal(live().length, 0);
});

test('a republish after an automatic dismissal revives the same entry', () => {
  const {clock, center, live} = setup();
  center.warning('First.', 'a');
  clock.advance(30_000);
  assert.equal(center.visible()[0].exiting, true);
  center.warning('Again.', 'a');
  assert.equal(center.visible().length, 1);
  assert.equal(center.visible()[0].exiting, false);
  clock.advance(29_999);
  assert.equal(live().length, 1);
  clock.advance(1);
  assert.equal(live().length, 0);
});

test('persistent notifications never arm; update rules start and stop the countdown', () => {
  const {clock, center, live} = setup();
  center.publish({id: 'p', category: 'yellow', message: 'Needs action.', persistent: true});
  assert.equal(clock.pending(), 0);
  clock.advance(120_000);
  assert.equal(live().length, 1);
  center.publish({id: 'p', category: 'green', message: 'Done.'});
  clock.advance(29_999);
  assert.equal(live().length, 1, 'countdown starts at the update');
  clock.advance(1);
  assert.equal(live().length, 0);

  center.publish({id: 'q', category: 'yellow', message: 'Temporary.'});
  clock.advance(5_000);
  center.publish({id: 'q', category: 'yellow', message: 'Now sticky.', persistent: true});
  assert.equal(clock.pending(), 0, 'becoming persistent disarms');
  clock.advance(120_000);
  assert.equal(live().filter((item) => item.id === 'q').length, 1);
});

test('hover pauses the countdown; overlapping focus keeps it paused', () => {
  const {clock, center, live} = setup();
  center.warning('Read me.', 'a');
  clock.advance(5_000);
  center.pause('a', 'hover');
  clock.advance(5_000);
  center.pause('a', 'focus');
  clock.advance(10_000);
  center.resume('a', 'hover');
  clock.advance(60_000);
  assert.equal(live().length, 1, 'focus still holds it');
  center.resume('a', 'focus');
  clock.advance(24_999);
  assert.equal(live().length, 1, '25 s were left at the first pause');
  clock.advance(1);
  assert.equal(live().length, 0);
});

test('hover for 20 s at 5 s removes it at 50 s', () => {
  const {clock, center, live} = setup();
  center.warning('Read me.', 'a');
  clock.advance(5_000);
  center.pause('a', 'hover');
  clock.advance(20_000);
  center.resume('a', 'hover');
  clock.advance(24_999);
  assert.equal(live().length, 1);
  clock.advance(1);
  assert.equal(live().length, 0);
});

test('an update while paused keeps the notification paused and extends it', () => {
  const {clock, center, live} = setup();
  center.warning('Read me.', 'a');
  clock.advance(25_000);
  center.pause('a', 'hover');
  center.error('Changed.', 'a');
  clock.advance(100_000);
  assert.equal(live().length, 1);
  center.resume('a', 'hover');
  clock.advance(29_999);
  assert.equal(live().length, 1);
  clock.advance(1);
  assert.equal(live().length, 0);
});

test('detach pauses everything; unseen notifications start when a view attaches', () => {
  const {clock, center, live} = setup();
  center.warning('Visible.', 'a');
  clock.advance(10_000);
  center.pause('a', 'hover'); // the element unmounts while hovered
  center.detach();
  clock.advance(60_000);
  assert.equal(live().length, 1, 'a remount gap consumes no visible time');
  center.error('While away.', 'b');
  assert.equal(center.visible().find((item) => item.id === 'b').shownAt, null);
  clock.advance(60_000);
  assert.equal(live().length, 2);
  center.attach();
  const shown = center.visible().find((item) => item.id === 'b');
  assert.equal(shown.shownAt, clock.now());
  clock.advance(19_999);
  assert.equal(live().length, 2, 'a: 20 s left; b: full 30 s');
  clock.advance(1);
  assert.deepEqual(live().map((item) => item.id), ['b']);
  clock.advance(9_999);
  assert.equal(live().length, 1);
  clock.advance(1);
  assert.equal(live().length, 0);
});

test('overlapping views: only the last detach pauses', () => {
  const {clock, center, live} = setup();
  center.attach();
  center.warning('Shared.', 'a');
  center.detach();
  clock.advance(29_999);
  assert.equal(live().length, 1);
  clock.advance(1);
  assert.equal(live().length, 0);
});

test('dismiss marks exiting and disarms; remove deletes; nothing fires afterwards', () => {
  const {clock, center, live} = setup();
  center.warning('Bye.', 'a');
  center.dismiss('a');
  assert.equal(center.visible()[0].exiting, true);
  assert.equal(clock.pending(), 0);
  assert.equal(live().length, 0);
  center.remove('a');
  assert.equal(center.visible().length, 0);
  clock.advance(120_000);
  assert.equal(center.visible().length, 0);
  assert.equal(NOTIFICATION_EXIT_MS, 300);
});

test('a manual dismissal is immediate, even inside the minimum visible time', () => {
  const {clock, center, live} = setup();
  center.warning('Bye.', 'a');
  clock.advance(100);
  center.dismiss('a');
  assert.equal(live().length, 0);
});

test('the visible snapshot keeps its identity until something changes', () => {
  const {clock, center} = setup();
  const empty = center.visible();
  assert.equal(center.visible(), empty);
  let notified = 0;
  const unsubscribe = center.subscribeVisible(() => { notified += 1; });
  center.warning('One.', 'a');
  const one = center.visible();
  assert.notEqual(one, empty);
  assert.equal(center.visible(), one);
  center.pause('a', 'hover');
  center.resume('a', 'hover');
  assert.equal(center.visible(), one, 'pause and resume are not visible changes');
  clock.advance(30_000);
  assert.notEqual(center.visible(), one);
  unsubscribe();
  const before = notified;
  center.warning('Two.', 'b');
  assert.equal(notified, before);
});

test('setScope clears only when the scope changes', () => {
  const {center} = setup();
  center.setScope('A');
  center.warning('Account A.', 'a');
  center.setScope('A');
  assert.equal(center.visible().length, 1, 'same scope keeps the list');
  center.setScope('B');
  assert.equal(center.visible().length, 0, 'a different scope clears without an exit animation');
  center.warning('Account B.', 'b');
  center.setScope('A');
  assert.equal(center.visible().length, 0);
});

test('the first scope adopts existing notifications', () => {
  const {center} = setup();
  center.warning('Before scope.', 'a');
  center.setScope('A');
  assert.equal(center.visible().length, 1);
});

test('the legacy subscribe listener still receives each publication', () => {
  const {center} = setup();
  const observed = [];
  center.subscribe((notification) => observed.push(notification));
  center.warning('One.', 'a');
  center.error('Two.', 'a');
  assert.equal(observed.length, 2);
  assert.notEqual(observed[0].revision, observed[1].revision);
});

test('an operation sequence on one preferred id stays readable for at least the minimum', () => {
  const {clock, center, live} = setup({durationMs: () => 1_000});
  const coordinator = new OperationFailureNotificationCoordinator();
  const failed = (id) => ({
    id, intent: 'storm.attack', actor: 'ui', priority: 100, status: 'failed', submittedAt: '2026-08-31T12:00:00Z',
    plan: {intent: 'storm.attack', effect: 'launch', stateRevision: 1, steps: [], summary: 'Apply preset'},
    failure: {kind: 'internal', message: 'Could not complete this action.', explanation: 'The action did not complete.', severity: 'error', toast: true},
  });
  const first = coordinator.next(failed('operation-1'), 'decoration-apply');
  assert.ok(first);
  center.publish(first);
  clock.advance(2_000);
  const second = coordinator.next(failed('operation-2'), 'decoration-apply');
  assert.ok(second);
  center.publish(second);
  assert.equal(center.visible().length, 1);
  clock.advance(NOTIFICATION_MINIMUM_VISIBLE_MS - 2_000 - 1);
  assert.equal(live().length, 1, 'the floor counts from the first appearance');
  clock.advance(1);
  assert.equal(live().length, 0);
});

test('Alerts renders from the store, keyed by id, with pause handlers and no component countdown', () => {
  const source = readFileSync(new URL(`${LAYOUT.components}Alerts.tsx`, import.meta.url), 'utf8');
  assert.match(source, /useSyncExternalStore\(Notifications\.subscribeVisible/);
  assert.match(source, /key=\{alert\.id\}/);
  assert.doesNotMatch(source, /key=\{`\$\{alert\.id\}-/);
  assert.match(source, /Notifications\.attach\(\)/);
  assert.match(source, /Notifications\.detach\(\)/);
  for (const handler of ['onMouseEnter', 'onMouseLeave', 'onFocus', 'onBlur']) assert.match(source, new RegExp(`${handler}=`));
  assert.match(source, /Notifications\.pause\(id, 'hover'\)/);
  assert.match(source, /Notifications\.resume\(id, 'focus'\)/);
  assert.match(source, /Notifications\.dismiss\(id\)/);
  assert.doesNotMatch(source, /setTimeout\(handleDismiss/);
  assert.doesNotMatch(source, /notificationDurationMs/);
  assert.match(source, /ui\.components\.alerts\.aria-label\.dismiss\.48845bff/);
});

test('Notifications.ts and Alerts.tsx match the other repository when it is available', (t) => {
  const peerRoot = process.env[LAYOUT.peerEnv];
  if (!peerRoot) return t.skip(`${LAYOUT.peerEnv} is not set; parity is checked with the diff commands in the PR description`);
  for (const name of ['Notifications.ts', 'Alerts.tsx']) {
    const peer = `${peerRoot}/${LAYOUT.peerComponents}${name}`;
    assert.ok(existsSync(peer), `${peer} exists`);
    assert.equal(readFileSync(new URL(`${LAYOUT.components}${name}`, import.meta.url), 'utf8'), readFileSync(peer, 'utf8'), `${name} differs from ${peer}`);
  }
});
