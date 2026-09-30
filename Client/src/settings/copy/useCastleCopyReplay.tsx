import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { LocalizedText } from '../../i18n/LocalizedText';
import { CastleCopyDialog } from '../components/CastleCopyDialog';
import { applyCastleCopy, previewCastleCopy, type CastleCopyContext, type CastleCopyDescriptor, type CastleCopySelectionInput } from './castleCopy';
import { replayCopy, replayReviewInput, type CastleCopyReplay } from './castleCopyReplay';

/**
 * The editor side of "Load latest and re-apply copy" (CIT-21 correction). `useCastleCopyReplayState` is called before the
 * draft session (its option tells the conflict notice a copy exists); `useCastleCopyReplayRun` runs after it. After the
 * latest settings are loaded and the editor's draft is reset from them, the copied castles are re-checked. When nothing
 * differs from what the player reviewed, the copy is applied to the draft and a status line says so; otherwise the copy
 * dialog reopens pre-filled with the differences. Nothing here saves or starts anything: Save stays the only write.
 */
type Phase = 'idle' | 'loaded' | 'replay';

export interface CastleCopyReplayState {
  replay: CastleCopyReplay | null;
  setReplay: (replay: CastleCopyReplay | null) => void;
  /** Passed to `useConfigurationDraftSession`; undefined until a copy has been applied. */
  sessionOption: { onReloaded: () => void; onDropped: () => void } | undefined;
  phase: Phase;
  setPhase: (phase: Phase) => void;
  status: boolean;
  setStatus: (value: boolean) => void;
  review: { input: CastleCopySelectionInput; sourceKey: string; empty: boolean } | null;
  setReview: (review: CastleCopyReplayState['review']) => void;
}

export function useCastleCopyReplayState(): CastleCopyReplayState {
  const [replay, setReplay] = useState<CastleCopyReplay | null>(null);
  const [phase, setPhase] = useState<Phase>('idle');
  const [status, setStatus] = useState(false);
  const [review, setReview] = useState<CastleCopyReplayState['review']>(null);
  const onReloaded = useCallback(() => setPhase('loaded'), []);
  const onDropped = useCallback(() => { setReplay(null); setStatus(false); setReview(null); setPhase('idle'); }, []);
  const sessionOption = useMemo(() => (replay ? { onReloaded, onDropped } : undefined), [onDropped, onReloaded, replay]);
  return { replay, setReplay, sessionOption, phase, setPhase, status, setStatus, review, setReview };
}

export interface CastleCopyReplayRunOptions<Draft, T> {
  descriptor: CastleCopyDescriptor<Draft, T>;
  /** The editor's current draft (already reset from the latest settings when the re-check runs). */
  draft: Draft;
  context: CastleCopyContext;
  featureLabel: string;
  applyDraft: (next: Draft) => void;
  isOpen: boolean;
}

export function useCastleCopyReplayRun<Draft, T>(state: CastleCopyReplayState, options: CastleCopyReplayRunOptions<Draft, T>): { status: React.ReactNode; dialog: React.ReactNode } {
  const { descriptor, draft, context, featureLabel, applyDraft, isOpen } = options;
  const { replay, setReplay, phase, setPhase, status, setStatus, review, setReview } = state;
  const appliedFromReview = useRef(false);
  const latest = useRef({ descriptor, draft, context, applyDraft, replay });
  latest.current = { descriptor, draft, context, applyDraft, replay };

  // The copy lives only while its editor is open.
  useEffect(() => {
    if (isOpen) return;
    setReplay(null); setStatus(false); setReview(null); setPhase('idle');
  }, [isOpen, setPhase, setReplay, setReview, setStatus]);

  // One more render after the latest settings load, so the editor's own effect has reset its draft from them.
  useEffect(() => {
    if (phase === 'loaded') setPhase('replay');
  }, [phase, setPhase]);

  useEffect(() => {
    if (phase !== 'replay') return;
    const { replay: record, descriptor: d, draft: current, context: c, applyDraft: apply } = latest.current;
    setPhase('idle');
    if (!record) return;
    const preview = previewCastleCopy(d, current, record.sourceKey, [...record.input.destinations], c);
    const outcome = replayCopy(preview, record);
    if (outcome.kind === 'identical') {
      apply(applyCastleCopy(d, current, preview, outcome.selection));
      setStatus(true);
      return;
    }
    setStatus(false);
    if (outcome.kind === 'source-empty') {
      setReplay(null);
      setReview({ input: record.input, sourceKey: record.sourceKey, empty: true });
      return;
    }
    setReview({ input: replayReviewInput(preview, record), sourceKey: record.sourceKey, empty: false });
  }, [phase, setPhase, setReplay, setReview, setStatus]);

  const statusNode = status && replay ? (
    <p className="mb-4 rounded-global border border-primary/30 bg-primary/10 px-4 py-3 text-xs font-semibold text-text-main" role="status" data-castle-copy-reapplied>
      <LocalizedText messageKey="castleCopy.reapplied" />
    </p>
  ) : null;

  const dialog = review ? (
    <CastleCopyDialog
      descriptor={descriptor}
      draft={draft}
      sourceKey={review.sourceKey}
      context={context}
      featureLabel={featureLabel}
      initialInput={review.input}
      noticeKey={review.empty ? undefined : 'castleCopy.replayChanged'}
      onApply={(next, applied) => { appliedFromReview.current = true; applyDraft(next); setReplay(applied); setStatus(false); }}
      onClose={() => {
        setReview(null);
        // Cancelling the review leaves the latest settings without the copy; there is nothing left to re-apply.
        if (appliedFromReview.current) appliedFromReview.current = false;
        else setReplay(null);
      }}
    />
  ) : null;
  return { status: statusNode, dialog };
}
