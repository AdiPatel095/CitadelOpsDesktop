import React, { useEffect, useMemo, useState } from 'react';
import { Minus, Plus } from 'lucide-react';
import { Button, Input, Modal } from './ui';

export interface AttackSetupSlot {
  itemId: number | null;
  quantity: number;
}

export interface AttackSetupLane {
  troops: AttackSetupSlot[];
  tools: AttackSetupSlot[];
}

export interface AttackSetupWave {
  L: AttackSetupLane;
  M: AttackSetupLane;
  R: AttackSetupLane;
}

export interface AttackSetupDraft {
  waves: AttackSetupWave[];
}

interface AttackSetupModalProps {
  isOpen: boolean;
  initialDraft?: AttackSetupDraft;
  onClose: () => void;
  onSave: (draft: AttackSetupDraft) => void;
}

const laneKeys = ['L', 'M', 'R'] as const;

const AttackSetupModal: React.FC<AttackSetupModalProps> = ({ isOpen, initialDraft, onClose, onSave }) => {
  const [draft, setDraft] = useState<AttackSetupDraft>(() => initialDraft ?? defaultDraft());

  useEffect(() => {
    if (isOpen) {
      setDraft(initialDraft ?? defaultDraft());
    }
  }, [initialDraft, isOpen]);

  const totals = useMemo(() => summarizeDraft(draft), [draft]);

  const setWaveCount = (count: number) => {
    const nextCount = Math.max(1, Math.min(10, Math.trunc(count) || 1));
    setDraft((current) => {
      if (current.waves.length === nextCount) {
        return current;
      }
      if (current.waves.length > nextCount) {
        return { waves: current.waves.slice(0, nextCount) };
      }
      return {
        waves: [
          ...current.waves,
          ...Array.from({ length: nextCount - current.waves.length }, () => emptyWave()),
        ],
      };
    });
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title="Attack setup"
      maxWidth="3xl"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" onClick={() => onSave(draft)}>
            Save setup
          </Button>
        </>
      }
    >
      <div className="space-y-5">
        <div className="grid gap-3 md:grid-cols-[11rem_1fr] md:items-center">
          <div className="flex items-center gap-2">
            <Button variant="secondary" size="icon" onClick={() => setWaveCount(draft.waves.length - 1)} title="Remove wave">
              <Minus className="w-4 h-4" />
            </Button>
            <Input
              type="number"
              min={1}
              max={10}
              value={draft.waves.length}
              onChange={(event) => setWaveCount(Number(event.target.value))}
              className="text-center"
            />
            <Button variant="secondary" size="icon" onClick={() => setWaveCount(draft.waves.length + 1)} title="Add wave">
              <Plus className="w-4 h-4" />
            </Button>
          </div>
          <div className="text-sm text-text-muted">
            {draft.waves.length} wave{draft.waves.length === 1 ? '' : 's'} | {totals.troops.toLocaleString()} troops |{' '}
            {totals.tools.toLocaleString()} tools
          </div>
        </div>

        <div className="space-y-3">
          {draft.waves.map((wave, waveIndex) => (
            <div key={waveIndex} className="rounded-global border border-border-base bg-bg-card p-4">
              <div className="font-semibold text-text-main mb-3">Wave {waveIndex + 1}</div>
              <div className="grid gap-3 md:grid-cols-3">
                {laneKeys.map((laneKey) => (
                  <LaneEditor
                    key={laneKey}
                    label={laneLabel(laneKey)}
                    lane={wave[laneKey]}
                    onChange={(lane) => {
                      setDraft((current) => ({
                        waves: current.waves.map((existingWave, index) =>
                          index === waveIndex ? { ...existingWave, [laneKey]: lane } : existingWave
                        ),
                      }));
                    }}
                  />
                ))}
              </div>
            </div>
          ))}
        </div>
      </div>
    </Modal>
  );
};

interface LaneEditorProps {
  label: string;
  lane: AttackSetupLane;
  onChange: (lane: AttackSetupLane) => void;
}

const LaneEditor: React.FC<LaneEditorProps> = ({ label, lane, onChange }) => (
  <div className="rounded-global border border-border-base bg-bg-app p-3">
    <div className="text-xs font-semibold uppercase tracking-wider text-text-muted mb-3">{label}</div>
    <SlotGroup
      label="Troops"
      slots={lane.troops}
      onChange={(troops) => onChange({ ...lane, troops })}
    />
    <SlotGroup
      label="Tools"
      slots={lane.tools}
      onChange={(tools) => onChange({ ...lane, tools })}
    />
  </div>
);

interface SlotGroupProps {
  label: string;
  slots: AttackSetupSlot[];
  onChange: (slots: AttackSetupSlot[]) => void;
}

const SlotGroup: React.FC<SlotGroupProps> = ({ label, slots, onChange }) => (
  <div className="space-y-2 mb-3 last:mb-0">
    <div className="text-[11px] text-text-muted">{label}</div>
    {slots.map((slot, index) => (
      <div key={index} className="grid grid-cols-2 gap-2">
        <Input
          type="number"
          min={0}
          placeholder="ID"
          value={slot.itemId ?? ''}
          onChange={(event) => onChange(updateSlot(slots, index, { itemId: nullableNumber(event.target.value) }))}
        />
        <Input
          type="number"
          min={0}
          placeholder="Qty"
          value={slot.quantity || ''}
          onChange={(event) => onChange(updateSlot(slots, index, { quantity: Number(event.target.value) || 0 }))}
        />
      </div>
    ))}
  </div>
);

function updateSlot(slots: AttackSetupSlot[], index: number, patch: Partial<AttackSetupSlot>): AttackSetupSlot[] {
  return slots.map((slot, slotIndex) => (slotIndex === index ? { ...slot, ...patch } : slot));
}

function nullableNumber(value: string): number | null {
  if (!value.trim()) {
    return null;
  }
  const parsed = Number(value);
  return Number.isFinite(parsed) ? Math.trunc(parsed) : null;
}

function summarizeDraft(draft: AttackSetupDraft): { troops: number; tools: number } {
  let troops = 0;
  let tools = 0;

  for (const wave of draft.waves) {
    for (const laneKey of laneKeys) {
      for (const slot of wave[laneKey].troops) {
        troops += slot.quantity;
      }
      for (const slot of wave[laneKey].tools) {
        tools += slot.quantity;
      }
    }
  }

  return { troops, tools };
}

function defaultDraft(): AttackSetupDraft {
  return {
    waves: [emptyWave()],
  };
}

function emptyWave(): AttackSetupWave {
  return {
    L: emptyLane(),
    M: emptyLane(),
    R: emptyLane(),
  };
}

function emptyLane(): AttackSetupLane {
  return {
    troops: Array.from({ length: 2 }, () => emptySlot()),
    tools: Array.from({ length: 2 }, () => emptySlot()),
  };
}

function emptySlot(): AttackSetupSlot {
  return {
    itemId: null,
    quantity: 0,
  };
}

function laneLabel(laneKey: (typeof laneKeys)[number]): string {
  if (laneKey === 'L') {
    return 'Left flank';
  }
  if (laneKey === 'M') {
    return 'Middle front';
  }
  return 'Right flank';
}

export default AttackSetupModal;
