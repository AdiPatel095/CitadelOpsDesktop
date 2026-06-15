import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { FrontendWebsocket } from '../../Websocket';
import { useAuth } from '../../context/AuthContext';
import { useLastKnownSnapshot } from '../../context/LastKnownSnapshotContext';
import {
  movementFromSnapshot,
  parseMovementUpdatePayload,
  type MovementState,
} from '../types/MovementState';

export interface MovementContextValue {
  movement: MovementState | null;
  refreshMovement: (refresh?: boolean) => void;
}

const MovementContext = createContext<MovementContextValue | undefined>(undefined);

export function MovementProvider({ children }: { children: ReactNode }) {
  const { gameLoggedIn } = useAuth();
  const { snapshot } = useLastKnownSnapshot();
  const [liveMovement, setLiveMovement] = useState<MovementState | null>(null);

  useEffect(() => {
    const handleMessage = (message: { type?: string; payload?: unknown }) => {
      if (message.type !== 'movementUpdate' || message.payload == null) return;
      const parsed = parseMovementUpdatePayload(message.payload);
      if (parsed) setLiveMovement(parsed);
    };
    FrontendWebsocket.addMessageListener(handleMessage);
    return () => FrontendWebsocket.removeMessageListener(handleMessage);
  }, []);

  const refreshMovement = useCallback((refresh = true) => {
    FrontendWebsocket.sendGetMovement(refresh);
  }, []);

  useEffect(() => {
    if (!gameLoggedIn) return;
    refreshMovement(true);
  }, [gameLoggedIn, refreshMovement]);

  const movement = useMemo((): MovementState | null => {
    if (gameLoggedIn) return liveMovement ?? { activeMovements: [] };
    return movementFromSnapshot(snapshot);
  }, [gameLoggedIn, liveMovement, snapshot]);

  const value = useMemo<MovementContextValue>(
    () => ({ movement, refreshMovement }),
    [movement, refreshMovement]
  );

  return <MovementContext.Provider value={value}>{children}</MovementContext.Provider>;
}

export function useMovement(): MovementContextValue {
  const ctx = useContext(MovementContext);
  if (ctx === undefined) {
    throw new Error('useMovement must be used within MovementProvider');
  }
  return ctx;
}
