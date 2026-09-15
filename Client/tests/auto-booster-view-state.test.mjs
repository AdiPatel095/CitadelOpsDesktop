import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const view = await vite.ssrLoadModule('/src/settings/AutoBoosterViewState.ts');

after(async () => {
  await vite.close();
});

const now = Date.parse('2026-09-15T18:00:00Z');
const currentEnd = '2026-09-16T02:30:00Z';
const nextEnd = '2026-09-17T02:30:00Z';

function record(outcome, overrides = {}) {
  return {
    globalEffectId: 2,
    occurrenceEndsAt: currentEnd,
    expiresAt: currentEnd,
    quotedRubyCost: 2500,
    quotedBonusValue: 50,
    minimumRubyReserve: 10000,
    rubyBefore: 20000,
    rubyBeforeObservedAt: '2026-09-15T17:59:50Z',
    requestedAt: '2026-09-15T18:00:00Z',
    dispatchedAt: '2026-09-15T18:00:01Z',
    requestOpcode: 'agb',
    debitUnverified: true,
    outcome,
    ...overrides,
  };
}

function inventory({ boost, purchase, endsAt = currentEnd } = {}) {
  return {
    globalEffects: { 2: { globalEffectId: 2, strength: 10, endsAt } },
    globalEffectBoosterOffers: { 2: { globalEffectId: 2, rubyCost: 2500, bonusValue: 50 } },
    globalEffectBoosts: boost === undefined ? undefined : { 2: boost },
    globalEffectPurchases: purchase === undefined ? undefined : { 2: purchase },
  };
}

test('current matching boosted evidence is active only until event expiry', () => {
  const data = inventory({ boost: { globalEffectId: 2, boosted: true, occurrenceEndsAt: currentEnd, observedAt: '2026-09-15T18:00:02Z' } });
  assert.equal(view.deriveAutoBoosterViewState(data, now).status, 'active');
  assert.equal(view.deriveAutoBoosterViewState(data, Date.parse(currentEnd)).status, 'waiting');
  assert.equal(view.formatAutoBoosterRemaining(currentEnd, Date.parse(currentEnd)), 'Window ended');
});

for (const [outcome, expected] of [
  ['accepted', 'accepted'],
  ['unresolved', 'unresolved'],
  ['rejected', 'rejected'],
]) {
  test(`${outcome} current purchase remains distinct from active`, () => {
    const purchase = record(outcome, outcome === 'rejected' ? {} : {
      dispatchedAt: '0001-01-01T00:00:00Z',
      activationObservedAt: '0001-01-01T00:00:00Z',
    });
    const derived = view.deriveAutoBoosterViewState(inventory({ purchase }), now);
    assert.equal(derived.status, expected);
    assert.notEqual(derived.statusLabel, 'Active · covered');
    assert.equal(derived.purchaseIsCurrent, true);
    if (outcome !== 'rejected') {
      assert.equal(view.formatAutoBoosterRequestProgress(purchase), 'Prepared · activation not observed');
    }
  });
}

test('accepted acknowledgement does not treat a Go zero activation time as observed', () => {
  const purchase = record('accepted', {
    resultCode: 0,
    resultObservedAt: '2026-09-15T18:00:02Z',
    activationObservedAt: '0001-01-01T00:00:00Z',
  });
  assert.equal(view.formatAutoBoosterRequestProgress(purchase), 'Dispatched · activation not observed');
  assert.equal(view.hasMeaningfulAutoBoosterTime(purchase.activationObservedAt), false);
});

test('matching confirmed purchase is covered while the current occurrence is live', () => {
  const derived = view.deriveAutoBoosterViewState(inventory({ purchase: record('confirmed') }), now);
  assert.equal(derived.status, 'active');
  assert.equal(derived.purchaseHeading, 'Current purchase record');
});

test('manual activation does not invent an automated request or purchase evidence', () => {
  const manual = record('confirmed', {
    quotedRubyCost: 0,
    quotedBonusValue: 0,
    minimumRubyReserve: 0,
    rubyBefore: 0,
    rubyBeforeObservedAt: '0001-01-01T00:00:00Z',
    requestedAt: '0001-01-01T00:00:00Z',
    dispatchedAt: undefined,
    requestOpcode: '',
    operationId: undefined,
    activationObservedAt: '2026-09-15T18:00:02Z',
  });
  const derived = view.deriveAutoBoosterViewState(inventory({
    boost: { globalEffectId: 2, boosted: true, occurrenceEndsAt: currentEnd, observedAt: '2026-09-15T18:00:02Z' },
    purchase: manual,
  }), now);
  assert.equal(derived.status, 'active');
  assert.equal(derived.purchaseHasRequest, false);
  assert.equal(derived.purchaseHeading, 'Current activation record');
  assert.equal(view.hasAutoBoosterRequestEvidence(manual), false);
});

test('stale accepted record is historical and cannot override a new occurrence', () => {
  const derived = view.deriveAutoBoosterViewState(inventory({ purchase: record('accepted'), endsAt: nextEnd }), now);
  assert.equal(derived.status, 'waiting');
  assert.equal(derived.purchaseIsCurrent, false);
  assert.equal(derived.purchaseHeading, 'Previous purchase record');
});

test('expired confirmed record is historical and cannot remain active', () => {
  const expiredEnd = '2026-09-15T17:30:00Z';
  const derived = view.deriveAutoBoosterViewState(inventory({
    purchase: record('confirmed', { occurrenceEndsAt: expiredEnd, expiresAt: expiredEnd }),
    endsAt: expiredEnd,
  }), now);
  assert.equal(derived.status, 'waiting');
  assert.equal(derived.purchaseHeading, 'Expired purchase record');
  assert.equal(derived.expiresAt, undefined);
});

test('missing current boost status never claims the effect is unboosted', () => {
  const derived = view.deriveAutoBoosterViewState(inventory(), now);
  assert.equal(derived.statusKnown, false);
  assert.equal(derived.statusLabel, 'Checking status');
});

test('remaining duration derives from the occurrence end', () => {
  assert.equal(view.formatAutoBoosterRemaining(currentEnd, now), '8h 30m remaining');
});

test('observed balance delta stays separate from unverified debit attribution', () => {
  const purchase = record('confirmed', { rubyAfterKnown: true, rubyAfter: 17500, observedRubyChange: 2500, debitUnverified: true });
  assert.equal(view.formatObservedRubyChange(purchase), '2,500 fewer rubies observed');
  assert.equal(purchase.debitUnverified, true);
});
