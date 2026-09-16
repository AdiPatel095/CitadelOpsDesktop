import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { test } from 'node:test';
import ts from 'typescript';

const source = await readFile(new URL('../src/settings/AutoBuyerClientState.ts', import.meta.url), 'utf8');
const compiled = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.ES2022, target: ts.ScriptTarget.ES2022 },
});
const {
  autoBuyerSpecialistRuntimeStatus,
  specialistMinimumDaysError,
  specialistRubyCeilingError,
} = await import(`data:text/javascript;base64,${Buffer.from(compiled.outputText).toString('base64')}`);

const now = Date.parse('2026-09-15T16:00:00Z');
const rule = { enabled: true, id: 0, minimumDays: 14, maximumRubyCostPerPurchase: 625 };
const freshRuntime = {
  timersObservedAt: new Date(now - 10_000).toISOString(),
  timersCurrentSession: true,
  rubyBalance: 50_000,
  rubyObservedAt: new Date(now - 10_000).toISOString(),
  rubyCurrentSession: true,
};
const status = (overrides = {}) => autoBuyerSpecialistRuntimeStatus(
  overrides.rule ?? rule,
  overrides.booster,
  overrides.latest ?? null,
  overrides.safeMaximum ?? 625,
  overrides.minimumRubyReserve ?? 1_000,
  overrides.runtime ?? freshRuntime,
  3_600,
  now,
);

test('specialist limits reject raw out-of-range and fractional values without normalization', () => {
  assert.equal(specialistMinimumDaysError(14), '');
  assert.equal(specialistMinimumDaysError(365), '');
  assert.match(specialistMinimumDaysError(13), /14 to 365/);
  assert.match(specialistMinimumDaysError(365.5), /whole number/);
  assert.match(specialistMinimumDaysError(366), /14 to 365/);

  assert.equal(specialistRubyCeilingError(625, 625), '');
  assert.match(specialistRubyCeilingError(624, 625), /at least 625/);
  assert.match(specialistRubyCeilingError(625.5, 625), /whole/);
  assert.match(specialistRubyCeilingError(-1, 625), /non-negative/);
  assert.match(specialistRubyCeilingError(0, 0), /unavailable/);
});

test('specialist status distinguishes authoritative coverage and timer freshness', () => {
  assert.equal(status({ booster: { permanent: true } }), 'Covered permanently');
  assert.equal(status({ booster: { expiresAt: new Date(now + 15 * 86_400_000).toISOString() } }), 'Covered');
  assert.equal(status({ runtime: { ...freshRuntime, timersObservedAt: undefined } }), 'Waiting for timer data');
  assert.equal(status({ runtime: { ...freshRuntime, timersCurrentSession: false } }), 'Waiting for current-session timer data');
  assert.equal(status({ runtime: { ...freshRuntime, timersObservedAt: new Date(now - 3_600_000).toISOString() } }), 'Waiting for fresh timer data');
  assert.equal(status({ booster: undefined }), 'Inactive · Ready to activate');
});

test('specialist status distinguishes receipt outcomes without overriding newer covered state', () => {
  const recent = { attemptedAt: new Date(now - 5_000).toISOString(), updatedAt: new Date(now - 5_000).toISOString() };
  assert.equal(status({ latest: { ...recent, outcome: 'purchasing' } }), 'Purchasing');
  assert.equal(status({ latest: { ...recent, outcome: 'verifying' } }), 'Verifying');
  assert.equal(status({ latest: { ...recent, outcome: 'rejected' } }), 'Rejected');
  assert.equal(status({ latest: { ...recent, outcome: 'not-sent' } }), 'Not sent · Waiting');
  assert.equal(status({ latest: { ...recent, outcome: 'expired-unconfirmed' } }), 'Unresolved');
  assert.equal(status({ latest: { ...recent, outcome: 'activation-confirmed-spend-unresolved' } }), 'Unresolved');

  const covered = { expiresAt: new Date(now + 15 * 86_400_000).toISOString() };
  assert.equal(status({ booster: covered, latest: { ...recent, outcome: 'rejected' } }), 'Covered');

  const oldReceipt = {
    outcome: 'rejected',
    attemptedAt: new Date(now - 30_000).toISOString(),
    updatedAt: new Date(now - 30_000).toISOString(),
  };
  assert.equal(status({ booster: { expiresAt: new Date(now + 2 * 86_400_000).toISOString() }, latest: oldReceipt }), 'Ready to renew');
  assert.equal(status({ latest: { ...oldReceipt, outcome: 'not-sent' } }), 'Inactive · Ready to activate');
  assert.equal(status({ latest: { ...oldReceipt, outcome: 'unresolved' } }), 'Unresolved');
});

test('specialist status separates price, ruby authority, freshness, and reserve blocks', () => {
  assert.equal(status({ safeMaximum: 0 }), 'Inactive · Waiting for validated price');
  assert.equal(status({ rule: { ...rule, maximumRubyCostPerPurchase: 624 } }), 'Inactive · Waiting: ceiling below validated maximum');
  assert.equal(status({ runtime: { ...freshRuntime, rubyBalance: undefined } }), 'Inactive · Waiting for ruby balance data');
  assert.equal(status({ runtime: { ...freshRuntime, rubyCurrentSession: false } }), 'Inactive · Waiting for current-session ruby balance');
  assert.equal(status({ runtime: { ...freshRuntime, rubyObservedAt: new Date(now - 60_001).toISOString() } }), 'Inactive · Waiting for fresh ruby balance');
  assert.equal(status({ runtime: { ...freshRuntime, rubyBalance: 1_624 }, minimumRubyReserve: 1_000 }), 'Inactive · Waiting for ruby reserve');
  assert.equal(status({ runtime: { ...freshRuntime, rubyBalance: 0 }, minimumRubyReserve: 0 }), 'Inactive · Waiting for ruby reserve');
});

test('modal refreshes runtime projection in place and renders truthful evidence', async () => {
  const [modal, contracts] = await Promise.all([
    readFile(new URL('../src/settings/components/AutoBuyerSettingsModal.tsx', import.meta.url), 'utf8'),
    readFile(new URL('../src/api/Contracts.ts', import.meta.url), 'utf8'),
  ]);
  assert.match(modal, /AUTO_BUYER_PROJECTION_REFRESH_MS = 15_000/);
  assert.match(modal, /window\.setTimeout\(refreshProjection, AUTO_BUYER_PROJECTION_REFRESH_MS\)/);
  assert.match(modal, /preserveRawValue/);
  assert.match(modal, /specialistMinimumDaysError/);
  assert.match(modal, /specialistRubyCeilingError/);
  assert.match(modal, /current\?\.permanent \? 'Permanent' : formatRemaining/);
  assert.match(modal, /latest \? <p/);
  assert.match(modal, /\(latest\.rubyAfter \?\? 0\)\.toLocaleString\(\)/);
  assert.match(modal, /formatObservedTimer\(latest\.timerAfter\)/);
  assert.match(contracts, /specialistRuntime\?: \{/);
  assert.match(contracts, /evidence\?: Array<\{/);
});
