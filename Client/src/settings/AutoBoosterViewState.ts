import type {
  EventInventoryStateV2,
  GlobalEffectBoosterOfferV2,
  GlobalEffectPurchaseRecordV2,
} from '../api/Contracts';
import { AUTO_BOOSTER_GLOBAL_EFFECT_ID, AUTO_BOOSTER_RUBY_COST } from './AutoBoosterClientState';

export type AutoBoosterDisplayStatus = 'active' | 'accepted' | 'unresolved' | 'rejected' | 'waiting';

export interface AutoBoosterViewState {
  status: AutoBoosterDisplayStatus;
  statusLabel: string;
  statusDetail: string;
  expiresAt?: string;
  offer?: GlobalEffectBoosterOfferV2;
  purchase?: GlobalEffectPurchaseRecordV2;
  purchaseIsCurrent: boolean;
  purchaseHasRequest: boolean;
  purchaseHeading?: string;
  statusKnown: boolean;
}

function instant(value: string | undefined): number | undefined {
  if (!value) return undefined;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : undefined;
}

function sameInstant(left: string | undefined, right: string | undefined): boolean {
  const leftTime = instant(left);
  const rightTime = instant(right);
  return leftTime !== undefined && rightTime !== undefined && leftTime === rightTime;
}

export function hasMeaningfulAutoBoosterTime(value: string | undefined): boolean {
  const parsed = instant(value);
  return parsed !== undefined && new Date(parsed).getUTCFullYear() > 1;
}

export function hasAutoBoosterRequestEvidence(record: GlobalEffectPurchaseRecordV2): boolean {
  return record.quotedRubyCost > 0
    && record.requestOpcode.trim().length > 0
    && hasMeaningfulAutoBoosterTime(record.requestedAt);
}

export function formatAutoBoosterRequestProgress(record: GlobalEffectPurchaseRecordV2): string {
  return `${hasMeaningfulAutoBoosterTime(record.dispatchedAt) ? 'Dispatched' : 'Prepared'} · ${hasMeaningfulAutoBoosterTime(record.activationObservedAt) ? 'activation observed' : 'activation not observed'}`;
}

function currentGuardDetail(
  offer: GlobalEffectBoosterOfferV2 | undefined,
  policyDetail: string | undefined,
  statusKnown: boolean,
): string {
  if (policyDetail?.trim()) return policyDetail.trim();
  if (!offer) return 'Waiting for a current account-specific quote.';
  if (offer.rubyCost !== AUTO_BOOSTER_RUBY_COST || offer.bonusValue <= 0) {
    return `The current ${offer.rubyCost.toLocaleString()}-ruby quote is outside the approved purchase guard.`;
  }
  if (!statusKnown) return 'Waiting for the current boosted-status result before any purchase.';
  return 'The current quote is approved; the ruby reserve is checked again before purchase.';
}

export function deriveAutoBoosterViewState(
  inventory: EventInventoryStateV2 | null | undefined,
  now: number,
  policyDetail?: string,
): AutoBoosterViewState {
  const key = String(AUTO_BOOSTER_GLOBAL_EFFECT_ID);
  const effect = inventory?.globalEffects?.[key];
  const offer = inventory?.globalEffectBoosterOffers?.[key];
  const boost = inventory?.globalEffectBoosts?.[key];
  const purchase = inventory?.globalEffectPurchases?.[key];
  const effectEnd = instant(effect?.endsAt);
  const effectIsCurrent = effect?.globalEffectId === AUTO_BOOSTER_GLOBAL_EFFECT_ID
    && effectEnd !== undefined
    && effectEnd > now;
  const boostMatches = effectIsCurrent
    && boost?.globalEffectId === AUTO_BOOSTER_GLOBAL_EFFECT_ID
    && sameInstant(boost.occurrenceEndsAt, effect?.endsAt);
  const purchaseIsCurrent = Boolean(
    effectIsCurrent
    && purchase?.globalEffectId === AUTO_BOOSTER_GLOBAL_EFFECT_ID
    && sameInstant(purchase.occurrenceEndsAt, effect?.endsAt)
    && sameInstant(purchase.expiresAt, effect?.endsAt)
    && (instant(purchase.expiresAt) ?? 0) > now,
  );
  const purchaseHasRequest = purchase ? hasAutoBoosterRequestEvidence(purchase) : false;
  const statusKnown = Boolean(boostMatches);
  const guardDetail = currentGuardDetail(offer, policyDetail, statusKnown);

  let status: AutoBoosterDisplayStatus = 'waiting';
  let statusLabel = statusKnown ? 'Waiting' : 'Checking status';
  let statusDetail = guardDetail;

  if (boostMatches && boost?.boosted === true) {
    status = 'active';
    statusLabel = 'Active · covered';
    statusDetail = 'The fortress-speed boost is active for this event window.';
  } else if (purchaseIsCurrent && purchase) {
    if (purchase.outcome === 'confirmed') {
      status = 'active';
      statusLabel = 'Active · covered';
      statusDetail = 'Activation was observed for this event window.';
    } else if (purchase.outcome === 'accepted') {
      status = 'accepted';
      statusLabel = 'Accepted · confirming';
      statusDetail = 'The game accepted the request; activation confirmation is still pending.';
    } else if (purchase.outcome === 'unresolved') {
      status = 'unresolved';
      statusLabel = 'Unresolved';
      statusDetail = 'The request may have been sent. Another purchase stays blocked until the game state is reconciled.';
    } else if (purchase.outcome === 'rejected') {
      status = 'rejected';
      statusLabel = 'Rejected';
      statusDetail = purchase.detail?.trim() || 'The game rejected the purchase for this event window.';
    }
  }

  const purchaseExpired = purchase && (instant(purchase.expiresAt) ?? 0) <= now;
  return {
    status,
    statusLabel,
    statusDetail,
    expiresAt: effectIsCurrent ? effect?.endsAt : undefined,
    offer,
    purchase,
    purchaseIsCurrent,
    purchaseHasRequest,
    purchaseHeading: purchase
      ? purchaseHasRequest
        ? purchaseIsCurrent ? 'Current purchase record' : purchaseExpired ? 'Expired purchase record' : 'Previous purchase record'
        : purchaseIsCurrent ? 'Current activation record' : purchaseExpired ? 'Expired activation record' : 'Previous activation record'
      : undefined,
    statusKnown,
  };
}

export function formatAutoBoosterRemaining(value: string | undefined, now: number): string {
  const end = instant(value);
  if (end === undefined) return 'No active window';
  const remainingMs = end - now;
  if (remainingMs <= 0) return 'Window ended';
  const totalMinutes = Math.max(1, Math.ceil(remainingMs / 60_000));
  const days = Math.floor(totalMinutes / 1_440);
  const hours = Math.floor((totalMinutes % 1_440) / 60);
  const minutes = totalMinutes % 60;
  if (days > 0) return `${days}d${hours > 0 ? ` ${hours}h` : ''} remaining`;
  return hours > 0 ? `${hours}h${minutes > 0 ? ` ${minutes}m` : ''} remaining` : `${minutes}m remaining`;
}

export function formatObservedRubyChange(record: GlobalEffectPurchaseRecordV2): string {
  if (!record.rubyAfterKnown) return 'No after balance observed';
  const change = record.observedRubyChange ?? (record.rubyBefore - (record.rubyAfter ?? record.rubyBefore));
  if (change > 0) return `${change.toLocaleString()} fewer rubies observed`;
  if (change < 0) return `${Math.abs(change).toLocaleString()} more rubies observed`;
  return 'No ruby change observed';
}
