import React from 'react';

export interface SwitchProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
  ariaLabel?: string;
}

export const Switch: React.FC<SwitchProps> = ({
  checked,
  onChange,
  disabled = false,
  size = 'md',
  className = '',
  ariaLabel = 'Toggle setting',
}) => {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`liquid-switch liquid-switch-${size} ${checked ? 'liquid-switch-on' : 'liquid-switch-off'} ${className}`}
    >
      <span className="sr-only">{ariaLabel}: {checked ? 'on' : 'off'}</span>
      <span aria-hidden="true" className="liquid-switch-rail" />
      <span aria-hidden="true" className="liquid-switch-thumb" />
    </button>
  );
};
