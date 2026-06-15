import React from 'react';
import type { CraftingStripRow } from '../../types/CastleFocusState.ts';

function formatQueueCount(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '';
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 10_000) return `${(n / 1000).toFixed(1)}k`;
  return n.toLocaleString();
}

export interface CraftingQueueSlotProps {
  row: CraftingStripRow;
  /** Box size in px (approximate square). */
  boxSize?: number;
}

/**
 * Sovereign crafting slot: recipe label from server (CRID → craftingRecipes.json) + optional **bv** quantity.
 */
const CraftingQueueSlot: React.FC<CraftingQueueSlotProps> = ({ row, boxSize = 58 }) => {
  const title = `${row.kind === 'active' ? 'Active' : 'Queued'}: ${row.label}${
    row.qty > 0 ? ` — ×${formatQueueCount(row.qty)}` : ''
  }`;
  const shortLabel = row.label.length > 22 ? `${row.label.slice(0, 20)}…` : row.label;

  return (
    <div
      className={`relative flex shrink-0 flex-col items-center justify-center gap-0.5 rounded-global border px-1 py-0.5 text-center leading-tight ${
        row.kind === 'active'
          ? 'border-primary ring-2 ring-primary/35 shadow-sm bg-bg-card'
          : 'border-border-light border-solid bg-bg-card'
      }`}
      style={{ width: boxSize + 14, minHeight: boxSize + 10 }}
      title={title}
    >
      <span className="text-[9px] font-medium text-text-muted line-clamp-3">{shortLabel}</span>
      {row.qty > 0 ? (
        <span
          className="absolute -right-1 -top-1 z-20 flex min-h-[1.125rem] min-w-[1.125rem] max-w-[3.25rem] items-center justify-center rounded-full bg-amber-600 px-1 text-[10px] font-bold leading-none text-white shadow-md ring-2 ring-bg-card"
          aria-label={`${formatQueueCount(row.qty)} in slot`}
        >
          {formatQueueCount(row.qty)}
        </span>
      ) : null}
    </div>
  );
};

export default CraftingQueueSlot;
