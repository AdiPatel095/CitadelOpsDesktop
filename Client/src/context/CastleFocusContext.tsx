import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { FrontendWebsocket } from '../websocket';
import { parseCastleFocusPayload, type CastleFocusState } from '../types/castleFocusState.ts';
import { useAuth } from './AuthContext';

export type PlayerCastleFocusParams = {
  castleId: number;
  kingdomId: number;
  mapX: number;
  mapY: number;
};

export interface CastleFocusContextValue {
  /** Latest server mirror of GameState castle focus (JAA + GCL directory). */
  castleFocus: CastleFocusState | null;
  /** Ask server to send the current focus snapshot (same as `getCastleFocus`). */
  refreshCastleFocus: () => void;
  /** Ask server to move in-game focus to this castle (JAA/JCA + troop refresh). */
  requestPlayerCastleFocus: (params: PlayerCastleFocusParams) => void;
}

const CastleFocusContext = createContext<CastleFocusContextValue | undefined>(undefined);

/**
 * Owns websocket `castleFocus` mirror + explicit requests to read or change server focus.
 * Must render inside {@link AuthProvider} (uses game login state).
 */
export function CastleFocusProvider({ children }: { children: ReactNode }) {
  const { gameLoggedIn } = useAuth();
  const [castleFocus, setCastleFocus] = useState<CastleFocusState | null>(null);

  useEffect(() => {
    const handleMessage = (message: { type?: string; payload?: unknown }) => {
      if (message.type !== 'castleFocus' || message.payload == null) return;
      setCastleFocus((prev) => {
        const cf = parseCastleFocusPayload(message.payload);
        if (!cf) return prev;
        let next = cf;
        if ((!cf.playerCastles || cf.playerCastles.length === 0) && prev?.playerCastles?.length) {
          next = { ...next, playerCastles: prev.playerCastles };
        }
        // JAA-only pushes may omit `slotProductionByLid`; keep prior strips for the same focused castle.
        if (
          prev &&
          cf.aid === prev.aid &&
          (!cf.slotProductionByLid || Object.keys(cf.slotProductionByLid).length === 0) &&
          prev.slotProductionByLid &&
          Object.keys(prev.slotProductionByLid).length > 0
        ) {
          next = { ...next, slotProductionByLid: prev.slotProductionByLid };
        }
        if (
          prev &&
          cf.aid === prev.aid &&
          (!cf.craftingQueues || cf.craftingQueues.length === 0) &&
          prev.craftingQueues &&
          prev.craftingQueues.length > 0
        ) {
          next = { ...next, craftingQueues: prev.craftingQueues };
        }
        return next;
      });
    };

    FrontendWebsocket.addMessageListener(handleMessage);
    return () => FrontendWebsocket.removeMessageListener(handleMessage);
  }, []);

  useEffect(() => {
    if (!gameLoggedIn) {
      setCastleFocus(null);
      return;
    }
    FrontendWebsocket.sendGetCastleFocus();
  }, [gameLoggedIn]);

  const refreshCastleFocus = useCallback(() => {
    FrontendWebsocket.sendGetCastleFocus();
  }, []);

  const requestPlayerCastleFocus = useCallback((params: PlayerCastleFocusParams) => {
    FrontendWebsocket.sendFocusPlayerCastle({
      castleId: params.castleId,
      kingdomId: params.kingdomId,
      mapX: params.mapX,
      mapY: params.mapY,
    });
  }, []);

  const value = useMemo<CastleFocusContextValue>(
    () => ({
      castleFocus,
      refreshCastleFocus,
      requestPlayerCastleFocus,
    }),
    [castleFocus, refreshCastleFocus, requestPlayerCastleFocus]
  );

  return <CastleFocusContext.Provider value={value}>{children}</CastleFocusContext.Provider>;
}

export function useCastleFocus(): CastleFocusContextValue {
  const ctx = useContext(CastleFocusContext);
  if (ctx === undefined) {
    throw new Error('useCastleFocus must be used within a CastleFocusProvider');
  }
  return ctx;
}
