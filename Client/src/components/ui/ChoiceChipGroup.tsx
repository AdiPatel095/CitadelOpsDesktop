import React, { type ReactNode } from 'react';

export type ChoiceChipValue = string | number;

export interface ChoiceChipOption<T extends ChoiceChipValue> {
  value: T;
  label: ReactNode;
  disabled?: boolean;
  title?: string;
}

export interface ChoiceChipGroupProps<T extends ChoiceChipValue> {
  options: readonly ChoiceChipOption<T>[];
  selected: readonly T[];
  onToggle: (value: T) => void;
  ariaLabel: string;
  disabled?: boolean;
  size?: 'sm' | 'md';
  className?: string;
}

export function ChoiceChipGroup<T extends ChoiceChipValue>({
  options,
  selected,
  onToggle,
  ariaLabel,
  disabled = false,
  size = 'md',
  className = '',
}: ChoiceChipGroupProps<T>) {
  const sizeClass = size === 'sm' ? 'px-2.5 py-1 text-[11px]' : 'px-3 py-1.5 text-xs';

  return (
    <div className={`flex flex-wrap gap-2 ${className}`} role="group" aria-label={ariaLabel}>
      {options.map((option) => {
        const active = selected.includes(option.value);
        return (
          <button
            key={String(option.value)}
            type="button"
            aria-pressed={active}
            disabled={disabled || option.disabled}
            title={option.title}
            onClick={() => onToggle(option.value)}
            className={`rounded-full border font-semibold transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 disabled:cursor-not-allowed disabled:opacity-45 ${sizeClass} ${
              active
                ? 'border-primary/45 bg-primary/12 text-primary'
                : 'border-border-base bg-bg-card/35 text-text-muted hover:text-text-main'
            }`}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
}
