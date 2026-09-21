import React from 'react';

export interface SwitchProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  size?: 'sm' | 'md' | 'lg';
  className?: string;
  ariaLabel: string;
  lang?: string;
  dir?: React.HTMLAttributes<HTMLButtonElement>['dir'];
  ariaLabelledBy?: string;
}

export const Switch: React.FC<SwitchProps> = ({
  checked,
  onChange,
  disabled = false,
  size = 'sm',
  className = '',
  ariaLabel,
  lang,
  dir,
  ariaLabelledBy,
}) => {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={ariaLabel}
      aria-labelledby={ariaLabelledBy}
      lang={lang}
      dir={dir}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`liquid-switch liquid-switch-${size} ${checked ? 'liquid-switch-on' : 'liquid-switch-off'} ${className}`}
    >
      <span aria-hidden="true" className="liquid-switch-rail" />
      <span aria-hidden="true" className="liquid-switch-thumb" />
    </button>
  );
};
