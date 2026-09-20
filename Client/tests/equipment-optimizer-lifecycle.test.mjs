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

test('priority labels reject raw or generic group names and preserve semantic detail', () => {
	const metadata = {
		101: { name: 'effect_name_melee_strength', effectTypeId: 10, effectTypeName: 'MeleeAttackPVP', sortCategory: 3, sortGroup: 7, categoryName: 'effect_category_3', effectGroupPassive: 'effect_group_3_7_passive', effectTemplate: '+{0}% melee attack strength PVP' },
		102: { effectTypeId: 11, effectTypeName: 'RangedAttackPVP', sortCategory: 3, sortGroup: 7, effectGroupPassive: 'Group 7', internalName: 'equipmentRangedAttackPVP' },
		103: { effectTypeId: 12, name: 'Effect 1' },
		201: { effectTypeId: 20, sortCategory: 3, sortGroup: 8, effectGroupPassive: '-{0}% melee combat strength PVE' },
		202: { effectTypeId: 21, sortCategory: 3, sortGroup: 9, effectGroupPassive: '+{0}% melee combat strength PVP' },
	};
	const grouped = stateHelpers.groupEquipmentPriorityEffects([202, 103, 102, 201, 101], metadata);
	assert.equal(grouped[0].label, 'Melee attack strength bonus PvP / Ranged Attack PvP');
	assert.equal(grouped[0].categoryLabel, 'Attack effects');
	assert.deepEqual(grouped[0].effectIDs, [101, 102]);
	assert.equal(grouped[1].label, 'Melee combat strength penalty PvE');
	assert.equal(grouped[2].label, 'Melee combat strength bonus PvP');
	assert.equal(grouped[3].label, 'Effect metadata unavailable');
	assert.equal(stateHelpers.descriptiveEquipmentEffectLabel(101, metadata[101]), 'Melee attack strength bonus PvP');
	assert.equal(stateHelpers.descriptiveEquipmentEffectLabel(103, metadata[103]), 'Effect metadata unavailable (ID 103)');
});

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

test('semantic initialization survives recreated groups with the same official catalog identity', () => {
	const section = 'equipment.optimizerPriorities.v2.44.commander.0.pvp';
	const before = lifecycle.equipmentPriorityCatalogKey(groups);
	const recreatedGroups = groups.map((group) => ({ ...group, effectIDs: [...group.effectIDs] }));
	const after = lifecycle.equipmentPriorityCatalogKey(recreatedGroups);
	assert.equal(after, before);
	assert.equal(lifecycle.equipmentOptimizerInitializationChange(section, section, before, after, '1:|2:official-group-1-2', '1:|2:official-group-1-2'), 'unchanged');
	assert.equal(lifecycle.equipmentPriorityProfileKey({ tier1: [], tier2: ['official-group-1-2'] }), '1:|2:official-group-1-2');
});

test('catalog-only initialization changes retain the preview while preference changes invalidate it', () => {
	const section = 'equipment.optimizerPriorities.v2.44.commander.0.pvp';
	const profile = '1:|2:official-group-1-2';
	assert.equal(lifecycle.equipmentOptimizerInitializationChange(undefined, section, '', 'catalog-a', '', profile), 'invalidate');
	assert.equal(lifecycle.equipmentOptimizerInitializationChange(section, section, 'catalog-a', 'catalog-a', profile, profile), 'unchanged');
	assert.equal(lifecycle.equipmentOptimizerInitializationChange(section, section, 'catalog-a', 'catalog-b', profile, profile), 'retain-preview');
	assert.equal(lifecycle.equipmentOptimizerInitializationChange(section, section, 'catalog-a', 'catalog-b', profile, '1:official-group-3-4|2:'), 'retain-preview');
	assert.equal(lifecycle.equipmentOptimizerInitializationChange(section, section, 'catalog-a', 'catalog-a', profile, '1:official-group-3-4|2:'), 'invalidate');
});

test('shared cap labels render actual maxima', () => {
	assert.equal(lifecycle.equipmentSharedCapLabel([]), '');
	assert.equal(lifecycle.equipmentSharedCapLabel([90]), 'max 90');
	assert.equal(lifecycle.equipmentSharedCapLabel([90, 120]), 'max 90 · max 120');
});

test('unnamed same-level items remain distinguishable by readable rolled effects', () => {
	const names = (id) => id === 61 ? 'Melee soldiers combat strength' : `Effect ${id}`;
	const first = lifecycle.equipmentOptimizerEffectSummary([{ definitionId: 61, values: [60] }], names);
	const second = lifecycle.equipmentOptimizerEffectSummary([{ definitionId: 61, values: [72.5] }], names);
	assert.equal(first, 'Melee soldiers combat strength +60');
	assert.equal(second, 'Melee soldiers combat strength +72.5');
	assert.notEqual(first, second);
	assert.equal(lifecycle.equipmentOptimizerEffectSummary([{ definitionId: 999, values: [4] }], names), 'Unknown effect (effect 999) +4');
});

test('extraction presentation binds the selected ruby ceiling and discloses other game costs', () => {
	const cost = { rubyExtractionCount: 2, maximumRubySpend: 400, relicExtractionCount: 1, socketInsertionCount: 3, fingerprint: 'quote' };
	assert.equal(lifecycle.equipmentExtractionApplyLabel(4, cost), 'Apply Alternative 4 · up to 400 rubies');
	assert.deepEqual(lifecycle.equipmentExtractionCostNotices(cost), [
		'2 ordinary gem extractions · up to 400 rubies',
		'Coin socketing costs also apply.',
		'Relic gem extraction costs also apply in relic fragments.',
	]);
});

test('apply no-op state follows the selected alternative rather than the batch leader', () => {
	const current = { equipment: { '1': 101 }, gems: {} };
	const unchanged = { ...current, useful: false };
	const tradeoff = { equipment: { '1': 201 }, gems: {}, useful: true };
	assert.equal(lifecycle.equipmentAlternativeApplyDisabled(current, unchanged, false), true);
	assert.equal(lifecycle.equipmentAlternativeApplyDisabled(current, tradeoff, false), false);
	assert.equal(lifecycle.equipmentAlternativeApplyDisabled(current, tradeoff, true), false);
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
	const looseOffMode = structuredClone(state);
	looseOffMode.inventory.gems['501'] = { id: 501, definitionId: 77, compatibleWearerId: 2, combatMode: 'pve', effects: [] };
	assert.equal(lifecycle.equipmentOptimizerSnapshotKey(looseOffMode, leader, 'pvp'), baseline);
	const attachedOffMode = structuredClone(looseOffMode);
	attachedOffMode.inventory.gems['501'].equipmentInstanceId = 101;
	assert.notEqual(lifecycle.equipmentOptimizerSnapshotKey(attachedOffMode, leader, 'pvp'), baseline);
	const changed = structuredClone(state);
	changed.inventory.equipment['101'].effects[0].values[0] = 61;
	assert.notEqual(lifecycle.equipmentOptimizerSnapshotKey(changed, leader, 'pvp'), baseline);
	const appearance = structuredClone(state);
	appearance.commanders['0'].equipment['5'] = 105;
	appearance.inventory.equipment['105'] = { id: 105, definitionId: 5005, slot: 5, typeId: 2, relic: false, relicKnown: true, effects: [] };
	const appearanceKey = lifecycle.equipmentOptimizerSnapshotKey(appearance, leader, 'pvp');
	assert.notEqual(appearanceKey, baseline);
	const gemmedAppearance = structuredClone(appearance);
	gemmedAppearance.inventory.gems['-505'] = { id: -505, definitionId: 78, equipmentInstanceId: 105, effects: [] };
	assert.notEqual(lifecycle.equipmentOptimizerSnapshotKey(gemmedAppearance, leader, 'pvp'), appearanceKey);
	const changedAppearanceFamily = structuredClone(gemmedAppearance);
	changedAppearanceFamily.inventory.equipment['105'].relic = true;
	assert.notEqual(lifecycle.equipmentOptimizerSnapshotKey(changedAppearanceFamily, leader, 'pvp'), lifecycle.equipmentOptimizerSnapshotKey(gemmedAppearance, leader, 'pvp'));
	assert.notEqual(`${baseline}|catalog:first`, `${baseline}|catalog:second`);
});
