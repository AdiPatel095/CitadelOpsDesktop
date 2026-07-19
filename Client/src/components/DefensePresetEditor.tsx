import React, { useMemo, useState } from 'react';
import { Eraser, Minus, PackageSearch, Plus, Shield, Waves } from 'lucide-react';
import type { DefenseToolSlotV2, DefenseWallSectionV2 } from '../api/Contracts';
import {
  cloneDefensePresetDraft,
  type DefensePresetDraft,
} from '../defensePresets/DefensePresetTypes';
import { useMetadata, type MetadataItem } from '../context/MetadataContext';
import { showToolPicker } from './ToolPickerModal';
import ToolImage from './ToolImage';
import { Badge, Button, Card, CardContent, Input, Modal } from './ui';

interface DefensePresetEditorProps {
  initialDraft: DefensePresetDraft;
  saving: boolean;
  stockQuantities?: Record<number, number>;
  onClose: () => void;
  onSave: (draft: DefensePresetDraft) => void;
}

type WallKey = 'left' | 'middle' | 'right';

const DefensePresetEditor: React.FC<DefensePresetEditorProps> = ({
  initialDraft,
  saving,
  stockQuantities,
  onClose,
  onSave,
}) => {
  const [draft, setDraft] = useState<DefensePresetDraft>(() => cloneDefensePresetDraft(initialDraft));
  const [validationError, setValidationError] = useState('');
  const { tools } = useMetadata();
  const wallToolIDs = useMemo(
    () => Object.values(tools).filter((tool) => isDefenseToolForSlot(tool, [1, 2])).map((tool) => tool.id),
    [tools],
  );
  const moatToolIDs = useMemo(
    () => Object.values(tools).filter((tool) => isDefenseToolForSlot(tool, [4])).map((tool) => tool.id),
    [tools],
  );

  const updateWall = (key: WallKey, update: (section: DefenseWallSectionV2) => DefenseWallSectionV2) => {
    setDraft((current) => ({
      ...current,
      wall: { ...current.wall, [key]: update(current.wall[key]) },
    }));
  };

  const submit = () => {
    const error = validateDraft(draft);
    if (error) {
      setValidationError(error);
      return;
    }
    setValidationError('');
    onSave(cloneDefensePresetDraft({ ...draft, name: draft.name.trim() }));
  };

  return (
    <Modal
      isOpen
      onClose={onClose}
      maxWidth="6xl"
      title={initialDraft.name.trim() ? `Edit ${initialDraft.name}` : 'Create defense preset'}
      footer={
        <>
          <Button variant="ghost" disabled={saving} onClick={onClose}>Cancel</Button>
          <Button isLoading={saving} onClick={submit}>Save preset</Button>
        </>
      }
    >
      <div className="space-y-5">
        <div>
          <label className="mb-2 block text-xs font-black uppercase tracking-wider text-text-muted">Preset name</label>
          <Input
            autoFocus
            value={draft.name}
            maxLength={120}
            placeholder="Full ranged 41 / 18 / 41"
            onChange={(event) => setDraft((current) => ({ ...current, name: event.target.value }))}
          />
          {draft.sourceCastleId != null ? (
            <p className="mt-2 text-xs text-text-muted">
              Captured from {draft.sourceCastleName || `Castle ${draft.sourceCastleId}`} and now editable as an independent preset.
            </p>
          ) : null}
        </div>

        {validationError ? (
          <div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error">
            {validationError}
          </div>
        ) : null}

        <section>
          <div className="mb-3 flex flex-wrap items-end justify-between gap-2">
            <div>
              <h3 className="flex items-center gap-2 text-base font-black text-text-main"><Shield className="h-4 w-4 text-primary" /> Wall</h3>
              <p className="mt-1 text-xs text-text-muted">The flank split must total 100%. Ranged percentage is converted to the game’s melee-percentage wire value.</p>
            </div>
            <Badge variant={wallSplitTotal(draft) === 100 ? 'primary' : 'danger'}>
              Split total {wallSplitTotal(draft)}%
            </Badge>
          </div>
          <div className="grid gap-3 xl:grid-cols-3">
            {(['left', 'middle', 'right'] as const).map((key) => {
              const section = draft.wall[key];
              return (
                <Card key={key} variant="solid">
                  <CardContent className="space-y-4">
                    <div className="flex items-center justify-between">
                      <h4 className="font-black capitalize text-text-main">{key === 'middle' ? 'Front' : key} flank</h4>
                      <Badge variant="outline">{section.toolSlots.length} slots</Badge>
                    </div>
                    <div className="grid grid-cols-2 gap-3">
                      <NumberField
                        label="Split %"
                        value={section.unitPercent}
                        minimum={0}
                        maximum={100}
                        onChange={(value) => updateWall(key, (current) => ({ ...current, unitPercent: value }))}
                      />
                      <NumberField
                        label="Ranged %"
                        value={100 - section.unitTypePercent}
                        minimum={0}
                        maximum={100}
                        onChange={(value) => updateWall(key, (current) => ({ ...current, unitTypePercent: 100 - value }))}
                      />
                    </div>
                    <ToolSlotGroup
                      label={`${key === 'middle' ? 'Front' : capitalize(key)} wall tools`}
                      slots={section.toolSlots}
                      allowedToolIDs={wallToolIDs}
                      stockQuantities={stockQuantities}
                      tools={tools}
                      onChange={(toolSlots) => updateWall(key, (current) => ({ ...current, toolSlots }))}
                    />
                  </CardContent>
                </Card>
              );
            })}
          </div>
        </section>

        <section>
          <div className="mb-3">
            <h3 className="flex items-center gap-2 text-base font-black text-text-main"><Waves className="h-4 w-4 text-primary" /> Moat</h3>
            <p className="mt-1 text-xs text-text-muted">Slot counts are part of the preset and are checked against a fresh castle snapshot before any write.</p>
          </div>
          <div className="grid gap-3 xl:grid-cols-3">
            {([
              ['leftToolSlots', 'Left moat'],
              ['middleToolSlots', 'Front moat'],
              ['rightToolSlots', 'Right moat'],
            ] as const).map(([key, label]) => (
              <Card key={key} variant="solid">
                <CardContent>
                  <ToolSlotGroup
                    label={label}
                    slots={draft.moat[key]}
                    allowedToolIDs={moatToolIDs}
                    stockQuantities={stockQuantities}
                    tools={tools}
                    onChange={(slots) => setDraft((current) => ({
                      ...current,
                      moat: { ...current.moat, [key]: slots },
                    }))}
                  />
                </CardContent>
              </Card>
            ))}
          </div>
        </section>

        <section className="rounded-global border border-border-base bg-bg-card/45 p-4">
          <label className="flex cursor-pointer items-start gap-3">
            <input
              type="checkbox"
              className="mt-1 h-4 w-4 accent-primary"
              checked={draft.keep != null}
              onChange={(event) => setDraft((current) => ({
                ...current,
                keep: event.target.checked ? { mauct: 0, unitTypePercent: 50 } : undefined,
              }))}
            />
            <span>
              <span className="block text-sm font-black text-text-main">Include keep allocation</span>
              <span className="mt-1 block text-xs text-text-muted">Keep tool rows remain untouched because their nonempty write semantics are not capture-confirmed.</span>
            </span>
          </label>
          {draft.keep ? (
            <div className="mt-4 grid gap-3 sm:grid-cols-2">
              <NumberField
                label="Keep capacity value"
                value={draft.keep.mauct}
                minimum={0}
                onChange={(value) => setDraft((current) => ({ ...current, keep: { ...current.keep!, mauct: value } }))}
              />
              <NumberField
                label="Keep ranged %"
                value={100 - draft.keep.unitTypePercent}
                minimum={0}
                maximum={100}
                onChange={(value) => setDraft((current) => ({
                  ...current,
                  keep: { ...current.keep!, unitTypePercent: 100 - value },
                }))}
              />
            </div>
          ) : null}
        </section>
      </div>
    </Modal>
  );
};

const ToolSlotGroup: React.FC<{
  label: string;
  slots: DefenseToolSlotV2[];
  allowedToolIDs: number[];
  stockQuantities?: Record<number, number>;
  tools: Record<number, MetadataItem>;
  onChange: (slots: DefenseToolSlotV2[]) => void;
}> = ({ label, slots, allowedToolIDs, stockQuantities, tools, onChange }) => {
  const replaceSlot = (index: number, slot: DefenseToolSlotV2) => {
    onChange(slots.map((candidate, candidateIndex) => candidateIndex === index ? slot : candidate));
  };

  const pickTool = async (index: number) => {
    const current = slots[index];
    const result = await showToolPicker({
      mode: 'single',
      title: `Select a tool for ${label.toLowerCase()}`,
      preselected: current.definitionId > 0 ? [current.definitionId] : [],
      allowedToolIds: allowedToolIDs,
      stockQuantities,
    });
    if (typeof result !== 'number') return;
    replaceSlot(index, { definitionId: result, amount: current.amount > 0 ? current.amount : 1 });
  };

  return (
    <div>
      <div className="mb-2 flex items-center justify-between gap-2">
        <span className="text-[10px] font-black uppercase tracking-wider text-text-muted">{label}</span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          leftIcon={<Plus className="h-3.5 w-3.5" />}
          onClick={() => onChange([...slots, { definitionId: -1, amount: 0 }])}
        >
          Add slot
        </Button>
      </div>
      <div className="space-y-2">
        {slots.length === 0 ? (
          <div className="rounded-global border border-dashed border-border-base px-3 py-4 text-center text-xs text-text-muted">No slots in this preset.</div>
        ) : slots.map((slot, index) => {
          const tool = slot.definitionId > 0 ? tools[slot.definitionId] : undefined;
          return (
            <div key={index} className="grid grid-cols-[2.25rem_minmax(0,1fr)_5.5rem_auto] items-end gap-2 rounded-global border border-border-base bg-bg-input/35 p-2">
              <button
                type="button"
                className="flex h-9 w-9 items-center justify-center rounded-global border border-border-base bg-bg-card/60 hover:border-primary/40"
                title="Choose tool"
                onClick={() => void pickTool(index)}
              >
                {slot.definitionId > 0 ? <ToolImage toolId={slot.definitionId} size={30} showLevel={false} /> : <PackageSearch className="h-4 w-4 text-text-muted" />}
              </button>
              <div className="min-w-0">
                <label className="mb-1 block truncate text-[9px] font-black uppercase tracking-wider text-text-muted">{tool?.name || 'Tool ID'}</label>
                <Input
                  type="number"
                  className="!px-2 !py-1.5 font-mono"
                  value={slot.definitionId > 0 ? slot.definitionId : ''}
                  placeholder="Empty"
                  onChange={(event) => {
                    const definitionId = toInteger(event.target.value, -1);
                    replaceSlot(index, definitionId > 0
                      ? { definitionId, amount: slot.amount > 0 ? slot.amount : 1 }
                      : { definitionId: -1, amount: 0 });
                  }}
                />
              </div>
              <div>
                <label className="mb-1 block text-[9px] font-black uppercase tracking-wider text-text-muted">Amount</label>
                <Input
                  type="number"
                  min={1}
                  max={999}
                  className="!px-2 !py-1.5 font-mono"
                  disabled={slot.definitionId <= 0}
                  value={slot.definitionId > 0 ? slot.amount : ''}
                  onChange={(event) => replaceSlot(index, { ...slot, amount: toInteger(event.target.value, 0) })}
                />
              </div>
              <div className="flex items-center pb-0.5">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  title="Clear tool"
                  disabled={slot.definitionId <= 0}
                  onClick={() => replaceSlot(index, { definitionId: -1, amount: 0 })}
                >
                  <Eraser className="h-3.5 w-3.5" />
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  title="Remove slot"
                  onClick={() => onChange(slots.filter((_, candidateIndex) => candidateIndex !== index))}
                >
                  <Minus className="h-3.5 w-3.5" />
                </Button>
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};

const NumberField: React.FC<{
  label: string;
  value: number;
  minimum: number;
  maximum?: number;
  onChange: (value: number) => void;
}> = ({ label, value, minimum, maximum, onChange }) => (
  <div>
    <label className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">{label}</label>
    <Input
      type="number"
      min={minimum}
      max={maximum}
      value={value}
      className="font-mono"
      onChange={(event) => onChange(toInteger(event.target.value, 0))}
    />
  </div>
);

function validateDraft(draft: DefensePresetDraft): string {
  if (!draft.name.trim()) return 'Enter a preset name.';
  if (wallSplitTotal(draft) !== 100) return 'The left, front, and right wall split must total 100%.';
  for (const [label, section] of Object.entries(draft.wall)) {
    if (!percent(section.unitPercent) || !percent(section.unitTypePercent)) return `${capitalize(label)} wall percentages must be between 0 and 100.`;
  }
  const groups = [
    ...Object.values(draft.wall).map((section) => section.toolSlots),
    draft.moat.leftToolSlots,
    draft.moat.middleToolSlots,
    draft.moat.rightToolSlots,
  ];
  for (const slots of groups) {
    for (const slot of slots) {
      if (slot.definitionId === -1 && slot.amount === 0) continue;
      if (!Number.isSafeInteger(slot.definitionId) || slot.definitionId <= 0 || !Number.isSafeInteger(slot.amount) || slot.amount <= 0 || slot.amount > 999) {
        return 'Each tool slot must be empty or contain a positive tool ID and an amount from 1 to 999.';
      }
    }
  }
  if (draft.keep && (!Number.isSafeInteger(draft.keep.mauct) || draft.keep.mauct < 0 || !percent(draft.keep.unitTypePercent))) {
    return 'Keep values are outside the supported range.';
  }
  return '';
}

function isDefenseToolForSlot(tool: MetadataItem, allowed: number[]): boolean {
  const toolUse = typeof tool.typ === 'string' ? tool.typ.trim().toLowerCase() : '';
  if (toolUse !== 'defence' && toolUse !== 'defense') return false;
  const raw = tool.slotTypes;
  const values = Array.isArray(raw) ? raw : typeof raw === 'string' ? raw.split(',') : [];
  return values.some((value) => allowed.includes(Number(value)));
}

function wallSplitTotal(draft: DefensePresetDraft): number {
  return draft.wall.left.unitPercent + draft.wall.middle.unitPercent + draft.wall.right.unitPercent;
}

function percent(value: number): boolean {
  return Number.isInteger(value) && value >= 0 && value <= 100;
}

function toInteger(value: string, fallback: number): number {
  if (!value.trim()) return fallback;
  const numeric = Number(value);
  return Number.isFinite(numeric) ? Math.trunc(numeric) : fallback;
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

export default DefensePresetEditor;
