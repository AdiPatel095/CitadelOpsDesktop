import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { RotateCcw } from 'lucide-react';
import { useCitadelAPI } from '../api/useCitadelAPI';
import { Button } from '../components/ui/Button';
import { Modal } from '../components/ui/Modal';
import { LocalizedText } from '../i18n/LocalizedText';
import {
  INITIAL_DRAFT_RECOVERY_MACHINE,
  classifyRecovered,
  clearDraft,
  compareDrafts,
  draftDigest,
  isRecoverableSection,
  nextDraftRecoveryMachine,
  readDraft,
  recordSavedAt,
  setEditorDirty,
  stableDigest,
  writeDraft,
  type DraftDifference,
  type DraftRecoveryEntry,
  type DraftRecoveryMachine,
} from './DraftRecovery';
import { scopeKey } from './onboarding/accountScope';
import type { useConfigurationDraftSession } from './ConfigurationDraftSession';

type DraftSessionApi = Pick<
  ReturnType<typeof useConfigurationDraftSession>,
  'ready' | 'loadKey' | 'sections' | 'snapshot' | 'recoverDraft' | 'latestSections'
>;

export interface UseDraftRecoveryOptions {
  /** The section the editor saves as its main value (`automation.*`). Other sections are never recorded. */
  section: string;
  isOpen: boolean;
  draftSession: DraftSessionApi;
  /** The section value the editor would save right now. Must be JSON. */
  draft: unknown;
  /**
   * The value `draft` has when the player has changed nothing: built in render from the saved section
   * (`draftSession.sections?.[section]`) with the same function the editor's load effect uses. It is never derived from
   * what the editor happens to hold, so it does not depend on when the editor applied its load. Save makes the saved
   * value the new loaded value; a restored draft leaves it alone, so a restored draft is a change.
   */
  loaded: unknown;
  /** The editor's other unsaved sub-drafts (JSON), restored by the editor from `draftSession.recoveredExtras`. */
  extras?: unknown;
  /** The value `extras` has when the player has changed nothing (what the editor's load effect leaves them as). */
  loadedExtras?: unknown;
  copyReapplied?: boolean;
}

export interface DraftRecovery {
  /** The recovered-draft banner, or null. Render it above the editor's content. */
  banner: React.ReactNode;
  /** The draft differs from what the editor loaded (never persisted as progress). */
  dirty: boolean;
}

const WRITE_DELAY_MS = 500;
const digestOf = draftDigest;

/**
 * Draft recovery for one editor (CIT-19). While the editor is open and its draft differs from what it loaded, the draft
 * is written (debounced) under this account/world and section. On the next open the editor shows a banner: Restore puts
 * the recovered draft into the editor (still unsaved), Discard drops it. If the saved settings changed since the draft
 * was made, Compare is required first. Restore never saves, never writes `automation.enabled` and never starts anything;
 * Save stays a normal compare-and-set against what is really saved. A successful save clears the record.
 *
 * What counts as a change is decided by `nextDraftRecoveryMachine` from digests of the saved data, not from timing.
 */
export function useDraftRecovery({ section, isOpen, draftSession, draft, loaded, extras, loadedExtras, copyReapplied }: UseDraftRecoveryOptions): DraftRecovery {
  const { state } = useCitadelAPI();
  const key = scopeKey(state);
  const active = isOpen && key !== '' && isRecoverableSection(section) && draftSession.ready;
  const currentDigest = digestOf(draft, extras);
  const loadedDigest = digestOf(loaded, loadedExtras);
  const savedDigest = stableDigest(draftSession.sections?.[section] ?? null);
  const loadKey = draftSession.loadKey;
  const revision = draftSession.snapshot?.revision ?? 0;

  // What the effects and timers below read. Kept in an effect (declared first, so it runs first) rather than in render.
  // `lastActive` is what the editor held the last time it was open and loaded: a write at close time is about that,
  // not about whatever the next render shows (another account, a reload in progress).
  const latestSections = draftSession.latestSections;
  const now = { draft, extras, digest: currentDigest, loadedDigest, key, section, savedDigest, revision, latestSections };
  const latest = useRef(now);
  const lastActive = useRef(now);
  useEffect(() => {
    latest.current = now;
    if (active) lastActive.current = now;
  });
  const machineRef = useRef<DraftRecoveryMachine>(INITIAL_DRAFT_RECOVERY_MACHINE);
  const [machine, setMachine] = useState<DraftRecoveryMachine>(INITIAL_DRAFT_RECOVERY_MACHINE);
  const [entry, setEntry] = useState<DraftRecoveryEntry | null>(null);
  // Compare is open for one recorded draft (identified by when it was recorded); a different record starts closed.
  const [comparingFor, setComparingFor] = useState<string | null>(null);
  const timer = useRef<number | null>(null);

  const cancelTimer = useCallback(() => {
    if (timer.current !== null) {
      window.clearTimeout(timer.current);
      timer.current = null;
    }
  }, []);

  // Records what the editor held (default: right now). Never when it equals the loaded value.
  const write = useCallback((from?: typeof latest.current) => {
    const current = from ?? latest.current;
    if (current.digest === current.loadedDigest) return;
    writeDraft({
      version: 1, section: current.section, accountKey: current.key, draft: current.draft,
      ...(current.extras !== undefined ? { extras: current.extras } : {}),
      baseRevision: current.revision, baseDigest: current.savedDigest, savedAt: new Date().toISOString(),
    });
  }, []);

  // The state machine runs whenever what it looks at changes (no timer, no baseline). Its answer is carried out here.
  useEffect(() => {
    // A pending write belongs to the load it was scheduled in (another account or a reload must not receive it).
    if (machineRef.current.loadKey !== loadKey) cancelTimer();
    const step = nextDraftRecoveryMachine(machineRef.current, { active, loadKey, loadedDigest, draftDigest: currentDigest, savedDigest });
    machineRef.current = step.machine;
    setMachine((current) => (current.loadKey === step.machine.loadKey && current.savedDigest === step.machine.savedDigest
      && current.settled === step.machine.settled && current.dirty === step.machine.dirty ? current : step.machine));
    if (step.action === 'write') {
      cancelTimer();
      if (active) {
        timer.current = window.setTimeout(() => { timer.current = null; write(); }, WRITE_DELAY_MS);
      } else {
        // The editor closed with unsaved changes: Cancel keeps the draft.
        write(lastActive.current);
      }
    } else if (step.action === 'clear') {
      cancelTimer();
      clearDraft(key, section);
    } else if (step.action === 'saved') {
      // A change of the SAVED section without a reload means this editor saved it.
      cancelTimer();
      clearDraft(key, section);
      recordSavedAt(key, section, new Date().toISOString());
      setEntry(null);
    }
  }, [active, cancelTimer, currentDigest, key, loadKey, loadedDigest, savedDigest, section, write]);

  const dirty = active && machine.dirty;

  // Presence for the checklist, and a final write if the editor goes away while it holds unsaved changes.
  useEffect(() => {
    if (!dirty) return undefined;
    setEditorDirty(key, section, true);
    return () => setEditorDirty(key, section, false);
  }, [dirty, key, section]);
  // The editor going away. Editors close by unmounting, and a Save and that close can land in one render, so the state
  // machine never sees the new saved section: read it from the session as it is right now. Saved means clear the record
  // and note the time; otherwise unsaved changes are written. Nothing here sets state.
  useEffect(() => () => {
    cancelTimer();
    const last = machineRef.current;
    if (last.loadKey === null) return;
    const current = latest.current;
    const savedNow = stableDigest(current.latestSections()?.[current.section] ?? null);
    if (last.savedDigest !== null && savedNow !== last.savedDigest) {
      clearDraft(current.key, current.section);
      recordSavedAt(current.key, current.section, new Date().toISOString());
    } else if (last.dirty) {
      write(lastActive.current);
    }
  }, [cancelTimer, write]);

  // On open: read what is waiting. A record identical to what just loaded has nothing to recover and is dropped.
  // Read once per load (never while the player edits); the banner state is set on the next microtask, and a load that
  // has already gone away never receives it.
  useEffect(() => {
    let cancelled = false;
    let waiting: DraftRecoveryEntry | null = null;
    const found = active ? readDraft(key, section) : null;
    if (found) {
      if (classifyRecovered(found, { draftDigest: latest.current.loadedDigest, savedDigest: latest.current.savedDigest }) === 'drop') clearDraft(key, section);
      else waiting = found;
    }
    queueMicrotask(() => { if (!cancelled) setEntry(waiting); });
    return () => { cancelled = true; };
  }, [active, loadKey, key, section]);

  const discard = useCallback(() => {
    clearDraft(key, section);
    setEntry(null);
    setComparingFor(null);
  }, [key, section]);

  const restore = useCallback(() => {
    if (!entry) return;
    draftSession.recoverDraft(section, entry.draft, entry.extras);
    clearDraft(key, section);
    setEntry(null);
    setComparingFor(null);
  }, [draftSession, entry, key, section]);

  const comparing = entry != null && comparingFor === entry.savedAt;
  const savedSince = entry != null && entry.baseDigest !== savedDigest;
  const differences = useMemo<DraftDifference[]>(
    () => (comparing && entry ? compareDrafts(draftSession.sections?.[section] ?? null, entry.draft) : []),
    [comparing, draftSession.sections, entry, section],
  );

  const banner = entry ? (
    <div className="mb-4 rounded-global border border-primary/30 bg-primary/10 px-4 py-3" role="status" data-draft-recovery={savedSince ? 'saved-since' : 'baseline-unchanged'}>
      <div className="flex flex-wrap items-center justify-between gap-3">
        <p className="min-w-0 flex-1 text-xs leading-relaxed text-text-main">
          <LocalizedText
            messageKey={savedSince ? 'draftRecovery.bannerSavedSince' : 'draftRecovery.banner'}
            params={{ savedAt: Date.parse(entry.savedAt) }}
          />
        </p>
        <div className="flex flex-wrap gap-2">
          {savedSince ? (
            <Button variant="outline" size="sm" onClick={() => setComparingFor(entry.savedAt)} leftIcon={<RotateCcw className="h-4 w-4" />}>
              <LocalizedText messageKey="draftRecovery.compare" />
            </Button>
          ) : (
            <Button variant="outline" size="sm" onClick={restore} leftIcon={<RotateCcw className="h-4 w-4" />}>
              <LocalizedText messageKey="draftRecovery.restore" />
            </Button>
          )}
          <Button variant="ghost" size="sm" onClick={discard}>
            <LocalizedText messageKey="draftRecovery.discard" />
          </Button>
        </div>
      </div>
      {comparing ? (
        <Modal
          isOpen
          onClose={() => setComparingFor(null)}
          maxWidth="2xl"
          title={<LocalizedText messageKey="draftRecovery.compareTitle" />}
          footer={(
            <>
              <Button variant="primary" onClick={restore}><LocalizedText messageKey="draftRecovery.restore" /></Button>
              <Button variant="outline" onClick={discard}><LocalizedText messageKey="draftRecovery.discard" /></Button>
              <Button variant="ghost" onClick={() => setComparingFor(null)}><LocalizedText messageKey="draftRecovery.close" /></Button>
            </>
          )}
        >
          <div className="space-y-3 text-xs" data-draft-compare>
            <p className="text-text-main"><LocalizedText messageKey="draftRecovery.compareIntro" /></p>
            {copyReapplied ? <p className="text-text-main"><LocalizedText messageKey="draftRecovery.compareIntroReappliedCopy" /></p> : null}
            {differences.length === 0 ? (
              <p className="text-text-muted"><LocalizedText messageKey="draftRecovery.noDifferences" /></p>
            ) : (
              <table className="w-full text-left">
                <thead className="text-[10px] uppercase tracking-wider text-text-muted">
                  <tr>
                    <th scope="col" className="py-1 pr-2 font-bold"><LocalizedText messageKey="draftRecovery.columnSetting" /></th>
                    <th scope="col" className="py-1 pr-2 font-bold"><LocalizedText messageKey="draftRecovery.columnSaved" /></th>
                    <th scope="col" className="py-1 font-bold"><LocalizedText messageKey="draftRecovery.columnRecovered" /></th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-border-base">
                  {differences.map((difference) => (
                    <tr key={difference.path}>
                      <td className="break-all py-1 pr-2 font-mono text-text-main" dir="ltr">{difference.path}</td>
                      <td className="break-all py-1 pr-2 font-mono text-text-muted" dir="ltr">{shown(difference.saved)}</td>
                      <td className="break-all py-1 font-mono text-text-main" dir="ltr">{shown(difference.recovered)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </div>
        </Modal>
      ) : null}
    </div>
  ) : null;

  return { banner, dirty };
}

function shown(value: unknown): string {
  if (value === undefined) return '—';
  const text = typeof value === 'string' ? value : JSON.stringify(value);
  return text.length > 120 ? `${text.slice(0, 117)}…` : text;
}
