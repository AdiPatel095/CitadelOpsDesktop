import React, { type ReactNode } from 'react';
import { Search } from 'lucide-react';
import { Input } from './Input';

export interface CollectionToolbarProps {
  summary: ReactNode;
  actions?: ReactNode;
  searchValue: string;
  onSearchChange: (value: string) => void;
  searchPlaceholder: string;
  searchLabel?: string;
  className?: string;
}

export const CollectionToolbar: React.FC<CollectionToolbarProps> = ({
  summary,
  actions,
  searchValue,
  onSearchChange,
  searchPlaceholder,
  searchLabel = searchPlaceholder,
  className = '',
}) => {
  const search = (
    <Input
      value={searchValue}
      onChange={(event) => onSearchChange(event.target.value)}
      placeholder={searchPlaceholder}
      aria-label={searchLabel}
      leftIcon={<Search className="h-4 w-4" />}
    />
  );

  return (
    <section className={`flex flex-wrap items-center justify-between gap-3 rounded-global border border-border-base bg-bg-card p-3 shadow-[var(--shadow-raised)] ${className}`}>
      <div className="flex flex-wrap items-center gap-2">{summary}</div>
      <div className={`flex w-full flex-wrap items-center justify-end gap-2 ${actions ? 'md:min-w-[30rem] md:flex-1' : 'sm:w-80'}`}>
        <div className={`w-full ${actions ? 'min-w-56 flex-1 lg:max-w-80' : ''}`}>{search}</div>
        {actions ? <div className="flex flex-wrap items-center justify-end gap-2">{actions}</div> : null}
      </div>
    </section>
  );
};
