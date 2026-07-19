import { createContext, useCallback, useContext, useEffect, useMemo, useRef, type ReactNode } from 'react';
import { useCitadelAPI } from '../../api/ApiContext';
import { CitadelAPI } from '../../api/CitadelClient';
import { movementViewFromState, type MovementViewModel } from '../types/MovementState';

const MOVEMENT_POLL_INTERVAL_MS = 5_000;

export interface MovementContextValue {
	movement: MovementViewModel | null;
	refreshMovement: (refresh?: boolean) => void;
}

const MovementContext = createContext<MovementContextValue | undefined>(undefined);

export function MovementProvider({ children }: { children: ReactNode }) {
	const { state, submitIntent } = useCitadelAPI();
	const refreshInFlight = useRef(false);
	const movement = useMemo(() => movementViewFromState(state), [state]);
	const requestMovementRefresh = useCallback((background: boolean) => {
		if (refreshInFlight.current) return;
		refreshInFlight.current = true;
		const request = background
			? CitadelAPI.submitIntent('game.refresh_movements', {}, { actor: 'movement-poll', priority: 20 })
			: submitIntent('game.refresh_movements');
		void request.catch((error) => {
			console.error('Could not refresh movements', error);
		}).finally(() => {
			refreshInFlight.current = false;
		});
	}, [submitIntent]);
	const refreshMovement = useCallback((refresh = true) => {
		if (refresh) requestMovementRefresh(false);
	}, [requestMovementRefresh]);
	useEffect(() => {
		if (!state?.session.loggedIn || !state.session.socketReady) return;
		requestMovementRefresh(true);
		const interval = window.setInterval(
			() => requestMovementRefresh(true),
			MOVEMENT_POLL_INTERVAL_MS,
		);
		return () => window.clearInterval(interval);
	}, [requestMovementRefresh, state?.session.loggedIn, state?.session.socketReady]);
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
