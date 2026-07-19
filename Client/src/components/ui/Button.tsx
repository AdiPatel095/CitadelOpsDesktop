import React, { ButtonHTMLAttributes } from 'react';

export interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: 'primary' | 'secondary' | 'ghost' | 'danger' | 'outline' | 'glass';
  size?: 'sm' | 'md' | 'lg' | 'icon';
  isLoading?: boolean;
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
}

export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  (
    {
      className = '',
      variant = 'primary',
      size = 'md',
      isLoading,
      leftIcon,
      rightIcon,
      children,
      disabled,
      ...props
    },
    ref
  ) => {
    const baseStyles = 'inline-flex items-center justify-center rounded-global font-semibold transition-all duration-200 active:scale-[0.98] focus:outline-none focus-visible:ring-2 focus-visible:ring-primary/40 focus-visible:ring-offset-2 focus-visible:ring-offset-bg-app disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap';
    
    const variants = {
      primary: 'bg-primary text-text-inverted border border-white/20 shadow-glow hover:bg-primary-hover hover:shadow-glow-active',
      secondary: 'bg-bg-card/55 text-text-main border border-border-light shadow-[var(--glass-shadow-compact)] backdrop-blur-2xl hover:bg-bg-card-hover/80 hover:border-primary/25',
      ghost: 'bg-transparent text-text-muted hover:text-text-main hover:bg-bg-card-hover/70',
      danger: 'bg-error/10 text-error border border-error/25 hover:bg-error/18 hover:border-error/45 shadow-[0_0_18px_color-mix(in_srgb,var(--color-error)_24%,transparent)]',
      outline: 'bg-bg-card/30 border border-primary/35 text-primary shadow-[0_0_18px_color-mix(in_srgb,var(--color-primary)_15%,transparent)] backdrop-blur-xl hover:bg-primary/10 hover:border-primary/55',
      glass: 'bg-bg-card/45 backdrop-blur-2xl border border-border-light shadow-[var(--glass-shadow-compact)] hover:bg-bg-card-hover/75 text-text-main',
    };

    const sizes = {
      sm: 'px-3 py-1.5 text-xs gap-1.5',
      md: 'px-4 py-2 text-sm gap-2',
      lg: 'px-6 py-3 text-base gap-3',
      icon: 'p-2 flex-shrink-0',
    };

    return (
      <button
        ref={ref}
        disabled={disabled || isLoading}
        className={`${baseStyles} ${variants[variant]} ${sizes[size]} ${className}`}
        {...props}
      >
        {isLoading && (
          <svg className="animate-spin -ml-1 mr-2 h-4 w-4 text-current" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4"></circle>
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path>
          </svg>
        )}
        {!isLoading && leftIcon}
        {children}
        {!isLoading && rightIcon}
      </button>
    );
  }
);

Button.displayName = 'Button';
