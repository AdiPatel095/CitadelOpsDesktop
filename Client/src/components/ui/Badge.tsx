import React, { HTMLAttributes } from 'react';

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: 'primary' | 'secondary' | 'success' | 'warning' | 'danger' | 'outline';
}

export const Badge = React.forwardRef<HTMLSpanElement, BadgeProps>(
  ({ className = '', variant = 'primary', children, ...props }, ref) => {
    const baseStyles = 'inline-flex items-center px-2 py-0.5 rounded-full text-xs font-semibold uppercase tracking-wider transition-colors';
    
    const variants = {
      primary: 'bg-primary/10 text-primary border border-primary/20',
      secondary: 'bg-bg-card-hover text-text-muted border border-border-base',
      success: 'bg-success/10 text-success border border-success/20',
      warning: 'bg-warning/10 text-warning border border-warning/20',
      danger: 'bg-error/10 text-error border border-error/20',
      outline: 'bg-transparent text-text-main border border-border-base',
    };

    return (
      <span
        ref={ref}
        className={`${baseStyles} ${variants[variant]} ${className}`}
        {...props}
      >
        {children}
      </span>
    );
  }
);

Badge.displayName = 'Badge';
