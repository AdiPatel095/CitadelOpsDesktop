import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const summaries = await vite.ssrLoadModule('/src/settings/disclosure/summaries.ts');
const { messages, describeMessage } = await vite.ssrLoadModule('/src/i18n/messages.ts');
const { formatMessage } = await vite.ssrLoadModule('/src/i18n/formatMessage.ts');
const towerState = await vite.ssrLoadModule('/src/settings/AutoTowerClientState.ts');

after(async () => {
  await vite.close();
});

const text = (line) => formatMessage(describeMessage(line.messageKey, line.params), 'en', {}).text;
const texts = (lines) => lines.map(text);

test('stored seconds are shown in the largest familiar unit without changing them', () => {
  assert.deepEqual(summaries.durationParts(60), { count: 1, unit: 'minute' });
  assert.deepEqual(summaries.durationParts(90), { count: 90, unit: 'second' });
  assert.deepEqual(summaries.durationParts(1800), { count: 30, unit: 'minute' });
  assert.deepEqual(summaries.durationParts(7200), { count: 2, unit: 'hour' });
  assert.deepEqual(summaries.durationParts(86400), { count: 1, unit: 'day' });
  assert.equal(text(summaries.checkIntervalLine(300)), 'Checks every 5 minutes');
  assert.equal(text(summaries.checkIntervalLine(45)), 'Checks every 45 seconds');
  assert.equal(text(summaries.mapRefreshLine(21600)), 'full map refresh every 6 hours');
});

test('paid travel and time skips are always summarized', () => {
  assert.equal(text(summaries.travelLine(-1)), 'Travel: travel feather (HBW -1), no paid boost');
  assert.equal(text(summaries.travelLine(1007)), 'Travel: horse or ship tier paid with coins');
  assert.equal(text(summaries.travelLine(1009)), 'Travel: a boost paid with Rubies on every launch');
  assert.deepEqual(texts(summaries.timeSkipLines(false, { MS1: 3 })), ['Time skips off']);
  assert.deepEqual(texts(summaries.timeSkipLines(true, { MS1: 3, MS2: 0, MS3: 1 })), ['Time skips on', '4 skips kept in reserve']);
  assert.deepEqual(texts(summaries.cooldownSkipLines(true, {})), ['Camp cooldowns are cleared with time skips', 'no skips kept in reserve']);
});

test('custom values are counted against the feature defaults', () => {
  const defaults = towerState.defaultAutoTowerClientState();
  assert.equal(summaries.countCustomValues(defaults, defaults, ['mapRefreshIntervalSec', 'horseTravelBoostId']), 0);
  assert.equal(summaries.countCustomValues({ ...defaults, horseTravelBoostId: 1008 }, defaults, ['mapRefreshIntervalSec', 'horseTravelBoostId']), 1);
});

test('feature summaries state consequences in player language', () => {
  assert.deepEqual(texts(summaries.towerAdvisorSummary({ useAdvisor: true, autoActivateAdvisor: true, maximumDailyTimeSkips: 3 })),
    ['Advisor chains on, activating with one token when inactive; up to 3 time skips a day']);
  assert.deepEqual(texts(summaries.towerAdvisorSummary({ useAdvisor: false, autoActivateAdvisor: true, maximumDailyTimeSkips: 3 })),
    ['Advisor chains off; no tokens or time skips used']);
  assert.deepEqual(texts(summaries.towerScanSummary(1800, [{ radius: 10, maidenOnly: true }, { radius: 20, maidenOnly: false }])),
    ['Map scan every 30 minutes', 'radius 10–20 tiles', 'maiden-supported commanders only at 1 castle']);
  assert.deepEqual(texts(summaries.stationFiltersSummary({ minRPTDays: 3, openGateFallback: true })),
    ['sends troops only to members with more than 3 days of Bird protection', 'opens the gates when troops cannot leave in time']);
  assert.deepEqual(texts(summaries.foodTimingSummary({ checkIntervalSec: 60, minimumShipmentSize: 1000, minimumStormShipmentSize: 10000 })),
    ['Checks every 1 minute', 'kingdom shipments from 1,000 · Storm deliveries from 10,000']);
  assert.deepEqual(texts(summaries.khanStopLimitsSummary({ maxRageChain: 2, requireActiveRageBooster: false, nomadPointThreshold: 0 })),
    ['new camp attacks stop after 2 accepted Khan retaliations', 'no Rage points booster required', 'no Nomad points stop']);
  assert.equal(text(summaries.rbcTrialLine({ enabled: true, targetX: 1234, targetY: 56 })), 'Trial on against the robber baron castle at 1234:56');
  assert.deepEqual(texts(summaries.beriAttackOptionsSummary({ attackCheckIntervalSec: 60, toolMinimums: { 1: 5, 2: 0 }, useTroopTransportTimeSkips: false })),
    ['Checks every 1 minute', 'coins keep 1 tool type at its minimum', 'troop transfers never use time skips']);
  const storm = texts(summaries.stormConstructionSummary({
    build: { allowResourceTransport: true, allowTimeSkips: false, allowPremium: true, allowDemolition: false, timeSkipReserve: {} },
    harbor: { enabled: true, targetLevel: 3 },
    decorationPresetId: '',
  }, false));
  assert.deepEqual(storm, [
    'no construction target; combat only', 'premium (Ruby) construction costs allowed', 'no demolition',
    'may ship resources from your other castles', 'Time skips off', 'harbor kept at level 3', 'no decoration',
  ]);
  assert.deepEqual(texts(summaries.birdTimingSummary({ minDelay: 6, maxDelay: 12, minSend: 0 })), ['sends after a random 6–12 hour delay · no minimum group size']);
  assert.deepEqual(texts(summaries.tciPresetsSummary(2, 'Main')), ['2 saved presets; editing Main']);
});

test('every summary, run and section key exists and keeps "runtime" out', async () => {
  const keys = Object.keys(messages).filter((key) => /^(settingsSummary\.|settingsRun\.|ui\.settings\.disclosure\.|ui\.settings\.components\.(settingsSection|automationRunStrip)\.)/.test(key));
  assert.ok(keys.length > 60);
  for (const key of keys) assert.doesNotMatch(messages[key], /runtime/i, key);
  const source = await readFile(new URL('../src/settings/disclosure/summaries.ts', import.meta.url), 'utf8');
  for (const [, key] of source.matchAll(/line\('([^']+)'/g)) assert.ok(messages[key], key);
});
