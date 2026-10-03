import { createContext, useContext } from 'react';
import type { MovementViewModel } from '../types/MovementState';

export interface MovementContextValue {
	movement: MovementViewModel | null;
	refreshMovement: (refresh?: boolean) => void;
}

export const MovementContext = createContext<MovementContextValue | undefined>(undefined);

export function useMovement(): MovementContextValue {
	const context = useContext(MovementContext);
	if (!context) throw new Error('useMovement must be used within MovementProvider');
	return context;
}
