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
const upgradeCaps = await vite.ssrLoadModule('/src/equipment/EquipmentUpgradeCaps.ts');

after(async () => {
  await vite.close();
});

test('official equipment upgrade caps distinguish normal and wire-proven relic items', () => {
	const expectedCaps = new Map([
		[0, 20],
		[1, 3],
		[2, 8],
		[3, 12],
		[4, 16],
		[5, 50],
	]);

	for (const [rarityID, levelCap] of expectedCaps) {
		assert.equal(upgradeCaps.officialUpgradeLevelCap('equipment', {
			rarityID, relicKnown: true, slot: 1,
		}), levelCap);
	}
	assert.equal(upgradeCaps.officialUpgradeLevelCap('equipment', {
		rarityID: 5, relic: true, relicKnown: true, slot: 1,
	}), 50);
	assert.equal(upgradeCaps.officialUpgradeLevelCap('equipment', {
		rarityID: 15, relic: true, relicKnown: true, slot: 6,
	}), 50);
});

test('upgrade caps fail closed for unknown, hero, appearance, and unverified equipment', () => {
	for (const classification of [
		{},
		{ rarityID: 0, slot: 1 },
		{ rarityID: 6, relicKnown: true, slot: 1 },
		{ rarityID: 10, relicKnown: true, slot: 6 },
		{ rarityID: 0, relicKnown: true, slot: 5 },
		{ rarityID: 15, relicKnown: true, slot: 6 },
	]) {
		assert.equal(upgradeCaps.officialUpgradeLevelCap('equipment', classification), null);
	}
	assert.equal(upgradeCaps.officialUpgradeLevelCap('gem'), 50);
	assert.equal(upgradeCaps.officialUpgradeLevelCap('gem', { rarityID: 999 }), 50);
});
