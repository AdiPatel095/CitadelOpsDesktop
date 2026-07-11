import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type ReactNode,
} from 'react';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { useLastKnownSnapshot } from '../../context/LastKnownSnapshotContext';
import { riftMapCoordsFromSnapshot, type RiftMapCoords } from '../types/RiftMapCoords';
import { parseRiftCRALaunchPayload, type RiftCRALaunchState } from '../types/RiftCRALaunch';
import { useCitadelAPI } from '../../api/ApiContext';

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
  const { configuration, submitIntent, updateConfiguration } = useCitadelAPI();
  const { castleFocus } = useCastleFocus();
  const { snapshot } = useLastKnownSnapshot();
  const riftCRALaunch = useMemo(
    () => parseRiftCRALaunchPayload(configuration?.sections['rift.launches']),
    [configuration?.sections],
  );

  const refreshRiftMapCoords = useCallback((refresh = true) => {
    void refresh;
  }, []);

  const refreshRiftCRALaunch = useCallback(() => {
  }, []);

  const replayRiftCRALaunch = useCallback((options: ReplayRiftCRALaunchOptions) => {
    if (!options.launchId) return;
    void submitIntent('rift.launch.replay', options);
  }, [submitIntent]);

  const renameRiftCRALaunch = useCallback((launchId: string, displayName: string) => {
    if (!launchId) return;
    const launches = riftCRALaunch.launches.map((entry) =>
      entry.id === launchId ? { ...entry, displayName: displayName.trim() } : entry
    );
    void updateConfiguration('rift.launches', { ...riftCRALaunch, launches, launchCount: launches.length });
  }, [riftCRALaunch, updateConfiguration]);

  const deleteRiftCRALaunch = useCallback((launchId: string) => {
    if (!launchId) return;
    const launches = riftCRALaunch.launches.filter((entry) => entry.id !== launchId);
    void updateConfiguration('rift.launches', { ...riftCRALaunch, launches, launchCount: launches.length });
  }, [riftCRALaunch, updateConfiguration]);

  const riftMapCoords = useMemo((): RiftMapCoords | null => {
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
    if (snapshot) {
      return riftMapCoordsFromSnapshot(snapshot, aid, kid, cx, cy);
    }
    return null;
  }, [snapshot, castleFocus]);

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
