import React, { InputHTMLAttributes } from 'react';

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  leftIcon?: React.ReactNode;
  rightIcon?: React.ReactNode;
  error?: string;
}

export const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className = '', leftIcon, rightIcon, error, ...props }, ref) => {
    return (
      <div className="w-full flex flex-col gap-1.5">
        <div className="relative flex items-center w-full">
          {leftIcon && (
            <div className="absolute left-3 text-text-muted pointer-events-none">
              {leftIcon}
            </div>
          )}
          <input
            ref={ref}
            className={`w-full bg-bg-app border px-4 py-2.5 text-text-main placeholder-text-muted focus:ring-1 focus:outline-none transition-colors duration-200 rounded-global text-sm
              ${error ? 'border-error focus:border-error focus:ring-error' : 'border-border-base focus:border-primary focus:ring-primary'}
              ${leftIcon ? 'pl-10' : ''}
              ${rightIcon ? 'pr-10' : ''}
              ${className}
            `}
            {...props}
          />
          {rightIcon && (
            <div className="absolute right-3 text-text-muted">
              {rightIcon}
            </div>
          )}
        </div>
        {error && <span className="text-xs text-error font-medium">{error}</span>}
      </div>
    );
  }
);

Input.displayName = 'Input';
