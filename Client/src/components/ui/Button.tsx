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
    const baseStyles = 'inline-flex items-center justify-center rounded-global font-semibold transition-all duration-200 active:scale-95 focus:outline-none disabled:opacity-50 disabled:cursor-not-allowed whitespace-nowrap';
    
    const variants = {
      primary: 'bg-primary text-text-inverted shadow-glow hover:bg-primary-hover hover:shadow-glow-active',
      secondary: 'bg-bg-card-hover text-text-main border border-border-base hover:border-border-light',
      ghost: 'bg-transparent text-text-muted hover:text-text-main hover:bg-bg-card-hover',
      danger: 'bg-error/10 text-error border border-error/20 hover:bg-error/20 hover:border-error/40 shadow-[0_0_15px_var(--color-error)]',
      outline: 'bg-transparent border-2 border-primary/40 text-primary hover:bg-primary/10 shadow-[0_0_15px_rgba(16,185,129,0.1)]',
      glass: 'bg-bg-card/80 backdrop-blur-md border border-border-base shadow-lg hover:bg-bg-card-hover text-text-main',
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
