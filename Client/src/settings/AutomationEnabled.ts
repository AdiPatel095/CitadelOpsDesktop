export interface AutomationEnabledControl {
  configured: boolean;
  enabled: boolean;
  expiresAtMs?: number;
}

export function parseAutomationEnabledControls(
  raw: unknown,
  nowMs = Date.now(),
): Record<string, AutomationEnabledControl> {
  if (!isRecord(raw)) return {};
  const result: Record<string, AutomationEnabledControl> = {};
  for (const [feature, value] of Object.entries(raw)) {
    if (typeof value === 'boolean') {
      result[feature] = { configured: value, enabled: value };
      continue;
    }
    if (!isRecord(value) || typeof value.enabled !== 'boolean' || typeof value.expiresAt !== 'string') continue;
    const expiresAtMs = Date.parse(value.expiresAt);
    if (!Number.isFinite(expiresAtMs) || expiresAtMs <= 0) continue;
    result[feature] = {
      configured: value.enabled,
      enabled: value.enabled && nowMs < expiresAtMs,
      expiresAtMs,
    };
  }
  return result;
}

export function nextAutomationExpirationMs(
  controls: Record<string, AutomationEnabledControl>,
  nowMs = Date.now(),
): number {
  return Object.values(controls).reduce((next, control) => {
    const expiresAtMs = control.expiresAtMs ?? 0;
    if (!control.configured || expiresAtMs <= nowMs) return next;
    return next === 0 || expiresAtMs < next ? expiresAtMs : next;
  }, 0);
}

export function timedAutomationEnabledValue(durationMinutes: number, nowMs = Date.now()): {
  enabled: true;
  expiresAt: string;
} {
  const minutes = Math.max(1, Math.min(10_080, Math.trunc(durationMinutes)));
  return {
    enabled: true,
    expiresAt: new Date(nowMs + minutes * 60_000).toISOString(),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
