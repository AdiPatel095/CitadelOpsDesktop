import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const root = fileURLToPath(new URL('..', import.meta.url));
const source = existsSync(`${root}/src/commandCenter`) ? '/src/commandCenter' : '/src';
const vite = await createServer({ root, configFile: false, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true, hmr: false } });
const { timedRunState } = await vite.ssrLoadModule(`${source}/components/automation/timedRun.ts`);
const { TIMER_TIP_STORAGE_KEY, timerTipDismissed, dismissTimerTip } = await vite.ssrLoadModule(`${source}/components/automation/timerTipStorage.ts`);
const { automationDuration } = await vite.ssrLoadModule(`${source}/i18n/automationDuration.ts`);
after(() => vite.close());

const now = Date.UTC(2026, 9, 1, 12);
for (const [label, expiresAt] of [['undefined', undefined], ['past', now - 1], ['exactly now', now]]) {
  test(`timed run is idle when expiration is ${label}`, () => {
    assert.deepEqual(timedRunState(expiresAt, now, 'en'), { kind: 'idle' });
  });
}

for (const locale of ['en', 'de']) {
  test(`active timed run uses the existing 80-minute duration in ${locale}`, () => {
    assert.deepEqual(timedRunState(now + 80 * 60_000, now, locale), {
      kind: 'active', duration: automationDuration(80, locale),
    });
  });
}

test('timer tip reads only the dismissal marker at the shared storage key', () => {
  assert.equal(TIMER_TIP_STORAGE_KEY, 'citadelops.automation.timerTipDismissed');
  assert.equal(timerTipDismissed(), false);
  for (const value of [null, '', '0', 'true', '1']) {
    assert.equal(timerTipDismissed({ getItem(key) {
      assert.equal(key, TIMER_TIP_STORAGE_KEY);
      return value;
    } }), value === '1');
  }
});

test('dismissing the timer tip persists its marker and suppresses the tip', () => {
  const values = new Map();
  const storage = {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, value),
  };
  assert.equal(timerTipDismissed(storage), false);
  assert.equal(dismissTimerTip(storage), true);
  assert.deepEqual([...values], [[TIMER_TIP_STORAGE_KEY, '1']]);
  assert.equal(timerTipDismissed(storage), true);
});

test('a storage read failure leaves the timer tip visible', () => {
  assert.equal(timerTipDismissed({ getItem() { throw new Error('Storage unavailable'); } }), false);
});

test('a storage write failure reports that dismissal needs a session-only flag', () => {
  assert.equal(dismissTimerTip(), false);
  assert.equal(dismissTimerTip({ setItem() { throw new Error('Storage unavailable'); } }), false);
});
