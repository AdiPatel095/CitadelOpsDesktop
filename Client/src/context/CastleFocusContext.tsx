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
import {
  parseCastleFocusPayload,
  type CastleFocusState,
  type PlayerCastleOption,
} from '../types/castleFocusState.ts';
import {
  buildCastleFocusFromSnapshot,
  buildCastleFocusFromStoredSnapshotFocus,
  playerCastleOptionsFromGameStateSnapshot,
} from '../utils/castleSnapshotHydration.ts';
import { useAuth } from './AuthContext';
import { useLastKnownSnapshot } from './LastKnownSnapshotContext';

export type PlayerCastleFocusParams = {
  castleId: number;
  kingdomId: number;
  mapX: number;
  mapY: number;
};

function optionKey(c: Pick<PlayerCastleOption, 'aid' | 'kingdomID'>): string {
  return `${c.aid}|${c.kingdomID}`;
}

function aidFromOptionKey(key: string | null): number {
  if (!key) return 0;
  const n = Number(key.split('|')[0]);
  return Number.isFinite(n) ? n : 0;
}

export interface CastleFocusContextValue {
  /** Effective focus for UI: live while connected; offline uses snapshot + optional user switch. */
  castleFocus: CastleFocusState | null;
  refreshCastleFocus: () => void;
  requestPlayerCastleFocus: (params: PlayerCastleFocusParams) => void;
  /** When disconnected, select which castle to show (key `aid|kingdomID`). Cleared on reconnect. */
  setOfflineCastleFocusKey: (key: string | null) => void;
  offlineCastleFocusKey: string | null;
}

const CastleFocusContext = createContext<CastleFocusContextValue | undefined>(undefined);

/**
 * Owns websocket `castleFocus` mirror + explicit requests to read or change server focus.
 * Must render inside {@link AuthProvider} and {@link LastKnownSnapshotProvider}.
 */
export function CastleFocusProvider({ children }: { children: ReactNode }) {
  const { gameLoggedIn } = useAuth();
  const { snapshot } = useLastKnownSnapshot();
  const [liveCastleFocus, setLiveCastleFocus] = useState<CastleFocusState | null>(null);
  const [offlineFocusKey, setOfflineFocusKey] = useState<string | null>(null);

  useEffect(() => {
    if (gameLoggedIn) {
      setOfflineFocusKey(null);
    }
  }, [gameLoggedIn]);

  useEffect(() => {
    const handleMessage = (message: { type?: string; payload?: unknown }) => {
      if (message.type !== 'castleFocus' || message.payload == null) return;
      setLiveCastleFocus((prev) => {
        const cf = parseCastleFocusPayload(message.payload);
        if (!cf) return prev;
        let next = cf;
        if ((!cf.playerCastles || cf.playerCastles.length === 0) && prev?.playerCastles?.length) {
          next = { ...next, playerCastles: prev.playerCastles };
        }
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
    if (gameLoggedIn) return;
    const hasAid = liveCastleFocus?.aid != null && liveCastleFocus.aid > 0;
    if (hasAid) return;
    const syn = buildCastleFocusFromStoredSnapshotFocus(snapshot);
    if (syn) {
      setLiveCastleFocus(syn);
    }
  }, [gameLoggedIn, liveCastleFocus, snapshot]);

  useEffect(() => {
    if (!gameLoggedIn) return;
    FrontendWebsocket.sendGetCastleFocus();
  }, [gameLoggedIn]);

  const castleFocus = useMemo((): CastleFocusState | null => {
    if (gameLoggedIn) {
      return liveCastleFocus;
    }

    const snap = snapshot;
    const optsFromSnap = snap ? playerCastleOptionsFromGameStateSnapshot(snap.gameState) : [];
    const optsFromLive = liveCastleFocus?.playerCastles ?? [];
    // Offline: prefer GCL from persisted snapshot so every castle has a stable aid|kingdomID row for the switcher
    // and buildCastleFocusFromSnapshot lookups; live mirror may only list the last in-game focus.
    const opts =
      optsFromSnap.length > 0 ? optsFromSnap : optsFromLive.length > 0 ? optsFromLive : [];

    const base =
      liveCastleFocus ?? (snap ? buildCastleFocusFromStoredSnapshotFocus(snap) : null);

    if (!offlineFocusKey) {
      if (base && (!base.playerCastles || base.playerCastles.length === 0) && opts.length > 0) {
        return { ...base, playerCastles: opts };
      }
      return base;
    }

    const aid = aidFromOptionKey(offlineFocusKey);
    const opt = opts.find((o) => optionKey(o) === offlineFocusKey);
    const kid = opt?.kingdomID ?? 0;

    if (base && Math.trunc(Number(base.aid)) === aid) {
      return {
        ...base,
        playerCastles: opts.length > 0 ? opts : base.playerCastles,
      };
    }

    const synthetic = snap ? buildCastleFocusFromSnapshot(snap, aid, kid) : null;
    if (synthetic) {
      return {
        ...synthetic,
        playerCastles: opts.length > 0 ? opts : synthetic.playerCastles,
      };
    }
    // Snapshot missing gameState/castle rows but GCL still has the pick — at least align aid/kingdom/name.
    if (opt && aid > 0) {
      const minimal = parseCastleFocusPayload({
        aid,
        kingdomID: kid,
        castleName: opt.name,
        playerCastles: opts.length > 0 ? opts : undefined,
      });
      if (minimal) return minimal;
    }
    if (base) {
      return { ...base, playerCastles: opts.length > 0 ? opts : base.playerCastles };
    }
    return null;
  }, [gameLoggedIn, liveCastleFocus, offlineFocusKey, snapshot]);

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

  const setOfflineCastleFocusKey = useCallback((key: string | null) => {
    setOfflineFocusKey(key);
  }, []);

  const value = useMemo<CastleFocusContextValue>(
    () => ({
      castleFocus,
      refreshCastleFocus,
      requestPlayerCastleFocus,
      setOfflineCastleFocusKey,
      offlineCastleFocusKey: offlineFocusKey,
    }),
    [castleFocus, refreshCastleFocus, requestPlayerCastleFocus, setOfflineCastleFocusKey, offlineFocusKey]
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
