import React, { type HTMLAttributes, type ReactNode } from 'react';

export interface MetricTileProps extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
  label: ReactNode;
  value: ReactNode;
  tone?: 'default' | 'brand' | 'success' | 'warning' | 'danger' | 'info';
  size?: 'sm' | 'md' | 'lg';
  monospace?: boolean;
  caption?: ReactNode;
}

export const MetricTile: React.FC<MetricTileProps> = ({
  label,
  value,
  tone = 'default',
  size = 'md',
  monospace = true,
  caption,
  className = '',
  ...props
}) => {
  const toneClass = {
    default: 'text-text-main',
    brand: 'text-primary',
    success: 'text-success',
    warning: 'text-warning',
    danger: 'text-error',
    info: 'text-info',
  }[tone];
  const borderClass = {
    default: 'border-border-base',
    brand: 'border-primary/25',
    success: 'border-success/20',
    warning: 'border-warning/20',
    danger: 'border-error/20',
    info: 'border-info/20',
  }[tone];
  const sizeClass = {
    sm: 'px-3 py-2 [&_.ui-metric-value]:text-sm',
    md: 'px-3 py-2.5 [&_.ui-metric-value]:text-base',
    lg: 'p-4 [&_.ui-metric-value]:text-2xl',
  }[size];

  return (
    <div className={`m3-metric-tile rounded-global border ${borderClass} ${sizeClass} ${className}`} {...props}>
      <div className="text-[10px] font-bold uppercase tracking-wider text-text-muted">{label}</div>
      <div className={`ui-metric-value mt-1 font-bold tabular-nums ${monospace ? 'font-mono' : ''} ${toneClass}`}>
        {typeof value === 'number' ? value.toLocaleString() : value}
      </div>
      {caption && <div className="mt-1 text-[11px] text-text-muted">{caption}</div>}
    </div>
  );
};
