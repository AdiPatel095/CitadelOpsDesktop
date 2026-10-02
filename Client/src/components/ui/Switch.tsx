import React from 'react';
import { Check } from 'lucide-react';
import './switch.css';

export interface SwitchProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  className?: string;
  ariaLabel: string;
  ariaDescribedBy?: string;
  ariaLabelledBy?: string;
  lang?: string;
  dir?: React.HTMLAttributes<HTMLButtonElement>['dir'];
}

export const Switch: React.FC<SwitchProps> = ({ checked, onChange, disabled = false,
  className = '', ariaLabel, ariaDescribedBy, ariaLabelledBy, lang, dir }) => (
  <button type="button" role="switch" aria-checked={checked} data-state={checked ? 'on' : 'off'}
    aria-label={ariaLabel} aria-describedby={ariaDescribedBy} aria-labelledby={ariaLabelledBy}
    lang={lang} dir={dir} disabled={disabled} onClick={() => onChange(!checked)}
    className={`ui-switch ${className}`}>
    <span aria-hidden="true" className="ui-switch__track" />
    <span aria-hidden="true" className="ui-switch__thumb">{checked && <Check size={12} aria-hidden="true" />}</span>
  </button>
);
