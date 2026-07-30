import React, { type HTMLAttributes, type ReactNode } from 'react';

export type StatusTone = 'brand' | 'success' | 'warning' | 'danger' | 'info' | 'neutral';

export interface StatusIndicatorProps extends Omit<HTMLAttributes<HTMLDivElement>, 'children'> {
  label: ReactNode;
  detail?: ReactNode;
  tone?: StatusTone;
}

export const StatusIndicator: React.FC<StatusIndicatorProps> = ({
  label,
  detail,
  tone = 'neutral',
  className = '',
  ...props
}) => (
  <div className={`ui-status ui-status-${tone} ${className}`} {...props}>
    <span className="ui-status-symbol" aria-hidden="true" />
    <span className="ui-status-label">{label}</span>
    {detail && <span className="ui-status-detail">{detail}</span>}
  </div>
);
