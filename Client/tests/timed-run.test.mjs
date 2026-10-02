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

// CIT-72 integration guards, added when the prepared helpers are wired into the view.
test('temporary activation has one visible control and preserves its existing write paths', async () => {
  const { readFileSync, readdirSync } = await import('node:fs');
  const { join } = await import('node:path');
  const src = `${root}${source}`;
  const read = relative => readFileSync(join(src, relative), 'utf8');
  function scan(directory) {
    for (const entry of readdirSync(directory, { withFileTypes: true })) {
      const path = join(directory, entry.name);
      if (entry.isDirectory()) scan(path);
      else if (/\.(tsx?|css|json)$/.test(path)) {
        const contents = readFileSync(path, 'utf8');
        assert.doesNotMatch(contents, /automation-right-click|ui\.rich\.views\.automationView\.right\.click/);
      }
    }
  }
  scan(src);
  const view = read('views/AutomationView.tsx');
  assert.equal((view.match(/<TimedRunButton\b/g) ?? []).length, 1);
  assert.ok(view.indexOf('<TimedRunButton') < view.indexOf('className="automation-function-settings"'));
  assert.match(view, /disabled=\{feature\.disabled\} onOpen=\{\(\) => onOpenAutomationDuration\(feature\.enabledKey, feature\.name\)\}/);
  assert.match(view, /onContextMenu=\{\(event\) => \{\s*event\.preventDefault\(\);\s*onOpenAutomationDuration\(feature\.enabledKey, feature\.name\)/);
  assert.equal((view.match(/<TimerTip\s*\/>/g) ?? []).length, 1);
  assert.ok(view.indexOf('<AutomationSafetyPanel') < view.indexOf('<TimerTip'));
  assert.ok(view.indexOf('<TimerTip') < view.indexOf('{goalEntry}'));
  assert.ok(view.indexOf('<AutomationStatusLines', view.indexOf('<TimedRunButton')) < view.indexOf('<Panel className="automation-card-metrics"'));
  assert.doesNotMatch(view, /formatTimedRemaining|automation\.daily/);
  const dialog = read('settings/components/AutomationDurationModal.tsx');
  assert.match(dialog, /!onPauseFor && currentUntil \? \(\s*<StopControl enabledKey=\{featureKey\} featureId=\{featureIdForEnabledKey\(featureKey\)!\} onStopped=\{onClose\}/);
  const stop = read('components/StopControl.tsx');
  assert.match(stop, /await setAutomationEnabled\(enabledKey, enabled\);\s*if \(!enabled\) onStopped\?\.\(\);/);
  const css = read('views/automation-card.css');
  assert.match(css, /align-items: stretch/);
  assert.doesNotMatch(css, /line-clamp|text-overflow|--accent|--primary/);
});

test('StopControl notifies only after a successful stop, including retry', async () => {
  const { readFileSync } = await import('node:fs');
  const { compileFunction } = await import('node:vm');
  const { createRequire } = await import('node:module');
  const external = createRequire(import.meta.url);
  const ts = external('typescript');
  const React = external('react');
  for (const scenario of ['stop', 'failure', 'retryStop', 'retryStart']) {
    let resolveWrite;
    let notified = 0;
    const writes = [];
    const pending = new Promise((resolve, reject) => { resolveWrite = scenario === 'failure' ? reject : resolve; });
    const Button = () => null;
    const auth = {
      automationEnabledByKey: { auto_towers: scenario === 'stop' || scenario === 'failure' },
      automationTimedUntilByKey: {}, gameLoggedIn: true,
      automationWriteFailures: scenario.startsWith('retry') ? { auto_towers: { intent: scenario === 'retryStop' ? 'stop' : 'start' } } : {},
      setAutomationEnabled: async (key, enabled) => { writes.push([key, enabled]); await pending; },
    };
    const boundaries = {
      react: { ...React, useMemo: fn => fn(), useState: initial => [initial, () => {}] },
      'lucide-react': { CircleStop: () => null, Info: () => null },
      '../api/ApiContext': { useCitadelAPI: () => ({ operations: [] }) },
      '../context/AuthContext': { useAuth: () => auth },
      '../i18n/LocalizedText': { LocalizedText: () => null },
      '../settings/readiness/stopSemantics': { describeStopSemantics: () => ({ inFlight: [], lines: [] }) },
      '../settings/disclosure/placement': { AUTOMATION_ENABLED_KEYS: {} },
      '../settings/readiness/useAutomationPlayerStatus': { useAutomationPlayerStatus: () => ({ overall: {} }) },
      './ui/StatusBadge': { StatusBadge: () => null }, './ui/Button': { Button },
    };
    const module = { exports: {} };
    const code = ts.transpileModule(readFileSync(`${root}${source}/components/StopControl.tsx`, 'utf8'), {
      compilerOptions: { jsx: ts.JsxEmit.React, module: ts.ModuleKind.CommonJS, esModuleInterop: true },
    }).outputText;
    compileFunction(code, ['module', 'exports', 'require'])(module, module.exports, id => {
      assert.ok(Object.hasOwn(boundaries, id), `Unexpected boundary: ${id}`); return boundaries[id];
    });
    const element = module.exports.StopControl({ enabledKey: 'auto_towers', featureId: 'autoTowers', onStopped: () => { notified++; } });
    function buttons(node) {
      if (!React.isValidElement(node)) return [];
      return node.type === Button ? [node] : React.Children.toArray(node.props.children).flatMap(buttons);
    }
    const [button] = buttons(element);
    assert.ok(button, scenario);
    button.props.onClick();
    assert.equal(notified, 0, 'must wait for the write');
    resolveWrite(scenario === 'failure' ? new Error('Write failed') : undefined);
    await new Promise(resolve => setImmediate(resolve));
    assert.deepEqual(writes, [['auto_towers', scenario === 'retryStart']]);
    assert.equal(notified, scenario === 'failure' || scenario === 'retryStart' ? 0 : 1, scenario);
  }
});
