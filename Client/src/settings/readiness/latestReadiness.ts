import type { ReadinessReport } from './Readiness';

/**
 * One readiness report per feature (CIT-20). The Automation row publishes the report it evaluated together with a
 * digest of the SAVED configuration it read; the Start check reuses it when the digest still matches and it is
 * fresh, and otherwise evaluates itself with the same catalog inputs. So the row and the confirmation cannot
 * disagree about what is blocked.
 */
export const LATEST_READINESS_MAX_AGE_MS = 30_000;

interface Published {
  report: ReadinessReport;
  digest: string;
  at: number;
}

const published = new Map<string, Published>();

/** FNV-1a over the JSON of the saved sections: cheap, stable, and changes whenever a saved value changes. */
export function savedSectionsDigest(sections: Record<string, unknown> | undefined): string {
  const text = JSON.stringify(sections ?? {});
  let hash = 0x811c9dc5;
  for (let index = 0; index < text.length; index += 1) {
    hash ^= text.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return `${text.length}:${hash.toString(16)}`;
}

export function publishReadiness(featureId: string, report: ReadinessReport, digest: string, now: number = Date.now()): void {
  published.set(featureId, { report, digest, at: now });
}

/** The published report for the same saved configuration, when it is not older than the maximum age. */
export function latestReadiness(featureId: string, digest: string, now: number = Date.now()): ReadinessReport | null {
  const entry = published.get(featureId);
  if (!entry || entry.digest !== digest || now - entry.at > LATEST_READINESS_MAX_AGE_MS) return null;
  return entry.report;
}

export function resetLatestReadinessForTests(): void {
  published.clear();
}
