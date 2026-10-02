import { useRef, type ReactNode, type KeyboardEvent } from 'react';
import { useLocale } from '../../i18n/LocaleContext';
import { Button } from './Button';
import { Select } from './Select';
import { nextTabIndex, segmentedMode } from './tabsLogic';
import './Tabs.css';
export interface ToggleGroupOption { value: string; label: ReactNode; icon?: ReactNode; title?: string; disabled?: boolean; }
export interface ToggleGroupProps {
  value: string; options: readonly ToggleGroupOption[]; onChange(value: string): void;
  ariaLabel: string; className?: string; size: 'header' | 'body'; fullWidth?: boolean; variant?: 'primary' | 'neutral';
}
export function ToggleGroup({ value, options, onChange, ariaLabel, className = '', size, fullWidth = false }: ToggleGroupProps) {
  const { direction } = useLocale();
  const buttons = useRef<(HTMLButtonElement | null)[]>([]);
  const active = options.findIndex(option => option.value === value && !option.disabled);
  const first = options.findIndex(option => !option.disabled);
  const keyboard = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    const key = event.key === 'ArrowUp' ? 'ArrowLeft' : event.key === 'ArrowDown' ? 'ArrowRight' : event.key;
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(key)) return;
    event.preventDefault();
    const next = nextTabIndex(key, index, options, (event.key === 'ArrowUp' || event.key === 'ArrowDown') ? false : direction === 'rtl');
    if (options[next]?.disabled) return;
    onChange(options[next].value); buttons.current[next]?.focus({ preventScroll: true });
  };
  if (segmentedMode(options.length) === 'select') return <Select value={value} options={options.map(option => ({ ...option }))} onChange={onChange} ariaLabel={ariaLabel} className={className} />;
  return <div role="radiogroup" aria-label={ariaLabel} dir={direction} className={`ui-segments ui-segments--${size} ${fullWidth ? 'ui-segments--full' : ''} ${className}`}>
    {options.map((option, index) => <Button variant="ghost" type="button" key={option.value} ref={node => { buttons.current[index] = node; }} role="radio" aria-checked={value === option.value} data-current-selection={value === option.value ? "true" : undefined}
      disabled={option.disabled} tabIndex={index === (active < 0 ? first : active) ? 0 : -1} title={option.title}
      className="ui-segments__option" onClick={() => onChange(option.value)} onKeyDown={event => keyboard(event, index)}>
      {option.icon}{option.label}
    </Button>)}
  </div>;
}
