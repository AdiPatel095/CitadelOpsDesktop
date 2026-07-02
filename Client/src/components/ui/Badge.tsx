import React, { HTMLAttributes } from 'react';

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: 'primary' | 'secondary' | 'success' | 'warning' | 'danger' | 'outline';
}

export const Badge = React.forwardRef<HTMLSpanElement, BadgeProps>(
  ({ className = '', variant = 'primary', children, ...props }, ref) => {
    const baseStyles = 'inline-flex items-center px-2.5 py-0.5 rounded-full text-xs font-semibold uppercase transition-colors backdrop-blur-xl';
    
    const variants = {
      primary: 'bg-primary/12 text-primary border border-primary/25 shadow-[0_0_18px_color-mix(in_srgb,var(--color-primary)_12%,transparent)]',
      secondary: 'bg-bg-card/55 text-text-muted border border-border-light',
      success: 'bg-success/12 text-success border border-success/25',
      warning: 'bg-warning/12 text-warning border border-warning/25',
      danger: 'bg-error/12 text-error border border-error/25',
      outline: 'bg-bg-card/30 text-text-main border border-border-base',
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
