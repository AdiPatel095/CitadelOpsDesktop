import React, { useMemo, useState } from 'react';
import { Castle, Eraser, LockKeyhole, Minus, PackageSearch, Plus, Shield, Sparkles, Waves } from 'lucide-react';
import type { DefenseToolSlotV2, DefenseWallSectionV2 } from '../api/Contracts';
import {
  DEFENSE_KEEP_TOOL_SLOT_COUNT,
  DEFENSE_MOAT_TOOL_SLOT_COUNT,
  DEFENSE_WALL_FLANK_TOOL_SLOT_COUNT,
  DEFENSE_WALL_MIDDLE_TOOL_SLOT_COUNT,
  isDefenseWallMiddleGateSlot,
  normalizeDefensePresetSlots,
  type DefensePresetDraft,
} from '../defensePresets/DefensePresetTypes';
import { useMetadata, type MetadataItem } from '../context/MetadataContext';
import { showToolPicker } from './ToolPickerModal';
import ToolImage from './ToolImage';
import {
  AddSlot,
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  Input,
  Modal,
  ModalTitle,
  QuantityAssetTile,
} from './ui';

interface DefensePresetEditorProps {
  initialDraft: DefensePresetDraft;
  saving: boolean;
  stockQuantities?: Record<number, number>;
  preservedKeepToolSlots?: {
    castleName: string;
    primary: DefenseToolSlotV2[];
    secondary: DefenseToolSlotV2[];
  };
  onClose: () => void;
  onSave: (draft: DefensePresetDraft) => void;
}

type WallKey = 'left' | 'middle' | 'right';

interface FixedToolSlotSpec {
  label: string;
  kind: 'wall' | 'gate' | 'moat' | 'courtyard' | 'sceat';
  number: number;
  allowedToolIDs: number[];
  locked?: boolean;
}

const DefensePresetEditor: React.FC<DefensePresetEditorProps> = ({
  initialDraft,
  saving,
  stockQuantities,
  preservedKeepToolSlots,
  onClose,
  onSave,
}) => {
  const [draft, setDraft] = useState<DefensePresetDraft>(() => normalizeDefensePresetSlots(initialDraft));
  const [validationError, setValidationError] = useState('');
  const { tools } = useMetadata();
  const wallToolIDs = useMemo(
    () => Object.values(tools).filter((tool) => isDefenseToolForSlot(tool, [1])).map((tool) => tool.id),
    [tools],
  );
  const gateToolIDs = useMemo(
    () => Object.values(tools).filter((tool) => isDefenseToolForSlot(tool, [2])).map((tool) => tool.id),
    [tools],
  );
  const moatToolIDs = useMemo(
    () => Object.values(tools).filter((tool) => isDefenseToolForSlot(tool, [4])).map((tool) => tool.id),
    [tools],
  );
  const courtyardToolIDs = useMemo(
    () => Object.values(tools).filter((tool) => isDefenseToolForSlot(tool, [5])).map((tool) => tool.id),
    [tools],
  );
  const sceatDefenseToolIDs = useMemo(
    () => Object.values(tools).filter((tool) => isDefenseToolForSlot(tool, [6])).map((tool) => tool.id),
    [tools],
  );
  const hasEditableCourtyardRows = draft.keep?.primaryToolSlots != null &&
    draft.keep.secondaryToolSlots != null;

  const updateWall = (key: WallKey, update: (section: DefenseWallSectionV2) => DefenseWallSectionV2) => {
    setDraft((current) => ({
      ...current,
      wall: { ...current.wall, [key]: update(current.wall[key]) },
    }));
  };

  const submit = () => {
    const error = validateDraft(draft, tools);
    if (error) {
      setValidationError(error);
      return;
    }
    setValidationError('');
    onSave(normalizeDefensePresetSlots({ ...draft, name: draft.name.trim() }));
  };

  return (
    <Modal
      isOpen
      onClose={onClose}
      maxWidth="full"
      title={(
        <ModalTitle
          icon={<Shield className="h-5 w-5" />}
          description="Fixed wall and gate positions match the defense command layout."
        >
          {initialDraft.name.trim() ? `Edit ${initialDraft.name}` : 'Create defense preset'}
        </ModalTitle>
      )}
      footer={
        <>
          <Button variant="ghost" disabled={saving} onClick={onClose}>Cancel</Button>
          <Button isLoading={saving} onClick={submit}>Save preset</Button>
        </>
      }
    >
      <div className="mx-auto flex w-full max-w-[2300px] flex-col gap-4">
        <section className="rounded-global border border-border-base bg-bg-card/65 p-3 shadow-[var(--glass-shadow-compact)] backdrop-blur-2xl">
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
        </section>

        {validationError ? (
          <div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error">
            {validationError}
          </div>
        ) : null}

        <section aria-label="Wall formation">
          <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
            <div>
              <h3 className="flex items-center gap-2 text-base font-black text-text-main">
                <Shield className="h-4 w-4 text-primary" /> Wall formation
              </h3>
              <p className="mt-1 text-xs text-text-muted">
                Every position is fixed. Select a tool from its card; gate positions accept gate tools only.
              </p>
            </div>
            <Badge variant={wallSplitTotal(draft) === 100 ? 'primary' : 'danger'}>
              Split total {wallSplitTotal(draft)}%
            </Badge>
          </div>

          <div className="overflow-x-auto pb-2 custom-scrollbar">
            <div className="grid min-w-[76rem] grid-cols-[minmax(20rem,1fr)_minmax(28rem,1.45fr)_minmax(20rem,1fr)] items-stretch gap-3">
              <DefenseFlankEditorCard
                label="Left flank"
                description="Four fixed wall-tool positions."
                slotSummary="4 wall"
                className="min-w-0"
                section={draft.wall.left}
                slotSpecs={flankWallSlotSpecs(wallToolIDs)}
                allowedToolIDs={wallToolIDs}
                stockQuantities={stockQuantities}
                tools={tools}
                onChange={(section) => updateWall('left', () => section)}
              />
              <DefenseFlankEditorCard
                label="Front flank"
                description="Two amber gate-only positions and four blue wall-only positions."
                slotSummary="4 wall · 2 gate"
                className="min-w-0"
                front
                section={draft.wall.middle}
                slotSpecs={middleWallSlotSpecs(wallToolIDs, gateToolIDs)}
                allowedToolIDs={wallToolIDs}
                stockQuantities={stockQuantities}
                tools={tools}
                onChange={(section) => updateWall('middle', () => section)}
              />
              <DefenseFlankEditorCard
                label="Right flank"
                description="Four fixed wall-tool positions."
                slotSummary="4 wall"
                className="min-w-0"
                section={draft.wall.right}
                slotSpecs={flankWallSlotSpecs(wallToolIDs)}
                allowedToolIDs={wallToolIDs}
                stockQuantities={stockQuantities}
                tools={tools}
                onChange={(section) => updateWall('right', () => section)}
              />
            </div>
          </div>

          <p className="mt-2 text-xs text-text-muted">
            The flank split must total 100%. Ranged percentage is converted to the game’s melee-percentage wire value.
          </p>
        </section>

        <section aria-label="Moat tools">
          <div className="mb-3 flex flex-wrap items-end justify-between gap-3">
            <div>
              <h3 className="flex items-center gap-2 text-base font-black text-text-main"><Waves className="h-4 w-4 text-info" /> Moat tools</h3>
              <p className="mt-1 text-xs text-text-muted">Each defense section has one fixed moat-tool position.</p>
            </div>
            <Badge variant="outline">3 fixed moat slots</Badge>
          </div>
          <div className="overflow-x-auto pb-2 custom-scrollbar">
            <div className="grid min-w-[54rem] grid-cols-3 items-stretch gap-3">
              {([
                ['leftToolSlots', 'Left moat'],
                ['middleToolSlots', 'Front moat'],
                ['rightToolSlots', 'Right moat'],
              ] as const).map(([key, label]) => (
                <DefenseToolSectionCard
                  key={key}
                  className="min-w-0"
                  label={label}
                  description="One fixed moat-tool position."
                  slotSummary="1 moat"
                  tone="info"
                  slots={draft.moat[key]}
                  slotSpecs={moatSlotSpecs(moatToolIDs)}
                  allowedToolIDs={moatToolIDs}
                  stockQuantities={stockQuantities}
                  tools={tools}
                  onChange={(slots) => setDraft((current) => ({
                    ...current,
                    moat: { ...current.moat, [key]: slots },
                  }))}
                />
              ))}
            </div>
          </div>
        </section>

        <section aria-label="Courtyard and keep">
          <div className="rounded-global border border-border-base bg-bg-card/45 p-4">
            <label className="flex cursor-pointer items-start gap-3">
              <input
                type="checkbox"
                className="mt-1 h-4 w-4 accent-primary"
                checked={draft.keep != null}
                onChange={(event) => setDraft((current) => ({
                  ...current,
                  keep: event.target.checked
                    ? {
                      mauct: 0,
                      unitTypePercent: 50,
                      primaryToolSlots: cloneFixedToolSlots(
                        preservedKeepToolSlots?.primary,
                        DEFENSE_KEEP_TOOL_SLOT_COUNT,
                      ),
                      secondaryToolSlots: cloneFixedToolSlots(
                        preservedKeepToolSlots?.secondary,
                        DEFENSE_KEEP_TOOL_SLOT_COUNT,
                      ),
                    }
                    : undefined,
                }))}
              />
              <span>
                <span className="block text-sm font-black text-text-main">Include courtyard setup</span>
                <span className="mt-1 block text-xs text-text-muted">
                  Capacity, ranged allocation, three keep-tool slots, and three Sceat-support slots are part of the preset.
                </span>
              </span>
            </label>
            {draft.keep ? (
              <div className="mt-4 grid gap-3 sm:grid-cols-2">
                <NumberField
                  label="Courtyard capacity value"
                  value={draft.keep.mauct}
                  minimum={0}
                  onChange={(value) => setDraft((current) => ({ ...current, keep: { ...current.keep!, mauct: value } }))}
                />
                <NumberField
                  label="Courtyard ranged %"
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
          </div>

          <div className="mb-3 mt-4 flex flex-wrap items-end justify-between gap-3">
            <div>
              <h3 className="flex items-center gap-2 text-base font-black text-text-main">
                <Castle className="h-4 w-4 text-primary" /> Courtyard tools
              </h3>
              <p className="mt-1 text-xs text-text-muted">
                DFK has three normal keep-tool slots and three Sceat defense-support slots. Each row is validated against its official catalog type.
              </p>
            </div>
            <div className="flex flex-wrap items-center gap-2">
              <Badge variant={hasEditableCourtyardRows ? 'primary' : 'secondary'}>
                {hasEditableCourtyardRows ? '6 fixed courtyard slots' : 'Preserved on apply'}
              </Badge>
              {draft.keep && !hasEditableCourtyardRows ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setDraft((current) => ({
                    ...current,
                    keep: current.keep ? {
                      ...current.keep,
                      primaryToolSlots: cloneFixedToolSlots(
                        preservedKeepToolSlots?.primary,
                        DEFENSE_KEEP_TOOL_SLOT_COUNT,
                      ),
                      secondaryToolSlots: cloneFixedToolSlots(
                        preservedKeepToolSlots?.secondary,
                        DEFENSE_KEEP_TOOL_SLOT_COUNT,
                      ),
                    } : current.keep,
                  }))}
                >
                  Include courtyard tools
                </Button>
              ) : null}
            </div>
          </div>
          <div className="overflow-x-auto pb-2 custom-scrollbar">
            <div className="grid min-w-[54rem] grid-cols-2 items-stretch gap-3">
              <DefenseToolSectionCard
                className="min-w-0"
                label="Keep tools"
                description={hasEditableCourtyardRows
                  ? 'Normal courtyard tools from official slot type 5.'
                  : preservedCourtyardDescription(preservedKeepToolSlots)}
                slotSummary={hasEditableCourtyardRows ? '3 keep slots' : '3 preserved'}
                tone={hasEditableCourtyardRows ? 'courtyard' : 'locked'}
                slots={draft.keep?.primaryToolSlots ??
                  preservedKeepToolSlots?.primary ??
                  emptyToolSlots(DEFENSE_KEEP_TOOL_SLOT_COUNT)}
                slotSpecs={courtyardSlotSpecs(courtyardToolIDs, !hasEditableCourtyardRows)}
                allowedToolIDs={hasEditableCourtyardRows ? courtyardToolIDs : []}
                stockQuantities={stockQuantities}
                tools={tools}
                onChange={(primaryToolSlots) => setDraft((current) => ({
                  ...current,
                  keep: current.keep ? { ...current.keep, primaryToolSlots } : current.keep,
                }))}
              />
              <DefenseToolSectionCard
                className="min-w-0"
                label="Sceat support tools"
                description={hasEditableCourtyardRows
                  ? 'Sceat defense support from official slot type 6.'
                  : preservedCourtyardDescription(preservedKeepToolSlots)}
                slotSummary={hasEditableCourtyardRows ? '3 Sceat slots' : '3 preserved'}
                tone={hasEditableCourtyardRows ? 'sceat' : 'locked'}
                slots={draft.keep?.secondaryToolSlots ??
                  preservedKeepToolSlots?.secondary ??
                  emptyToolSlots(DEFENSE_KEEP_TOOL_SLOT_COUNT)}
                slotSpecs={sceatSlotSpecs(sceatDefenseToolIDs, !hasEditableCourtyardRows)}
                allowedToolIDs={hasEditableCourtyardRows ? sceatDefenseToolIDs : []}
                stockQuantities={stockQuantities}
                tools={tools}
                onChange={(secondaryToolSlots) => setDraft((current) => ({
                  ...current,
                  keep: current.keep ? { ...current.keep, secondaryToolSlots } : current.keep,
                }))}
              />
            </div>
          </div>
        </section>
      </div>
    </Modal>
  );
};

const DefenseFlankEditorCard: React.FC<{
  className?: string;
  label: string;
  description: string;
  slotSummary: string;
  front?: boolean;
  section: DefenseWallSectionV2;
  slotSpecs: FixedToolSlotSpec[];
  allowedToolIDs: number[];
  stockQuantities?: Record<number, number>;
  tools: Record<number, MetadataItem>;
  onChange: (section: DefenseWallSectionV2) => void;
}> = ({
  className = '',
  label,
  description,
  slotSummary,
  front = false,
  section,
  slotSpecs,
  allowedToolIDs,
  stockQuantities,
  tools,
  onChange,
}) => (
  <Card
    variant="solid"
    className={`liquid-prominent-header-card ${front ? 'ring-1 ring-warning/25' : ''} ${className}`}
    aria-label={`${label} defense`}
  >
    <CardHeader className="liquid-card-header-prominent !m-0 !min-h-0 min-w-0 flex-wrap items-start gap-3">
      <div className="flex min-w-0 items-center gap-3">
        <span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-global border ${
          front ? 'border-warning/40 bg-warning/12 text-warning' : 'border-primary/40 bg-primary/12 text-primary'
        }`}>
          {front ? <Castle className="h-5 w-5" /> : <Shield className="h-5 w-5" />}
        </span>
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h4 className="text-base font-black text-text-main">{label}</h4>
            <Badge variant={front ? 'warning' : 'outline'}>{slotSummary}</Badge>
          </div>
          <p className="mt-1 text-xs text-text-muted">{description}</p>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-2">
        <NumberField
          className="w-24"
          label="Split %"
          value={section.unitPercent}
          minimum={0}
          maximum={100}
          onChange={(value) => onChange({ ...section, unitPercent: value })}
        />
        <NumberField
          className="w-24"
          label="Ranged %"
          value={100 - section.unitTypePercent}
          minimum={0}
          maximum={100}
          onChange={(value) => onChange({ ...section, unitTypePercent: 100 - value })}
        />
      </div>
    </CardHeader>
    <CardContent className="liquid-prominent-header-content !px-3 !pb-3">
      <ToolSlotGroup
        label={`${label} fixed defense positions`}
        slots={section.toolSlots}
        allowedToolIDs={allowedToolIDs}
        fixedSlotSpecs={slotSpecs}
        stockQuantities={stockQuantities}
        tools={tools}
        onChange={(toolSlots) => onChange({ ...section, toolSlots })}
      />
    </CardContent>
  </Card>
);

const DefenseToolSectionCard: React.FC<{
  className?: string;
  label: string;
  description: string;
  slotSummary: string;
  tone: 'info' | 'courtyard' | 'sceat' | 'locked';
  slots: DefenseToolSlotV2[];
  slotSpecs: FixedToolSlotSpec[];
  allowedToolIDs: number[];
  stockQuantities?: Record<number, number>;
  tools: Record<number, MetadataItem>;
  onChange: (slots: DefenseToolSlotV2[]) => void;
}> = ({
  className = '',
  label,
  description,
  slotSummary,
  tone,
  slots,
  slotSpecs,
  allowedToolIDs,
  stockQuantities,
  tools,
  onChange,
}) => {
  const locked = tone === 'locked';
  const headerTone = defenseSectionHeaderTone(tone);
  return (
    <Card
      variant="solid"
      className={`liquid-prominent-header-card ring-1 ${headerTone.ring} ${className}`}
      aria-label={`${label} defense tools`}
    >
      <CardHeader className="liquid-card-header-prominent !m-0 !min-h-0 min-w-0 flex-wrap items-start gap-3">
        <div className="flex min-w-0 items-center gap-3">
          <span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-global border ${headerTone.icon}`}>
            {defenseSectionIcon(tone)}
          </span>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <h4 className="text-base font-black text-text-main">{label}</h4>
              <Badge variant={locked ? 'secondary' : tone === 'sceat' ? 'warning' : 'outline'}>{slotSummary}</Badge>
            </div>
            <p className="mt-1 text-xs text-text-muted">{description}</p>
          </div>
        </div>
      </CardHeader>
      <CardContent className="liquid-prominent-header-content !px-3 !pb-3">
        <ToolSlotGroup
          label={`${label} fixed defense positions`}
          slots={slots}
          allowedToolIDs={allowedToolIDs}
          fixedSlotSpecs={slotSpecs}
          stockQuantities={stockQuantities}
          tools={tools}
          onChange={onChange}
        />
      </CardContent>
    </Card>
  );
};

const ToolSlotGroup: React.FC<{
  label: string;
  slots: DefenseToolSlotV2[];
  allowedToolIDs: number[];
  stockQuantities?: Record<number, number>;
  tools: Record<number, MetadataItem>;
  fixedSlotSpecs?: FixedToolSlotSpec[];
  onChange: (slots: DefenseToolSlotV2[]) => void;
}> = ({ label, slots, allowedToolIDs, stockQuantities, tools, fixedSlotSpecs, onChange }) => {
  const renderedSlots = fixedSlotSpecs
    ? fixedSlotSpecs.map((_, index) => slots[index] ?? { definitionId: -1, amount: 0 })
    : slots;

  const replaceSlot = (index: number, slot: DefenseToolSlotV2) => {
    onChange(renderedSlots.map((candidate, candidateIndex) => candidateIndex === index ? slot : candidate));
  };

  const pickTool = async (index: number) => {
    const current = renderedSlots[index];
    const slotSpec = fixedSlotSpecs?.[index];
    if (slotSpec?.locked) return;
    const result = await showToolPicker({
      mode: 'single',
      title: `Select a tool for ${slotSpec ? `${slotSpec.label.toLowerCase()} in ` : ''}${label.toLowerCase()}`,
      preselected: current.definitionId > 0 ? [current.definitionId] : [],
      allowedToolIds: slotSpec?.allowedToolIDs ?? allowedToolIDs,
      stockQuantities,
    });
    if (typeof result !== 'number') return;
    replaceSlot(index, { definitionId: result, amount: current.amount > 0 ? current.amount : 1 });
  };

  if (fixedSlotSpecs) {
    return (
      <div>
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <span className="text-[10px] font-black uppercase tracking-wider text-text-muted">{label}</span>
          <span className="text-[10px] font-semibold text-text-muted">
            {fixedSlotSpecs.some((slot) => slot.locked)
              ? 'These values are read-only and preserved from the target castle.'
              : 'Select the image to change the assigned tool.'}
          </span>
        </div>
        <div className="overflow-x-auto pb-2 custom-scrollbar">
          <div className="mx-auto flex w-max min-w-full items-start justify-center gap-2" role="list" aria-label={label}>
            {renderedSlots.map((slot, index) => (
              <FixedDefenseToolSlotCard
                key={index}
                slot={slot}
                slotSpec={fixedSlotSpecs[index]}
                stockQuantities={stockQuantities}
                tools={tools}
                locked={fixedSlotSpecs[index].locked}
                onPick={() => void pickTool(index)}
                onChange={(nextSlot) => replaceSlot(index, nextSlot)}
              />
            ))}
          </div>
        </div>
      </div>
    );
  }

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
        {renderedSlots.length === 0 ? (
          <div className="rounded-global border border-dashed border-border-base px-3 py-4 text-center text-xs text-text-muted">No slots in this preset.</div>
        ) : renderedSlots.map((slot, index) => {
          const tool = slot.definitionId > 0 ? tools[slot.definitionId] : undefined;
          const slotSpec = fixedSlotSpecs?.[index];
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
                <label className="mb-1 block truncate text-[9px] font-black uppercase tracking-wider text-text-muted">
                  {slotSpec ? `${slotSpec.label}${tool?.name ? ` · ${tool.name}` : ''}` : tool?.name || 'Tool ID'}
                </label>
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

const FixedDefenseToolSlotCard: React.FC<{
  slot: DefenseToolSlotV2;
  slotSpec: FixedToolSlotSpec;
  stockQuantities?: Record<number, number>;
  tools: Record<number, MetadataItem>;
  locked?: boolean;
  onPick: () => void;
  onChange: (slot: DefenseToolSlotV2) => void;
}> = ({ slot, slotSpec, stockQuantities, tools, locked = false, onPick, onChange }) => {
  const hasTool = slot.definitionId > 0;
  const tool = hasTool ? tools[slot.definitionId] : undefined;
  const kindLabel = defenseSlotKindLabel(slotSpec.kind);
  const purposeLabel = slotSpec.kind === 'courtyard'
    ? 'Normal courtyard defense'
    : slotSpec.kind === 'sceat'
      ? 'Sceat defense support'
      : `${kindLabel} defense`;
  const pickerDisabled = locked || slotSpec.allowedToolIDs.length === 0;
  const available = hasTool && stockQuantities ? stockQuantities[slot.definitionId] ?? 0 : undefined;
  const tone = defenseSlotTone(slotSpec.kind);

  return (
    <div className={`flex w-32 shrink-0 flex-col items-center rounded-global border p-2 ${tone.card}`} role="listitem">
      <div className="mb-2 flex w-full items-center justify-between gap-1 text-[10px] font-black uppercase tracking-wide">
        <span className={tone.text}>{kindLabel} slot {slotSpec.number}</span>
        <span className="max-w-[2.75rem] truncate font-mono text-text-muted">{hasTool ? `#${slot.definitionId}` : 'Empty'}</span>
      </div>

      {hasTool ? (
        <>
          {locked ? (
            <div className="relative h-[88px] w-[88px] shrink-0">
              <div className={`flex h-full w-full items-center justify-center rounded-xl border bg-bg-input/55 ${tone.accent}`}>
                <ToolImage toolId={slot.definitionId} size={80} showLevel={false} className="rounded-xl" />
              </div>
              <span className="absolute bottom-0 right-0 z-10 translate-x-1/4 translate-y-1/4 rounded-full bg-white px-2.5 py-0.5 text-center font-mono text-[10px] font-bold tabular-nums text-slate-900 shadow-md ring-1 ring-black/10">
                ×{slot.amount.toLocaleString()}
              </span>
            </div>
          ) : (
            <QuantityAssetTile
              size={88}
              visual={(
                <button
                  type="button"
                  onClick={onPick}
                  disabled={pickerDisabled}
                  className={`flex h-full w-full items-center justify-center rounded-xl border bg-bg-input/55 transition focus:outline-none focus:ring-2 focus:ring-primary/45 disabled:cursor-not-allowed disabled:opacity-40 ${tone.accent}`}
                  title={`Change ${slotSpec.label.toLowerCase()} ${purposeLabel.toLowerCase()} tool`}
                  aria-label={`Change ${slotSpec.label} ${purposeLabel.toLowerCase()} tool`}
                >
                  <ToolImage toolId={slot.definitionId} size={80} showLevel={false} className="rounded-xl" />
                </button>
              )}
              quantity={(
                <input
                  type="number"
                  min={1}
                  max={999}
                  value={slot.amount || ''}
                  onChange={(event) => onChange({ ...slot, amount: toInteger(event.target.value, 0) })}
                  onClick={(event) => event.stopPropagation()}
                  placeholder="0"
                  className="w-12 bg-transparent p-0 text-center font-mono text-[11px] font-black tabular-nums text-slate-900 outline-none"
                  aria-label={`${slotSpec.label} amount`}
                  title={`${slotSpec.label} amount`}
                />
              )}
              onRemove={() => onChange({ definitionId: -1, amount: 0 })}
              removeLabel={`Clear ${slotSpec.label}`}
            />
          )}
          {locked ? (
            <span className="mt-2 line-clamp-2 flex h-9 w-full items-center justify-center text-center text-[11px] font-bold leading-tight text-text-main">
              {tool?.name || `Tool #${slot.definitionId}`}
            </span>
          ) : (
            <button
              type="button"
              onClick={onPick}
              disabled={pickerDisabled}
              className="mt-2 line-clamp-2 h-9 w-full text-center text-[11px] font-bold leading-tight text-text-main transition hover:text-primary disabled:cursor-not-allowed"
              title={tool?.name || `Tool #${slot.definitionId}`}
            >
              {tool?.name || `Tool #${slot.definitionId}`}
            </button>
          )}
          <span className="mt-1 max-w-full truncate text-center font-mono text-[10px] leading-none text-text-muted">
            {locked ? purposeLabel : available == null ? purposeLabel : `${available.toLocaleString()} owned`}
          </span>
          <span className={`mt-2 rounded-full border px-2 py-0.5 text-[9px] font-black uppercase tracking-wide ${tone.chip}`}>
            {locked ? 'Preserved' : `${kindLabel} only`}
          </span>
        </>
      ) : (
        <>
          {locked ? (
            <div className="flex h-[88px] w-[88px] shrink-0 flex-col items-center justify-center gap-2 rounded-global border-2 border-dashed border-border-base bg-bg-input/25 text-text-muted">
              <LockKeyhole className="h-5 w-5" />
              <span className="text-[9px] font-black uppercase tracking-wide">Preserved</span>
            </div>
          ) : (
            <AddSlot
              label={`Choose ${kindLabel.toLowerCase()} tool`}
              icon={defenseSlotIcon(slotSpec.kind)}
              layout="stacked"
              onClick={onPick}
              disabled={pickerDisabled}
              className={`h-[88px] w-[88px] shrink-0 px-2 text-[9px] disabled:cursor-not-allowed disabled:opacity-40 ${tone.add}`}
              title={`Choose a ${purposeLabel.toLowerCase()} tool for ${slotSpec.label.toLowerCase()}`}
              aria-label={`Choose ${slotSpec.label} ${purposeLabel.toLowerCase()} tool`}
            />
          )}
          <span className="mt-2 flex h-9 items-center text-center text-[11px] font-bold leading-tight text-text-muted">
            Empty {kindLabel.toLowerCase()} slot
          </span>
          <span className="mt-1 text-center font-mono text-[10px] leading-none text-text-muted">{purposeLabel}</span>
          <span className={`mt-2 rounded-full border px-2 py-0.5 text-[9px] font-black uppercase tracking-wide ${tone.chip}`}>
            {locked ? 'Preserved' : `${kindLabel} only`}
          </span>
        </>
      )}
    </div>
  );
};

const NumberField: React.FC<{
  className?: string;
  label: string;
  value: number;
  minimum: number;
  maximum?: number;
  onChange: (value: number) => void;
}> = ({ className = '', label, value, minimum, maximum, onChange }) => (
  <div className={className}>
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

function validateDraft(draft: DefensePresetDraft, tools: Record<number, MetadataItem>): string {
  if (!draft.name.trim()) return 'Enter a preset name.';
  if (wallSplitTotal(draft) !== 100) return 'The left, front, and right wall split must total 100%.';
  if (draft.wall.left.toolSlots.length !== DEFENSE_WALL_FLANK_TOOL_SLOT_COUNT ||
      draft.wall.middle.toolSlots.length !== DEFENSE_WALL_MIDDLE_TOOL_SLOT_COUNT ||
      draft.wall.right.toolSlots.length !== DEFENSE_WALL_FLANK_TOOL_SLOT_COUNT) {
    return 'Wall presets must contain four left wall slots, four front wall slots, two front gate slots, and four right wall slots.';
  }
  if (draft.moat.leftToolSlots.length !== DEFENSE_MOAT_TOOL_SLOT_COUNT ||
      draft.moat.middleToolSlots.length !== DEFENSE_MOAT_TOOL_SLOT_COUNT ||
      draft.moat.rightToolSlots.length !== DEFENSE_MOAT_TOOL_SLOT_COUNT) {
    return 'Moat presets must contain one fixed slot for the left, front, and right moat.';
  }
  for (const [label, section] of Object.entries(draft.wall)) {
    if (!percent(section.unitPercent) || !percent(section.unitTypePercent)) return `${capitalize(label)} wall percentages must be between 0 and 100.`;
  }
  const groups = [
    ...Object.values(draft.wall).map((section) => section.toolSlots),
    draft.moat.leftToolSlots,
    draft.moat.middleToolSlots,
    draft.moat.rightToolSlots,
    ...(draft.keep?.primaryToolSlots ? [draft.keep.primaryToolSlots] : []),
    ...(draft.keep?.secondaryToolSlots ? [draft.keep.secondaryToolSlots] : []),
  ];
  for (const slots of groups) {
    for (const slot of slots) {
      if (slot.definitionId === -1 && slot.amount === 0) continue;
      if (!Number.isSafeInteger(slot.definitionId) || slot.definitionId <= 0 || !Number.isSafeInteger(slot.amount) || slot.amount <= 0 || slot.amount > 999) {
        return 'Each tool slot must be empty or contain a positive tool ID and an amount from 1 to 999.';
      }
    }
  }
  const wallSlotGroups: Array<[string, DefenseToolSlotV2[]]> = [
    ['Left wall', draft.wall.left.toolSlots],
    ['Right wall', draft.wall.right.toolSlots],
  ];
  for (const [label, slots] of wallSlotGroups) {
    for (let index = 0; index < slots.length; index++) {
      const error = validateToolForSlot(slots[index], `${label} slot ${index + 1}`, [1], 'wall', tools);
      if (error) return error;
    }
  }
  for (let index = 0; index < draft.wall.middle.toolSlots.length; index++) {
    const gate = isDefenseWallMiddleGateSlot(index);
    const number = gate ? (index === 1 ? 1 : 2) : [0, 2, 3, 5].indexOf(index) + 1;
    const error = validateToolForSlot(
      draft.wall.middle.toolSlots[index],
      `Front ${gate ? 'gate' : 'wall'} slot ${number}`,
      [gate ? 2 : 1],
      gate ? 'gate' : 'wall',
      tools,
    );
    if (error) return error;
  }
  const moatGroups: Array<[string, DefenseToolSlotV2[]]> = [
    ['Left moat', draft.moat.leftToolSlots],
    ['Front moat', draft.moat.middleToolSlots],
    ['Right moat', draft.moat.rightToolSlots],
  ];
  for (const [label, slots] of moatGroups) {
    for (let index = 0; index < slots.length; index++) {
      const error = validateToolForSlot(slots[index], `${label} slot ${index + 1}`, [4], 'moat', tools);
      if (error) return error;
    }
  }
  if (draft.keep && (!Number.isSafeInteger(draft.keep.mauct) || draft.keep.mauct < 0 || !percent(draft.keep.unitTypePercent))) {
    return 'Keep values are outside the supported range.';
  }
  if (draft.keep) {
    const hasPrimaryToolSlots = draft.keep.primaryToolSlots != null;
    const hasSecondaryToolSlots = draft.keep.secondaryToolSlots != null;
    if (hasPrimaryToolSlots !== hasSecondaryToolSlots) {
      return 'Courtyard presets must include both fixed three-slot rows or preserve both rows.';
    }
    if (draft.keep.primaryToolSlots && draft.keep.secondaryToolSlots) {
      if (draft.keep.primaryToolSlots.length !== DEFENSE_KEEP_TOOL_SLOT_COUNT ||
          draft.keep.secondaryToolSlots.length !== DEFENSE_KEEP_TOOL_SLOT_COUNT) {
        return 'Courtyard presets must contain three keep-tool slots and three Sceat-support slots.';
      }
      for (let index = 0; index < DEFENSE_KEEP_TOOL_SLOT_COUNT; index++) {
        const keepError = validateToolForSlot(
          draft.keep.primaryToolSlots[index],
          `Keep tool slot ${index + 1}`,
          [5],
          'normal courtyard',
          tools,
        );
        if (keepError) return keepError;
        const sceatError = validateToolForSlot(
          draft.keep.secondaryToolSlots[index],
          `Sceat support slot ${index + 1}`,
          [6],
          'Sceat support',
          tools,
        );
        if (sceatError) return sceatError;
      }
    }
  }
  return '';
}

function validateToolForSlot(
  slot: DefenseToolSlotV2,
  label: string,
  allowedSlotTypes: number[],
  kind: string,
  tools: Record<number, MetadataItem>,
): string {
  if (slot.definitionId === -1 && slot.amount === 0) return '';
  const tool = tools[slot.definitionId];
  return tool && isDefenseToolForSlot(tool, allowedSlotTypes)
    ? ''
    : `${label} must contain a ${kind} defense tool.`;
}

function flankWallSlotSpecs(allowedToolIDs: number[]): FixedToolSlotSpec[] {
  return Array.from({ length: DEFENSE_WALL_FLANK_TOOL_SLOT_COUNT }, (_, index) => ({
    label: `Wall slot ${index + 1}`,
    kind: 'wall',
    number: index + 1,
    allowedToolIDs,
  }));
}

function middleWallSlotSpecs(wallToolIDs: number[], gateToolIDs: number[]): FixedToolSlotSpec[] {
  let wallIndex = 0;
  let gateIndex = 0;
  return Array.from({ length: DEFENSE_WALL_MIDDLE_TOOL_SLOT_COUNT }, (_, index) => {
    const gate = isDefenseWallMiddleGateSlot(index);
    const number = gate ? ++gateIndex : ++wallIndex;
    return {
      label: `${gate ? 'Gate' : 'Wall'} slot ${number}`,
      kind: gate ? 'gate' : 'wall',
      number,
      allowedToolIDs: gate ? gateToolIDs : wallToolIDs,
    };
  });
}

function moatSlotSpecs(allowedToolIDs: number[]): FixedToolSlotSpec[] {
  return Array.from({ length: DEFENSE_MOAT_TOOL_SLOT_COUNT }, (_, index) => ({
    label: `Moat slot ${index + 1}`,
    kind: 'moat',
    number: index + 1,
    allowedToolIDs,
  }));
}

function courtyardSlotSpecs(allowedToolIDs: number[], locked: boolean): FixedToolSlotSpec[] {
  return Array.from({ length: DEFENSE_KEEP_TOOL_SLOT_COUNT }, (_, index) => ({
    label: `Keep tool slot ${index + 1}`,
    kind: 'courtyard',
    number: index + 1,
    allowedToolIDs,
    locked,
  }));
}

function sceatSlotSpecs(allowedToolIDs: number[], locked: boolean): FixedToolSlotSpec[] {
  return Array.from({ length: DEFENSE_KEEP_TOOL_SLOT_COUNT }, (_, index) => ({
    label: `Sceat support slot ${index + 1}`,
    kind: 'sceat',
    number: index + 1,
    allowedToolIDs,
    locked,
  }));
}

function defenseSlotKindLabel(kind: FixedToolSlotSpec['kind']): string {
  if (kind === 'gate') return 'Gate';
  if (kind === 'moat') return 'Moat';
  if (kind === 'courtyard') return 'Courtyard';
  if (kind === 'sceat') return 'Sceat';
  return 'Wall';
}

function defenseSlotIcon(kind: FixedToolSlotSpec['kind']): React.ReactNode {
  if (kind === 'gate') return <Castle className="h-5 w-5" />;
  if (kind === 'moat') return <Waves className="h-5 w-5" />;
  if (kind === 'courtyard') return <Castle className="h-5 w-5" />;
  if (kind === 'sceat') return <Sparkles className="h-5 w-5" />;
  return <Shield className="h-5 w-5" />;
}

function defenseSlotTone(kind: FixedToolSlotSpec['kind']): {
  card: string;
  accent: string;
  text: string;
  chip: string;
  add: string;
} {
  if (kind === 'gate') {
    return {
      card: 'border-warning/35 bg-warning/5 shadow-[0_0_24px_-16px_var(--warning)]',
      accent: 'border-warning/55 hover:border-warning text-warning',
      text: 'text-warning',
      chip: 'border-warning/35 bg-warning/10 text-warning',
      add: '!border-warning/45 !text-warning',
    };
  }
  if (kind === 'moat') {
    return {
      card: 'border-info/30 bg-info/5 shadow-[0_0_24px_-16px_var(--info)]',
      accent: 'border-info/55 hover:border-info text-info',
      text: 'text-info',
      chip: 'border-info/35 bg-info/10 text-info',
      add: '!border-info/45 !text-info',
    };
  }
  if (kind === 'courtyard') {
    return {
      card: 'border-primary/30 bg-primary/5 shadow-[0_0_24px_-16px_var(--primary)]',
      accent: 'border-primary/50 hover:border-primary text-primary',
      text: 'text-primary',
      chip: 'border-primary/35 bg-primary/10 text-primary',
      add: '!border-primary/45 !text-primary',
    };
  }
  if (kind === 'sceat') {
    return {
      card: 'border-warning/30 bg-warning/5 shadow-[0_0_24px_-16px_var(--warning)]',
      accent: 'border-warning/50 hover:border-warning text-warning',
      text: 'text-warning',
      chip: 'border-warning/35 bg-warning/10 text-warning',
      add: '!border-warning/45 !text-warning',
    };
  }
  return {
    card: 'border-primary/20 bg-bg-app/42',
    accent: 'border-primary/40 hover:border-primary/75 text-primary',
    text: 'text-primary',
    chip: 'border-primary/30 bg-primary/8 text-primary',
    add: '',
  };
}

function defenseSectionHeaderTone(tone: 'info' | 'courtyard' | 'sceat' | 'locked'): {
  ring: string;
  icon: string;
} {
  if (tone === 'courtyard') {
    return {
      ring: 'ring-primary/20',
      icon: 'border-primary/35 bg-primary/10 text-primary',
    };
  }
  if (tone === 'sceat') {
    return {
      ring: 'ring-warning/25',
      icon: 'border-warning/35 bg-warning/10 text-warning',
    };
  }
  if (tone === 'locked') {
    return {
      ring: 'ring-border-base/60',
      icon: 'border-border-base bg-bg-input/45 text-text-muted',
    };
  }
  return {
    ring: 'ring-info/20',
    icon: 'border-info/35 bg-info/10 text-info',
  };
}

function defenseSectionIcon(tone: 'info' | 'courtyard' | 'sceat' | 'locked'): React.ReactNode {
  if (tone === 'courtyard') return <Castle className="h-5 w-5" />;
  if (tone === 'sceat') return <Sparkles className="h-5 w-5" />;
  if (tone === 'locked') return <LockKeyhole className="h-5 w-5" />;
  return <Waves className="h-5 w-5" />;
}

function cloneFixedToolSlots(slots: DefenseToolSlotV2[] | undefined, count: number): DefenseToolSlotV2[] {
  return Array.from({ length: count }, (_, index) => {
    const slot = slots?.[index];
    return slot
      ? { definitionId: Math.trunc(slot.definitionId), amount: Math.trunc(slot.amount) }
      : { definitionId: -1, amount: 0 };
  });
}

function preservedCourtyardDescription(
  preserved: DefensePresetEditorProps['preservedKeepToolSlots'],
): string {
  return preserved
    ? `Current ${preserved.castleName} values. Include the rows to make them editable.`
    : 'Loaded from the target castle at apply time until these rows are included.';
}

function emptyToolSlots(count: number): DefenseToolSlotV2[] {
  return Array.from({ length: count }, () => ({ definitionId: -1, amount: 0 }));
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
