import React from 'react';

export interface ToggleGroupOption {
  value: string;
  label: React.ReactNode;
}

export interface ToggleGroupProps {
  value: string;
  options: ToggleGroupOption[];
  onChange: (value: string) => void;
  className?: string;
  size?: 'sm' | 'md' | 'lg';
  fullWidth?: boolean;
  variant?: 'primary' | 'neutral';
}

export const ToggleGroup: React.FC<ToggleGroupProps> = ({
  value,
  options,
  onChange,
  className = '',
  size = 'md',
  fullWidth = false,
  variant = 'primary',
}) => {
  const sizes = {
    sm: 'text-xs',
    md: 'text-sm',
    lg: 'text-base',
  };

  const btnSizes = {
    sm: 'px-2.5 py-1',
    md: 'px-5 py-1.5',
    lg: 'px-7 py-2',
  };

  const activeBtnSizes = {
    sm: 'px-3 py-1.5 -mx-0.5 -my-0.5',
    md: 'px-6 py-2.5 -mx-1 -my-1',
    lg: 'px-8 py-3 -mx-1 -my-1',
  };

  const activeStyles = {
    primary: 'text-text-inverted shadow-[0_0_15px_rgba(16,185,129,0.2)] bg-primary',
    neutral: 'bg-bg-card shadow-lg border border-border-base text-text-main',
  };

  return (
    <div
      className={`${fullWidth ? 'flex w-full' : 'inline-flex'} items-center bg-bg-app border border-border-base rounded-[24px] relative ${sizes[size]} ${className}`}
      role="group"
    >
      {options.map((option) => {
        const isActive = value === option.value;
        return (
          <button
            key={option.value}
            type="button"
            onClick={() => onChange(option.value)}
            className={`
              relative z-10 font-semibold rounded-[24px] transition-all duration-200 ease-out whitespace-nowrap
              ${fullWidth ? 'flex-1' : ''}
              ${isActive ? activeBtnSizes[size] : btnSizes[size]}
              ${isActive ? activeStyles[variant] : 'text-text-muted hover:text-text-main'}
            `}
          >
            {option.label}
          </button>
        );
      })}
    </div>
  );
};
