import { createContext, useContext, useMemo, type ReactNode } from 'react';
import { useCitadelAPI } from '../api/ApiContext';

export interface LastKnownSnapshotContextValue {
  snapshot: Record<string, unknown> | null;
  savedAtUnix: number | null;
}

const LastKnownSnapshotContext = createContext<LastKnownSnapshotContextValue | undefined>(undefined);

export function LastKnownSnapshotProvider({ children }: { children: ReactNode }) {
  const { state } = useCitadelAPI();
  const savedAtUnix = state ? Math.floor(Date.parse(state.updatedAt) / 1000) : null;
  const snapshot = useMemo<Record<string, unknown> | null>(() => state ? {
    schemaVersion: state.schemaVersion,
    savedAtUnix,
    gameState: state,
  } : null, [savedAtUnix, state]);
  const value = useMemo(() => ({ snapshot, savedAtUnix }), [savedAtUnix, snapshot]);
  return <LastKnownSnapshotContext.Provider value={value}>{children}</LastKnownSnapshotContext.Provider>;
}

export function useLastKnownSnapshot(): LastKnownSnapshotContextValue {
  const context = useContext(LastKnownSnapshotContext);
  if (!context) throw new Error('useLastKnownSnapshot must be used within LastKnownSnapshotProvider');
  return context;
}
