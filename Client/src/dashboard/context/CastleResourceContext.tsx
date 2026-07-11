import {
	createContext,
	useCallback,
	useContext,
	useMemo,
	useState,
	type ReactNode,
} from 'react';
import { useCitadelAPI } from '../../api/ApiContext';
import { playerCastleInfoFromState } from '../../api/StateAdapters';
import { useMetadata } from '../../context/MetadataContext';
import type { PlayerCastleInfo } from '../models/PlayerCastleInfo';

type CastleResourceMap = Map<number, PlayerCastleInfo>;
type LoadingStatus = Record<number, boolean>;

interface CastleResourceContextType {
	castleResources: CastleResourceMap;
	isCastleResourcesLoading: LoadingStatus;
	getCastle: (castleId: number) => PlayerCastleInfo | undefined;
	requestCastleResource: (castleId: number) => void;
}

const CastleResourceContext = createContext<CastleResourceContextType | undefined>(undefined);

export function useCastleResources(): CastleResourceContextType {
	const context = useContext(CastleResourceContext);
	if (!context) throw new Error('useCastleResources must be used within a CastleResourceProvider');
	return context;
}

export function CastleResourceProvider({ children }: { children: ReactNode }) {
	const { state, submitIntent } = useCitadelAPI();
	const { resources, buildings, isLoading: metadataLoading } = useMetadata();
	const [isCastleResourcesLoading, setIsCastleResourcesLoading] = useState<LoadingStatus>({});
	const castleResources = useMemo(() => {
		const result = new Map<number, PlayerCastleInfo>();
		if (metadataLoading) return result;
		for (const castle of Object.values(state?.castles ?? {})) {
			result.set(castle.id, playerCastleInfoFromState(castle, resources, buildings));
		}
		return result;
	}, [buildings, metadataLoading, resources, state?.castles]);

	const requestCastleResource = useCallback((castleId: number) => {
		if (castleId <= 0) return;
		setIsCastleResourcesLoading((current) => ({ ...current, [castleId]: true }));
		void submitIntent('game.focus_castle', { castleId })
			.catch((error) => console.error(`Could not refresh castle ${castleId}`, error))
			.finally(() => setIsCastleResourcesLoading((current) => ({ ...current, [castleId]: false })));
	}, [submitIntent]);

	const getCastle = useCallback(
		(castleId: number) => castleResources.get(castleId),
		[castleResources],
	);
	const value = useMemo<CastleResourceContextType>(() => ({
		castleResources,
		isCastleResourcesLoading,
		getCastle,
		requestCastleResource,
	}), [castleResources, getCastle, isCastleResourcesLoading, requestCastleResource]);

	return <CastleResourceContext.Provider value={value}>{children}</CastleResourceContext.Provider>;
}
