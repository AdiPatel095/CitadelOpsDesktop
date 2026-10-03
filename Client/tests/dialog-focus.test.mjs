import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const focus = await vite.ssrLoadModule('/src/settings/components/dialogFocus.ts');

after(async () => {
  await vite.close();
});

/** Manual frame queue: `tick()` runs the callbacks scheduled for the next frame. */
function frames() {
  let queue = [];
  let next = 1;
  return {
    request(callback) { const handle = next++; queue.push({ handle, callback }); return handle; },
    cancel(handle) { queue = queue.filter((entry) => entry.handle !== handle); },
    tick() { const current = queue; queue = []; current.forEach((entry) => entry.callback()); },
    get pending() { return queue.length; },
  };
}

function element(name, log, state = {}) {
  return {
    name,
    isConnected: true,
    disabled: false,
    inert: false,
    ...state,
    closest(selector) { return selector === '[inert]' && this.inert ? {} : null; },
    focus() { log.push(name); },
  };
}

test('first open and close: focus moves into the dialog after its own frame, then back to the trigger', () => {
  const log = [];
  const scheduler = frames();
  const trigger = element('Review and save assignments', log);
  const cancel = element('Cancel', log);
  // Open: the dialog focuses its first control on the next frame; the handoff wins one frame later.
  focus.moveFocusAfterDialog(() => cancel, scheduler);
  log.push('Close modal (dialog default)');
  scheduler.tick();
  scheduler.tick();
  assert.deepEqual(log, ['Close modal (dialog default)', 'Cancel']);
  // Close on the very first use: the return target is known without any earlier frame having run.
  log.length = 0;
  log.push('Guide (settings modal first control)');
  focus.moveFocusAfterDialog(() => trigger, scheduler);
  scheduler.tick();
  scheduler.tick();
  assert.deepEqual(log, ['Guide (settings modal first control)', 'Review and save assignments']);
});

test('a trigger inside briefly inert content receives focus once the content is interactive again', () => {
  const log = [];
  const scheduler = frames();
  const trigger = element('trigger', log, { inert: true });
  focus.moveFocusAfterDialog(() => trigger, scheduler);
  scheduler.tick();
  scheduler.tick();
  assert.deepEqual(log, [], 'never focuses an inert element');
  trigger.inert = false;
  scheduler.tick();
  assert.deepEqual(log, ['trigger']);
  assert.equal(scheduler.pending, 0);
});

test('closing before the open handoff runs cancels it; retries stop after the attempt limit', () => {
  const log = [];
  const scheduler = frames();
  const cancelOpen = focus.moveFocusAfterDialog(() => element('Cancel', log), scheduler);
  scheduler.tick();
  cancelOpen();
  scheduler.tick();
  assert.deepEqual(log, []);
  const gone = element('gone', log, { isConnected: false });
  focus.moveFocusAfterDialog(() => gone, scheduler, 3);
  for (let index = 0; index < 10; index += 1) scheduler.tick();
  assert.deepEqual(log, []);
  assert.equal(scheduler.pending, 0);
  assert.equal(focus.focusable(element('x', log, { disabled: true })), false);
  assert.equal(focus.focusable(null), false);
});

test('the commander panel records the return target when the dialog opens', async () => {
  const { readFile } = await import('node:fs/promises');
  const source = await readFile(new URL('../src/settings/components/CommanderAssignmentPanel.tsx', import.meta.url), 'utf8');
  assert.match(source, /onClick=\{\(event\) => openConfirm\(event\.currentTarget\)\}/);
  assert.match(source, /moveFocusAfterDialog\(\(\) => cancelButton\.current, browserFrames\)/);
  assert.doesNotMatch(source, /wasConfirming/, 'no dependency on an earlier animation frame having run');
});
