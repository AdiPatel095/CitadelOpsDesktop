import React from 'react';

export interface SwitchProps {
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
  size?: 'sm' | 'md' | 'lg';
}

export const Switch: React.FC<SwitchProps> = ({
  checked,
  onChange,
  disabled = false,
  size = 'md',
}) => {
  const sizes = {
    sm: { w: 'w-8', h: 'h-4', dot: 'w-3 h-3', translate: 'translate-x-4' },
    md: { w: 'w-11', h: 'h-6', dot: 'w-5 h-5', translate: 'translate-x-5' },
    lg: { w: 'w-14', h: 'h-7', dot: 'w-6 h-6', translate: 'translate-x-7' },
  };

  const currentSize = sizes[size];

  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`
        relative inline-flex shrink-0 cursor-pointer rounded-full border
        transition-colors duration-200 ease-in-out focus:outline-none focus-visible:ring-2 
        focus-visible:ring-primary focus-visible:ring-offset-2 focus-visible:ring-offset-bg-app
        ${currentSize.w} ${currentSize.h}
        ${checked ? 'bg-primary border-primary/40 shadow-glow' : 'bg-bg-card/55 border-border-base backdrop-blur-xl'}
        ${disabled ? 'opacity-50 cursor-not-allowed' : ''}
      `}
    >
      <span className="sr-only">Toggle switch</span>
      <span
        aria-hidden="true"
        className={`
          pointer-events-none inline-block rounded-full bg-white shadow-lg ring-0
          transition duration-200 ease-in-out ${currentSize.dot}
          ${checked ? currentSize.translate : 'translate-x-0'}
        `}
      />
    </button>
  );
};
