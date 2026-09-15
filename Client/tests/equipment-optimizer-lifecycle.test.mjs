import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
	root: clientRoot,
	appType: 'custom',
	logLevel: 'silent',
	server: { middlewareMode: true },
});
const lifecycle = await vite.ssrLoadModule('/src/equipment/components/EquipmentOptimizerLifecycle.ts');
const stateHelpers = await vite.ssrLoadModule('/src/equipment/components/EquipmentOptimizerState.ts');

after(async () => vite.close());

const groups = [
	{ key: 'official-group-1-2', label: 'Combat strength', category: 1, categoryLabel: 'Unit effects', group: 2, effectIDs: [61, 152] },
	{ key: 'official-group-3-4', label: 'Wall strength', category: 3, categoryLabel: 'Attack effects', group: 4, effectIDs: [88] },
];
const effects = {
	61: { effectTypeId: 61, sortCategory: 1, sortGroup: 2, effectGroupPassive: 'Combat strength' },
	152: { effectTypeId: 152, sortCategory: 1, sortGroup: 2, effectGroupPassive: 'Combat strength' },
	88: { effectTypeId: 88, sortCategory: 3, sortGroup: 4, effectGroupPassive: 'Wall strength' },
};

test('v1-v4 profiles migrate to official groups and retain an unavailable inventory choice', () => {
	for (const [raw, expected] of [
		[{ version: 1, tier1: [61], tier2: [88] }, { tier1: ['official-group-1-2'], tier2: ['official-group-3-4'] }],
		[{ version: 2, tier1: ['official-group-1-2'], tier2: ['official-group-3-4'] }, { tier1: ['official-group-1-2'], tier2: ['official-group-3-4'] }],
		[{ version: 3, tier1: ['effect-type-61'], tier2: ['effect-type-88'] }, { tier1: ['official-group-1-2'], tier2: ['official-group-3-4'] }],
		[{ version: 4, tier1: ['official-group-1-2'], tier2: ['official-group-3-4'] }, { tier1: ['official-group-1-2'], tier2: ['official-group-3-4'] }],
	]) {
		assert.deepEqual(stateHelpers.readEquipmentPriorityProfile(raw, groups, effects), expected);
	}
	const inventoryFilteredGroups = groups.slice(0, 1);
	assert.deepEqual(
		stateHelpers.readEquipmentPriorityProfile({ version: 4, tier1: [], tier2: ['official-group-3-4'] }, groups, effects),
		{ tier1: [], tier2: ['official-group-3-4'] },
	);
	assert.deepEqual(
		stateHelpers.readEquipmentPriorityProfile({ version: 4, tier1: [], tier2: ['official-group-3-4'] }, inventoryFilteredGroups, effects),
		{ tier1: [], tier2: [] },
	);
});

test('semantic initialization scope survives candidate identity changes within the same official groups', () => {
	const section = 'equipment.optimizerPriorities.v2.44.commander.0.pvp';
	const before = lifecycle.equipmentOptimizerScopeKey(section, groups);
	const recreatedGroups = groups.map((group) => ({ ...group, effectIDs: [...group.effectIDs] }));
	assert.equal(lifecycle.equipmentOptimizerScopeKey(section, recreatedGroups), before);
	assert.equal(lifecycle.equipmentPriorityProfileKey({ tier1: [], tier2: ['official-group-1-2'] }), '1:|2:official-group-1-2');
});

test('relevant snapshot ignores unrelated revision and catches equipment or catalog changes', () => {
	const state = {
		revision: 1,
		account: { worldId: 'world', playerId: 44 },
		player: { id: 44, level: 70 },
		session: { serverUrl: 'world', generation: 2, connectionGeneration: 3 },
		commanders: { '0': { id: 0, available: true, equipment: { '1': 101 }, gems: {} } },
		castellans: {},
		inventory: { equipment: { '101': { id: 101, definitionId: 5001, slot: 1, typeId: 2, effects: [{ definitionId: 61, wireId: 1, values: [60] }] } }, gems: {} },
	};
	const leader = { kind: 'commander', id: 0 };
	const baseline = lifecycle.equipmentOptimizerSnapshotKey(state, leader, 'pvp');
	assert.equal(lifecycle.equipmentOptimizerSnapshotKey({ ...state, revision: 2, player: { ...state.player, level: 71 } }, leader, 'pvp'), baseline);
	const changed = structuredClone(state);
	changed.inventory.equipment['101'].effects[0].values[0] = 61;
	assert.notEqual(lifecycle.equipmentOptimizerSnapshotKey(changed, leader, 'pvp'), baseline);
	assert.notEqual(`${baseline}|catalog:first`, `${baseline}|catalog:second`);
});
