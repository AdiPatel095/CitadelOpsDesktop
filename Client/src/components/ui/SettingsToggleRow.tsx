import React, { type HTMLAttributes, type ReactNode } from 'react';
import { Switch } from './Switch';

export interface SettingsToggleRowProps extends Omit<HTMLAttributes<HTMLDivElement>, 'title' | 'onChange'> {
  title: ReactNode;
  description?: ReactNode;
  icon?: ReactNode;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  disabledReason?: ReactNode;
  ariaLabel?: string;
  tone?: 'default' | 'warning' | 'danger';
  switchSize?: 'sm' | 'md' | 'lg';
}

export const SettingsToggleRow: React.FC<SettingsToggleRowProps> = ({
  title,
  description,
  icon,
  checked,
  onChange,
  disabled = false,
  disabledReason,
  ariaLabel,
  tone = 'default',
  switchSize = 'sm',
  className = '',
  ...props
}) => {
  const toneClass = {
    default: 'border-border-base bg-bg-input/35',
    warning: 'border-warning/25 bg-warning/5',
    danger: 'border-error/25 bg-error/5',
  }[tone];
  const iconClass = tone === 'warning' ? 'text-warning' : tone === 'danger' ? 'text-error' : 'text-primary';
  const accessibleName = ariaLabel ?? (typeof title === 'string' ? title : 'Toggle setting');

  return (
    <div className={`flex items-start justify-between gap-4 rounded-global border px-4 py-3 ${toneClass} ${disabled ? 'opacity-55' : ''} ${className}`} {...props}>
      <div className="min-w-0">
        <div className="flex items-center gap-2 text-sm font-bold text-text-main">
          {icon && <span className={iconClass} aria-hidden="true">{icon}</span>}
          {title}
        </div>
        {description && <div className="mt-0.5 text-[11px] font-medium leading-relaxed text-text-muted">{description}</div>}
        {disabled && disabledReason && <div className="mt-1 text-[11px] font-semibold text-warning">{disabledReason}</div>}
      </div>
      <Switch checked={checked} onChange={onChange} disabled={disabled} size={switchSize} ariaLabel={accessibleName} />
    </div>
  );
};
