import { useEffect, useId, useRef, useState, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { ChevronDown, MoreHorizontal } from 'lucide-react';
import { Button, type ButtonSize } from './Button';
import './overflow-menu.css';

export interface OverflowMenuProps {
  label: string;
  items: Array<{ id: string; label: ReactNode; icon?: ReactNode; destructive?: boolean; disabled?: boolean; onSelect(): void }>;
  triggerIcon?: ReactNode; size?: ButtonSize; disabled?: boolean;
}
export function OverflowMenu({ label, items, triggerIcon, size = 'sm', disabled }: OverflowMenuProps) {
  const id = useId();
  const trigger = useRef<HTMLButtonElement>(null);
  const menu = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [closing, setClosing] = useState(false);
  const ordered = [...items.filter(item => !item.destructive), ...items.filter(item => item.destructive)];
  const destructive = items.length > 0 && items.every(item => item.destructive);
  const isDisabled = disabled || items.every(item => item.disabled);
  const close = (restore = false) => { setClosing(true); setOpen(false); if (restore) trigger.current?.focus(); };
  const focus = (index: number) => {
    const enabled = Array.from(menu.current?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not([aria-disabled="true"])') ?? []);
    enabled[(index + enabled.length) % enabled.length]?.focus();
  };
  useEffect(() => {
    if (!open) return;
    const reposition = () => {
      const rect = trigger.current!.getBoundingClientRect();
      const width = menu.current?.offsetWidth ?? 240;
      const height = menu.current?.offsetHeight ?? 0;
      menu.current!.style.setProperty('--menu-top', `${Math.max(8, Math.min(rect.bottom + 4, window.innerHeight - height - 8))}px`);
      menu.current!.style.setProperty('--menu-left', `${Math.max(8, Math.min(getComputedStyle(trigger.current!).direction === 'rtl' ? window.innerWidth - rect.right : rect.left, window.innerWidth - width - 8))}px`);
    };
    reposition();
    menu.current?.querySelector<HTMLButtonElement>('[role="menuitem"]:not([aria-disabled="true"])')?.focus();
    const outside = (event: PointerEvent) => {
      if (!menu.current?.contains(event.target as Node) && !trigger.current?.contains(event.target as Node)) { setClosing(true); setOpen(false); }
    };
    document.addEventListener('pointerdown', outside);
    window.addEventListener('resize', reposition);
    window.addEventListener('scroll', reposition, true);
    return () => { document.removeEventListener('pointerdown', outside); window.removeEventListener('resize', reposition); window.removeEventListener('scroll', reposition, true); };
  }, [open]);
  return <>
    <Button ref={trigger} size={size} variant={destructive ? 'danger' : 'ghost'} iconOnly={!destructive}
      aria-label={label} leftIcon={destructive ? triggerIcon ?? <MoreHorizontal /> : <MoreHorizontal />} rightIcon={destructive ? <ChevronDown /> : undefined}
      disabled={isDisabled} aria-haspopup="menu" aria-expanded={open} aria-controls={id} type="button"
      onClick={() => { if (open) close(); else { setClosing(false); setOpen(true); } }} onKeyDown={event => {
        if (['Enter', ' ', 'ArrowDown'].includes(event.key)) { event.preventDefault(); setClosing(false); setOpen(true); }
      }}>{destructive ? label : null}</Button>
    {(open || closing) && createPortal(<div className="cit-overflow-layer" data-state={open ? 'open' : 'closing'} aria-hidden={!open || undefined}>
      <div className="cit-overflow-scrim" aria-hidden="true" />
      <div ref={menu} onAnimationEnd={event => { if (event.target === event.currentTarget && !open) setClosing(false); }} id={id} role="menu" aria-label={label} className="cit-overflow-menu" onKeyDown={event => {
        const enabled = Array.from(menu.current!.querySelectorAll<HTMLButtonElement>('[role="menuitem"]:not([aria-disabled="true"])'));
        const index = enabled.indexOf(document.activeElement as HTMLButtonElement);
        if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(true); }
        else if (event.key === 'Tab') close(true);
        else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
          event.preventDefault(); focus(event.key === 'Home' ? 0 : event.key === 'End' ? enabled.length - 1 : index + (event.key === 'ArrowDown' ? 1 : -1));
        }
      }}>
        {ordered.map((item, index) => <div key={item.id}>
          {item.destructive && index > 0 && !ordered[index - 1].destructive && <div role="separator" className="cit-overflow-divider" />}
          <button type="button" role="menuitem" tabIndex={-1} aria-disabled={item.disabled || undefined} data-destructive={item.destructive || undefined}
            onClick={() => { if (!item.disabled) { close(true); item.onSelect(); } }}>
            {item.icon}<span>{item.label}</span>
          </button>
        </div>)}
      </div>
    </div>, document.body)}
  </>;
}
