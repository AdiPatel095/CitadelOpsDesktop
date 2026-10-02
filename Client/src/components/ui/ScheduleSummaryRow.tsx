import React, { type HTMLAttributes, type ReactNode } from 'react';
import { CalendarDays } from 'lucide-react';
import { Button, type ButtonSize } from './Button';

export interface ScheduleSummaryRowProps extends Omit<HTMLAttributes<HTMLDivElement>, 'title'> {
  summary: ReactNode;
  onEdit: () => void;
  title?: ReactNode;
  actionLabel?: string;
  actionSize?: ButtonSize;
  status?: ReactNode;
}

export const ScheduleSummaryRow: React.FC<ScheduleSummaryRowProps> = ({
  summary,
  onEdit,
  title = 'Weekly schedule',
  actionLabel = 'Schedule',
  actionSize = 'sm',
  status,
  className = '',
  ...props
}) => (
  <div className={`flex flex-wrap items-center justify-between gap-3 rounded-global border border-border-base bg-bg-input/35 px-4 py-3 ${className}`} {...props}>
    <div className="min-w-0">
      <div className="text-body font-semibold text-text-main">{title}</div>
      <div className="mt-0.5 text-caption font-medium text-text-muted">{summary}</div>
    </div>
    <div className="flex shrink-0 items-center gap-2">
      {status}
      <Button variant="secondary" size={actionSize} onClick={onEdit} leftIcon={<CalendarDays className="h-4 w-4" />}>
        {actionLabel}
      </Button>
    </div>
  </div>
);
