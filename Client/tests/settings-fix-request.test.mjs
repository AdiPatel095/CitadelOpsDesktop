import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { after, beforeEach, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const fix = await vite.ssrLoadModule('/src/settings/readiness/settingsFixRequest.ts');
const focus = await vite.ssrLoadModule('/src/settings/readiness/focusReadinessTarget.ts');

after(async () => {
  await vite.close();
});
beforeEach(() => fix.resetPendingRequestsForTests());

/** A frame scheduler with a manual clock: each frame advances 16 ms. */
function makeScheduler() {
  let time = 0;
  let next = 1;
  const queue = new Map();
  return {
    scheduler: {
      request(callback) { const id = next++; queue.set(id, callback); return id; },
      cancel(id) { queue.delete(id); },
      now: () => time,
    },
    /** Runs one frame; false when nothing was scheduled. */
    frame(onFrame) {
      const entries = [...queue.entries()];
      queue.clear();
      time += 16;
      onFrame?.();
      for (const [, callback] of entries) callback();
      return entries.length > 0;
    },
    pending: () => queue.size,
    advance(ms) { time += ms; },
  };
}

/** A control that exists only after `existsAt` frames and is inert (editor still opening) until `interactiveAt`. */
function makeDom({ existsAt = 0, interactiveAt = 0 } = {}) {
  const state = { frame: 0, active: null, focusCalls: 0 };
  const control = {
    isConnected: true,
    get disabled() { return state.frame < interactiveAt; },
    matches: (selector) => selector.includes('button'),
    closest: () => null,
    querySelector: () => null,
    scrollIntoView() {},
    focus() { state.active = control; state.focusCalls += 1; },
  };
  const doc = {
    getElementById: (id) => (id === 'target' && state.frame >= existsAt ? control : null),
    get activeElement() { return state.active; },
  };
  return { state, control, doc };
}

function run(dom, { frames = 60, steal } = {}) {
  const clock = makeScheduler();
  focus.focusReadinessTargetWhenReady('target', clock.scheduler, dom.doc);
  for (let index = 0; index < frames; index += 1) {
    if (!clock.frame(() => { dom.state.frame += 1; if (steal?.(dom.state.frame)) dom.state.active = { other: true }; })) break;
  }
  return clock;
}

test('a fix requested before the editor exists is a pending record the late editor still takes and ends focused', () => {
  const request = { id: 'enabled-castles' };
  fix.requestSettingsFix('autoTowers', request, () => {}, 1_000);
  // The editor mounts later: nothing was listening when the request was made, the record is still there.
  const pending = fix.takePendingSettingsFix('autoTowers', 1_400);
  assert.ok(pending, 'pending record survives until a listener mounts');
  assert.equal(fix.takePendingSettingsFix('autoTowers', 1_400), null, 'consumed once');
  assert.equal(pending.control, 'auto-towers-castles');

  // Its content mounts after 20 frames and stays inert until frame 35; the modal's own initial focus lands later.
  const dom = makeDom({ existsAt: 20, interactiveAt: 35 });
  run(dom, { steal: (frame) => frame === 38 });
  assert.equal(dom.state.active, dom.control, 'focus ends on the fixed control');
  assert.ok(dom.state.focusCalls >= 2, 'focus was re-asserted after something else took it');
});

test('a pending record older than 10 seconds is ignored and never steals focus later', () => {
  fix.requestSettingsFix('autoTowers', { id: 'enabled-castles' }, () => {}, 1_000);
  assert.equal(fix.takePendingSettingsFix('autoTowers', 1_000 + fix.PENDING_REQUEST_TTL_MS + 1), null);
  assert.equal(fix.takePendingSettingsFix('autoTowers', 1_000 + fix.PENDING_REQUEST_TTL_MS + 2), null, 'the stale record was dropped');
  fix.requestSettingsFix('autoTowers', { id: 'enabled-castles' }, () => {}, 5_000);
  assert.ok(fix.takePendingSettingsFix('autoTowers', 5_000 + fix.PENDING_REQUEST_TTL_MS), 'exactly at the limit still counts');
});

test('the Start confirmation Fix-first path uses the same pending mechanism', () => {
  fix.requestReadinessRowFocus('autoTowers', 100);
  assert.equal(fix.takePendingRowFocus('autoTowers', 200), true);
  assert.equal(fix.takePendingRowFocus('autoTowers', 200), false, 'consumed once');
  fix.requestReadinessRowFocus('autoTowers', 100);
  assert.equal(fix.takePendingRowFocus('autoTowers', 100 + fix.PENDING_REQUEST_TTL_MS + 1), false, 'stale row request ignored');
  fix.requestReadinessRowFocus('autoTowers', 100);
  fix.clearPendingRowFocus('autoTowers');
  assert.equal(fix.takePendingRowFocus('autoTowers', 101), false, 'cleared once the event path delivered it');
});

test('focus waits for the control to exist and be interactive, and gives up at the bounded deadline', () => {
  const inert = makeDom({ existsAt: 0, interactiveAt: 10 });
  run(inert, { frames: 8 });
  assert.equal(inert.state.focusCalls, 0, 'not focused while the control is disabled');
  run(makeDom({ existsAt: 10_000 }), { frames: 400 }); // never appears: must stop scheduling on its own
  const clock = makeScheduler();
  focus.focusReadinessTargetWhenReady('target', clock.scheduler, makeDom({ existsAt: 10_000 }).doc);
  let frames = 0;
  while (clock.frame() && frames < 1000) frames += 1;
  assert.ok(frames <= focus.FOCUS_MAX_FRAMES + 2, 'stops within the frame budget');
  const timed = makeScheduler();
  focus.focusReadinessTargetWhenReady('target', timed.scheduler, makeDom({ existsAt: 10_000 }).doc);
  timed.frame();
  timed.advance(focus.FOCUS_DEADLINE_MS + 1);
  timed.frame();
  assert.equal(timed.pending(), 0, 'stops at the 3 second deadline');
});

test('a control inside a disabled fieldset or an inert container is not focusable yet', () => {
  const blocked = { isConnected: true, disabled: false, matches: () => true, closest: () => ({}), querySelector: () => null, scrollIntoView() {}, focus() {} };
  assert.equal(focus.readyToFocus(blocked), false);
  assert.equal(focus.readyToFocus({ ...blocked, closest: () => null }), true);
  assert.equal(focus.readyToFocus({ ...blocked, closest: () => null, isConnected: false }), false);
  assert.equal(focus.readyToFocus(null), false);
});

test('the editor hook consumes the pending record on mount and when it becomes interactive, and uses the bounded focus', async () => {
  const hook = await readFile(new URL('../src/settings/disclosure/useSettingsDisclosure.ts', import.meta.url), 'utf8');
  assert.match(hook, /takePendingSettingsFix\(featureId\)/);
  assert.match(hook, /if \(!ready\) return undefined;/);
  assert.match(hook, /focusReadinessTargetWhenReady\(focusRequest\.controlId\)/);
  assert.doesNotMatch(hook, /requestAnimationFrame/, 'no single-frame focus');
  const row = await readFile(new URL('../src/components/AutomationReadinessRow.tsx', import.meta.url), 'utf8');
  assert.match(row, /takePendingRowFocus\(/);
  assert.match(row, /ROW_FOCUS_EVENT/);
});
