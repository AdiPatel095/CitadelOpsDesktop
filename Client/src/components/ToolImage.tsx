import React, { useEffect, useMemo, useState } from 'react';
import { Hammer } from 'lucide-react';
import { useMetadata } from '../context/MetadataContext';
import LevelBadge from './LevelBadge';

interface ToolImageProps {
  toolId: number;
  size?: number;
  showLevel?: boolean;
  className?: string;
}

const ToolImage: React.FC<ToolImageProps> = ({ toolId, size = 40, showLevel = true, className = '' }) => {
  const { getTool } = useMetadata();
  const [sourceIndex, setSourceIndex] = useState(0);
  const metadata = getTool(toolId);
  const level = metadataLevel(metadata?.level);
  const sources = useMemo(() => {
    const metadataSrc = typeof metadata?.image === 'string' ? metadata.image.trim() : '';
    const directMetadataSrc = metadataSrc.toLowerCase().endsWith('.png') ? '' : metadataSrc;
    return uniqueSources([
      webpVariant(metadataSrc),
      `/game-data/tools/images/${toolId}.webp`,
      directMetadataSrc,
    ]);
  }, [metadata?.image, toolId]);
  const src = sources[sourceIndex];
  const failed = !src;
  const title = metadata?.name ?? `Tool ${toolId}`;

  useEffect(() => {
    setSourceIndex(0);
  }, [sources]);

  return (
    <span
      className={`relative inline-flex shrink-0 items-center justify-center rounded-global bg-bg-app ${className}`}
      style={{ width: size, height: size }}
      title={title}
      aria-label={title}
    >
      {!failed ? (
        <img
          src={src}
          alt={title}
          width={size}
          height={size}
          className="h-full w-full object-contain"
          draggable={false}
          onError={() => setSourceIndex((current) => current + 1)}
        />
      ) : (
        <Hammer className="h-1/2 w-1/2 text-text-muted" />
      )}
      {showLevel && level && <LevelBadge level={level} imageSize={size} />}
    </span>
  );
};

function uniqueSources(values: string[]): string[] {
  const out: string[] = [];
  for (const value of values) {
    if (value && !out.includes(value)) {
      out.push(value);
    }
  }
  return out;
}

function webpVariant(value: string): string {
  if (!value || !value.toLowerCase().endsWith('.png')) {
    return '';
  }
  return `${value.slice(0, -4)}.webp`;
}

function metadataLevel(value: unknown): number | undefined {
  const parsed = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.trunc(parsed) : undefined;
}

export default ToolImage;
