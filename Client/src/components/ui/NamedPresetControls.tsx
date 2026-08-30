import React, { type ReactNode } from 'react';
import { Plus, Trash2 } from 'lucide-react';
import { Button } from './Button';
import { Card } from './Card';
import { Input } from './Input';
import { Select, type SelectOption } from './Select';

export interface NamedPresetControlsProps {
  name: string;
  onNameChange: (value: string) => void;
  nameError?: string;
  selectedID: string;
  onSelectedIDChange: (value: string) => void;
  options: SelectOption[];
  onApply: () => void;
  onSaveAsNew: () => void;
  onDelete: () => void;
  help?: ReactNode;
  className?: string;
  disabled?: boolean;
}

export const NamedPresetControls: React.FC<NamedPresetControlsProps> = ({
  name,
  onNameChange,
  nameError,
  selectedID,
  onSelectedIDChange,
  options,
  onApply,
  onSaveAsNew,
  onDelete,
  help,
  className = '',
  disabled = false,
}) => (
  <Card variant="solid" className={`shrink-0 border-border-base bg-bg-app p-4 ${className}`}>
    <div className="mb-3 text-xs font-bold uppercase tracking-wider text-primary">Presets</div>
    <div className="flex flex-col gap-4 lg:flex-row lg:flex-wrap lg:items-end">
      <label className="flex min-w-0 flex-1 flex-col gap-1.5 md:min-w-[220px]">
        <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Preset name</span>
        <Input
          type="text"
          placeholder="Name for new preset or rename on save"
          value={name}
          onChange={(event) => onNameChange(event.target.value)}
          error={nameError}
          disabled={disabled}
        />
      </label>
      <div className="flex min-w-0 flex-[2] flex-col gap-1.5 md:min-w-[280px]">
        <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted">Load preset</span>
        <div className="flex flex-col gap-2 md:flex-row">
          <div className="min-w-0 flex-1">
            <Select value={selectedID} onChange={onSelectedIDChange} options={options} ariaLabel="Load preset" disabled={disabled} />
          </div>
          <Button variant="outline" onClick={onApply} disabled={disabled} className="w-full shrink-0 bg-bg-card md:w-auto">Apply</Button>
        </div>
      </div>
      <div className="grid grid-cols-2 gap-2 md:flex md:flex-wrap">
        <Button
          variant="secondary"
          onClick={onSaveAsNew}
          disabled={disabled}
          className="min-w-0 border-info/40 px-2 text-info hover:bg-info/10 md:px-3"
          leftIcon={<Plus className="h-4 w-4" />}
        >
          Save as new
        </Button>
        <Button variant="danger" disabled={disabled || !selectedID} onClick={onDelete} className="min-w-0 px-2 md:px-3" leftIcon={<Trash2 className="h-4 w-4" />}>
          Delete
        </Button>
      </div>
    </div>
    {help && <p className="mt-3 text-xs text-text-muted">{help}</p>}
  </Card>
);
