import React, { createContext, useCallback, useContext, useEffect, useMemo, useState } from 'react';
import { TOOL_METADATA, TROOP_METADATA } from '../config/Constants';

export interface MetadataItem {
  id: number;
  name: string;
  image?: string;
  level?: number;
  [key: string]: unknown;
}

interface MetadataContextValue {
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
  decorations: Record<number, MetadataItem>;
  isLoading: boolean;
  getTroop: (id: number) => MetadataItem | undefined;
  getTool: (id: number) => MetadataItem | undefined;
  getDecoration: (id: number) => MetadataItem | undefined;
}

const MetadataContext = createContext<MetadataContextValue | undefined>(undefined);

const metadataSources = {
  troops: ['/game-data/units/index.json', '/game-data/troops/index.json'],
  tools: ['/game-data/tools/index.json'],
  decorations: ['/game-data/decorations/index.json'],
};

export const MetadataProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [troops, setTroops] = useState<Record<number, MetadataItem>>(() => fallbackTroops());
  const [tools, setTools] = useState<Record<number, MetadataItem>>(() => fallbackTools());
  const [decorations, setDecorations] = useState<Record<number, MetadataItem>>({});
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      setIsLoading(true);
      const [loadedTroops, loadedTools, loadedDecorations] = await Promise.all([
        fetchFirstIndex(metadataSources.troops),
        fetchFirstIndex(metadataSources.tools),
        fetchFirstIndex(metadataSources.decorations),
      ]);

      if (cancelled) {
        return;
      }

      if (Object.keys(loadedTroops).length > 0) {
        setTroops((current) => ({ ...current, ...loadedTroops }));
      }
      if (Object.keys(loadedTools).length > 0) {
        setTools((current) => ({ ...current, ...loadedTools }));
      }
      if (Object.keys(loadedDecorations).length > 0) {
        setDecorations(loadedDecorations);
      }
      setIsLoading(false);
    };

    void load();

    return () => {
      cancelled = true;
    };
  }, []);

  const getTroop = useCallback((id: number) => troops[id], [troops]);
  const getTool = useCallback((id: number) => tools[id], [tools]);
  const getDecoration = useCallback((id: number) => decorations[id], [decorations]);

  const value = useMemo<MetadataContextValue>(
    () => ({
      troops,
      tools,
      decorations,
      isLoading,
      getTroop,
      getTool,
      getDecoration,
    }),
    [decorations, getDecoration, getTool, getTroop, isLoading, tools, troops]
  );

  return <MetadataContext.Provider value={value}>{children}</MetadataContext.Provider>;
};

export function useMetadata(): MetadataContextValue {
  const context = useContext(MetadataContext);
  if (!context) {
    throw new Error('useMetadata must be used within a MetadataProvider');
  }
  return context;
}

async function fetchFirstIndex(urls: string[]): Promise<Record<number, MetadataItem>> {
  for (const url of urls) {
    try {
      const response = await fetch(url, { cache: 'no-cache' });
      if (!response.ok) {
        continue;
      }

      const payload = await response.json();
      const parsed = parseMetadataIndex(payload);
      if (Object.keys(parsed).length > 0) {
        return parsed;
      }
    } catch {
      continue;
    }
  }

  return {};
}

function parseMetadataIndex(value: unknown): Record<number, MetadataItem> {
  const rows = Array.isArray(value)
    ? value
    : isRecord(value) && Array.isArray(value.items)
      ? value.items
      : isRecord(value)
        ? Object.entries(value).map(([id, item]) => ({ id, ...(isRecord(item) ? item : { name: String(item) }) }))
        : [];

  const out: Record<number, MetadataItem> = {};
  for (const row of rows) {
    if (!isRecord(row)) {
      continue;
    }

    const id = toID(row.id ?? row.wodID ?? row.wid ?? row.ID);
    if (id <= 0) {
      continue;
    }

    const name = stringValue(row.name ?? row.Name ?? row.title ?? row.label);
    out[id] = {
      ...row,
      id,
      name: name || `Item ${id}`,
    };
  }

  return out;
}

function fallbackTroops(): Record<number, MetadataItem> {
  const out: Record<number, MetadataItem> = {};
  for (const [id, meta] of Object.entries(TROOP_METADATA)) {
    const numericID = Number(id);
    out[numericID] = {
      id: numericID,
      ...meta,
      image: `/game-data/troops/images/${numericID}.webp`,
    };
  }
  return out;
}

function fallbackTools(): Record<number, MetadataItem> {
  const out: Record<number, MetadataItem> = {};
  for (const [id, meta] of Object.entries(TOOL_METADATA)) {
    const numericID = Number(id);
    out[numericID] = {
      id: numericID,
      ...meta,
      image: `/game-data/tools/images/${numericID}.webp`,
    };
  }
  return out;
}

function toID(value: unknown): number {
  const parsed = typeof value === 'number' ? value : Number(value);
  return Number.isFinite(parsed) ? Math.trunc(parsed) : 0;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
