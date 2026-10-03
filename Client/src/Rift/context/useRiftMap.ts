import { createContext, useContext } from 'react';
import type { RiftMapCoords } from '../types/RiftMapCoords';
import type { RiftCRALaunchState } from '../types/RiftCRALaunch';
import type { CRACommanderSelection } from '../../api/Contracts';
import type { AttackSetupDraft } from '../../components/AttackSetupModal';
import type { HorseTravelBoostID } from '../../settings/HorseTravelBoost';

export type ReplayRiftCRALaunchOptions = {
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
};

export interface RiftMapContextValue {
  riftMapCoords: RiftMapCoords | null;
  riftCRALaunch: RiftCRALaunchState | null;
  refreshRiftMapCoords: (refresh?: boolean) => void;
  replayRiftCRALaunch: (options: ReplayRiftCRALaunchOptions) => Promise<void>;
  renameRiftCRALaunch: (launchId: string, displayName: string) => Promise<void>;
  deleteRiftCRALaunch: (launchId: string) => Promise<void>;
}

export const RiftMapContext = createContext<RiftMapContextValue | undefined>(undefined);

export function useRiftMap(): RiftMapContextValue {
  const ctx = useContext(RiftMapContext);
  if (ctx === undefined) {
    throw new Error('useRiftMap must be used within RiftMapProvider');
  }
  return ctx;
}
