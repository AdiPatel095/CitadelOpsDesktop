export interface AutoBeriWorldSettings {
	minTroopsToTransfer: number;
	beriCastleId: number;
	transferTroopId: number;
	sourceCastleId: number;
	wireCastleId: number;
	troopSpaceCheckIntervalSec: number;
}

export const DEFAULT_AUTO_BERI_WORLD_SETTINGS: AutoBeriWorldSettings = {
	minTroopsToTransfer: 1,
	beriCastleId: 0,
	transferTroopId: 0,
	sourceCastleId: 0,
	wireCastleId: -1,
	troopSpaceCheckIntervalSec: 30,
};

export function parseAutoBeriWorldSettings(payload: unknown): AutoBeriWorldSettings {
	const value = isRecord(payload) ? payload : {};
	return {
		minTroopsToTransfer: nonNegativeInteger(value.minTroopsToTransfer, 1),
		beriCastleId: nonNegativeInteger(value.beriCastleId ?? value.beriCastleCID, 0),
		transferTroopId: nonNegativeInteger(value.transferTroopId ?? value.transferTroopWID, 0),
		sourceCastleId: nonNegativeInteger(value.sourceCastleId ?? value.kutSourceCastleSCID, 0),
		wireCastleId: integer(value.wireCastleId ?? value.kutCastleCID, -1),
		troopSpaceCheckIntervalSec: clamp(integer(value.troopSpaceCheckIntervalSec, 30), 5, 3600),
	};
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function integer(value: unknown, fallback: number): number {
	const parsed = Number(value);
	return Number.isFinite(parsed) ? Math.trunc(parsed) : fallback;
}

function nonNegativeInteger(value: unknown, fallback: number): number {
	return Math.max(0, integer(value, fallback));
}

function clamp(value: number, minimum: number, maximum: number): number {
	return Math.min(maximum, Math.max(minimum, value));
}
