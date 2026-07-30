import {
	createContext,
	useCallback,
	useContext,
	useEffect,
	useMemo,
	useState,
	type ReactNode,
} from 'react';
import { useCitadelAPI } from '../api/ApiContext';
import type { CastleStateV2 } from '../api/Contracts';
import { focusedCastleFromState } from '../api/Selectors';
import { useAuth } from './AuthContext';

export interface CastleFocusContextValue {
	castle: CastleStateV2 | null;
	castles: CastleStateV2[];
	refreshCastle: () => void;
	selectCastle: (castleId: number) => void;
	offlineCastleId: number | null;
}

const CastleFocusContext = createContext<CastleFocusContextValue | undefined>(undefined);

export function CastleFocusProvider({ children }: { children: ReactNode }) {
	const { gameLoggedIn } = useAuth();
	const { state, submitIntent } = useCitadelAPI();
	const [offlineCastleId, setOfflineCastleId] = useState<number | null>(null);

	useEffect(() => {
		if (gameLoggedIn) setOfflineCastleId(null);
	}, [gameLoggedIn]);

	const castles = useMemo(
		() => Object.values(state?.castles ?? {}).sort((left, right) => (
			left.kingdomId - right.kingdomId
			|| (left.name ?? '').localeCompare(right.name ?? '')
			|| left.id - right.id
		)),
		[state?.castles],
	);
	const castle = useMemo(
		() => focusedCastleFromState(state, gameLoggedIn ? null : offlineCastleId),
		[gameLoggedIn, offlineCastleId, state],
	);

	const focusLiveCastle = useCallback((castleId: number) => {
		if (castleId <= 0) return;
		void submitIntent('game.focus_castle', { castleId }).catch((error) => {
			console.error(`Could not focus castle ${castleId}`, error);
		});
	}, [submitIntent]);

	const selectCastle = useCallback((castleId: number) => {
		if (castleId <= 0) return;
		if (gameLoggedIn) {
			focusLiveCastle(castleId);
			return;
		}
		setOfflineCastleId(castleId);
	}, [focusLiveCastle, gameLoggedIn]);

	const refreshCastle = useCallback(() => {
		if (castle) focusLiveCastle(castle.id);
	}, [castle, focusLiveCastle]);

	const value = useMemo<CastleFocusContextValue>(() => ({
		castle,
		castles,
		refreshCastle,
		selectCastle,
		offlineCastleId,
	}), [castle, castles, offlineCastleId, refreshCastle, selectCastle]);

	return <CastleFocusContext.Provider value={value}>{children}</CastleFocusContext.Provider>;
}

export function useCastleFocus(): CastleFocusContextValue {
	const context = useContext(CastleFocusContext);
	if (!context) throw new Error('useCastleFocus must be used within a CastleFocusProvider');
	return context;
}
