import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  type ReactNode,
} from 'react';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { riftMapCoordsFromState, type RiftMapCoords } from '../types/RiftMapCoords';
import { parseRiftCRALaunchPayload, type RiftCRALaunchState } from '../types/RiftCRALaunch';
import { useCitadelAPI } from '../../api/ApiContext';
import type { CRACommanderSelection } from '../../api/Contracts';
import type { AttackSetupDraft } from '../../components/AttackSetupModal';
import type { HorseTravelBoostID } from '../../settings/HorseTravelBoost';

export type { CRACommanderSelection, CRACommanderSelectionStrategy } from '../../api/Contracts';

export interface ReplayRiftCRALaunchOptions {
  launchId: string;
  /** Exact legacy override. Mutually exclusive with commanderSelection. */
  commanderID?: number;
  /** Deterministic candidate pool; each selected commander produces one CRA command. */
  commanderSelection?: CRACommanderSelection;
  sourceCastleId?: number;
  sourceX?: number;
  sourceY?: number;
  horseTravelBoostId?: HorseTravelBoostID;
  attackSetup?: AttackSetupDraft;
  /** Local wall-clock arrival at the Rift (unix seconds). Omit or 0 for immediate resend. */
  arriveAtUnix?: number;
}

export interface RiftMapContextValue {
  riftMapCoords: RiftMapCoords | null;
  riftCRALaunch: RiftCRALaunchState | null;
  refreshRiftMapCoords: (refresh?: boolean) => void;
  replayRiftCRALaunch: (options: ReplayRiftCRALaunchOptions) => Promise<void>;
  renameRiftCRALaunch: (launchId: string, displayName: string) => Promise<void>;
  deleteRiftCRALaunch: (launchId: string) => Promise<void>;
}

const RiftMapContext = createContext<RiftMapContextValue | undefined>(undefined);

export function RiftMapProvider({ children }: { children: ReactNode }) {
  const { state, submitIntent } = useCitadelAPI();
  const { castle } = useCastleFocus();
  const riftCRALaunch = useMemo(() => {
    const launches = Object.values(state?.rift?.launches ?? {}).map((entry) => {
      const scheduled = state?.scheduled?.[`rift:${entry.id}`];
      const executeAt = scheduled?.status === 'scheduled' || scheduled?.status === 'running'
        ? Date.parse(scheduled.executeAt)
        : Number.NaN;
      return {
        ...entry,
        ...(Number.isFinite(executeAt) && (entry.oneWayTTSeconds ?? 0) > 0
          ? { scheduledArriveAtUnix: Math.floor(executeAt / 1000) + (entry.oneWayTTSeconds ?? 0) }
          : {}),
      };
    });
    return parseRiftCRALaunchPayload({ launches, launchCount: launches.length });
  }, [state?.rift?.launches, state?.scheduled]);

  const refreshRiftMapCoords = useCallback((refresh = true) => {
    if (!refresh || !castle) return;
    void submitIntent('map.query', {
      kingdomId: castle.kingdomId,
      x1: castle.x - 25,
      y1: castle.y - 25,
      x2: castle.x + 25,
      y2: castle.y + 25,
    });
  }, [castle, submitIntent]);

  const replayRiftCRALaunch = useCallback(async (options: ReplayRiftCRALaunchOptions) => {
    if (!options.launchId) return;
    await submitIntent('rift.launch.replay', options);
  }, [submitIntent]);

  const renameRiftCRALaunch = useCallback(async (launchId: string, displayName: string) => {
    if (!launchId) return;
    await submitIntent('rift.template.rename', { launchId, displayName: displayName.trim() });
  }, [submitIntent]);

  const deleteRiftCRALaunch = useCallback(async (launchId: string) => {
    if (!launchId) return;
    await submitIntent('rift.template.delete', { launchId });
  }, [submitIntent]);

  const riftMapCoords = useMemo(
    () => riftMapCoordsFromState(state, castle),
    [castle, state],
  );

  const value = useMemo<RiftMapContextValue>(
    () => ({
      riftMapCoords,
      riftCRALaunch,
      refreshRiftMapCoords,
      replayRiftCRALaunch,
      renameRiftCRALaunch,
      deleteRiftCRALaunch,
    }),
    [
      riftMapCoords,
      riftCRALaunch,
      refreshRiftMapCoords,
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
