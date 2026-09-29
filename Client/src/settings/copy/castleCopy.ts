import type { CastleStateV2, GameStateV2 } from '../../api/Contracts';
import type { MetadataItem } from '../../context/MetadataContext';
import type { MessageKey, MessageParameters } from '../../i18n/messages';
import type { ReadinessFix } from '../readiness/Readiness';
import type { SettingsFeatureId } from '../disclosure/placement';
import type { ObservationContext } from '../requirements/observationFreshness';

/**
 * Preview and copy of castle-scoped setup (CIT-21). Pure and feature-agnostic: one core, one small descriptor per
 * feature. A copy is a DRAFT operation: `applyCastleCopy` returns a new draft for the open editor; nothing is
 * written until that editor's own Save, and Save never starts an automation. Nothing is substituted: an entry the
 * destination cannot take is dropped and named, never replaced by another unit, tool, castle or commander.
 */

export interface CastleCandidate {
  /** Configuration key of the castle in the edited map (usually the castle id; Queue Production keeps stable Storm keys). */
  key: string;
  liveId: number;
  name: string;
  kingdomId: number;
  slotType?: number;
  castle: CastleStateV2 | null;
  /** Facts the feature's editor already knows about this castle (Bird: Direwolves are reserved for Auto Fortress). */
  flags?: Readonly<Record<string, boolean>>;
}

/** A value ready to render: a message key plus parameters (official names come from metadata, never invented). */
export interface CastleCopyValue {
  messageKey: MessageKey;
  params?: MessageParameters;
}

export interface CastleCopyContext {
  state: GameStateV2 | null;
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
  metadataReady: boolean;
  /** Session, connection and hosted presence: unit counts are current only under the D1 rule. */
  observation: ObservationContext;
  /** Every castle the editor lists today, in display order. */
  candidates: readonly CastleCandidate[];
  /** Bird: Auto Fortress is on and its saved kingdoms (`automation.autoFortress`). */
  fortress?: { enabled: boolean; section: unknown };
  /** Queue Production: item ids the game offers at a castle; undefined while the catalog is not loaded. */
  allowedItemIds?: (castle: CastleCandidate) => readonly number[] | undefined;
  /** Queue Production: the castle takes its items from calendar slots, not from the list. */
  usesScheduledItems?: (castle: CastleCandidate) => boolean;
}

export interface CastleCopyField<T> {
  id: string;
  labelKey: MessageKey;
  /** Widens what the automation covers (Towers/Queue `enabled`): never included by default, only by an explicit include. */
  consequential?: boolean;
  read(record: T): unknown;
  /** The destination's current value as this editor shows it (defaults to `read`); Bird hides reserved Direwolf rows. */
  readFor?(record: T, destination: CastleCandidate): unknown;
  write(record: T, value: unknown, destination: CastleCandidate): T;
  /** Shown beside a changed value, for example "rotation restarts". */
  noteKey?: MessageKey;
  equal?(left: unknown, right: unknown): boolean;
  describe(value: unknown, context: CastleCopyContext): CastleCopyValue;
  /** List fields: the identity of an entry, so dropped entries can be named. */
  entryId?(entry: unknown): number;
}

export type CastleCopyState = 'compatible' | 'unknown' | 'unavailable' | 'incompatible';

export interface CastleCopyReason {
  id: string;
  /** `incompatible` is never includable; `unavailable` and `unknown` may be included explicitly. */
  state: Exclude<CastleCopyState, 'compatible'>;
  messageKey: MessageKey;
  params?: MessageParameters;
  fix?: ReadinessFix;
  /** List entries this reason keeps out of the copy for the destination (dropped, never substituted). */
  fieldId?: string;
  dropEntryIds?: readonly number[];
}

export interface CastleCopyDescriptor<Draft, T> {
  featureId: SettingsFeatureId;
  /** Placement section that hosts the Copy control. */
  sectionId: string;
  /** The field whose list, once fully dropped, makes a destination incompatible. */
  primaryFieldId: string;
  fields: readonly CastleCopyField<T>[];
  /** What a copy never touches, as one complete sentence (Maya's footer pattern). */
  notCopiedKey: MessageKey;
  /** Label of the explicit include for the consequential field. */
  enabledIncludeKey?: MessageKey;
  defaultRecord(): T;
  recordFor(draft: Draft, key: string): T | undefined;
  /** True when the record holds setup worth copying (differs from the default). */
  isConfigured(record: T): boolean;
  validate(source: T, destination: CastleCandidate, context: CastleCopyContext): CastleCopyReason[];
  apply(draft: Draft, key: string, next: T, destination: CastleCandidate): Draft;
}

export type CastleCopyChangeKind = 'set' | 'kept-difference' | 'same';

export interface CastleCopyChange {
  fieldId: string;
  from: unknown;
  /** What the destination would hold: the source value minus entries dropped for this destination. */
  to: unknown;
  kind: CastleCopyChangeKind;
  includedByDefault: boolean;
  /** Entries left out for this destination. */
  dropped: readonly number[];
}

export interface CastleCopyDestinationPreview {
  key: string;
  castle: CastleCandidate;
  state: CastleCopyState;
  reasons: CastleCopyReason[];
  changes: CastleCopyChange[];
}

export interface CastleCopyPreview {
  sourceKey: string;
  /** False when the source castle has no setup to copy. */
  sourceConfigured: boolean;
  destinations: CastleCopyDestinationPreview[];
  unsupported: MessageKey;
}

const severity: Record<CastleCopyState, number> = { compatible: 0, unknown: 1, unavailable: 2, incompatible: 3 };

function equalValues<T>(field: CastleCopyField<T>, left: unknown, right: unknown): boolean {
  return field.equal ? field.equal(left, right) : JSON.stringify(left) === JSON.stringify(right);
}

function withoutEntries<T>(field: CastleCopyField<T>, value: unknown, dropped: ReadonlySet<number>): unknown {
  if (!field.entryId || !Array.isArray(value) || dropped.size === 0) return value;
  return value.filter((entry) => !dropped.has(field.entryId!(entry)));
}

/** Worst state wins; a reason that only drops list entries leaves the destination includable unless nothing is left to copy. */
function destinationState(reasons: readonly CastleCopyReason[], remainingPrimary: number, primaryDropped: boolean): CastleCopyState {
  let state: CastleCopyState = 'compatible';
  for (const reason of reasons) {
    const partial = reason.state === 'incompatible' && (reason.dropEntryIds?.length ?? 0) > 0;
    const effective: CastleCopyState = partial ? (remainingPrimary > 0 || !primaryDropped ? 'unavailable' : 'incompatible') : reason.state;
    if (severity[effective] > severity[state]) state = effective;
  }
  return state;
}

export function previewCastleCopy<Draft, T>(
  descriptor: CastleCopyDescriptor<Draft, T>,
  draft: Draft,
  sourceKey: string,
  destinationKeys: readonly string[],
  context: CastleCopyContext,
): CastleCopyPreview {
  const source = descriptor.recordFor(draft, sourceKey);
  const sourceConfigured = source !== undefined && descriptor.isConfigured(source);
  const defaults = descriptor.defaultRecord();
  const destinations: CastleCopyDestinationPreview[] = [];
  for (const key of destinationKeys) {
    if (key === sourceKey) continue;
    const castle = context.candidates.find((candidate) => candidate.key === key);
    if (!castle) continue;
    if (!source || !sourceConfigured) {
      destinations.push({ key, castle, state: 'compatible', reasons: [], changes: [] });
      continue;
    }
    const reasons = descriptor.validate(source, castle, context);
    const destinationRecord = descriptor.recordFor(draft, key);
    const dropped = new Map<string, Set<number>>();
    for (const reason of reasons) {
      if (!reason.fieldId || !reason.dropEntryIds) continue;
      const set = dropped.get(reason.fieldId) ?? new Set<number>();
      reason.dropEntryIds.forEach((id) => set.add(id));
      dropped.set(reason.fieldId, set);
    }
    let remainingPrimary = 1;
    let primaryDropped = false;
    const changes = descriptor.fields.map((field): CastleCopyChange => {
      const sourceValue = field.read(source);
      const drops = dropped.get(field.id) ?? new Set<number>();
      const to = withoutEntries(field, sourceValue, drops);
      if (field.id === descriptor.primaryFieldId && Array.isArray(sourceValue)) {
        remainingPrimary = Array.isArray(to) ? to.length : 0;
        primaryDropped = drops.size > 0;
      }
      const current = (field.readFor ?? field.read)(destinationRecord ?? defaults, castle);
      const kind: CastleCopyChangeKind = equalValues(field, current, to) ? 'same'
        : equalValues(field, current, field.read(defaults)) ? 'set' : 'kept-difference';
      return {
        fieldId: field.id, from: current, to, kind,
        includedByDefault: kind === 'set' && !field.consequential,
        dropped: [...drops],
      };
    });
    destinations.push({ key, castle, state: destinationState(reasons, remainingPrimary, primaryDropped), reasons, changes });
  }
  return { sourceKey, sourceConfigured, destinations, unsupported: descriptor.notCopiedKey };
}

/** `(destination, field)` pairs to write. Only these are touched. */
export type CastleCopySelection = Readonly<Record<string, ReadonlySet<string>>>;

export interface CastleCopySelectionInput {
  /** Destinations the player ticked (an unavailable or unknown destination needs its own tick). */
  destinations: ReadonlySet<string>;
  /** Fields switched on for the copy; a consequential field is on only by an explicit include. */
  fields: ReadonlySet<string>;
  /** Fields the player chose to overwrite on a destination that holds a different custom value. */
  keptIncludes: Readonly<Record<string, ReadonlySet<string>>>;
}

/** Compatible destinations and every field except the consequential one; nothing that needs an explicit include. */
export function defaultCopyInput<Draft, T>(descriptor: CastleCopyDescriptor<Draft, T>, preview: CastleCopyPreview): CastleCopySelectionInput {
  return {
    destinations: new Set(preview.destinations.filter((destination) => destination.state === 'compatible').map((destination) => destination.key)),
    fields: new Set(descriptor.fields.filter((field) => !field.consequential).map((field) => field.id)),
    keptIncludes: {},
  };
}

export function buildCopySelection(preview: CastleCopyPreview, input: CastleCopySelectionInput): CastleCopySelection {
  const selection: Record<string, Set<string>> = {};
  for (const destination of preview.destinations) {
    if (!input.destinations.has(destination.key) || destination.state === 'incompatible') continue;
    const fieldIds = new Set<string>();
    for (const change of destination.changes) {
      if (!input.fields.has(change.fieldId) || change.kind === 'same') continue;
      if (change.kind === 'kept-difference' && !input.keptIncludes[destination.key]?.has(change.fieldId)) continue;
      fieldIds.add(change.fieldId);
    }
    if (fieldIds.size > 0) selection[destination.key] = fieldIds;
  }
  return selection;
}

export function selectionSize(selection: CastleCopySelection): number {
  return Object.values(selection).reduce((total, fields) => total + fields.size, 0);
}

/**
 * Writes only the reviewed `(destination, field)` pairs into a new draft. Values, not increments: applying the same
 * selection twice gives the same draft, so a retry after a failed Save cannot apply anything twice. An empty
 * selection returns the draft unchanged (same reference).
 */
export function applyCastleCopy<Draft, T>(
  descriptor: CastleCopyDescriptor<Draft, T>,
  draft: Draft,
  preview: CastleCopyPreview,
  selection: CastleCopySelection,
): Draft {
  let next = draft;
  for (const destination of preview.destinations) {
    const fieldIds = selection[destination.key];
    if (!fieldIds || fieldIds.size === 0 || destination.state === 'incompatible') continue;
    let record = descriptor.recordFor(next, destination.key) ?? descriptor.defaultRecord();
    let changed = false;
    for (const field of descriptor.fields) {
      if (!fieldIds.has(field.id)) continue;
      const change = destination.changes.find((entry) => entry.fieldId === field.id);
      if (!change || change.kind === 'same') continue;
      record = field.write(record, change.to, destination.castle);
      changed = true;
    }
    if (changed) next = descriptor.apply(next, destination.key, record, destination.castle);
  }
  return next;
}

/** Source castles worth offering: those whose setup differs from the default. */
export function configuredSources<Draft, T>(descriptor: CastleCopyDescriptor<Draft, T>, draft: Draft, context: CastleCopyContext): CastleCandidate[] {
  return context.candidates.filter((candidate) => {
    const record = descriptor.recordFor(draft, candidate.key);
    return record !== undefined && descriptor.isConfigured(record);
  });
}

export function canIncludeDestination(destination: CastleCopyDestinationPreview): boolean {
  return destination.state !== 'incompatible' && destination.changes.some((change) => change.kind !== 'same');
}

// ——— Selection edits (pure; the dialog keeps its state with these) ———

function toggled(set: ReadonlySet<string>, id: string, on: boolean): ReadonlySet<string> {
  const next = new Set(set);
  if (on) next.add(id); else next.delete(id);
  return next;
}

export function withDestination(input: CastleCopySelectionInput, key: string, on: boolean): CastleCopySelectionInput {
  return { ...input, destinations: toggled(input.destinations, key, on) };
}

export function withField(input: CastleCopySelectionInput, fieldId: string, on: boolean): CastleCopySelectionInput {
  return { ...input, fields: toggled(input.fields, fieldId, on) };
}

export function withKeptInclude(input: CastleCopySelectionInput, key: string, fieldId: string, on: boolean): CastleCopySelectionInput {
  return { ...input, keptIncludes: { ...input.keptIncludes, [key]: toggled(input.keptIncludes[key] ?? new Set<string>(), fieldId, on) } };
}

/** "Select all compatible": never selects an unavailable, unknown or incompatible destination. */
export function withAllCompatible(input: CastleCopySelectionInput, preview: CastleCopyPreview): CastleCopySelectionInput {
  const keys = new Set(input.destinations);
  for (const destination of preview.destinations) {
    if (destination.state === 'compatible' && canIncludeDestination(destination)) keys.add(destination.key);
  }
  return { ...input, destinations: keys };
}

/** The reason states as readiness states, so the dialog reuses the readiness line conventions. */
export function reasonAsCheckState(state: CastleCopyReason['state']): 'blocked' | 'pending' | 'unavailable' {
  return state === 'incompatible' ? 'blocked' : state === 'unavailable' ? 'pending' : 'unavailable';
}
