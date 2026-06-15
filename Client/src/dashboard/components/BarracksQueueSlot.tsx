import React from 'react';
import UnitImage from '../../components/UnitImage';
import ToolImage from '../../components/ToolImage';
import { useMetadata } from '../../context/MetadataContext';
import type { BarracksQueueRow } from '../../types/CastleFocusState.ts';

function formatQueueCount(n: number): string {
  if (!Number.isFinite(n) || n <= 0) return '0';
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 10_000) return `${(n / 1000).toFixed(1)}k`;
  return n.toLocaleString();
}

export interface BarracksQueueSlotProps {
  row: BarracksQueueRow;
  /** Icon size in px (container is slightly larger for the count badge). */
  imageSize?: number;
  isTool?: boolean;
}

/**
 * Single recruitment slot: troop/tool PNG + alert-style count for TUA.
 */
const BarracksQueueSlot: React.FC<BarracksQueueSlotProps> = ({ row, imageSize = 40, isTool = false }) => {
  const { getTroop, getTool } = useMetadata();
  const metadata = isTool ? getTool(row.wid) : getTroop(row.wid);
  const name = metadata?.name ?? `Item ${row.wid}`;

  const batch =
    row.pid != null && row.pid > 0
      ? ` · batch ${row.pid}`
      : row.spid != null && row.spid > 0
        ? ` · run ${row.spid}`
        : '';
  const title = `${row.kind === 'active' ? 'Active' : 'Queued'}: ${name}${batch} — ×${formatQueueCount(row.tua)}`;

  return (
    <div
      className={`relative flex shrink-0 items-center justify-center rounded-global border bg-bg-card ${
        row.kind === 'active'
          ? 'border-primary ring-2 ring-primary/35 shadow-sm'
          : 'border-border-light border-solid'
      }`}
      style={{ width: imageSize + 18, height: imageSize + 18 }}
      title={title}
    >
      {isTool ? (
        <ToolImage toolId={row.wid} size={imageSize} className="!block" />
      ) : (
        <UnitImage unitId={row.wid} size={imageSize} showLevel={true} className="!block" />
      )}
      <span
        className="absolute -right-1 -top-1 z-20 flex min-h-[1.125rem] min-w-[1.125rem] max-w-[3.25rem] items-center justify-center rounded-full bg-red-600 px-1 text-[10px] font-bold leading-none text-white shadow-md ring-2 ring-bg-card"
        aria-label={`${formatQueueCount(row.tua)} in slot`}
      >
        {formatQueueCount(row.tua)}
      </span>
    </div>
  );
};

export default BarracksQueueSlot;
