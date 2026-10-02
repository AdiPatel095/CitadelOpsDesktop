import { useCallback, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { ChevronLeft, ChevronRight } from 'lucide-react';
import { Button } from './Button';
import { Select } from './Select';
import { nextTabIndex, tabsMode } from './tabsLogic';
import './Tabs.css';

export interface TabsProps {
  items: readonly { value: string; label: string; disabled?: boolean }[];
  value: string;
  onChange(value: string): void;
  ariaLabel: string;
  previousLabel: string;
  nextLabel: string;
  idBase: string;
  className?: string;
}

export function Tabs({ items, value, onChange, ariaLabel, previousLabel, nextLabel, idBase, className = '' }: TabsProps) {
  const [medium, setMedium] = useState(() => window.matchMedia('(min-width: 768px)').matches);
  const [edges, setEdges] = useState({ left: false, right: false, rtl: false });
  const scroller = useRef<HTMLDivElement>(null);
  const buttons = useRef(new Map<string, HTMLButtonElement>());
  const previousRect = useRef<{ left: number; width: number } | null>(null);
  const mode = tabsMode(items.length, medium);
  const selected = items.find(item => item.value === value);
  const signature = items.map(item => item.value).join('\0');
  const updateEdges = useCallback(() => {
    const element = scroller.current;
    if (!element) return;
    const rect = element.getBoundingClientRect();
    const children = [...element.querySelectorAll<HTMLElement>('[role="tab"]')];
    const left = children.some(child => child.getBoundingClientRect().left < rect.left - 1);
    const right = children.some(child => child.getBoundingClientRect().right > rect.right + 1);
    const rtl = getComputedStyle(element).direction === 'rtl';
    setEdges(current => current.left === left && current.right === right && current.rtl === rtl ? current : { left, right, rtl });
  }, []);
  useLayoutEffect(() => {
    const query = window.matchMedia('(min-width: 768px)');
    const changed = () => setMedium(query.matches);
    query.addEventListener('change', changed);
    return () => query.removeEventListener('change', changed);
  }, []);
  useLayoutEffect(() => {
    const element = scroller.current;
    if (!element) return;
    updateEdges();
    const syncGeometry = () => {
      const selected = element.querySelector<HTMLElement>('[aria-selected="true"]');
      if (selected) {
        const viewport = element.getBoundingClientRect(); const target = selected.getBoundingClientRect();
        const delta = target.left < viewport.left ? target.left - viewport.left : target.right > viewport.right ? target.right - viewport.right : 0;
        if (delta) element.scrollBy({ left: delta, behavior: 'instant' });
      }
      updateEdges();
    };
    const observer = new ResizeObserver(syncGeometry);
    const directionObserver = new MutationObserver(syncGeometry);
    directionObserver.observe(document.documentElement, { attributes: true, subtree: true, attributeFilter: ['dir'] });
    observer.observe(element);
    buttons.current.forEach(button => observer.observe(button));
    element.addEventListener('scroll', updateEdges, { passive: true });
    return () => { observer.disconnect(); directionObserver.disconnect(); element.removeEventListener('scroll', updateEdges); };
  }, [mode, signature, updateEdges]);
  useLayoutEffect(() => {
    const element = scroller.current;
    const button = buttons.current.get(value);
    if (!element || !button) { previousRect.current = null; return; }
    const reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
    const indicator = button.querySelector<HTMLElement>('.ui-tabs__indicator');
    const rect = { left: button.offsetLeft, width: button.offsetWidth };
    if (indicator && previousRect.current && !reduced) {
      const duration = parseFloat(getComputedStyle(element).getPropertyValue('--duration-base')) || 200;
      indicator.animate([
        { transform: `translateX(${previousRect.current.left - rect.left}px) scaleX(${previousRect.current.width / rect.width})` },
        { transform: 'translateX(0) scaleX(1)' },
      ], { duration, easing: getComputedStyle(element).getPropertyValue('--ease-standard').trim() || 'ease' });
    }
    previousRect.current = rect;
    const viewport = element.getBoundingClientRect();
    const target = button.getBoundingClientRect();
    const delta = target.left < viewport.left ? target.left - viewport.left : target.right > viewport.right ? target.right - viewport.right : 0;
    if (delta) element.scrollBy({ left: delta, behavior: reduced ? 'instant' : 'smooth' });
    updateEdges();
  }, [value, mode, signature, updateEdges]);
  const scroll = (side: 'left' | 'right') => scroller.current?.scrollBy({
    left: (side === 'left' ? -1 : 1) * scroller.current.clientWidth * 0.8,
    behavior: window.matchMedia('(prefers-reduced-motion: reduce)').matches ? 'instant' : 'smooth',
  });
  if (mode === 'select') return <div className={`ui-tabs ui-tabs--select ${className}`}>
    <Select value={value} options={items.map(item => ({ ...item }))} onChange={onChange} ariaLabel={ariaLabel} />
    <span id={`${idBase}-tab-${value}`} className="ui-tabs__label">{selected?.label ?? ariaLabel}</span>
  </div>;
  const activeIndex = items.findIndex(item => item.value === value && !item.disabled);
  const firstEnabled = items.findIndex(item => !item.disabled);
  return <div className={`ui-tabs ${className}`}>
    {medium && edges.left && <Button type="button" variant="ghost" iconOnly className="ui-tabs__arrow ui-tabs__arrow--left" aria-label={edges.rtl ? nextLabel : previousLabel} onClick={() => scroll('left')}><ChevronLeft /></Button>}
    <div ref={scroller} role="tablist" aria-label={ariaLabel} className="ui-tabs__scroller" data-fade-left={edges.left} data-fade-right={edges.right}>
      {items.map((item, index) => <Button variant="ghost" type="button" key={item.value}
        ref={node => { if (node) buttons.current.set(item.value, node); else buttons.current.delete(item.value); }}
        id={`${idBase}-tab-${item.value}`} role="tab" aria-selected={item.value === value} data-current-selection={item.value === value ? "true" : undefined}
        aria-controls={`${idBase}-panel-${item.value}`} disabled={item.disabled}
        tabIndex={index === (activeIndex < 0 ? firstEnabled : activeIndex) ? 0 : -1}
        className="ui-tabs__tab" data-text={item.label} onClick={() => onChange(item.value)}
        onKeyDown={event => {
          if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return;
          event.preventDefault();
          const next = nextTabIndex(event.key, index, items, getComputedStyle(event.currentTarget).direction === 'rtl');
          if (!items[next] || items[next].disabled) return;
          onChange(items[next].value); buttons.current.get(items[next].value)?.focus({ preventScroll: true });
        }}>
        <span>{item.label}</span>{item.value === value && <span className="ui-tabs__indicator" aria-hidden="true" />}
      </Button>)}
    </div>
    {medium && edges.right && <Button type="button" variant="ghost" iconOnly className="ui-tabs__arrow ui-tabs__arrow--right" aria-label={edges.rtl ? previousLabel : nextLabel} onClick={() => scroll('right')}><ChevronRight /></Button>}
  </div>;
}

export function TabPanel({ idBase, value, children }: { idBase: string; value: string; children: ReactNode }) {
  return <div className="ui-tab-panel" id={`${idBase}-panel-${value}`} role="tabpanel" aria-labelledby={`${idBase}-tab-${value}`} tabIndex={0}>{children}</div>;
}
