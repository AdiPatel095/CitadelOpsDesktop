import React, { type ReactNode } from 'react';
import { X } from 'lucide-react';

export interface QuantityAssetTileProps {
  visual: ReactNode;
  quantity: number | string;
  onRemove: () => void;
  removeLabel: string;
  size?: number;
  className?: string;
}

export const QuantityAssetTile: React.FC<QuantityAssetTileProps> = ({
  visual,
  quantity,
  onRemove,
  removeLabel,
  size = 76,
  className = '',
}) => (
  <div className={`group relative flex flex-col items-center ${className}`} style={{ width: size + 8 }}>
    <button
      type="button"
      onClick={onRemove}
      className="absolute -right-1 -top-1 z-20 flex h-5 w-5 items-center justify-center rounded-full bg-error text-white opacity-0 shadow-md transition-opacity hover:brightness-110 focus-visible:opacity-100 group-hover:opacity-100 group-focus-within:opacity-100"
      aria-label={removeLabel}
    >
      <X className="h-3 w-3" />
    </button>
    <div className="relative shrink-0" style={{ width: size, height: size }}>
      {visual}
      <span className="absolute bottom-0 right-0 z-10 max-w-[calc(100%+8px)] translate-x-1/4 translate-y-1/4 truncate rounded-full bg-white px-2.5 py-0.5 text-center text-[10px] font-bold tabular-nums text-slate-900 shadow-md ring-1 ring-black/10">
        {typeof quantity === 'number' ? quantity.toLocaleString() : quantity}
      </span>
    </div>
  </div>
);
