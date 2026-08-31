import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { after, beforeEach, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const { CitadelAPI } = await vite.ssrLoadModule('/src/api/CitadelClient.ts');

const originalFetch = globalThis.fetch;
const originalWindow = globalThis.window;
const requests = [];
const retentionOptions = [
  { value: 'none', label: 'No history', description: 'Do not save history.' },
	{ value: '24h', label: '24 hours', description: 'Keep 24 hours.', days: 1, recordings: 24 },
	{ value: '100d', label: '100 days', description: 'Keep 100 days.', days: 100, recordings: 2400 },
	{ value: '1y', label: '365 days', description: 'Keep one year.', days: 365, recordings: 8760 },
];
const recordingIntervalOptions = [60, 300, 600, 900, 1800, 3600].map((seconds) => ({
	seconds,
	label: seconds === 3600 ? '1 hour' : `${seconds / 60} minutes`,
	description: `Record every ${seconds} seconds.`,
	recordingsPerDay: 86400 / seconds,
}));

const policy = (configured = '24h', recordingIntervalSeconds = 3600) => ({
  revision: 41,
  configured,
  effective: configured,
	configuredDays: configured === 'none' ? undefined : configured === '1y' ? 365 : Number.parseInt(configured, 10),
	effectiveDays: configured === 'none' ? undefined : configured === '1y' ? 365 : Number.parseInt(configured, 10),
	recordingIntervalSeconds,
	recordingIntervalOptions,
  hosted: false,
  maximum: 'unlimited',
  options: retentionOptions,
	storage: {
		format: 'jsonl',
		currentBytes: 4096,
		estimatedBytesPerRecording: 1024,
		basis: 'saved-samples',
		sampledRecordings: 4,
	},
});

beforeEach(() => {
  requests.length = 0;
  globalThis.window = {
    location: { origin: 'http://127.0.0.1:41731', pathname: '/settings' },
  };
  globalThis.fetch = async (input, init = {}) => {
    requests.push({
      url: typeof input === 'string' ? input : input.url,
      method: init.method ?? 'GET',
      body: init.body,
      cache: init.cache,
      credentials: init.credentials,
    });
    const requestInput = init.body ? JSON.parse(String(init.body)) : {};
    const configured = requestInput.retention ?? '24h';
	const recordingIntervalSeconds = requestInput.recordingIntervalSeconds ?? 3600;
    const payload = init.method === 'POST'
		? { policy: policy(configured, recordingIntervalSeconds), report: { retention: configured, recordingIntervalSeconds, complete: true } }
      : policy();
    return new Response(JSON.stringify(payload), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    });
  };
});

after(async () => {
  globalThis.fetch = originalFetch;
  globalThis.window = originalWindow;
  await vite.close();
});

test('local EXE reads and applies My Stats retention on the direct runtime root', async () => {
  const loaded = await CitadelAPI.getPlayerHistoryRetention();
  const applied = await CitadelAPI.applyPlayerHistoryRetention('1y', loaded.revision);

  assert.equal(applied.policy.configured, '1y');
  assert.deepEqual(requests, [
    {
      url: '/api/v2/history/player-tracker/retention',
      method: 'GET',
      body: undefined,
      cache: 'no-store',
      credentials: 'include',
    },
    {
      url: '/api/v2/history/player-tracker/retention/apply',
      method: 'POST',
      body: JSON.stringify({ retention: '1y', expectedRevision: 41 }),
      cache: undefined,
      credentials: 'include',
    },
  ]);
});

test('hosted account route keeps My Stats retention reads and writes on its shard', async () => {
  globalThis.window.location.pathname = '/accounts/account-a/settings';
	const mountedScope = CitadelAPI.runtimeScope();
	globalThis.window.location.pathname = '/accounts/account-b/settings';

  const loaded = await CitadelAPI.getPlayerHistoryRetention(mountedScope);
  await CitadelAPI.applyPlayerHistoryRetention('none', loaded.revision, mountedScope);

  assert.deepEqual(requests.map(({ url, method }) => ({ url, method })), [
    {
      url: '/accounts/account-a/api/v2/history/player-tracker/retention',
      method: 'GET',
    },
    {
      url: '/accounts/account-a/api/v2/history/player-tracker/retention/apply',
      method: 'POST',
    },
  ]);
});

test('local EXE accepts an arbitrary whole-day My Stats limit', async () => {
	const loaded = await CitadelAPI.getPlayerHistoryRetention();
	const applied = await CitadelAPI.applyPlayerHistoryRetention('137d', loaded.revision);

	assert.equal(applied.policy.configured, '137d');
	assert.equal(JSON.parse(requests[1].body).retention, '137d');
});

test('local EXE sends the selected My Stats recording frequency atomically', async () => {
	const loaded = await CitadelAPI.getPlayerHistoryRetention();
	const applied = await CitadelAPI.applyPlayerHistoryRetention('100d', loaded.revision, 300);

	assert.equal(applied.policy.recordingIntervalSeconds, 300);
	assert.deepEqual(JSON.parse(requests[1].body), {
		retention: '100d',
		recordingIntervalSeconds: 300,
		expectedRevision: 41,
	});
});

test('Settings route includes the My Stats storage control and required choices contract', async () => {
	const [appSource, contextSource, settingsSource, playerTrackerSource] = await Promise.all([
		readFile(new URL('../src/App.tsx', import.meta.url), 'utf8'),
		readFile(new URL('../src/api/ApiContext.tsx', import.meta.url), 'utf8'),
		readFile(new URL('../src/views/SettingsView.tsx', import.meta.url), 'utf8'),
		readFile(new URL('../src/playerTracker/components/PlayerTrackerView.tsx', import.meta.url), 'utf8'),
	]);

  assert.match(appSource, /settings:\s*lazy\(\(\) => import\('\.\/views\/SettingsView'\)\)/);
	assert.match(settingsSource, /title="My Stats Storage"/);
	assert.match(settingsSource, /ariaLabel="My Stats saved history window"/);
	assert.match(settingsSource, /ariaLabel="My Stats recording frequency"/);
	assert.match(settingsSource, /aria-label="Custom My Stats retention days"/);
	assert.match(settingsSource, /projectedPlayerHistoryRecordings/);
	assert.match(settingsSource, /playerHistoryRecordingsPerDay/);
	assert.match(settingsSource, /does not send additional game scan commands/);
	assert.match(settingsSource, /estimatedBytesPerRecording/);
	assert.doesNotMatch(settingsSource, /title="Experimental Battle Research"/);
	assert.match(settingsSource, /getPlayerHistoryRetention\(\)/);
	assert.match(settingsSource, /applyPlayerHistoryRetention\(\s*nextRetention,\s*nextIntervalSeconds,\s*playerHistoryRetention\.revision/);
	assert.match(settingsSource, /retentionChanged && playerHistoryRetention\?\.maximumDays/);
	assert.match(contextSource, /const runtimeScope = useRef\(CitadelAPI\.runtimeScope\(\)\)\.current/);
	assert.match(contextSource, /let policy = await CitadelAPI\.getPlayerHistoryRetention\(runtimeScope\)/);
	assert.match(contextSource, /policy\.revision,\s*recordingIntervalSeconds,\s*runtimeScope/);
	assert.match(playerTrackerSource, /bucketMetricPoints\(selectedPoints, selectedRange, scopedTracker\.intervalSeconds\)/);
	assert.match(playerTrackerSource, /bucketMetricPoints\(troopPoints, troopRange, scopedTracker\.intervalSeconds\)/);
	assert.doesNotMatch(playerTrackerSource, /Hover for hourly points/);
	assert.deepEqual(retentionOptions.map(({ value }) => value), ['none', '24h', '100d', '1y']);
	assert.deepEqual(recordingIntervalOptions.map(({ seconds }) => seconds), [60, 300, 600, 900, 1800, 3600]);
});
