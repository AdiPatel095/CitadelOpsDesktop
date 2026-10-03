import { buildPresetDocumentUpdate } from '../configuration/PresetDocumentUpdate';

/**
 * Generic core of app-created preset records (CIT-15 attack presets, CIT-16
 * defense presets). A module's inline setup is persisted as an ordinary record
 * in a preset document (`attacks.presets`, `defense.presets`) carrying an `app`
 * marker, so the unchanged runtime keeps resolving compositions by preset id.
 * The marker, not the id, defines ownership: only the owning module slot edits
 * the record, and only while it is the sole referrer. Any other reuse promotes
 * the record to a normal user preset by clearing the marker.
 *
 * Document-specific adapters: `attackPresets/AppCreatedPresets.ts`,
 * `defensePresets/AppCreatedDefensePresets.ts`.
 */

/** Present only on records created by a module's inline setup. */
export interface AppCreatedPresetMarker {
  section: string;
  slot: string;
}

export interface AppCreatedRecord {
  id: string;
  name: string;
  app?: AppCreatedPresetMarker;
}

/** Raw preset document as written to configuration. */
export type PresetDocumentValue = Record<string, unknown> & { version: 1; presets: unknown[] };

/** A module field holding a record id, as far as ownership decisions need it. */
export interface RecordReference {
  section: string;
  slot: string;
  presetId: string;
}

/** Malformed markers are dropped, so a damaged record is treated as an ordinary user preset. */
export function parseAppCreatedPresetMarker(value: unknown): AppCreatedPresetMarker | undefined {
  if (value == null || typeof value !== 'object' || Array.isArray(value)) return undefined;
  const record = value as Record<string, unknown>;
  if (typeof record.section !== 'string' || typeof record.slot !== 'string') return undefined;
  const section = record.section.trim();
  const slot = record.slot.trim();
  return section && slot ? { section, slot } : undefined;
}

export function recordOwner(record: AppCreatedRecord): AppCreatedPresetMarker | null {
  return record.app ? { section: record.app.section, slot: record.app.slot } : null;
}

export function isRecordOwnedBy(record: AppCreatedRecord, section: string, slot: string): boolean {
  return record.app?.section === section && record.app.slot === slot;
}

/** Not derivable from the owner: a promoted record must never collide with the owner's next record. */
export function newAppCreatedRecordId(section: string, slot: string): string {
  const bytes = new Uint8Array(4);
  globalThis.crypto.getRandomValues(bytes);
  const suffix = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
  return `app:${section}:${slot}:${suffix}`;
}

/** Generated title with a numeric suffix on collision (case-insensitive, trimmed). */
export function uniqueRecordName(existing: readonly { name: string }[], base: string): string {
  const taken = new Set(existing.map((record) => record.name.trim().toLowerCase()));
  let candidate = base;
  let suffix = 2;
  while (taken.has(candidate.toLowerCase())) candidate = `${base} ${suffix++}`;
  return candidate;
}

export function withoutRecordMarker<T extends AppCreatedRecord>(record: T): T {
  const copy = { ...record };
  delete copy.app;
  return copy;
}

export interface UpsertOwnedRecordOptions<T extends AppCreatedRecord> {
  /** True when the existing owned record already holds the requested composition. */
  unchanged: (existing: T) => boolean;
  /** The existing owned record with the new composition. */
  update: (existing: T) => T;
  /** A new owned record with this id (the adapter sets name, timestamps and the marker). */
  create: (id: string) => T;
}

/**
 * Upserts this slot's app-created record. The current record is reused only
 * when its id matches AND its marker names this slot; otherwise a new record is
 * created. An unchanged composition leaves the document untouched.
 */
export function upsertOwnedRecord<T extends AppCreatedRecord>(
  rawDocument: unknown,
  current: readonly T[],
  section: string,
  slot: string,
  currentId: string,
  options: UpsertOwnedRecordOptions<T>,
): { document: PresetDocumentValue; presetId: string; changed: boolean } {
  const existing = currentId
    ? current.find((record) => record.id === currentId && isRecordOwnedBy(record, section, slot))
    : undefined;
  if (existing && options.unchanged(existing)) {
    return { document: buildPresetDocumentUpdate(rawDocument ?? {}, current, current), presetId: existing.id, changed: false };
  }
  const next = existing ? options.update(existing) : options.create(newAppCreatedRecordId(section, slot));
  const records = existing
    ? current.map((record) => record === existing ? next : record)
    : [...current, next];
  return { document: buildPresetDocumentUpdate(rawDocument ?? {}, current, records), presetId: next.id, changed: true };
}

/** Clears the `app` marker of the named app-created records; everything else is kept verbatim. */
export function promoteRecords<T extends AppCreatedRecord>(
  rawDocument: unknown,
  current: readonly T[],
  ids: readonly string[],
): { document: PresetDocumentValue; promoted: string[] } {
  const targets = new Set(ids);
  const promoted: string[] = [];
  const records = current.map((record) => {
    if (!targets.has(record.id) || !record.app) return record;
    promoted.push(record.id);
    return withoutRecordMarker(record);
  });
  return { document: clearRawMarkers(buildPresetDocumentUpdate(rawDocument ?? {}, current, records), rawDocument, promoted), promoted };
}

/**
 * Cleanup for one module section: deletes this section's app-created records
 * that no reference points to, and promotes this section's records that another
 * slot references. Never touches user presets or other sections' records.
 */
export function removeUnreferencedRecords<T extends AppCreatedRecord>(
  rawDocument: unknown,
  current: readonly T[],
  section: string,
  references: readonly RecordReference[],
): { document: PresetDocumentValue; removed: string[]; promoted: string[] } {
  const removed: string[] = [];
  const promoted: string[] = [];
  const records: T[] = [];
  for (const record of current) {
    if (record.app?.section !== section) {
      records.push(record);
      continue;
    }
    const referrers = references.filter((reference) => reference.presetId === record.id);
    if (referrers.length === 0) {
      removed.push(record.id);
      continue;
    }
    const marker = record.app;
    if (referrers.some((reference) => reference.section !== marker.section || reference.slot !== marker.slot)) {
      promoted.push(record.id);
      records.push(withoutRecordMarker(record));
      continue;
    }
    records.push(record);
  }
  return { document: clearRawMarkers(buildPresetDocumentUpdate(rawDocument ?? {}, current, records), rawDocument, promoted), removed, promoted };
}

/**
 * Promotion only clears the marker: write the original raw record without `app`
 * so unknown or forward-version per-record fields survive (CIT-15 L1).
 */
export function clearRawMarkers(
  document: PresetDocumentValue,
  rawDocument: unknown,
  promoted: readonly string[],
): PresetDocumentValue {
  if (promoted.length === 0) return document;
  const targets = new Set(promoted);
  const rawRecords = rawDocument != null && typeof rawDocument === 'object' && Array.isArray((rawDocument as { presets?: unknown }).presets)
    ? (rawDocument as { presets: unknown[] }).presets
    : [];
  const rawById = new Map<string, Record<string, unknown>>();
  for (const candidate of rawRecords) {
    if (candidate == null || typeof candidate !== 'object' || Array.isArray(candidate)) continue;
    const record = candidate as Record<string, unknown>;
    if (typeof record.id === 'string' && targets.has(record.id)) rawById.set(record.id, record);
  }
  return {
    ...document,
    presets: document.presets.map((entry) => {
      const id = entry != null && typeof entry === 'object' ? (entry as { id?: unknown }).id : undefined;
      const raw = typeof id === 'string' ? rawById.get(id) : undefined;
      if (!raw) return entry;
      const copy = { ...raw };
      delete copy.app;
      return copy;
    }),
  };
}

export function emptyRecordDocument(): PresetDocumentValue {
  return { version: 1, presets: [] };
}
