import { useSyncExternalStore } from 'react';

/**
 * Draft recovery storage (CIT-19). A recovered draft is an unsaved editor draft written to this browser so a crash or a
 * closed window does not lose it. It is isolated by account/world and section, holds only the section value (plus an
 * opaque JSON of the editor's own unsaved sub-drafts), and is never applied by itself: the editor shows a banner and the
 * player chooses Restore or Discard. Nothing here saves configuration, writes `automation.enabled` or starts anything.
 *
 * Retained data is limited on purpose: only allowlisted configuration sections can be recorded, values that look like
 * credentials are refused, entries expire after `DRAFT_RETENTION_DAYS`, and a storage failure means recovery is simply
 * unavailable for that visit.
 */
export const DRAFT_STORAGE_PREFIX = 'citadelops.draft.v1.';
export const DRAFT_RETENTION_DAYS = 14;
const DAY_MS = 24 * 60 * 60 * 1000;
/** Largest recorded draft (serialized), so a runaway editor cannot fill the browser's storage. */
export const MAX_DRAFT_BYTES = 256 * 1024;

const ALLOWED_SECTIONS = new Set(['attackPresets', 'defense.presets', 'scheduler']);
const AUTOMATION_SECTION = /^automation\.[A-Za-z][A-Za-z0-9]*$/;
/** Names that indicate credential material; a draft containing one anywhere is never recorded. */
const CREDENTIAL_KEY = /pass(word|phrase)?|secret|token|credential|authorization|api[-_]?key|cookie|session[-_]?id/i;

/** Only settings sections can be recorded: never login, connection, browser or account sections. */
export function isRecoverableSection(section: string): boolean {
  return ALLOWED_SECTIONS.has(section) || AUTOMATION_SECTION.test(section);
}

export interface DraftRecoveryEntry {
  version: 1;
  section: string;
  accountKey: string;
  /** The section value the editor would save. */
  draft: unknown;
  /** The editor's other unsaved sub-drafts (for example an inline attack setup), opaque JSON. */
  extras?: unknown;
  /** Revision and digest of the saved section when the editor loaded it. */
  baseRevision: number;
  baseDigest: string;
  savedAt: string;
}

type DraftStorage = Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>;

function browserStorage(): DraftStorage | null {
  try {
    return globalThis.localStorage ?? null;
  } catch {
    return null;
  }
}

export const draftStorageKey = (accountKey: string, section: string): string => `${DRAFT_STORAGE_PREFIX}${accountKey}.${section}`;

function stableStringify(value: unknown): string {
  if (value === undefined) return 'null';
  if (value === null || typeof value !== 'object') return JSON.stringify(value) ?? 'null';
  if (Array.isArray(value)) return `[${value.map(stableStringify).join(',')}]`;
  const record = value as Record<string, unknown>;
  return `{${Object.keys(record).filter((key) => record[key] !== undefined).sort().map((key) => `${JSON.stringify(key)}:${stableStringify(record[key])}`).join(',')}}`;
}

/** Stable digest of a JSON value: key order does not matter, `undefined` is `null`. */
export function stableDigest(value: unknown): string {
  const text = stableStringify(value);
  let hash = 0x811c9dc5;
  for (let index = 0; index < text.length; index += 1) {
    hash ^= text.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return `${text.length}:${hash.toString(16)}`;
}

/** True when any object key in the value names credential material. */
export function containsCredentialKey(value: unknown, depth = 0): boolean {
  if (depth > 40 || value === null || typeof value !== 'object') return false;
  if (Array.isArray(value)) return value.some((item) => containsCredentialKey(item, depth + 1));
  return Object.entries(value as Record<string, unknown>).some(([key, item]) => CREDENTIAL_KEY.test(key) || containsCredentialKey(item, depth + 1));
}

export type RecordRefusal = 'section-not-allowed' | 'account-unknown' | 'credential-like' | 'too-large' | 'storage-unavailable';

/** Records a draft. Returns `null` when written, otherwise why it was refused. */
export function writeDraft(entry: DraftRecoveryEntry, storage: DraftStorage | null = browserStorage()): RecordRefusal | null {
  if (!entry.accountKey) return 'account-unknown';
  if (!isRecoverableSection(entry.section)) return 'section-not-allowed';
  if (containsCredentialKey(entry.draft) || containsCredentialKey(entry.extras)) return 'credential-like';
  if (!storage) return 'storage-unavailable';
  try {
    const text = JSON.stringify(entry);
    if (text.length > MAX_DRAFT_BYTES) return 'too-large';
    storage.setItem(draftStorageKey(entry.accountKey, entry.section), text);
    notifyDrafts();
    return null;
  } catch {
    return 'storage-unavailable';
  }
}

export function clearDraft(accountKey: string, section: string, storage: DraftStorage | null = browserStorage()): void {
  if (!storage || !accountKey) return;
  try {
    storage.removeItem(draftStorageKey(accountKey, section));
  } catch {
    // Nothing to clear when storage is unavailable.
  }
  notifyDrafts();
}

/** The recorded draft for one account and section; expired or malformed entries are dropped. */
export function readDraft(
  accountKey: string,
  section: string,
  now: number = Date.now(),
  storage: DraftStorage | null = browserStorage(),
): DraftRecoveryEntry | null {
  if (!storage || !accountKey || !isRecoverableSection(section)) return null;
  const key = draftStorageKey(accountKey, section);
  try {
    const raw = storage.getItem(key);
    if (!raw) return null;
    const parsed = JSON.parse(raw) as Partial<DraftRecoveryEntry> | null;
    const savedAt = typeof parsed?.savedAt === 'string' ? Date.parse(parsed.savedAt) : NaN;
    const valid = parsed != null
      && parsed.version === 1
      && parsed.section === section
      && parsed.accountKey === accountKey
      && typeof parsed.baseDigest === 'string'
      && Number.isFinite(savedAt)
      && parsed.draft !== undefined;
    if (!valid || now - savedAt > DRAFT_RETENTION_DAYS * DAY_MS || savedAt - now > DAY_MS) {
      storage.removeItem(key);
      return null;
    }
    return parsed as DraftRecoveryEntry;
  } catch {
    return null;
  }
}

/** Whether a recovered draft is waiting for this account and section. */
export function hasRecoveredDraft(accountKey: string, section: string, now: number = Date.now(), storage: DraftStorage | null = browserStorage()): boolean {
  return readDraft(accountKey, section, now, storage) !== null;
}

export interface DraftDifference {
  /** Dotted path of the differing value inside the section. */
  path: string;
  saved: unknown;
  recovered: unknown;
}

const MAX_DIFFERENCES = 200;

/** The differing leaf values between the saved section and the recovered draft, for the Compare view. */
export function compareDrafts(saved: unknown, recovered: unknown): DraftDifference[] {
  const differences: DraftDifference[] = [];
  const walk = (left: unknown, right: unknown, path: string) => {
    if (differences.length >= MAX_DIFFERENCES) return;
    if (stableStringify(left) === stableStringify(right)) return;
    const leftObject = left !== null && typeof left === 'object';
    const rightObject = right !== null && typeof right === 'object';
    if (leftObject && rightObject && Array.isArray(left) === Array.isArray(right)) {
      const keys = new Set([...Object.keys(left as object), ...Object.keys(right as object)]);
      for (const key of [...keys].sort()) {
        walk((left as Record<string, unknown>)[key], (right as Record<string, unknown>)[key], path ? `${path}.${key}` : key);
      }
      return;
    }
    differences.push({ path: path || '(section)', saved: left, recovered: right });
  };
  walk(saved, recovered, '');
  return differences;
}

export type RecoveredState = 'drop' | 'baseline-unchanged' | 'saved-since';

/** Digest of what an editor holds: the section value plus its opaque sub-drafts. */
export const draftDigest = (draft: unknown, extras: unknown): string => stableDigest({ d: draft, e: extras ?? null });

/**
 * What to do with a recorded draft when an editor has loaded (CIT-19): drop it when it is identical to what just loaded
 * (nothing to recover); offer Restore when the saved settings are unchanged since it was made; require Compare when they
 * were saved again in between.
 */
export function classifyRecovered(
  found: Pick<DraftRecoveryEntry, 'draft' | 'extras' | 'baseDigest'>,
  loaded: { draftDigest: string; savedDigest: string },
): RecoveredState {
  if (draftDigest(found.draft, found.extras) === loaded.draftDigest) return 'drop';
  return found.baseDigest === loaded.savedDigest ? 'baseline-unchanged' : 'saved-since';
}

/** The saved section changed while the editor stayed loaded: this editor's save (or one it made) succeeded. */
export function savedWhileLoaded(
  previous: { loadKey: string; digest: string } | null,
  current: { loadKey: string; digest: string },
): boolean {
  return previous !== null && previous.loadKey === current.loadKey && previous.digest !== current.digest;
}

/**
 * The recovery state machine of one open editor (CIT-19). It is pure: `useDraftRecovery` feeds it what it can see each
 * time an input changes and does what it answers. There is no timer and no baseline taken "after the editor loaded":
 * the loaded value is derived from the saved data (`loadedDigest`), so the order in which an editor applies its load
 * cannot matter.
 *
 * - `settled`: the editor's draft has equalled the loaded value since this load (or since the last save). Before that the
 *   editor is still applying what it loaded, so nothing it holds counts as a change. An editor whose draft never equals
 *   the loaded value never settles and records nothing (fail-safe).
 * - `dirty`: settled, and the draft now differs from the loaded value.
 * - `action`: `write` (record the draft: debounced while open, at once when the editor closes), `clear` (the draft was
 *   changed back to what was loaded: drop the record), `saved` (the saved section changed under an open editor: this
 *   editor's save succeeded, so drop the record and note the time), or `none`.
 */
export interface DraftRecoveryMachine {
  loadKey: string | null;
  savedDigest: string | null;
  settled: boolean;
  dirty: boolean;
}

export type DraftRecoveryAction = 'none' | 'write' | 'clear' | 'saved';

export interface DraftRecoveryInput {
  active: boolean;
  loadKey: string;
  /** Digest of what the draft holds when the player changed nothing (derived from the saved data). */
  loadedDigest: string;
  /** Digest of what the editor holds right now. */
  draftDigest: string;
  /** Digest of the saved section. */
  savedDigest: string;
}

export const INITIAL_DRAFT_RECOVERY_MACHINE: DraftRecoveryMachine = { loadKey: null, savedDigest: null, settled: false, dirty: false };

export function nextDraftRecoveryMachine(
  previous: DraftRecoveryMachine,
  input: DraftRecoveryInput,
): { machine: DraftRecoveryMachine; action: DraftRecoveryAction } {
  const sameLoad = previous.loadKey !== null && previous.loadKey === input.loadKey;
  // The saved section changed while the same load stayed open: this editor saved it. A save and a close in one step is
  // still a save, so the close must not write what was just saved as a draft.
  const saved = sameLoad && previous.savedDigest !== null && previous.savedDigest !== input.savedDigest;
  if (!input.active) {
    return { machine: INITIAL_DRAFT_RECOVERY_MACHINE, action: saved ? 'saved' : previous.dirty ? 'write' : 'none' };
  }
  const equal = input.draftDigest === input.loadedDigest;
  // A save makes the saved value the new loaded value, so the editor must settle on it again.
  const settled = equal || (sameLoad && previous.settled && !saved);
  const dirty = settled && !equal;
  const action: DraftRecoveryAction = saved ? 'saved' : dirty ? 'write' : previous.dirty && sameLoad ? 'clear' : 'none';
  return { machine: { loadKey: input.loadKey, savedDigest: input.savedDigest, settled, dirty }, action };
}

// ——— Presence: what is unsaved in an open editor right now, and when this device last saw a section saved ———

const dirtyEditors = new Map<string, number>();
const listeners = new Set<() => void>();
let version = 0;

function notifyDrafts(): void {
  version += 1;
  for (const listener of listeners) listener();
}

const subscribe = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };

/** An editor reports whether its draft differs from what it loaded. Never persisted. */
export function setEditorDirty(accountKey: string, section: string, dirty: boolean): void {
  const key = `${accountKey}\u0000${section}`;
  if (dirty === dirtyEditors.has(key)) return;
  if (dirty) dirtyEditors.set(key, Date.now());
  else dirtyEditors.delete(key);
  notifyDrafts();
}

export function isEditorDirty(accountKey: string, section: string): boolean {
  return dirtyEditors.has(`${accountKey}\u0000${section}`);
}

export type DraftLine = 'none' | 'unsaved' | 'recovered';

/** The Draft line of the state legend: unsaved in an open editor, recovered and waiting for review, or none. */
export function draftLine(accountKey: string, section: string, now: number = Date.now()): DraftLine {
  if (isEditorDirty(accountKey, section)) return 'unsaved';
  return hasRecoveredDraft(accountKey, section, now) ? 'recovered' : 'none';
}

/** Re-renders when any editor's dirty state or any recorded draft changes. */
export function useDraftLine(accountKey: string, section: string): DraftLine {
  useSyncExternalStore(subscribe, () => version, () => version);
  return draftLine(accountKey, section);
}

export const SAVED_AT_PREFIX = 'citadelops.saved.v1.';

/** This device saw the section saved at this time (written when an editor's save succeeds). */
export function recordSavedAt(accountKey: string, section: string, at: string, storage: DraftStorage | null = browserStorage()): void {
  if (!storage || !accountKey) return;
  try {
    storage.setItem(`${SAVED_AT_PREFIX}${accountKey}.${section}`, at);
  } catch {
    // The legend then says "saved" without a time.
  }
  notifyDrafts();
}

export function readSavedAt(accountKey: string, section: string, storage: DraftStorage | null = browserStorage()): string | undefined {
  if (!storage || !accountKey) return undefined;
  try {
    const value = storage.getItem(`${SAVED_AT_PREFIX}${accountKey}.${section}`);
    return value && Number.isFinite(Date.parse(value)) ? value : undefined;
  } catch {
    return undefined;
  }
}

export function resetDraftPresenceForTests(): void {
  dirtyEditors.clear();
  version += 1;
}
