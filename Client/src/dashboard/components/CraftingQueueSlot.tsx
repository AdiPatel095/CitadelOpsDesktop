import React, { useEffect, useState } from 'react';

export interface CraftingQueueRow {
  recipeId: number;
  label: string;
  imageUrl?: string;
  amount: number;
  active: boolean;
}

function formatQueueCount(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '';
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 10_000) return `${(n / 1000).toFixed(1)}k`;
  return n.toLocaleString();
}

export interface CraftingQueueSlotProps {
  row: CraftingQueueRow;
  /** Box size in px (approximate square). */
  boxSize?: number;
}

/**
 * Sovereign crafting slot: recipe label, icon, and total output quantity for the queued job.
 */
const CraftingQueueSlot: React.FC<CraftingQueueSlotProps> = ({ row, boxSize = 58 }) => {
  const [imageFailed, setImageFailed] = useState(false);
  const imageUrl = row.imageUrl?.trim() ?? '';
  const showImage = imageUrl !== '' && !imageFailed;
  const title = `${row.active ? 'Active' : 'Queued'}: ${row.label}${
    row.amount > 0 ? ` — ×${formatQueueCount(row.amount)}` : ''
  }`;
  const shortLabel = row.label.length > 22 ? `${row.label.slice(0, 20)}…` : row.label;

  useEffect(() => {
    setImageFailed(false);
  }, [imageUrl]);

  return (
    <div
      className={`relative flex shrink-0 flex-col items-center justify-center gap-0.5 rounded-global border px-1 py-0.5 text-center leading-tight ${
        row.active
          ? 'border-primary ring-2 ring-primary/35 shadow-sm bg-bg-card'
          : 'border-border-light border-solid bg-bg-card'
      }`}
      style={{ width: boxSize + 14, minHeight: boxSize + 10 }}
      title={title}
    >
      {showImage ? (
        <img
          src={imageUrl}
          alt={row.label}
          width={boxSize}
          height={boxSize}
          loading="lazy"
          decoding="async"
          className="object-contain"
          draggable={false}
          onError={() => setImageFailed(true)}
        />
      ) : (
        <span className="text-[9px] font-medium text-text-muted line-clamp-3">{shortLabel}</span>
      )}
      {row.amount > 0 ? (
        <span
          className="absolute -right-1 -top-1 z-20 flex min-h-[1.125rem] min-w-[1.125rem] max-w-[3.25rem] items-center justify-center rounded-full bg-amber-600 px-1 text-[10px] font-bold leading-none text-white shadow-md ring-2 ring-bg-card"
          aria-label={`${formatQueueCount(row.amount)} in slot`}
        >
          {formatQueueCount(row.amount)}
        </span>
      ) : null}
    </div>
  );
};

export default CraftingQueueSlot;
