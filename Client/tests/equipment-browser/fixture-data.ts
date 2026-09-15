import type { CatalogManifest, ConfigurationSnapshot, GameStateV2 } from '../../src/api/Contracts';
import type { EquipmentLeader } from '../../src/equipment/components/EquipmentTypes';
import type { FixtureMetadata } from './metadata-context.mock';

export type FixtureLeaderKind = 'commander' | 'castellan';
export type MigrationSeed = 'empty' | 'v1' | 'v2' | 'v3' | 'v4';

const slots = [1, 2, 3, 4, 6] as const;

export function fixtureEffects(expanded = false): FixtureMetadata['effects'] {
	const names = [
		'Melee combat strength', 'Melee damage at the wall',
		'Ranged combat strength', 'Ranged damage at the wall',
		'Courtyard combat strength', 'Courtyard unit limit',
		'Wall protection reduction', 'Gate protection reduction',
	];
	const effects: FixtureMetadata['effects'] = {};
	for (let index = 0; index < 8; index += 1) {
		const group = Math.floor(index / 2) + 1;
		effects[9001 + index] = {
			id: 9001 + index,
			name: names[index]!,
			internalName: `fixture_effect_${9001 + index}`,
			effectTypeId: 101 + index,
			sortCategory: group < 3 ? 3 : 5,
			sortGroup: group,
			categoryName: group < 3 ? 'Attack effects' : 'Pre-battle effects',
			effectGroupPassive: group === 1 ? 'Melee combat strength' : group === 2 ? 'Ranged combat strength' : group === 3 ? 'Courtyard strength' : 'Fortification reduction',
		};
	}
	// This ID starts outside candidateEffectIDs. Adding it exercises candidate-ID
	// churn while the official semantic group and catalog digest stay unchanged.
	effects[9011] = {
		id: 9011,
		name: 'Melee combat strength against newer targets',
		internalName: 'fixture_effect_9011',
		effectTypeId: 111,
		sortCategory: 3,
		sortGroup: 1,
		categoryName: 'Attack effects',
		effectGroupPassive: 'Melee combat strength',
	};
	if (expanded) {
		effects[9012] = {
			id: 9012,
			name: 'New official melee modifier',
			internalName: 'fixture_effect_9012',
			effectTypeId: 112,
			sortCategory: 3,
			sortGroup: 1,
			categoryName: 'Attack effects',
			effectGroupPassive: 'Melee combat strength',
		};
	}
	return effects;
}

export function fixtureState(kind: FixtureLeaderKind, inventoryMode: 'normal' | 'few' | 'no-gear'): GameStateV2 {
	const equipmentCount = inventoryMode === 'normal' ? (kind === 'commander' ? 334 : 253) : inventoryMode === 'few' ? 4 : 0;
	const gemCount = inventoryMode === 'normal' && kind === 'commander' ? 16 : 0;
	const equipmentType = kind === 'commander' ? 2 : 1;
	const baseID = kind === 'commander' ? 1000 : 2000;
	const equipment: Record<string, Record<string, unknown>> = {};
	const gems: Record<string, Record<string, unknown>> = {};
	const equipped: Record<string, number> = {};
	const socketed: Record<string, number> = {};
	for (let index = 0; index < equipmentCount; index += 1) {
		const slot = slots[index % slots.length]!;
		const id = baseID + index;
		equipment[String(id)] = {
			id, definitionId: 3000 + baseID + index, slot, typeId: equipmentType,
			rarityId: index % 6, setId: index % 12, level: index % 21,
			effects: [
				{ wireId: index % 8 + 1, definitionId: 9001 + index % 8, values: [index % 37 + 1] },
				{ wireId: (index + 3) % 8 + 1, definitionId: 9001 + (index + 3) % 8, values: [index % 19 + 1] },
			],
		};
		if (equipped[String(slot)] == null) equipped[String(slot)] = id;
	}
	for (let index = 0; index < gemCount; index += 1) {
		const id = 5000 + index;
		gems[String(id)] = {
			id, definitionId: 7000 + index, compatibleWearerId: equipmentType,
			combatMode: 'pvp', level: index % 16,
			effects: [{ wireId: 301, definitionId: 9001 + index % 8, values: [index % 13 + 1] }],
		};
		if (index < 4) socketed[String(index + 1)] = id;
	}
	const leader = { id: 0, name: kind === 'commander' ? 'Fixture Commander' : 'Fixture Castellan', available: true, equipment: equipped, gems: socketed };
	return {
		schemaVersion: 2, revision: 41, updatedAt: new Date().toISOString(), catalogVersion: 'fixture', languageVersion: 'fixture-en',
		session: { generation: 7, baselineGeneration: 7, connectionGeneration: 3, status: 'ready', loggedIn: true, socketReady: true, serverUrl: 'fixture://local', changedAt: new Date().toISOString() },
		account: { worldId: 'cit6-fixture', playerId: 77 },
		player: { id: 77, name: 'Synthetic Account', resources: {}, currencies: {}, vip: {}, achievements: { points: 0, completed: {}, progress: {} }, legendSkills: { activeIds: [], skillPoints: 0, resetRemainingSec: 0, resetCount: 0, sceatSkillIds: [], sceatActivations: [] } },
		commanders: kind === 'commander' ? { '0': leader } : {},
		castellans: kind === 'castellan' ? { '0': leader } : {},
		inventory: { constructionItems: {}, constructionOffers: {}, constructionOffersKingdomId: 0, equipment, gems, gemStacks: {}, items: {} },
		castles: {}, generals: {}, movements: {}, movementSnapshot: {} as never, stationing: {}, scheduled: {}, rift: {} as never,
		subscriptions: {}, market: { castles: {}, caravanLevelLoaded: false }, kingdomTransport: { unlocks: {}, pending: [], pendingUnits: [] },
		beri: { availableTroops: 0, troopsByUnit: {} }, alliance: {} as never, alliances: {}, allianceHelpRequests: {} as never,
		map: {}, towerCooldowns: {}, towerQueue: {} as never, invasion: {} as never, storm: {} as never, nomadCamps: {} as never,
		khan: {} as never, dailyAttacks: {} as never, attackDialog: {}, attackPresets: [], eventScores: {} as never,
		advisor: {} as never, commandContext: {} as never, automations: {}, reports: {} as never, observations: {},
	} as GameStateV2;
}

export function fixtureLeader(state: GameStateV2, kind: FixtureLeaderKind): EquipmentLeader {
	const source = kind === 'commander' ? state.commanders['0']! : state.castellans['0']!;
	return {
		kind, id: 0, name: source.name ?? `Fixture ${kind}`, position: 0,
		available: 'available' in source ? source.available : true,
		equipment: source.equipment, gems: source.gems,
	};
}

export function fixtureMetadata(state: GameStateV2, expanded = false): FixtureMetadata {
	const equipments: FixtureMetadata['equipments'] = {};
	const gems: FixtureMetadata['gems'] = {};
	for (const item of Object.values(state.inventory.equipment)) {
		if (item.id % 11 !== 0) equipments[item.definitionId] = { id: item.definitionId, name: `Official ${slotName(item.slot)} ${item.definitionId}` };
	}
	for (const gem of Object.values(state.inventory.gems)) {
		if (gem.id % 5 !== 0) gems[gem.definitionId] = { id: gem.definitionId, name: `Official PvP gem ${gem.definitionId}` };
	}
	return { effects: fixtureEffects(expanded), equipments, gems };
}

export function fixtureCatalogs(digest = 'fixture-digest-a'): CatalogManifest {
	return {
		metadata: { itemVersion: 'fixture', languageVersion: 'fixture-en', sourceUrl: 'fixture://official-items', digestSha256: digest, fetchedAt: new Date().toISOString(), loadedAt: new Date().toISOString() },
		catalogs: [],
	};
}

export function fixtureConfiguration(seed: MigrationSeed, kind: FixtureLeaderKind): ConfigurationSnapshot {
	const section = `equipment.optimizerPriorities.v2.77.${kind}.0.pvp`;
	const values: Record<Exclude<MigrationSeed, 'empty'>, unknown> = {
		v1: { version: 1, tier1: [9001], tier2: [9003] },
		v2: { version: 2, tier1: ['official-group-3-1'], tier2: ['official-group-3-2'] },
		v3: { version: 3, tier1: ['effect-type-101'], tier2: ['effect-type-103'] },
		v4: { version: 4, tier1: ['official-group-3-1'], tier2: ['official-group-3-2'] },
	};
	return { schemaVersion: 2, revision: 1, updatedAt: new Date().toISOString(), sections: seed === 'empty' ? {} : { [section]: values[seed] } };
}

function slotName(slot: number): string {
	return ({ 1: 'Armor', 2: 'Weapon', 3: 'Helmet', 4: 'Artifact', 6: 'Hero' } as Record<number, string>)[slot] ?? `Slot ${slot}`;
}
