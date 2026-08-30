import React, { useEffect, useMemo, useState } from 'react';
import {
  Camera,
  Copy,
  Edit3,
  Library,
  Plus,
  RefreshCw,
  Shield,
  ShieldCheck,
  Trash2,
} from 'lucide-react';
import type { CastleStateV2, DefenseToolSlotV2 } from '../api/Contracts';
import { useCitadelAPI } from '../api/ApiContext';
import DefensePresetEditor from '../components/DefensePresetEditor';
import { Notifications } from '../components/Notifications';
import ToolImage from '../components/ToolImage';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CollectionToolbar,
  EmptyState,
  MetricTile,
  Select,
} from '../components/ui';
import { useMetadata } from '../context/MetadataContext';
import { buildPresetDocumentUpdate } from '../configuration/PresetDocumentUpdate';
import {
  DEFENSE_PRESETS_SECTION,
  type AppDefensePreset,
  type DefensePresetDraft,
  cloneDefensePresetDraft,
  defensePresetDraftFromCastle,
  emptyDefensePresetDraft,
  parseDefensePresetDocument,
  summarizeDefensePreset,
} from '../defensePresets/DefensePresetTypes';

interface EditorState {
  presetID: string | null;
  draft: DefensePresetDraft;
}

interface Compatibility {
  variant: 'success' | 'warning' | 'danger' | 'secondary';
  label: string;
  detail: string;
}

const DefensePresetsView: React.FC = () => {
  const {
    state,
    configuration,
    updateConfiguration,
    submitIntent,
    refreshState,
  } = useCitadelAPI();
  const { tools } = useMetadata();
  const [query, setQuery] = useState('');
  const [selectedCastleID, setSelectedCastleID] = useState('');
  const [editor, setEditor] = useState<EditorState | null>(null);
  const [saving, setSaving] = useState(false);
  const [pendingID, setPendingID] = useState<string | null>(null);
  const [applyingID, setApplyingID] = useState<string | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const document = useMemo(
    () => parseDefensePresetDocument(configuration?.sections[DEFENSE_PRESETS_SECTION]),
    [configuration?.sections],
  );
  const castles = useMemo(
    () => Object.values(state?.castles ?? {})
      .filter((castle) => castle.kingdomId === 0)
      .sort((left, right) => Number(right.focused) - Number(left.focused) || left.id - right.id),
    [state?.castles],
  );
  const selectedCastle = castles.find((castle) => String(castle.id) === selectedCastleID) ?? null;
  const filteredPresets = useMemo(() => {
    const normalized = query.trim().toLowerCase();
    if (!normalized) return document.presets;
    return document.presets.filter((preset) => (
      preset.name.toLowerCase().includes(normalized) ||
      preset.sourceCastleName?.toLowerCase().includes(normalized)
    ));
  }, [document.presets, query]);

  useEffect(() => {
    if (castles.length === 0) {
      if (selectedCastleID) setSelectedCastleID('');
      return;
    }
    if (!castles.some((castle) => String(castle.id) === selectedCastleID)) {
      setSelectedCastleID(String(castles.find((castle) => castle.focused)?.id ?? castles[0].id));
    }
  }, [castles, selectedCastleID]);

  const saveDocument = async (presets: AppDefensePreset[], successMessage: string) => {
		const savedDocument = configuration?.sections[DEFENSE_PRESETS_SECTION];
		const sourceDocument = savedDocument ?? { version: 1, presets: [] };
    await updateConfiguration(
      DEFENSE_PRESETS_SECTION,
			buildPresetDocumentUpdate(sourceDocument, document.presets, presets),
			savedDocument === undefined ? undefined : { expectedValue: savedDocument },
    );
    Notifications.success(successMessage);
  };

  const handleSave = async (draft: DefensePresetDraft) => {
    if (saving) return;
    const editingID = editor?.presetID;
    const existing = editingID
      ? document.presets.find((preset) => preset.id === editingID)
      : undefined;
    if (editingID && !existing) {
      Notifications.error('This defense preset was deleted by another dashboard. Close the editor and create a new preset if needed.');
      return;
    }
    setSaving(true);
    const now = new Date().toISOString();
    const preset: AppDefensePreset = {
      ...cloneDefensePresetDraft(draft),
      id: existing?.id ?? createID(),
      createdAt: existing?.createdAt ?? now,
      updatedAt: now,
    };
    const presets = existing
      ? document.presets.map((candidate) => candidate.id === existing.id ? preset : candidate)
      : [...document.presets, preset];
    try {
      await saveDocument(presets, existing ? 'Defense preset updated.' : 'Defense preset created.');
      setEditor(null);
    } catch (error) {
      Notifications.error(errorMessage(error, 'Could not save defense preset.'));
    } finally {
      setSaving(false);
    }
  };

  const handleDuplicate = async (preset: AppDefensePreset) => {
    if (pendingID) return;
    setPendingID(preset.id);
    const now = new Date().toISOString();
    const duplicate: AppDefensePreset = {
      ...cloneDefensePresetDraft(preset),
      id: createID(),
      name: uniqueCopyName(preset.name, document.presets),
      createdAt: now,
      updatedAt: now,
    };
    try {
      await saveDocument([...document.presets, duplicate], 'Defense preset duplicated.');
    } catch (error) {
      Notifications.error(errorMessage(error, 'Could not duplicate defense preset.'));
    } finally {
      setPendingID(null);
    }
  };

  const handleDelete = async (preset: AppDefensePreset) => {
    if (pendingID || !window.confirm(`Delete “${preset.name}”? This cannot be undone.`)) return;
    setPendingID(preset.id);
    try {
      await saveDocument(
        document.presets.filter((candidate) => candidate.id !== preset.id),
        'Defense preset deleted.',
      );
    } catch (error) {
      Notifications.error(errorMessage(error, 'Could not delete defense preset.'));
    } finally {
      setPendingID(null);
    }
  };

  const handleRefresh = async () => {
    if (!selectedCastle || refreshing) return;
    setRefreshing(true);
    try {
      const receipt = await submitIntent('defense.refresh', { castleId: selectedCastle.id }, { actor: 'ui:defense-presets' });
      if (receipt.status === 'failed') throw new Error(receipt.error || 'Defense refresh failed.');
      await refreshState();
      Notifications.success('Defense state refreshed.');
    } catch (error) {
      Notifications.error(errorMessage(error, 'Could not refresh defense state.'));
    } finally {
      setRefreshing(false);
    }
  };

  const handleApply = async (preset: AppDefensePreset) => {
    if (!selectedCastle || applyingID) return;
    const compatibility = presetCompatibility(preset, selectedCastle, tools);
    const warning = compatibility.variant === 'danger' ? `\n\nCurrent warning: ${compatibility.detail}` : '';
    if (!window.confirm(`Apply “${preset.name}” to ${castleLabel(selectedCastle)}? A fresh defense read will run before any setup change.${warning}`)) return;
    setApplyingID(preset.id);
    Notifications.publish({
      id: 'defense-preset-apply',
      category: 'yellow',
      message: `Applying “${preset.name}” to ${castleLabel(selectedCastle)}…`,
      persistent: true,
    });
    try {
      const receipt = await submitIntent('defense.preset.apply', {
        castleId: selectedCastle.id,
        presetId: preset.id,
        presetName: preset.name,
        wall: cloneDefensePresetDraft(preset).wall,
        moat: cloneDefensePresetDraft(preset).moat,
        ...(preset.keep ? { keep: { ...preset.keep } } : {}),
      }, { actor: 'ui:defense-presets' });
      if (receipt.status === 'failed') throw new Error(receipt.error || 'Defense preset apply failed.');
      if (receipt.status === 'cancelled') {
        Notifications.publish({ id: 'defense-preset-apply', category: 'yellow', message: 'Defense preset apply was cancelled.' });
      } else {
        await refreshState();
        Notifications.publish({ id: 'defense-preset-apply', category: 'green', message: 'Defense preset applied and read back successfully.' });
      }
    } catch (error) {
      Notifications.publish({
        id: 'defense-preset-apply',
        category: 'red',
        message: errorMessage(error, 'Could not apply defense preset.'),
      });
    } finally {
      setApplyingID(null);
    }
  };

  const stockQuantities = selectedCastle ? numericRecord(selectedCastle.defense.inventory) : undefined;
  const editorContextCastle = editor
    ? castles.find((castle) => castle.id === editor.draft.sourceCastleId) ?? selectedCastle
    : null;

  return (
    <div className="mx-auto flex w-full max-w-[1800px] flex-col gap-5 pb-10">
      <CollectionToolbar
        summary={(
          <>
            <Badge variant={document.presets.length > 0 ? 'primary' : 'secondary'}>
              {document.presets.length} preset{document.presets.length === 1 ? '' : 's'}
            </Badge>
            <Badge variant="outline" className="normal-case tracking-normal">Stored by CitadelOps</Badge>
          </>
        )}
        actions={(
          <>
            <Select
              value={selectedCastleID}
              options={castles.map((castle) => ({
                value: String(castle.id),
                label: `${castleLabel(castle)} · ${castle.x}:${castle.y}`,
                searchText: `${castleLabel(castle)} ${castle.id} ${castle.x} ${castle.y}`,
              }))}
              onChange={setSelectedCastleID}
              placeholder={castles.length > 0 ? 'Apply target' : 'No primary castles'}
              searchable
              disabled={castles.length === 0}
              icon={<Shield className="h-4 w-4" />}
              ariaLabel="Apply target castle"
              className="w-44 2xl:w-52"
            />
            <Button
              variant="outline"
              isLoading={refreshing}
              disabled={!selectedCastle || applyingID != null}
              leftIcon={<RefreshCw className="h-4 w-4" />}
              title="Refresh the selected castle's defense"
              aria-label="Refresh defense"
              onClick={() => void handleRefresh()}
            >
              <span className="hidden 2xl:inline">Refresh defense</span>
            </Button>
            <Button
              variant="secondary"
              disabled={!selectedCastle || !hasDefenseObservation(selectedCastle)}
              leftIcon={<Camera className="h-4 w-4" />}
              title={!selectedCastle || !hasDefenseObservation(selectedCastle) ? 'Refresh the selected castle before capturing it' : 'Capture the currently observed setup'}
              aria-label="Capture current defense"
              onClick={() => selectedCastle && setEditor({ presetID: null, draft: defensePresetDraftFromCastle(selectedCastle) })}
            >
              <span className="hidden 2xl:inline">Capture current</span>
            </Button>
            <Button
              leftIcon={<Plus className="h-4 w-4" />}
              title="Create a defense preset"
              aria-label="New defense preset"
              onClick={() => setEditor({ presetID: null, draft: emptyDefensePresetDraft() })}
            >
              <span className="hidden 2xl:inline">New preset</span>
            </Button>
          </>
        )}
        searchValue={query}
        onSearchChange={setQuery}
        searchPlaceholder="Search defense presets"
      />

      {filteredPresets.length > 0 ? (
        <div className="grid gap-4 xl:grid-cols-2">
          {filteredPresets.map((preset) => (
            <PresetCard
              key={preset.id}
              preset={preset}
              target={selectedCastle}
              compatibility={presetCompatibility(preset, selectedCastle, tools)}
              busy={pendingID === preset.id || applyingID != null}
              applying={applyingID === preset.id}
              onApply={() => void handleApply(preset)}
              onEdit={() => setEditor({ presetID: preset.id, draft: cloneDefensePresetDraft(preset) })}
              onDuplicate={() => void handleDuplicate(preset)}
              onDelete={() => void handleDelete(preset)}
            />
          ))}
        </div>
      ) : (
        <EmptyState
          size="lg"
          icon={<Library className="h-6 w-6" />}
          title={query.trim() ? 'No matching presets' : 'Create your first defense preset'}
          description={query.trim()
            ? 'Try a different preset or source-castle name.'
            : 'Build one manually without live defense state, or refresh a castle and capture its current setup as a starting point.'}
          action={!query.trim() ? (
            <Button leftIcon={<Plus className="h-4 w-4" />} onClick={() => setEditor({ presetID: null, draft: emptyDefensePresetDraft() })}>
              Create preset
            </Button>
          ) : undefined}
        />
      )}

      {editor ? (
        <DefensePresetEditor
          key={`${editor.presetID ?? 'new'}-${editor.draft.sourceCastleId ?? 'manual'}`}
          initialDraft={editor.draft}
          saving={saving}
          stockQuantities={stockQuantities}
          preservedKeepToolSlots={editorContextCastle ? {
            castleName: castleLabel(editorContextCastle),
            primary: editorContextCastle.defense.keep.primaryToolSlots,
            secondary: editorContextCastle.defense.keep.secondaryToolSlots,
          } : undefined}
          onClose={() => { if (!saving) setEditor(null); }}
          onSave={(draft) => void handleSave(draft)}
        />
      ) : null}
    </div>
  );
};

const PresetCard: React.FC<{
  preset: AppDefensePreset;
  target: CastleStateV2 | null;
  compatibility: Compatibility;
  busy: boolean;
  applying: boolean;
  onApply: () => void;
  onEdit: () => void;
  onDuplicate: () => void;
  onDelete: () => void;
}> = ({ preset, target, compatibility, busy, applying, onApply, onEdit, onDuplicate, onDelete }) => {
  const summary = summarizeDefensePreset(preset);
  return (
    <Card variant="solid" className="liquid-prominent-header-card overflow-hidden">
      <div className="flex flex-wrap items-start justify-between gap-3 border-b border-border-base bg-bg-card/45 px-5 py-4">
        <div className="min-w-0">
          <div className="flex items-center gap-2">
            <Shield className="h-4 w-4 shrink-0 text-primary" />
            <h2 className="truncate text-base font-black text-text-main">{preset.name}</h2>
          </div>
          <p className="mt-1 text-xs text-text-muted">
            Updated {formatUpdatedAt(preset.updatedAt)}
            {preset.sourceCastleName ? ` · captured from ${preset.sourceCastleName}` : ''}
          </p>
        </div>
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="icon" disabled={busy} onClick={onEdit} title="Edit preset"><Edit3 className="h-4 w-4" /></Button>
          <Button variant="ghost" size="icon" disabled={busy} onClick={onDuplicate} title="Duplicate preset"><Copy className="h-4 w-4" /></Button>
          <Button variant="ghost" size="icon" disabled={busy} onClick={onDelete} title="Delete preset" className="hover:!text-error"><Trash2 className="h-4 w-4" /></Button>
        </div>
      </div>
      <CardContent className="space-y-4">
        <div className="grid grid-cols-2 gap-2 md:grid-cols-4">
          <MetricTile label="Left" value={`${preset.wall.left.unitPercent}%`} />
          <MetricTile label="Front" value={`${preset.wall.middle.unitPercent}%`} />
          <MetricTile label="Right" value={`${preset.wall.right.unitPercent}%`} />
          <MetricTile label="Tools" value={summary.toolAmount.toLocaleString()} />
        </div>
        <div className="grid gap-3 sm:grid-cols-[1fr_auto]">
          <div className="rounded-global border border-border-base bg-bg-app/35 p-3">
            <div className="mb-2 text-[9px] font-black uppercase tracking-wider text-text-muted">Tool types</div>
            <div className="flex min-h-[2.125rem] items-center gap-1.5 overflow-hidden">
              {summary.toolTypes.length > 0 ? summary.toolTypes.slice(0, 8).map((id) => (
                <ToolImage key={id} toolId={id} size={34} showLevel={false} />
              )) : <span className="text-xs text-text-muted">No tools assigned</span>}
              {summary.toolTypes.length > 8 ? <Badge variant="secondary">+{summary.toolTypes.length - 8}</Badge> : null}
            </div>
          </div>
          <div className="flex flex-wrap content-start items-start gap-1.5 sm:max-w-48">
            <Badge variant="outline">Left · 4 wall</Badge>
            <Badge variant="warning">Front · 4 wall + 2 gate</Badge>
            <Badge variant="outline">Right · 4 wall</Badge>
            <Badge variant="outline">{summary.moatSlots} moat slots</Badge>
            <Badge variant={preset.keep ? 'primary' : 'secondary'}>
              {summary.courtyardSlots > 0
                ? `${summary.courtyardSlots} courtyard slots`
                : preset.keep
                  ? 'Courtyard tools preserved'
                  : 'Keep unchanged'}
            </Badge>
            <Badge variant="secondary">{rangedLabel(preset)}</Badge>
          </div>
        </div>
        <div className={`rounded-global border px-3 py-2.5 ${compatibilityClasses(compatibility.variant)}`}>
          <div className="text-xs font-black">{compatibility.label}</div>
          <div className="mt-1 text-xs opacity-80">{compatibility.detail}</div>
        </div>
        <Button
          className="w-full"
          disabled={!target || busy}
          isLoading={applying}
          leftIcon={<ShieldCheck className="h-4 w-4" />}
          onClick={onApply}
        >
          {target ? `Apply to ${castleLabel(target)}` : 'Select a castle to apply'}
        </Button>
      </CardContent>
    </Card>
  );
};

function presetCompatibility(
  preset: AppDefensePreset,
  castle: CastleStateV2 | null,
  tools: Record<number, { name: string }>,
): Compatibility {
  if (!castle) return { variant: 'secondary', label: 'No target selected', detail: 'Choose a primary-kingdom castle to check this preset.' };
  if (!hasDefenseObservation(castle)) {
    return {
      variant: 'warning',
      label: 'Fresh check required',
      detail: 'The preset can still be applied; CitadelOps will read DFC and validate slot counts and inventory first.',
    };
  }
  const countMismatch = firstSlotCountMismatch(preset, castle);
  if (countMismatch) {
    return { variant: 'danger', label: 'Layout mismatch', detail: countMismatch };
  }

  const required = toolAmounts(
    preset.wall.left.toolSlots,
    preset.wall.middle.toolSlots,
    preset.wall.right.toolSlots,
    preset.moat.leftToolSlots,
    preset.moat.middleToolSlots,
    preset.moat.rightToolSlots,
    ...(preset.keep?.primaryToolSlots && preset.keep.secondaryToolSlots
      ? [preset.keep.primaryToolSlots, preset.keep.secondaryToolSlots]
      : []),
  );
  const released = toolAmounts(
    castle.defense.wall.left.toolSlots,
    castle.defense.wall.middle.toolSlots,
    castle.defense.wall.right.toolSlots,
    castle.defense.moat.leftToolSlots,
    castle.defense.moat.middleToolSlots,
    castle.defense.moat.rightToolSlots,
    castle.defense.keep.primaryToolSlots,
    castle.defense.keep.secondaryToolSlots,
  );
  for (const [definitionID, amount] of required) {
    const available = inventoryAmount(castle, definitionID) + (released.get(definitionID) ?? 0);
    if (available < amount) return shortageCompatibility(definitionID, amount, available, tools);
  }
  return {
    variant: 'success',
    label: 'Compatible with observed defense',
    detail: 'Wall, gate, moat, and included courtyard slots fit the available-plus-assigned tools. The server will repeat this check against a fresh read before writing.',
  };
}

function firstSlotCountMismatch(preset: AppDefensePreset, castle: CastleStateV2): string {
  const groups: Array<[string, number, number]> = [
    ['left wall', preset.wall.left.toolSlots.length, castle.defense.wall.left.toolSlots.length],
    ['front wall', preset.wall.middle.toolSlots.length, castle.defense.wall.middle.toolSlots.length],
    ['right wall', preset.wall.right.toolSlots.length, castle.defense.wall.right.toolSlots.length],
    ['left moat', preset.moat.leftToolSlots.length, castle.defense.moat.leftToolSlots.length],
    ['front moat', preset.moat.middleToolSlots.length, castle.defense.moat.middleToolSlots.length],
    ['right moat', preset.moat.rightToolSlots.length, castle.defense.moat.rightToolSlots.length],
    ...(preset.keep?.primaryToolSlots && preset.keep.secondaryToolSlots
      ? [
        ['keep tools', preset.keep.primaryToolSlots.length, castle.defense.keep.primaryToolSlots.length],
        ['Sceat support tools', preset.keep.secondaryToolSlots.length, castle.defense.keep.secondaryToolSlots.length],
      ] as Array<[string, number, number]>
      : []),
  ];
  const mismatch = groups.find(([, presetCount, castleCount]) => presetCount !== castleCount);
  return mismatch ? `${capitalize(mismatch[0])} has ${mismatch[1]} preset slots but the castle has ${mismatch[2]}.` : '';
}

function shortageCompatibility(
  definitionID: number,
  required: number,
  available: number,
  tools: Record<number, { name: string }>,
): Compatibility {
  return {
    variant: 'danger',
    label: 'Tool shortage in observed state',
    detail: `${tools[definitionID]?.name || `Tool #${definitionID}`} requires ${required.toLocaleString()}, but ${available.toLocaleString()} are free or releasable from the current defense.`,
  };
}

function toolAmounts(...groups: DefenseToolSlotV2[][]): Map<number, number> {
  const amounts = new Map<number, number>();
  for (const slots of groups) {
    for (const slot of slots) {
      if (slot.definitionId <= 0 || slot.amount <= 0) continue;
      amounts.set(slot.definitionId, (amounts.get(slot.definitionId) ?? 0) + slot.amount);
    }
  }
  return amounts;
}

function inventoryAmount(castle: CastleStateV2, definitionID: number): number {
  return castle.defense.inventory[String(definitionID)] ?? 0;
}

function hasDefenseObservation(castle: CastleStateV2): boolean {
  return isObservedTimestamp(castle.defense.observedAt) && isObservedTimestamp(castle.defense.inventoryObservedAt);
}

function isObservedTimestamp(value: string | undefined): boolean {
  if (!value) return false;
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) && timestamp > 0;
}

function numericRecord(value: Record<string, number>): Record<number, number> {
  return Object.fromEntries(Object.entries(value).map(([key, amount]) => [Number(key), amount]));
}

function rangedLabel(preset: AppDefensePreset): string {
  const ranged = [
    100 - preset.wall.left.unitTypePercent,
    100 - preset.wall.middle.unitTypePercent,
    100 - preset.wall.right.unitTypePercent,
  ];
  return ranged.every((value) => value === ranged[0]) ? `${ranged[0]}% ranged` : 'Mixed ranged split';
}

function compatibilityClasses(variant: Compatibility['variant']): string {
  if (variant === 'success') return 'border-success/25 bg-success/8 text-success';
  if (variant === 'danger') return 'border-error/25 bg-error/8 text-error';
  if (variant === 'warning') return 'border-warning/25 bg-warning/8 text-warning';
  return 'border-border-base bg-bg-input/35 text-text-muted';
}

function castleLabel(castle: CastleStateV2): string {
  return castle.name?.trim() || `Castle ${castle.id}`;
}

function createID(): string {
  return crypto.randomUUID?.() ?? `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;
}

function uniqueCopyName(name: string, presets: AppDefensePreset[]): string {
  const existing = new Set(presets.map((preset) => preset.name.toLowerCase()));
  let candidate = `${name} copy`;
  let suffix = 2;
  while (existing.has(candidate.toLowerCase())) candidate = `${name} copy ${suffix++}`;
  return candidate;
}

function formatUpdatedAt(value: string): string {
  const date = new Date(value);
  if (!Number.isFinite(date.getTime()) || date.getTime() === 0) return 'previously';
  return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

export default DefensePresetsView;
