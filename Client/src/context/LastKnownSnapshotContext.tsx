import {
  createContext,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { FrontendWebsocket } from '../Websocket';

export interface LastKnownSnapshotContextValue {
  /** Latest `lastKnownGameStateSnapshot` message from the server (on-disk JSON). */
  snapshot: Record<string, unknown> | null;
  savedAtUnix: number | null;
}

const LastKnownSnapshotContext = createContext<LastKnownSnapshotContextValue | undefined>(undefined);

export function LastKnownSnapshotProvider({ children }: { children: ReactNode }) {
  const [snapshot, setSnapshot] = useState<Record<string, unknown> | null>(null);

  useEffect(() => {
    const handleMessage = (message: { type?: string; payload?: unknown }) => {
      if (message.type !== 'lastKnownGameStateSnapshot' || message.payload == null) return;
      if (typeof message.payload !== 'object') return;
      setSnapshot(message.payload as Record<string, unknown>);
    };

    FrontendWebsocket.addMessageListener(handleMessage);
    return () => FrontendWebsocket.removeMessageListener(handleMessage);
  }, []);

  const savedAtUnix = useMemo(() => {
    const v = snapshot?.savedAtUnix;
    return typeof v === 'number' && Number.isFinite(v) ? v : null;
  }, [snapshot]);

  const value = useMemo<LastKnownSnapshotContextValue>(
    () => ({ snapshot, savedAtUnix }),
    [snapshot, savedAtUnix]
  );

  return (
    <LastKnownSnapshotContext.Provider value={value}>{children}</LastKnownSnapshotContext.Provider>
  );
}

export function useLastKnownSnapshot(): LastKnownSnapshotContextValue {
  const ctx = useContext(LastKnownSnapshotContext);
  if (ctx === undefined) {
    throw new Error('useLastKnownSnapshot must be used within LastKnownSnapshotProvider');
  }
  return ctx;
}
