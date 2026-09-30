import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { RotateCcw } from 'lucide-react';
import { useCitadelAPI } from '../api/ApiContext';
import { Button } from '../components/ui/Button';
import { Modal } from '../components/ui/Modal';
import { LocalizedText } from '../i18n/LocalizedText';
import {
  classifyRecovered,
  clearDraft,
  compareDrafts,
  draftDigest,
  savedWhileLoaded,
  isRecoverableSection,
  readDraft,
  recordSavedAt,
  setEditorDirty,
  stableDigest,
  writeDraft,
  type DraftDifference,
  type DraftRecoveryEntry,
} from './DraftRecovery';
import { scopeKey } from './onboarding/accountScope';
import type { useConfigurationDraftSession } from './ConfigurationDraftSession';

type DraftSessionApi = Pick<
  ReturnType<typeof useConfigurationDraftSession>,
  'ready' | 'loadKey' | 'sections' | 'snapshot' | 'recoverDraft'
>;

export interface UseDraftRecoveryOptions {
  /** The section the editor saves as its main value (`automation.*`). Other sections are never recorded. */
  section: string;
  isOpen: boolean;
  draftSession: DraftSessionApi;
  /** The section value the editor would save right now. Must be JSON. */
  draft: unknown;
  /** The editor's other unsaved sub-drafts (JSON), restored by the editor from `draftSession.recoveredExtras`. */
  extras?: unknown;
}

export interface DraftRecovery {
  /** The recovered-draft banner, or null. Render it above the editor's content. */
  banner: React.ReactNode;
  /** The draft differs from what the editor loaded (never persisted as progress). */
  dirty: boolean;
}

const WRITE_DELAY_MS = 500;
const digestOf = draftDigest;

interface Baseline {
  loadKey: string;
  /** Digest of the editor's draft right after the saved section loaded. */
  digest: string;
  /** Digest of the saved section at that moment. */
  savedDigest: string;
  revision: number;
}

/**
 * Draft recovery for one editor (CIT-19). While the editor is open and its draft differs from what it loaded, the draft
 * is written (debounced) under this account/world and section. On the next open the editor shows a banner: Restore puts
 * the recovered draft into the editor (still unsaved), Discard drops it. If the saved settings changed since the draft
 * was made, Compare is required first. Restore never saves, never writes `automation.enabled` and never starts anything;
 * Save stays a normal compare-and-set against what is really saved. A successful save clears the record.
 */
export function useDraftRecovery({ section, isOpen, draftSession, draft, extras }: UseDraftRecoveryOptions): DraftRecovery {
  const { state } = useCitadelAPI();
  const key = scopeKey(state);
  const active = isOpen && key !== '' && isRecoverableSection(section) && draftSession.ready;
  const currentDigest = digestOf(draft, extras);
  const savedDigest = stableDigest(draftSession.sections?.[section] ?? null);
  const loadKey = draftSession.loadKey;
  const revision = draftSession.snapshot?.revision ?? 0;

  const latest = useRef({ draft, extras, digest: currentDigest, key, section, savedDigest, revision });
  latest.current = { draft, extras, digest: currentDigest, key, section, savedDigest, revision };
  const baselineRef = useRef<Baseline | null>(null);
  const [baseline, setBaseline] = useState<Baseline | null>(null);
  const [entry, setEntry] = useState<DraftRecoveryEntry | null>(null);
  const [comparing, setComparing] = useState(false);
  const timer = useRef<number | null>(null);

  const cancelTimer = useCallback(() => {
    if (timer.current !== null) {
      window.clearTimeout(timer.current);
      timer.current = null;
    }
  }, []);

  const setBaselineBoth = useCallback((next: Baseline | null) => {
    baselineRef.current = next;
    setBaseline(next);
  }, []);

  // The baseline is taken once the editor has loaded the saved section and applied it to its draft (a macrotask later,
  // after the editor's own load effect), and again only when the saved configuration is loaded again.
  useEffect(() => {
    if (!active) {
      setBaselineBoth(null);
      return undefined;
    }
    const handle = window.setTimeout(() => {
      const current = latest.current;
      setBaselineBoth({ loadKey, digest: current.digest, savedDigest: current.savedDigest, revision: current.revision });
    }, 0);
    return () => window.clearTimeout(handle);
  }, [active, loadKey, setBaselineBoth]);

  const dirty = active && baseline !== null && baseline.loadKey === loadKey && currentDigest !== baseline.digest;

  const write = useCallback(() => {
    const current = latest.current;
    const base = baselineRef.current;
    if (!base || current.digest === base.digest) return;
    writeDraft({
      version: 1, section: current.section, accountKey: current.key, draft: current.draft,
      ...(current.extras !== undefined ? { extras: current.extras } : {}),
      baseRevision: base.revision, baseDigest: base.savedDigest, savedAt: new Date().toISOString(),
    });
  }, []);

  // Debounced write while dirty.
  useEffect(() => {
    if (!dirty) return undefined;
    timer.current = window.setTimeout(() => { timer.current = null; write(); }, WRITE_DELAY_MS);
    return cancelTimer;
  }, [cancelTimer, currentDigest, dirty, write]);

  // Presence for the checklist, and a final write when the editor closes with unsaved changes (Cancel keeps the draft).
  useEffect(() => {
    if (!dirty) return undefined;
    setEditorDirty(key, section, true);
    return () => {
      cancelTimer();
      write();
      setEditorDirty(key, section, false);
    };
  }, [cancelTimer, dirty, key, section, write]);

  // A change of the SAVED section without a reload means this editor saved it: clear the record, note the time, and
  // treat the saved draft as the new baseline.
  const previousSaved = useRef<{ loadKey: string; digest: string } | null>(null);
  useEffect(() => {
    if (!active) {
      previousSaved.current = null;
      return;
    }
    const previous = previousSaved.current;
    previousSaved.current = { loadKey, digest: savedDigest };
    if (savedWhileLoaded(previous, { loadKey, digest: savedDigest })) {
      cancelTimer();
      clearDraft(key, section);
      recordSavedAt(key, section, new Date().toISOString());
      const current = latest.current;
      setBaselineBoth({ loadKey, digest: current.digest, savedDigest: current.savedDigest, revision: current.revision });
      setEntry(null);
    }
  }, [active, cancelTimer, key, loadKey, savedDigest, section, setBaselineBoth]);

  // On open: read what is waiting. A record identical to what just loaded has nothing to recover and is dropped.
  useEffect(() => {
    setComparing(false);
    if (!active || !baseline || baseline.loadKey !== loadKey) {
      setEntry(null);
      return;
    }
    const found = readDraft(key, section);
    if (!found) {
      setEntry(null);
      return;
    }
    if (classifyRecovered(found, { draftDigest: baseline.digest, savedDigest: baseline.savedDigest }) === 'drop') {
      clearDraft(key, section);
      setEntry(null);
      return;
    }
    setEntry(found);
    // Only when the editor (re)loads, never while the player edits.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [active, baseline?.loadKey, key, section]);

  const discard = useCallback(() => {
    clearDraft(key, section);
    setEntry(null);
    setComparing(false);
  }, [key, section]);

  const restore = useCallback(() => {
    if (!entry) return;
    draftSession.recoverDraft(section, entry.draft, entry.extras);
    clearDraft(key, section);
    setEntry(null);
    setComparing(false);
  }, [draftSession, entry, key, section]);

  const savedSince = entry != null && baseline != null && entry.baseDigest !== baseline.savedDigest;
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
            <Button variant="outline" size="sm" onClick={() => setComparing(true)} leftIcon={<RotateCcw className="h-4 w-4" />}>
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
          onClose={() => setComparing(false)}
          maxWidth="2xl"
          title={<LocalizedText messageKey="draftRecovery.compareTitle" />}
          footer={(
            <>
              <Button variant="primary" onClick={restore}><LocalizedText messageKey="draftRecovery.restore" /></Button>
              <Button variant="outline" onClick={discard}><LocalizedText messageKey="draftRecovery.discard" /></Button>
              <Button variant="ghost" onClick={() => setComparing(false)}><LocalizedText messageKey="draftRecovery.close" /></Button>
            </>
          )}
        >
          <div className="space-y-3 text-xs" data-draft-compare>
            <p className="text-text-main"><LocalizedText messageKey="draftRecovery.compareIntro" /></p>
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
