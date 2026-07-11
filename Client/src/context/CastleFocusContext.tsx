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
import { castleFocusFromState } from '../api/StateAdapters';
import type { CastleFocusState } from '../types/CastleFocusState';
import { useMetadata } from './MetadataContext';
import { useAuth } from './AuthContext';

export type PlayerCastleFocusParams = {
	castleId: number;
	kingdomId: number;
	mapX: number;
	mapY: number;
};

export interface CastleFocusContextValue {
	castleFocus: CastleFocusState | null;
	refreshCastleFocus: () => void;
	requestPlayerCastleFocus: (params: PlayerCastleFocusParams) => void;
	setOfflineCastleFocusKey: (key: string | null) => void;
	offlineCastleFocusKey: string | null;
}

const CastleFocusContext = createContext<CastleFocusContextValue | undefined>(undefined);

export function CastleFocusProvider({ children }: { children: ReactNode }) {
	const { gameLoggedIn } = useAuth();
	const { state, submitIntent } = useCitadelAPI();
	const { buildings } = useMetadata();
	const [offlineFocusKey, setOfflineFocusKey] = useState<string | null>(null);

	useEffect(() => {
		if (gameLoggedIn) setOfflineFocusKey(null);
	}, [gameLoggedIn]);

	const selectedCastleID = !gameLoggedIn ? castleIDFromOptionKey(offlineFocusKey) : undefined;
	const castleFocus = useMemo(
		() => castleFocusFromState(state, selectedCastleID || undefined, buildings),
		[buildings, selectedCastleID, state],
	);

	const requestFocus = useCallback((castleId: number) => {
		if (castleId <= 0) return;
		void submitIntent('game.focus_castle', { castleId }).catch((error) => {
			console.error(`Could not focus castle ${castleId}`, error);
		});
	}, [submitIntent]);

	const refreshCastleFocus = useCallback(() => {
		const focused = Object.values(state?.castles ?? {}).find((castle) => castle.focused)
			?? Object.values(state?.castles ?? {})[0];
		if (focused) requestFocus(focused.id);
	}, [requestFocus, state?.castles]);

	const requestPlayerCastleFocus = useCallback((params: PlayerCastleFocusParams) => {
		requestFocus(params.castleId);
	}, [requestFocus]);

	const value = useMemo<CastleFocusContextValue>(() => ({
		castleFocus,
		refreshCastleFocus,
		requestPlayerCastleFocus,
		setOfflineCastleFocusKey: setOfflineFocusKey,
		offlineCastleFocusKey: offlineFocusKey,
	}), [castleFocus, offlineFocusKey, refreshCastleFocus, requestPlayerCastleFocus]);

	return <CastleFocusContext.Provider value={value}>{children}</CastleFocusContext.Provider>;
}

export function useCastleFocus(): CastleFocusContextValue {
	const context = useContext(CastleFocusContext);
	if (!context) throw new Error('useCastleFocus must be used within a CastleFocusProvider');
	return context;
}

function castleIDFromOptionKey(key: string | null): number {
	if (!key) return 0;
	const id = Number(key.split('|')[0]);
	return Number.isFinite(id) ? Math.trunc(id) : 0;
}
