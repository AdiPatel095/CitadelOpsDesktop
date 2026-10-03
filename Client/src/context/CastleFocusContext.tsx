import { CastleFocusContext, type CastleFocusContextValue } from './useCastleFocus';
export type { CastleFocusContextValue } from './useCastleFocus';
import {
	useCallback,
	useEffect,
	useMemo,
	useState,
	type ReactNode,
} from 'react';
import { useCitadelAPI } from '../api/useCitadelAPI';
import { focusedCastleFromState } from '../api/Selectors';
import { useAuth } from './useAuth';


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
