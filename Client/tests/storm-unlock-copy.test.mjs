import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const storm = await vite.ssrLoadModule('/src/settings/readiness/stormReadiness.ts');
const stormState = await vite.ssrLoadModule('/src/settings/AutoStormClientState.ts');
const options = await vite.ssrLoadModule('/src/settings/StormCastleOptions.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const COPY = 'The Storm castle unlock needs a currently offered official castle; none is offered right now. Wait for the next offer or turn the unlock off.';
const LIVE = { session: { generation: 5, baselineGeneration: 5, changedAt: '2026-09-29T09:00:00Z' }, connected: true };
const castle = (id, kingdomId, stationed) => ({ id, name: `C${id}`, kingdomId, slotType: 1, units: { stationed }, unitsObservedAt: '0001-01-01T00:00:00Z' });

function evaluate({ unlock, unlockOffer, unlocked = false }) {
  const base = stormState.defaultAutoStormClientState();
  const castles = { 1: castle(1, 0, { 1: 5000 }) };
  const state = { castles, kingdomTransport: { unlocks: { 4: { kingdomId: 4, unlocked, created: unlocked } } }, commanders: {}, market: {} };
  return storm.evaluateStormReadiness({
    draft: { ...base, unlock }, forts: { source: 'none' }, islands: { source: 'none' }, state, stormCastle: null,
    document: { version: 1, presets: [] }, troops: { 1: { id: 1, name: 'A', meleeAttack: 5 } }, tools: {}, metadataReady: true,
    observation: LIVE, buildActive: false, unlockOffer,
  });
}

test('unlock on with no official castle offered: `unlock` is blocked with the exact copy and the panel cannot read Ready', () => {
  const report = evaluate({ unlock: { enabled: true, prebuiltCastleId: 0 }, unlockOffer: { loaded: true, offeredIds: [] } });
  const unlock = report.checks.find((check) => check.id === 'unlock');
  assert.equal(unlock.state, 'blocked');
  assert.equal(messages[unlock.messageKey], COPY);
  assert.equal(messages['stormReadiness.unlockNotOffered'], COPY);
  assert.equal(report.overall, 'blocked');
  assert.equal(unlock.fix, 'settings', 'Fix routes to the unlock control');
  assert.equal(report.checks.filter((check) => check.id === 'unlock').length, 1, 'one unlock check, not two');
});

test('the block also holds when Storm is already unlocked, because Save is blocked on the same rule', () => {
  const report = evaluate({ unlock: { enabled: true, prebuiltCastleId: 0 }, unlockOffer: { loaded: true, offeredIds: [] }, unlocked: true });
  assert.equal(report.checks.find((check) => check.id === 'unlock').state, 'blocked');
  assert.equal(report.overall, 'blocked');
});

test('an offered castle that is not the selected one blocks with the choose-a-castle copy; a valid selection or the unlock off does not block', () => {
  const wrong = evaluate({ unlock: { enabled: true, prebuiltCastleId: 9 }, unlockOffer: { loaded: true, offeredIds: [1, 2] } });
  const check = wrong.checks.find((entry) => entry.id === 'unlock');
  assert.equal(check.state, 'blocked');
  assert.match(messages[check.messageKey], /Choose a currently available official Storm castle/);
  const fine = evaluate({ unlock: { enabled: true, prebuiltCastleId: 1 }, unlockOffer: { loaded: true, offeredIds: [1, 2] } });
  assert.notEqual(fine.checks.find((entry) => entry.id === 'unlock')?.state, 'blocked' , 'a valid offer does not block');
  const off = evaluate({ unlock: { enabled: false, prebuiltCastleId: 0 }, unlockOffer: { loaded: true, offeredIds: [] } });
  assert.ok(off.checks.every((entry) => entry.messageKey !== 'stormReadiness.unlockNotOffered'), 'turning the unlock off clears the block');
  const loading = evaluate({ unlock: { enabled: true, prebuiltCastleId: 0 }, unlockOffer: { loaded: false, offeredIds: [] } });
  assert.ok(loading.checks.every((entry) => entry.messageKey !== 'stormReadiness.unlockNotOffered'), 'nothing is claimed while the offers are still loading');
});

test('the shared parser reads the offered castles for kingdom 4 within the player level', () => {
  const rows = [
    { preBuiltCastleID: 7, spaceIDs: '4', minLevel: 10, comment2: 'CheapCamp' },
    { preBuiltCastleID: 8, spaceIDs: '1,2', minLevel: 1 },
    { preBuiltCastleID: 9, spaceIDs: '4', minLevel: 90 },
  ];
  assert.deepEqual(options.parseStormCastleOptions(rows, 40).map((option) => option.id), [7]);
  assert.deepEqual(options.parseStormCastleOptions(rows).map((option) => option.id), [7, 9], 'unknown level does not filter');
});

test('the unlock toggle stays operable so the player can turn it off', async () => {
  const modal = await readFile(new URL('../src/settings/components/AutoStormSettingsModal.tsx', import.meta.url), 'utf8');
  const block = modal.slice(modal.indexOf('checked={draft.unlock.enabled}'), modal.indexOf('checked={draft.unlock.enabled}') + 400);
  assert.match(block, /disabled=\{!draft\.unlock\.enabled && \(loadingStormCastleOptions \|\| stormCastleOptions\.length === 0\)\}/);
  assert.match(modal, /unlockOffer: \{ loaded: !loadingStormCastleOptions && !stormCastleOptionsError, offeredIds: stormCastleOptions\.map/);
  assert.match(modal, /const unlockValid = !draft\.unlock\.enabled \|\| Boolean\(selectedUnlockOption\)/, 'Save stays blocked until an offer is chosen or the unlock is off');
});
