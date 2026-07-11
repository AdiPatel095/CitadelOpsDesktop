import type { CastleFocusState, PlayerCastleOption } from '../types/CastleFocusState';
import type {
	CastleBuildingRow,
	CastleProductionTotal,
	CastleResourcesAmount,
	CastleStorageMax,
	PlayerCastleInfo,
} from '../dashboard/models/PlayerCastleInfo';
import type { GAMMovement, MovementState } from '../Movement/types/MovementState';
import type { CastleStateV2, GameStateV2, MovementStateV2 } from './Contracts';

type DefinitionNames = Record<number, { name: string }>;

export function castleFocusFromState(
	state: GameStateV2 | null,
	selectedCastleId?: number,
	buildingDefinitions: DefinitionNames = {},
): CastleFocusState | null {
	if (!state) return null;
	const castles = Object.values(state.castles);
	const selected = selectedCastleId != null
		? castles.find((castle) => castle.id === selectedCastleId)
		: castles.find((castle) => castle.focused);
	if (!selected) return null;
	return {
		aid: selected.id,
		kingdomID: selected.kingdomId,
		mapPX: selected.x,
		mapPY: selected.y,
		castleName: selected.name,
		catalogVersion: state.catalogVersion,
		bgRows: buildingRows(selected, buildingDefinitions),
		bdRows: [],
		playerCastles: playerCastleOptions(castles),
	};
}

export function movementFromState(state: GameStateV2 | null): MovementState | null {
	if (!state) return null;
	const receivedUnix = Math.floor(new Date(state.updatedAt).getTime() / 1000);
	const activeMovements = Object.values(state.movements).map((movement) => movementRow(movement, receivedUnix));
	const movementByCommander = new Map<number, GAMMovement>();
	for (const movement of activeMovements) {
		if (movement.commanderID >= 0) movementByCommander.set(movement.commanderID, movement);
	}
	const commanderStatuses = Object.values(state.commanders).map((commander) => {
		const movement = movementByCommander.get(commander.id) ?? null;
		return {
			commanderID: commander.id,
			name: commander.name ?? '',
			visiblePosition: commander.id,
			status: movement == null ? (commander.available ? 'free' as const : 'unknown' as const) : movement.d === 1 ? 'returning' as const : 'outbound' as const,
			busy: movement != null || !commander.available,
			movement,
		};
	});
	if (commanderStatuses.length === 0) {
		for (const [commanderID, movement] of movementByCommander) {
			commanderStatuses.push({
				commanderID,
				name: '',
				visiblePosition: commanderID,
				status: movement.d === 1 ? 'returning' : 'outbound',
				busy: true,
				movement,
			});
		}
	}
	const gam = state.observations.gam;
	const lastSnapshotUnix = gam ? Math.floor(new Date(gam.lastSeenAt).getTime() / 1000) : 0;
	const freshnessWindowSec = 45;
	return {
		activeMovements,
		commanderStatuses,
		snapshotReady: gam?.inboundCount > 0,
		snapshotFresh: lastSnapshotUnix > 0 && Date.now() / 1000-lastSnapshotUnix <= freshnessWindowSec,
		lastSnapshotUnix,
		freshnessWindowSec,
	};
}

export function playerCastleInfoFromState(
	castle: CastleStateV2,
	resourceDefinitions: Record<number, { name: string; internalName?: unknown }>,
	buildingDefinitions: DefinitionNames,
): PlayerCastleInfo {
	const amount: Record<string, number> = {};
	const production: Record<string, number> = {};
	const storage: Record<string, number> = {};
	for (const definition of Object.values(resourceDefinitions)) {
		const internalName = typeof definition.internalName === 'string'
			? definition.internalName.toLowerCase()
			: '';
		if (!internalName) continue;
		amount[`${internalName}_amount`] = 0;
		production[`${internalName}_prod`] = 0;
		storage[`${internalName}_max`] = 0;
	}
	for (const [rawID, balance] of Object.entries(castle.resources)) {
		const definition = resourceDefinitions[Number(rawID)];
		const internalName = typeof definition?.internalName === 'string'
			? definition.internalName.toLowerCase()
			: '';
		if (!internalName) continue;
		amount[`${internalName}_amount`] = balance.amount;
		if (balance.productionPerHour != null) production[`${internalName}_prod`] = balance.productionPerHour;
		if (balance.capacity != null) storage[`${internalName}_max`] = balance.capacity;
	}
	return {
		castleName: castle.name ?? `Castle ${castle.id}`,
		aid: castle.id,
		mapKingdomID: castle.kingdomId,
		mapX: castle.x,
		mapY: castle.y,
		amount: amount as unknown as CastleResourcesAmount,
		production: production as unknown as CastleProductionTotal,
		storage: storage as unknown as CastleStorageMax,
		troops: {
			kingdomID: castle.kingdomId,
			x: castle.x,
			y: castle.y,
			troopsI: castle.units.stationed,
			troopsTU: castle.units.traveling,
			troopsHI: castle.units.hospital,
			troopsSHI: castle.units.specialHospital,
			troopsMixed: castle.units.total,
		},
		bgRows: buildingRows(castle, buildingDefinitions),
		bdRows: [],
	};
}

function playerCastleOptions(castles: CastleStateV2[]): PlayerCastleOption[] {
	return castles
		.map((castle) => ({
			aid: castle.id,
			kingdomID: castle.kingdomId,
			name: castle.name ?? `Castle ${castle.id}`,
			mapX: castle.x,
			mapY: castle.y,
		}))
		.sort((left, right) => left.kingdomID-right.kingdomID || left.name.localeCompare(right.name));
}

function buildingRows(castle: CastleStateV2, definitions: DefinitionNames): CastleBuildingRow[] {
	return Object.values(castle.buildings).map((building) => ({
		buildingID: building.definitionId,
		oid: building.instanceId,
		name: definitions[building.definitionId]?.name ?? `Building ${building.definitionId}`,
		level: building.level ?? 0,
		x: building.gridX ?? 0,
		y: building.gridY ?? 0,
		r: building.rotation,
	}));
}

function movementRow(movement: MovementStateV2, receivedUnix: number): GAMMovement {
	return {
		mid: movement.id,
		pt: movement.progressSeconds ?? 0,
		tt: movement.travelSeconds ?? 0,
		d: movement.direction,
		kid: movement.kingdomId,
		sid: 0,
		oid: movement.ownerPlayerId ?? 0,
		targetType: movement.typeId ?? 0,
		targetX: movement.targetX,
		targetY: movement.targetY,
		sourceX: movement.sourceX ?? 0,
		sourceY: movement.sourceY ?? 0,
		commanderID: movement.commanderId ?? -1,
		troopArray: Object.entries(movement.units).map(([id, amount]) => [Number(id), amount]),
		pwd: 0,
		twd: 0,
		receivedUnix,
	};
}
