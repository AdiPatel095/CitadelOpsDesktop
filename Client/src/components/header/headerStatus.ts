import { AUTOMATION_FEATURE_ORDER } from '../../settings/automationFeatureNames';
import type { SettingsFeatureId } from '../../settings/disclosure/placement';
import type { PlayerStatus, PlayerStatusDescription } from '../../settings/readiness/playerStatus';

export type AttentionStatus = 'needs-attention' | 'blocked' | 'paused';

export interface FeatureStatus {
  featureId: SettingsFeatureId;
  /** Classify with desktopLocked = false, so the desktop Lock alone does not count. */
  status: PlayerStatus;
  reason: PlayerStatusDescription['reason'];
}

export interface SignalInput {
  now: number;
  station: { enabled: boolean; threatCount: number; nextImpactAt: number; status: PlayerStatus }; // 0 = unknown impact
  bird: { enabled: boolean; nextWakeAt: number; nextCastleName: string }; // 0 = no wake
  features: FeatureStatus[];
}

export type PrioritySignal =
  | { kind: 'incoming'; count: number; firstImpactInMs: number | null }
  | { kind: 'attention'; count: number; worst: AttentionStatus }
  | { kind: 'nextBird'; castleName: string; dueInMs: number };

const ATTENTION_RANK: Record<AttentionStatus, number> = { 'needs-attention': 0, blocked: 1, paused: 2 };
const FEATURE_RANK = new Map<string, number>(AUTOMATION_FEATURE_ORDER.map((id, index) => [id, index]));

function isAttentionStatus(status: PlayerStatus): status is AttentionStatus {
  return status === 'needs-attention' || status === 'blocked' || status === 'paused';
}

/** Preserve entries and their localized reasons; never sort the caller's array in place. */
export function attentionEntries(features: FeatureStatus[]): FeatureStatus[] {
  return features.filter((entry): entry is FeatureStatus & { status: AttentionStatus } => isAttentionStatus(entry.status))
    .sort((a, b) => {
      const severity = ATTENTION_RANK[a.status] - ATTENTION_RANK[b.status];
      if (severity !== 0) return severity;
      const order = (FEATURE_RANK.get(a.featureId) ?? AUTOMATION_FEATURE_ORDER.length) - (FEATURE_RANK.get(b.featureId) ?? AUTOMATION_FEATURE_ORDER.length);
      if (order < 0 || order > 0) return order;
      return a.featureId < b.featureId ? -1 : a.featureId > b.featureId ? 1 : 0;
    });
}

export function prioritySignal(input: SignalInput): PrioritySignal | null {
  const { now, station, bird, features } = input;
  if (station.enabled && station.threatCount > 0 && station.status !== 'needs-attention' && station.status !== 'blocked') {
    return {
      kind: 'incoming',
      count: station.threatCount,
      firstImpactInMs: station.nextImpactAt > 0 ? Math.max(0, station.nextImpactAt - now) : null,
    };
  }
  const attention = attentionEntries(features);
  const worst = attention[0]?.status;
  if (worst !== undefined && isAttentionStatus(worst)) {
    return { kind: 'attention', count: attention.length, worst };
  }
  if (bird.enabled && bird.nextWakeAt > 0) {
    return { kind: 'nextBird', castleName: bird.nextCastleName, dueInMs: Math.max(0, bird.nextWakeAt - now) };
  }
  return null;
}
