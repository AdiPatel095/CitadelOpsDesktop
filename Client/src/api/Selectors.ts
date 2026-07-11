import type { CastleStateV2, GameStateV2 } from './Contracts';

export interface CastleOptionV2 {
	id: number;
	name: string;
	type: string;
	kingdomId: number;
	x: number;
	y: number;
}

export function castleOptionsFromState(state: GameStateV2 | null): CastleOptionV2[] {
	if (!state) return [];
	return Object.values(state.castles)
		.map((castle) => ({
			id: castle.id,
			name: castle.name?.trim() || `Castle ${castle.id}`,
			type: castle.slotType != null ? `Slot ${castle.slotType}` : `Kingdom ${castle.kingdomId}`,
			kingdomId: castle.kingdomId,
			x: castle.x,
			y: castle.y,
		}))
		.sort((left, right) => left.kingdomId - right.kingdomId || left.name.localeCompare(right.name) || left.id - right.id);
}

export function focusedCastleFromState(
	state: GameStateV2 | null,
	selectedCastleId?: number | null,
): CastleStateV2 | null {
	if (!state) return null;
	const castles = Object.values(state.castles);
	if (selectedCastleId != null && selectedCastleId > 0) {
		const selected = castles.find((castle) => castle.id === selectedCastleId);
		if (selected) return selected;
	}
	return castles.find((castle) => castle.focused) ?? castles[0] ?? null;
}

export function castleDisplayName(castle: CastleStateV2 | null | undefined): string {
	if (!castle || castle.id <= 0) return '—';
	return castle.name?.trim() || `Castle ${castle.id}`;
}

export interface DecorationTooltipRow {
	definitionId: number;
	count: number;
	name: string;
}

export function decorationTooltipRows(
	castle: CastleStateV2 | null | undefined,
	getDecoration: (id: number) => { name: string } | undefined,
): DecorationTooltipRow[] {
	if (!castle) return [];
	const counts = new Map<number, number>();
	for (const building of Object.values(castle.buildings)) {
		if (!getDecoration(building.definitionId)?.name?.trim()) continue;
		counts.set(building.definitionId, (counts.get(building.definitionId) ?? 0) + 1);
	}
	return Array.from(counts, ([definitionId, count]) => ({
		definitionId,
		count,
		name: getDecoration(definitionId)?.name.trim() || `Decoration ${definitionId}`,
	})).sort((left, right) => left.name.localeCompare(right.name) || left.definitionId - right.definitionId);
}
