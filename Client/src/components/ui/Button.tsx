import React, { type ButtonHTMLAttributes, type ReactNode } from 'react';
import './button.css';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
export type ButtonSize = 'sm' | 'md' | 'lg';
export interface ButtonStyle { variant?: ButtonVariant; size?: ButtonSize; iconOnly?: boolean; className?: string }
export function buttonAttributes({ variant = 'secondary', size = 'md', iconOnly = false, className = '' }: ButtonStyle = {}): { className: string; 'data-variant': ButtonVariant; 'data-size': ButtonSize } {
  return { className: `cit-button${iconOnly ? ' cit-button-icon' : ''} ${className}`.trim(), 'data-variant': variant, 'data-size': size };
}
export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement>, Omit<ButtonStyle, 'className'> {
  isLoading?: boolean; leftIcon?: ReactNode; rightIcon?: ReactNode;
}
export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = 'secondary', size = 'md', iconOnly, isLoading, leftIcon, rightIcon, children, disabled, title, ...props }, ref) => {
    if (import.meta.env.DEV && iconOnly && !props['aria-label']) console.warn('Button iconOnly requires aria-label');
    return (
      <button {...props} {...buttonAttributes({ className, variant, size, iconOnly })} ref={ref}
        title={title ?? (iconOnly ? props['aria-label'] : undefined)} disabled={disabled || isLoading} aria-busy={isLoading || props['aria-busy']}>
        {isLoading ? <svg className="cit-button-spinner" aria-hidden="true" viewBox="0 0 24 24"><circle cx="12" cy="12" r="9" fill="none" stroke="currentColor" strokeWidth="3" strokeDasharray="42 15" /></svg> : leftIcon ?? (isLoading !== undefined ? <span className="cit-button-loading-slot" aria-hidden="true" /> : null)}
        {children}
        {rightIcon}
      </button>
    );
  }
);
Button.displayName = 'Button';
