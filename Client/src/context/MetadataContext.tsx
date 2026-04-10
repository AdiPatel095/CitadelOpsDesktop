import React, { createContext, useContext, useEffect, useState, ReactNode } from 'react';

export interface MetadataItem {
  id: number;
  name: string;
  type: string;
  image: string | null;
}

export interface MetadataState {
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
  decorations: Record<number, MetadataItem>;
  loading: boolean;
  error: string | null;
}

interface MetadataContextValue extends MetadataState {
  getTroop: (id: number) => MetadataItem | undefined;
  getTool: (id: number) => MetadataItem | undefined;
  getDecoration: (id: number) => MetadataItem | undefined;
  getTroopImageUrl: (id: number) => string | undefined;
  getToolImageUrl: (id: number) => string | undefined;
  getDecorationImageUrl: (id: number) => string | undefined;
}

const MetadataContext = createContext<MetadataContextValue | undefined>(undefined);
const FRONTEND_METADATA_BASE = '/game-data';

export function MetadataProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<MetadataState>({
    troops: {},
    tools: {},
    decorations: {},
    loading: true,
    error: null,
  });

  useEffect(() => {
    let mounted = true;

    const fetchMetadata = async () => {
      try {
        const [troopsRes, toolsRes, decosRes] = await Promise.all([
          fetch(`${FRONTEND_METADATA_BASE}/troops/index.json`),
          fetch(`${FRONTEND_METADATA_BASE}/tools/index.json`),
          fetch(`${FRONTEND_METADATA_BASE}/decorations/index.json`)
        ]);

        if (!troopsRes.ok || !toolsRes.ok || !decosRes.ok) {
          throw new Error('Failed to fetch metadata');
        }

        const troopsArr: MetadataItem[] = await troopsRes.json();
        const toolsArr: MetadataItem[] = await toolsRes.json();
        const decosArr: MetadataItem[] = await decosRes.json();

        if (mounted) {
          const toMap = (arr: MetadataItem[]) => arr.reduce((acc, item) => {
            acc[item.id] = item;
            return acc;
          }, {} as Record<number, MetadataItem>);

          setState({
            troops: toMap(troopsArr),
            tools: toMap(toolsArr),
            decorations: toMap(decosArr),
            loading: false,
            error: null,
          });
        }
      } catch (error) {
        if (mounted) {
          setState(prev => ({
            ...prev,
            loading: false,
            error: error instanceof Error ? error.message : 'Unknown error'
          }));
          console.error('Error fetching metadata:', error);
        }
      }
    };

    fetchMetadata();
    return () => { mounted = false; };
  }, []);

  const getTroop = (id: number) => state.troops[id];
  const getTool = (id: number) => state.tools[id];
  const getDecoration = (id: number) => state.decorations[id];

  const getTroopImageUrl = (id: number) => {
    return `${FRONTEND_METADATA_BASE}/troops/images/${id}.png`;
  };

  const getToolImageUrl = (id: number) => {
    return `${FRONTEND_METADATA_BASE}/tools/images/${id}.png`;
  };

  const getDecorationImageUrl = (id: number) => {
    return `${FRONTEND_METADATA_BASE}/decorations/images/${id}.png`;
  };

  return (
    <MetadataContext.Provider value={{
      ...state,
      getTroop,
      getTool,
      getDecoration,
      getTroopImageUrl,
      getToolImageUrl,
      getDecorationImageUrl
    }}>
      {children}
    </MetadataContext.Provider>
  );
}

export function useMetadata() {
  const context = useContext(MetadataContext);
  if (context === undefined) {
    throw new Error('useMetadata must be used within a MetadataProvider');
  }
  return context;
}
