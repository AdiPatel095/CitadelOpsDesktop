import React, { type ButtonHTMLAttributes, type ReactNode } from 'react';
import { Plus } from 'lucide-react';

export interface AddSlotProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  label: string;
  icon?: ReactNode;
  layout?: 'icon' | 'stacked' | 'inline';
}

export const AddSlot = React.forwardRef<HTMLButtonElement, AddSlotProps>(({
  label,
  icon = <Plus className="h-5 w-5" />,
  layout = 'stacked',
  className = '',
  type = 'button',
  ...props
}, ref) => {
  const layoutClass = {
    icon: 'items-center justify-center',
    stacked: 'flex-col items-center justify-center gap-2 text-xs font-bold uppercase tracking-wide',
    inline: 'items-center justify-center gap-2 text-sm font-semibold',
  }[layout];

  return (
    <button
      ref={ref}
      type={type}
      aria-label={props['aria-label'] ?? label}
      className={`flex rounded-global border-2 border-dashed border-border-base bg-bg-card/45 text-text-muted transition-colors hover:border-primary hover:bg-primary/5 hover:text-primary ${layoutClass} ${className}`}
      {...props}
    >
      {icon}
      {layout === 'icon' ? <span className="sr-only">{label}</span> : <span>{label}</span>}
    </button>
  );
});

AddSlot.displayName = 'AddSlot';
