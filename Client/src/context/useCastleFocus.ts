import { createContext, useContext } from 'react';
import type { CastleStateV2 } from '../api/Contracts';

export interface CastleFocusContextValue {
	castle: CastleStateV2 | null;
	castles: CastleStateV2[];
	refreshCastle: () => void;
	selectCastle: (castleId: number) => void;
	offlineCastleId: number | null;
}

export const CastleFocusContext = createContext<CastleFocusContextValue | undefined>(undefined);

export function useCastleFocus(): CastleFocusContextValue {
	const context = useContext(CastleFocusContext);
	if (!context) throw new Error('useCastleFocus must be used within a CastleFocusProvider');
	return context;
}
