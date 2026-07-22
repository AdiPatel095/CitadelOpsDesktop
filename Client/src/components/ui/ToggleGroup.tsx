import React, { useCallback, useLayoutEffect, useMemo, useRef } from 'react';

export interface ToggleGroupOption {
  value: string;
  label: React.ReactNode;
  icon?: React.ReactNode;
  /** Native tooltip (useful for truncated labels in scrollable groups) */
  title?: string;
}

export interface ToggleGroupProps {
  value: string;
  options: readonly ToggleGroupOption[];
  onChange: (value: string) => void;
  ariaLabel: string;
  className?: string;
  size: 'header' | 'body';
  fullWidth?: boolean;
  variant?: 'primary' | 'neutral';
}

export const ToggleGroup: React.FC<ToggleGroupProps> = ({
  value,
  options,
  onChange,
  ariaLabel,
  className = '',
  size,
  fullWidth = false,
  variant = 'primary',
}) => {
  const groupRef = useRef<HTMLDivElement | null>(null);
  const indicatorRef = useRef<HTMLSpanElement | null>(null);
  const buttonRefs = useRef<Array<HTMLButtonElement | null>>([]);

  const selectByKeyboard = (event: React.KeyboardEvent<HTMLButtonElement>, currentIndex: number) => {
    let nextIndex = currentIndex;

    switch (event.key) {
      case 'ArrowLeft':
      case 'ArrowUp':
        nextIndex = (currentIndex - 1 + options.length) % options.length;
        break;
      case 'ArrowRight':
      case 'ArrowDown':
        nextIndex = (currentIndex + 1) % options.length;
        break;
      case 'Home':
        nextIndex = 0;
        break;
      case 'End':
        nextIndex = options.length - 1;
        break;
      default:
        return;
    }

    event.preventDefault();
    const nextOption = options[nextIndex];
    if (!nextOption) return;
    onChange(nextOption.value);
    buttonRefs.current[nextIndex]?.focus();
  };

  const activeIndex = options.findIndex((option) => option.value === value);
  const optionSignature = useMemo(
    () => options.map((option) => option.value).join('\u0000'),
    [options],
  );
  const syncIndicator = useCallback(() => {
    const indicator = indicatorRef.current;
    const activeButton = buttonRefs.current[activeIndex];
    if (!indicator || !activeButton) {
      indicator?.classList.remove('liquid-toggle-indicator-ready');
      return;
    }

    indicator.style.setProperty('--liquid-toggle-indicator-x', `${activeButton.offsetLeft}px`);
    indicator.style.setProperty('--liquid-toggle-indicator-width', `${activeButton.offsetWidth}px`);
    indicator.classList.add('liquid-toggle-indicator-ready');
  }, [activeIndex]);

  useLayoutEffect(() => {
    syncIndicator();

    if (typeof ResizeObserver === 'undefined') return undefined;
    const observer = new ResizeObserver(syncIndicator);
    if (groupRef.current) observer.observe(groupRef.current);
    buttonRefs.current.forEach((button) => {
      if (button) observer.observe(button);
    });
    return () => observer.disconnect();
  }, [optionSignature, syncIndicator]);

  return (
    <div
      ref={groupRef}
      className={`liquid-toggle-group liquid-toggle-group-${size} ${fullWidth ? 'liquid-toggle-group-full' : ''} ${className}`}
      role="radiogroup"
      aria-label={ariaLabel}
    >
      <span
        ref={indicatorRef}
        className={`liquid-toggle-indicator liquid-toggle-indicator-${variant}`}
        aria-hidden="true"
      />
      {options.map((option, index) => {
        const isActive = value === option.value;
        const tip =
          option.title ??
          (typeof option.label === 'string' || typeof option.label === 'number' ? String(option.label) : undefined);
        return (
          <button
            key={option.value}
            ref={(node) => { buttonRefs.current[index] = node; }}
            type="button"
            role="radio"
            aria-checked={isActive}
            tabIndex={isActive || (activeIndex === -1 && index === 0) ? 0 : -1}
            title={tip}
            onClick={() => onChange(option.value)}
            onKeyDown={(event) => selectByKeyboard(event, index)}
            className={`liquid-toggle-btn liquid-toggle-btn-${size} ${fullWidth ? 'liquid-toggle-btn-full' : ''} ${
              isActive
                ? `liquid-toggle-btn-active liquid-toggle-btn-active-${variant}`
                : 'liquid-toggle-btn-inactive'
            }`}
          >
            {option.icon && <span className="liquid-toggle-btn-icon">{option.icon}</span>}
            {option.label}
          </button>
        );
      })}
    </div>
  );
};
