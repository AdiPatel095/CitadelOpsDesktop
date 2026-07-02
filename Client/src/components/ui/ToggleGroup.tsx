import React from 'react';

export interface ToggleGroupOption {
  value: string;
  label: React.ReactNode;
  icon?: React.ReactNode;
  /** Native tooltip (useful for truncated labels in scrollable groups) */
  title?: string;
}

export interface ToggleGroupProps {
  value: string;
  options: ToggleGroupOption[];
  onChange: (value: string) => void;
  className?: string;
  size?: 'sm' | 'md' | 'lg';
  fullWidth?: boolean;
  variant?: 'primary' | 'neutral';
}

export const ToggleGroup: React.FC<ToggleGroupProps> = ({
  value,
  options,
  onChange,
  className = '',
  size = 'md',
  fullWidth = false,
  variant = 'primary',
}) => {
  return (
    <div
      className={`liquid-toggle-group liquid-toggle-group-${size} ${fullWidth ? 'liquid-toggle-group-full' : ''} ${className}`}
      role="group"
    >
      {options.map((option) => {
        const isActive = value === option.value;
        const tip =
          option.title ??
          (typeof option.label === 'string' || typeof option.label === 'number' ? String(option.label) : undefined);
        return (
          <button
            key={option.value}
            type="button"
            title={tip}
            onClick={() => onChange(option.value)}
            className={`liquid-toggle-btn liquid-toggle-btn-${size} ${fullWidth ? 'liquid-toggle-btn-full' : ''} ${
              isActive
                ? `liquid-toggle-btn-active liquid-toggle-btn-active-${variant}`
                : 'liquid-toggle-btn-inactive'
            }`}
          >
            {option.icon && <span className="liquid-toggle-btn-icon">{option.icon}</span>}
            {option.label}
          </button>
        );
      })}
    </div>
  );
};
