import { createContext, useCallback, useContext, useEffect, useMemo, type ReactNode } from 'react';
import { useCitadelAPI } from '../../api/ApiContext';
import { movementViewFromState, type MovementViewModel } from '../types/MovementState';

export interface MovementContextValue {
	movement: MovementViewModel | null;
	refreshMovement: (refresh?: boolean) => void;
}

const MovementContext = createContext<MovementContextValue | undefined>(undefined);

export function MovementProvider({ children }: { children: ReactNode }) {
	const { state, submitIntent } = useCitadelAPI();
	const movement = useMemo(() => movementViewFromState(state), [state]);
	const refreshMovement = useCallback((refresh = true) => {
		if (!refresh) return;
		void submitIntent('game.refresh_movements').catch((error) => {
			console.error('Could not refresh movements', error);
		});
	}, [submitIntent]);
	useEffect(() => {
		if (state?.session.loggedIn) refreshMovement(true);
	}, [refreshMovement, state?.session.loggedIn]);
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
