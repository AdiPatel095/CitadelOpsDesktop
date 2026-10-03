import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

// CIT-30: unchanged movement polls no longer advance the movement snapshot time,
// so a launch control must not read that time as staleness when the view model
// declares no freshness window (freshnessWindowSec 0, the current default).
const vite = await createServer({
  root: fileURLToPath(new URL('..', import.meta.url)), appType: 'custom', logLevel: 'silent', server: { middlewareMode: true },
});
after(() => vite.close());
const { commanderStatusForLaunch, movementSnapshotFresh } = await vite.ssrLoadModule(
  fileURLToPath(new URL('../src/Movement/types/CommanderActivity.ts', import.meta.url)),
);

const now = 1_790_000_000;
const viewModel = (overrides = {}) => ({
  activeMovements: [],
  commanderStatuses: [
    { commanderId: 7, name: '', visiblePosition: 7, status: 'free', busy: false, movement: null },
    { commanderId: 8, name: '', visiblePosition: 8, status: 'outbound', busy: true, movement: { id: 1, direction: 0, travelSeconds: 900, progressSeconds: 20 } },
  ],
  snapshotReady: true,
  lastSnapshotUnix: now - 6 * 3600, // hours-old sighting: quiet polls do not advance it
  freshnessWindowSec: 0,
  ...overrides,
});

test('with no freshness window an old snapshot time is still fresh', () => {
  assert.equal(commanderStatusForLaunch(viewModel(), 7, true, now, undefined), 'free');
  assert.equal(commanderStatusForLaunch(viewModel(), 8, true, now, undefined), 'outbound');
  assert.equal(movementSnapshotFresh(viewModel(), true, now), true);
});

test('a positive freshness window still expires stale snapshots', () => {
  const windowed = viewModel({ freshnessWindowSec: 45 });
  assert.equal(commanderStatusForLaunch(windowed, 7, true, now, undefined), 'unknown');
  const recent = viewModel({ freshnessWindowSec: 45, lastSnapshotUnix: now - 10 });
  assert.equal(commanderStatusForLaunch(recent, 7, true, now, undefined), 'free');
  assert.equal(commanderStatusForLaunch(viewModel({ freshnessWindowSec: 45, lastSnapshotUnix: 0 }), 7, true, now, undefined), 'unknown');
});

test('readiness, login and identity gates are unchanged', () => {
  assert.equal(commanderStatusForLaunch(viewModel({ snapshotReady: false }), 7, true, now, undefined), 'syncing');
  assert.equal(commanderStatusForLaunch(viewModel(), 7, false, now, undefined), 'unknown');
  assert.equal(commanderStatusForLaunch(viewModel(), undefined, true, now, undefined), 'unknown');
  assert.equal(commanderStatusForLaunch(viewModel(), -1, true, now, undefined), 'unknown');
  assert.equal(commanderStatusForLaunch(null, 7, true, now, 'busy'), 'busy');
  assert.equal(commanderStatusForLaunch(null, 7, true, now, undefined), 'syncing');
  assert.equal(commanderStatusForLaunch(viewModel(), 99, true, now, undefined), 'unknown');
});

test('an outbound commander past travel time reads busy or posted', () => {
  const done = viewModel({
    commanderStatuses: [{ commanderId: 8, name: '', visiblePosition: 8, status: 'outbound', busy: true, movement: { id: 1, direction: 0, travelSeconds: 900, progressSeconds: 900 } }],
  });
  assert.equal(commanderStatusForLaunch(done, 8, true, now, undefined), 'busy');
  const posted = viewModel({
    commanderStatuses: [{ commanderId: 8, name: '', visiblePosition: 8, status: 'outbound', busy: true, movement: { id: 1, direction: 0, travelSeconds: 900, progressSeconds: 900, returnsAt: '2026-09-30T13:00:00Z' } }],
  });
  assert.equal(commanderStatusForLaunch(posted, 8, true, now, undefined), 'posted');
});
