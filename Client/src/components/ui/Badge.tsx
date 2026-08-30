import React, { HTMLAttributes } from 'react';

export interface BadgeProps extends HTMLAttributes<HTMLSpanElement> {
  variant?: 'primary' | 'secondary' | 'success' | 'warning' | 'danger' | 'outline';
}

export const Badge = React.forwardRef<HTMLSpanElement, BadgeProps>(
  ({ className = '', variant = 'primary', children, ...props }, ref) => {
    const baseStyles = 'm3-chip inline-flex items-center px-2.5 py-0.5 text-xs font-semibold transition-colors';
    
    const variants = {
      primary: 'm3-chip-primary',
      secondary: 'm3-chip-secondary',
      success: 'm3-chip-success',
      warning: 'm3-chip-warning',
      danger: 'm3-chip-danger',
      outline: 'm3-chip-outline',
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
