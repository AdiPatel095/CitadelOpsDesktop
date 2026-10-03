import {
  buildCopySelection,
  type CastleCopyPreview,
  type CastleCopySelection,
  type CastleCopySelectionInput,
  type CastleCopyState,
} from './castleCopy';

/**
 * Re-applying a reviewed copy onto the latest saved settings (CIT-21 correction). When a Save is rejected because the
 * settings were saved elsewhere, the editor reloads the latest saved section and the reviewed copy is applied again
 * instead of being lost. The record keeps what the player reviewed: which castle, which choices, and what each
 * destination was going to receive. Pure: nothing here writes, saves or starts anything.
 */
export interface CastleCopyReplay {
  sourceKey: string;
  /** The choices as the player made them in the dialog. */
  input: CastleCopySelectionInput;
  /** The `(destination, field)` pairs that were applied to the draft. */
  appliedSelection: CastleCopySelection;
  /** What each applied destination was, and what it was going to hold, when the copy was applied. */
  applied: Readonly<Record<string, { state: CastleCopyState; to: Readonly<Record<string, string>> }>>;
}

const cloneInput = (input: CastleCopySelectionInput): CastleCopySelectionInput => ({
  destinations: new Set(input.destinations),
  fields: new Set(input.fields),
  keptIncludes: Object.fromEntries(Object.entries(input.keptIncludes).map(([key, fields]) => [key, new Set(fields)])),
});

const cloneSelection = (selection: CastleCopySelection): CastleCopySelection => (
  Object.fromEntries(Object.entries(selection).map(([key, fields]) => [key, new Set(fields)]))
);

/** Records a copy at the moment it is applied to the draft. */
export function makeReplay(
  sourceKey: string,
  input: CastleCopySelectionInput,
  preview: CastleCopyPreview,
  selection: CastleCopySelection,
): CastleCopyReplay {
  const applied: Record<string, { state: CastleCopyState; to: Record<string, string> }> = {};
  for (const destination of preview.destinations) {
    const fields = selection[destination.key];
    if (!fields || fields.size === 0) continue;
    const to: Record<string, string> = {};
    for (const change of destination.changes) if (fields.has(change.fieldId)) to[change.fieldId] = JSON.stringify(change.to ?? null);
    applied[destination.key] = { state: destination.state, to };
  }
  return { sourceKey, input: cloneInput(input), appliedSelection: cloneSelection(selection), applied };
}

export type ReplayOutcome =
  | { kind: 'identical'; selection: CastleCopySelection }
  /** Something differs (a pair, a destination's state or a value): the player reviews before it is applied. */
  | { kind: 'review'; selection: CastleCopySelection; reasons: readonly ReplayDifference[] }
  /** The source castle no longer holds setup: nothing to copy, the record is dropped. */
  | { kind: 'source-empty' };

export type ReplayDifference = 'pairs' | 'state' | 'value' | 'incompatible';

/**
 * Compares the copy re-checked against the latest settings with what was reviewed. It is identical only when the
 * same `(destination, field)` pairs are selected, every selected destination is in the state it was in, none is
 * incompatible, and each pair would write the same value.
 */
export function replayCopy(preview: CastleCopyPreview, replay: CastleCopyReplay): ReplayOutcome {
  if (!preview.sourceConfigured) return { kind: 'source-empty' };
  const selection = buildCopySelection(preview, replay.input);
  const reasons = new Set<ReplayDifference>();
  const keys = new Set([...Object.keys(selection), ...Object.keys(replay.appliedSelection)]);
  for (const key of keys) {
    const now = selection[key] ?? new Set<string>();
    const before = replay.appliedSelection[key] ?? new Set<string>();
    if (now.size !== before.size || [...now].some((field) => !before.has(field))) reasons.add('pairs');
  }
  for (const destination of preview.destinations) {
    const before = replay.applied[destination.key];
    if (!before || !selection[destination.key]) continue;
    if (destination.state === 'incompatible') reasons.add('incompatible');
    else if (destination.state !== before.state) reasons.add('state');
    for (const change of destination.changes) {
      if (!selection[destination.key].has(change.fieldId)) continue;
      if (before.to[change.fieldId] !== JSON.stringify(change.to ?? null)) reasons.add('value');
    }
  }
  return reasons.size === 0 ? { kind: 'identical', selection } : { kind: 'review', selection, reasons: [...reasons] };
}

/**
 * The choices to pre-fill when the dialog reopens for review: the player's own choices, except that nothing the
 * re-check marked unavailable, unknown or incompatible is pre-selected (it can still be ticked by hand where allowed).
 */
export function replayReviewInput(preview: CastleCopyPreview, replay: CastleCopyReplay): CastleCopySelectionInput {
  const compatible = new Set(preview.destinations.filter((destination) => destination.state === 'compatible').map((destination) => destination.key));
  const input = cloneInput(replay.input);
  return {
    destinations: new Set([...input.destinations].filter((key) => compatible.has(key))),
    fields: input.fields,
    keptIncludes: Object.fromEntries(Object.entries(input.keptIncludes).filter(([key]) => compatible.has(key))),
  };
}
