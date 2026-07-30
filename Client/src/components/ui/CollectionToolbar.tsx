import React, { type ReactNode } from 'react';
import { Search } from 'lucide-react';
import { Input } from './Input';

export interface CollectionToolbarProps {
  summary: ReactNode;
  searchValue: string;
  onSearchChange: (value: string) => void;
  searchPlaceholder: string;
  searchLabel?: string;
  className?: string;
}

export const CollectionToolbar: React.FC<CollectionToolbarProps> = ({
  summary,
  searchValue,
  onSearchChange,
  searchPlaceholder,
  searchLabel = searchPlaceholder,
  className = '',
}) => (
  <section className={`flex flex-wrap items-center justify-between gap-3 rounded-global border border-border-base bg-bg-card/55 p-3 shadow-[var(--glass-shadow-compact)] backdrop-blur-2xl ${className}`}>
    <div className="flex flex-wrap items-center gap-2">{summary}</div>
    <div className="w-full sm:w-80">
      <Input
        value={searchValue}
        onChange={(event) => onSearchChange(event.target.value)}
        placeholder={searchPlaceholder}
        aria-label={searchLabel}
        leftIcon={<Search className="h-4 w-4" />}
      />
    </div>
  </section>
);
