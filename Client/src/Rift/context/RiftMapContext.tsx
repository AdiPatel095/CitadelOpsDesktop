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
import { useCastleFocus } from '../../context/CastleFocusContext';
import { useLastKnownSnapshot } from '../../context/LastKnownSnapshotContext';
import {
  parseRiftMapCoordsPayload,
  riftMapCoordsFromSnapshot,
  type RiftMapCoords,
} from '../types/RiftMapCoords';
import { parseRiftCRALaunchPayload, type RiftCRALaunchState } from '../types/RiftCRALaunch';

export interface ReplayRiftCRALaunchOptions {
  launchId: string;
  commanderID?: number;
  sourceX?: number;
  sourceY?: number;
  /** Local wall-clock arrival at the Rift (unix seconds). Omit or 0 for immediate resend. */
  arriveAtUnix?: number;
}

export interface RiftMapContextValue {
  riftMapCoords: RiftMapCoords | null;
  riftCRALaunch: RiftCRALaunchState | null;
  refreshRiftMapCoords: (refresh?: boolean) => void;
  refreshRiftCRALaunch: () => void;
  replayRiftCRALaunch: (options?: ReplayRiftCRALaunchOptions) => void;
  renameRiftCRALaunch: (launchId: string, displayName: string) => void;
  deleteRiftCRALaunch: (launchId: string) => void;
}

const RiftMapContext = createContext<RiftMapContextValue | undefined>(undefined);

export function RiftMapProvider({ children }: { children: ReactNode }) {
  const { gameLoggedIn } = useAuth();
  const { castleFocus } = useCastleFocus();
  const { snapshot } = useLastKnownSnapshot();
  const [liveCoords, setLiveCoords] = useState<RiftMapCoords | null>(null);
  const [riftCRALaunch, setRiftCRALaunch] = useState<RiftCRALaunchState | null>(null);

  useEffect(() => {
    const handleMessage = (message: { type?: string; payload?: unknown }) => {
      if (message.type === 'riftMapCoords' && message.payload != null) {
        const parsed = parseRiftMapCoordsPayload(message.payload);
        if (parsed) setLiveCoords(parsed);
      }
      if (message.type === 'riftCRALaunch' && message.payload != null) {
        const parsed = parseRiftCRALaunchPayload(message.payload);
        if (parsed) setRiftCRALaunch(parsed);
      }
    };
    FrontendWebsocket.addMessageListener(handleMessage);
    return () => FrontendWebsocket.removeMessageListener(handleMessage);
  }, []);

  const refreshRiftMapCoords = useCallback((refresh = true) => {
    void refresh;
  }, []);

  const refreshRiftCRALaunch = useCallback(() => {
    FrontendWebsocket.sendGetRiftCRALaunch();
  }, []);

  const replayRiftCRALaunch = useCallback((options: ReplayRiftCRALaunchOptions) => {
    if (!options.launchId) return;
    FrontendWebsocket.sendReplayRiftCRALaunch(options);
  }, []);

  const renameRiftCRALaunch = useCallback((launchId: string, displayName: string) => {
    if (!launchId) return;
    setRiftCRALaunch((prev) => {
      if (!prev) return prev;
      return {
        ...prev,
        launches: prev.launches.map((entry) =>
          entry.id === launchId ? { ...entry, displayName: displayName.trim() } : entry
        ),
      };
    });
    FrontendWebsocket.sendRenameRiftCRALaunch(launchId, displayName);
  }, []);

  const deleteRiftCRALaunch = useCallback((launchId: string) => {
    if (!launchId) return;
    setRiftCRALaunch((prev) => {
      if (!prev) return prev;
      const launches = prev.launches.filter((entry) => entry.id !== launchId);
      return {
        ...prev,
        launches,
        launchCount: launches.length,
      };
    });
    FrontendWebsocket.sendDeleteRiftCRALaunch(launchId);
  }, []);

  const riftMapCoords = useMemo((): RiftMapCoords | null => {
    if (gameLoggedIn && liveCoords) return liveCoords;
    const aid = castleFocus?.aid ?? 0;
    const kid = castleFocus?.kingdomID ?? 0;
    let cx = castleFocus?.mapPX ?? 0;
    let cy = castleFocus?.mapPY ?? 0;
    if ((cx === 0 && cy === 0) && castleFocus?.playerCastles?.length) {
      const match = castleFocus.playerCastles.find((c) => c.aid === aid && c.kingdomID === kid);
      if (match) {
        cx = match.mapX;
        cy = match.mapY;
      }
    }
    if (!gameLoggedIn && snapshot) {
      return riftMapCoordsFromSnapshot(snapshot, aid, kid, cx, cy);
    }
    return liveCoords;
  }, [gameLoggedIn, liveCoords, snapshot, castleFocus]);

  const value = useMemo<RiftMapContextValue>(
    () => ({
      riftMapCoords,
      riftCRALaunch,
      refreshRiftMapCoords,
      refreshRiftCRALaunch,
      replayRiftCRALaunch,
      renameRiftCRALaunch,
      deleteRiftCRALaunch,
    }),
    [
      riftMapCoords,
      riftCRALaunch,
      refreshRiftMapCoords,
      refreshRiftCRALaunch,
      replayRiftCRALaunch,
      renameRiftCRALaunch,
      deleteRiftCRALaunch,
    ]
  );

  return <RiftMapContext.Provider value={value}>{children}</RiftMapContext.Provider>;
}

export function useRiftMap(): RiftMapContextValue {
  const ctx = useContext(RiftMapContext);
  if (ctx === undefined) {
    throw new Error('useRiftMap must be used within RiftMapProvider');
  }
  return ctx;
}
