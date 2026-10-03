import assert from 'node:assert/strict';
import test from 'node:test';
import { stationHeaderPill } from '../src/components/stationHeaderPill.ts';

const now = Date.parse('2026-09-30T12:00:00Z');
const defaults = {
  enabled: true, status: '', threatCount: 0, nextImpact: 0, now,
  stationName: 'Auto Station', blockedLabel: 'Blocked',
};
const cases = [
  ['threat preserves the incoming warning', { status: 'threat', threatCount: 2, nextImpact: now + 300_000 }, 'warning', '2 incoming · 5m'],
  ['threat without impact preserves checking', { status: 'threat', threatCount: 2 }, 'warning', '2 incoming · checking'],
  ['blocked with incoming matches threat', { status: 'blocked', threatCount: 2, nextImpact: now + 300_000 }, 'warning', '2 incoming · 5m'],
  ['blocked without incoming uses existing labels', { status: 'blocked' }, 'warning', 'Auto Station · Blocked'],
  ['blocked fallback uses supplied localized labels', { status: 'blocked', stationName: 'Station', blockedLabel: 'Blockiert' }, 'warning', 'Station · Blockiert'],
  ['blocked with count alone keeps incoming visible', { status: 'blocked', threatCount: 1 }, 'warning', '1 incoming · checking'],
  ['blocked with impact alone retains the countdown', { status: 'blocked', nextImpact: now + 300_000 }, 'warning', '0 incoming · 5m'],
  ['blocked at impact shows now', { status: 'blocked', threatCount: 1, nextImpact: now }, 'warning', '1 incoming · now'],
  ['blocked rounds impact seconds as before', { status: 'blocked', threatCount: 1, nextImpact: now + 61_001 }, 'warning', '1 incoming · 1m 2s'],
  ['protected with incoming stays healthy', { status: 'protected', threatCount: 2 }, 'on', '2 incoming protected'],
  ['protected without incoming stays healthy', { status: 'protected' }, 'on', 'Troops protected'],
  ['healthy with no status stays armed', {}, 'on', 'Auto Station armed'],
  ['disabled remains off even when blocked', { enabled: false, status: 'blocked' }, 'off', 'Auto Station off'],
  ['evacuating remains a warning', { status: 'evacuating' }, 'warning', 'Auto Station evacuating…'],
  ['recalling remains healthy', { status: 'recalling' }, 'on', 'Auto Station recalling…'],
  ['waiting remains a warning', { status: 'waiting' }, 'warning', 'Auto Station waiting'],
  ['error retains its error tone', { status: 'error' }, 'error', 'Auto Station error'],
];

for (const [name, overrides, tone, text] of cases) {
  test(name, () => {
    const input = Object.freeze({ ...defaults, ...overrides });
    assert.deepEqual(stationHeaderPill(input), { tone, text });
  });
}
