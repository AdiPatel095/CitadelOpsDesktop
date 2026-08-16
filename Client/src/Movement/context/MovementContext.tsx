import { createContext, useCallback, useContext, useEffect, useMemo, useRef, type ReactNode } from 'react';
import { useCitadelAPI } from '../../api/ApiContext';
import { CitadelAPI } from '../../api/CitadelClient';
import { movementViewFromState, type MovementViewModel } from '../types/MovementState';

const MOVEMENT_SYNC_RETRY_MS = 5_000;

export interface MovementContextValue {
	movement: MovementViewModel | null;
	refreshMovement: (refresh?: boolean) => void;
}

const MovementContext = createContext<MovementContextValue | undefined>(undefined);

export function MovementProvider({ children }: { children: ReactNode }) {
	const { state, submitIntent } = useCitadelAPI();
	const refreshInFlight = useRef(false);
	const refreshedConnectionGeneration = useRef(0);
	const movement = useMemo(() => movementViewFromState(state), [state]);
	const requestMovementRefresh = useCallback(async (background: boolean): Promise<boolean> => {
		if (refreshInFlight.current) return false;
		refreshInFlight.current = true;
		const request = background
			? CitadelAPI.submitIntent('game.refresh_movements', {}, { actor: 'movement-sync', priority: 20 })
			: submitIntent('game.refresh_movements');
		try {
			await request;
			return true;
		} catch (error) {
			console.error('Could not refresh movements', error);
			return false;
		} finally {
			refreshInFlight.current = false;
		}
	}, [submitIntent]);
	const refreshMovement = useCallback((refresh = true) => {
		if (refresh) void requestMovementRefresh(false);
	}, [requestMovementRefresh]);
	useEffect(() => {
		if (!state?.session.loggedIn || !state.session.socketReady || state.session.generation <= 0 ||
			state.session.baselineGeneration !== state.session.generation || state.session.connectionGeneration <= 0) return;
		const connectionGeneration = state.session.connectionGeneration;
		if (state.movementSnapshot.version > 0 &&
			state.movementSnapshot.connectionGeneration === connectionGeneration) {
			refreshedConnectionGeneration.current = connectionGeneration;
			return;
		}
		if (refreshedConnectionGeneration.current === connectionGeneration) return;
		let cancelled = false;
		let retry: number | undefined;
		const refresh = async () => {
			refreshedConnectionGeneration.current = connectionGeneration;
			if (await requestMovementRefresh(true) || cancelled) return;
			refreshedConnectionGeneration.current = 0;
			retry = window.setTimeout(refresh, MOVEMENT_SYNC_RETRY_MS);
		};
		void refresh();
		return () => {
			cancelled = true;
			if (retry !== undefined) window.clearTimeout(retry);
		};
	}, [
		requestMovementRefresh,
		state?.session.baselineGeneration,
		state?.session.connectionGeneration,
		state?.session.generation,
		state?.session.loggedIn,
		state?.session.socketReady,
		state?.movementSnapshot.connectionGeneration,
		state?.movementSnapshot.version,
	]);
	const value = useMemo<MovementContextValue>(
		() => ({ movement, refreshMovement }),
		[movement, refreshMovement],
	);
	return <MovementContext.Provider value={value}>{children}</MovementContext.Provider>;
}

export function useMovement(): MovementContextValue {
	const context = useContext(MovementContext);
	if (!context) throw new Error('useMovement must be used within MovementProvider');
	return context;
}
