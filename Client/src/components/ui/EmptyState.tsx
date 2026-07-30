import React, { type HTMLAttributes, type ReactNode } from 'react';

export interface EmptyStateProps extends HTMLAttributes<HTMLDivElement> {
  title: ReactNode;
  description?: ReactNode;
  icon?: ReactNode;
  action?: ReactNode;
  size?: 'sm' | 'md' | 'lg';
  surface?: 'plain' | 'outlined';
}

export const EmptyState: React.FC<EmptyStateProps> = ({
  title,
  description,
  icon,
  action,
  size = 'md',
  surface = 'outlined',
  className = '',
  ...props
}) => {
  const sizeClass = {
    sm: 'min-h-28 px-4 py-5',
    md: 'min-h-40 px-5 py-8',
    lg: 'min-h-64 px-6 py-10',
  }[size];
  const surfaceClass = surface === 'outlined'
    ? 'rounded-global border border-dashed border-border-base bg-bg-card/45'
    : '';

  return (
    <div className={`flex flex-col items-center justify-center text-center ${sizeClass} ${surfaceClass} ${className}`} {...props}>
      {icon && (
        <span className="mb-4 flex h-14 w-14 items-center justify-center rounded-full border border-border-base bg-bg-input/60 text-text-muted" aria-hidden="true">
          {icon}
        </span>
      )}
      <div className="text-lg font-black text-text-main">{title}</div>
      {description && <div className="mt-2 max-w-xl text-sm text-text-muted">{description}</div>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  );
};
