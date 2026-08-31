import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { after, beforeEach, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const originalWindow = globalThis.window;
const originalEventSource = globalThis.EventSource;
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const { CitadelAPI } = await vite.ssrLoadModule('/src/api/CitadelClient.ts');
const {
  eventPaginationSelectionKey,
  reconcileEventPage,
} = await vite.ssrLoadModule('/src/worldIntelligence/components/WorldEventPagination.ts');
const { advanceSuccessfulLoadGeneration } = await vite.ssrLoadModule(
  '/src/worldIntelligence/components/WorldIntelligenceLoadGeneration.ts',
);

class RecordingEventSource {
  static instances = [];

  constructor(url, init) {
    this.url = url;
    this.init = init;
    this.closed = false;
    this.listeners = new Map();
    RecordingEventSource.instances.push(this);
  }

  addEventListener(name, listener) {
    this.listeners.set(name, listener);
  }

  close() {
    this.closed = true;
  }
}

beforeEach(() => {
  RecordingEventSource.instances = [];
  globalThis.window = {
    location: { origin: 'http://127.0.0.1:41731', pathname: '/' },
  };
  globalThis.EventSource = RecordingEventSource;
});

after(async () => {
  globalThis.window = originalWindow;
  globalThis.EventSource = originalEventSource;
  await vite.close();
});

test('local EXE subscription resolves against the runtime root', () => {
  const statuses = [];
  const unsubscribe = CitadelAPI.subscribeWorldIntelligenceUpdates(
    'ep-live-us1-game.goodgamestudios.com',
    () => {},
    (status) => statuses.push(status),
  );

  assert.equal(RecordingEventSource.instances.length, 1);
  const source = RecordingEventSource.instances[0];
  const endpoint = new URL(source.url);
  assert.equal(endpoint.origin, 'http://127.0.0.1:41731');
  assert.equal(endpoint.pathname, '/api/v2/world-intelligence/subscribe');
  assert.equal(endpoint.searchParams.get('worldId'), 'ep-live-us1-game.goodgamestudios.com');
  assert.deepEqual(source.init, { withCredentials: true });
  assert.deepEqual(statuses, ['connecting']);

  unsubscribe();
  assert.equal(source.closed, true);
});

test('account-hosted subscription retains the account runtime prefix', () => {
  globalThis.window.location.pathname = '/accounts/account-a/world-intelligence';
  const unsubscribe = CitadelAPI.subscribeWorldIntelligenceLeaderboard(
    { worldId: 'test-world', occurrenceId: 'run/special' },
    () => {},
    () => {},
  );

  assert.equal(RecordingEventSource.instances.length, 1);
  const source = RecordingEventSource.instances[0];
  const endpoint = new URL(source.url);
  assert.equal(
    endpoint.pathname,
    '/accounts/account-a/api/v2/world-intelligence/event-runs/run%2Fspecial/subscribe',
  );
  assert.equal(endpoint.searchParams.get('worldId'), 'test-world');
  assert.deepEqual(source.init, { withCredentials: true });

  unsubscribe();
  assert.equal(source.closed, true);
});

test('current Storm metrics remain the complete default beside dated history', async () => {
	const [source, viewSource] = await Promise.all([
		readFile(new URL('../src/worldIntelligence/components/WorldEventHistory.tsx', import.meta.url), 'utf8'),
		readFile(new URL('../src/worldIntelligence/components/WorldIntelligenceView.tsx', import.meta.url), 'utf8'),
	]);
	assert.match(source, /metric: stormBoard\.metric, limit: 5_000/);
	assert.match(source, /const defaultRunKey = selectedEvent\?\.publicBoard\s*\? stormRunKey/);
	assert.match(source, /label: 'Live Storm metrics'/);
	assert.match(source, /stormGroup\.publicBoard = publicBoard/);
	assert.match(source, /if \(!previous \|\| worldUpdate\.eventRunsRevision > previous\.eventRunsRevision\)/);
	assert.match(source, /if \(!previous \|\| worldUpdate\.rankingsRevision > previous\.rankingsRevision\)/);
	assert.match(source, /const stormApplied = stormResponse\.available\s*\? advanceSuccessfulLoadGeneration/);
	assert.match(viewSource, /if \(!previous \|\| worldUpdate\.coverageRevision > previous\.coverageRevision\)/);
	assert.match(viewSource, /const requestID = \+\+coverageRequest\.current/);
	assert.match(viewSource, /if \(requestID !== coverageRequest\.current\) return/);
});

test('a failed ready refresh cannot suppress an earlier successful Storm load', () => {
	let applied = 0;
	const initialLoad = 1;
	const readyRefresh = 2;

	// The ready refresh failed, so it never advances the applied generation.
	const acceptedInitial = advanceSuccessfulLoadGeneration(applied, initialLoad);
	assert.equal(acceptedInitial, initialLoad);
	applied = acceptedInitial;

	const acceptedReady = advanceSuccessfulLoadGeneration(applied, readyRefresh);
	assert.equal(acceptedReady, readyRefresh);
	applied = acceptedReady;
	assert.equal(advanceSuccessfulLoadGeneration(applied, initialLoad), null);
});

test('Might and Honor accept successful generations independently', () => {
	let mightApplied = 0;
	let honorApplied = 0;
	mightApplied = advanceSuccessfulLoadGeneration(mightApplied, 2);
	assert.equal(mightApplied, 2);
	// Honor generation 2 failed and therefore never advanced its own counter.
	honorApplied = advanceSuccessfulLoadGeneration(honorApplied, 1);
	assert.equal(honorApplied, 1);
});

test('page 2 survives same-board refreshes and leaderboard deltas', () => {
  const selection = eventPaginationSelectionKey({
    worldId: 'test-world',
    eventKey: 'storm-islands:102',
    runKey: 'run-1',
    boardKey: 'run-1:13:',
    league: '__all_leagues__',
    searchQuery: '',
    sortKey: 'score',
    sortDirection: 'descending',
  });
  const initialRows = Array.from({ length: 60 }, (_, index) => ({ playerId: index + 1 }));
  const refreshedRows = initialRows.map((entry) => ({ ...entry }));
  const deltaRows = [...refreshedRows, { playerId: 61 }];

  let page = 1;
  page = reconcileEventPage(page, selection, selection, refreshedRows.length, 25);
  assert.equal(page, 1, 'a replacement snapshot must preserve UI page 2');
  page = reconcileEventPage(page, selection, selection, deltaRows.length, 25);
  assert.equal(page, 1, 'a same-board delta must preserve UI page 2');
});

test('pagination resets for semantic filters and clamps after contraction', () => {
  const base = {
    worldId: 'test-world', eventKey: 'storm-islands:102', runKey: 'run-1',
    boardKey: 'run-1:13:', league: '__all_leagues__', searchQuery: '',
    sortKey: 'score', sortDirection: 'descending',
  };
  const selection = eventPaginationSelectionKey(base);
  const filteredSelection = eventPaginationSelectionKey({ ...base, searchQuery: 'alpha' });

  assert.equal(reconcileEventPage(1, selection, filteredSelection, 60, 25), 0);
  assert.equal(reconcileEventPage(1, selection, selection, 20, 25), 0);
});
