import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const roles = await vite.ssrLoadModule('/src/settings/UnitRole.ts');

after(async () => {
  await vite.close();
});

test('unitCombatRole compares the best attack value with the best defence value', () => {
  assert.equal(roles.unitCombatRole({ id: 1, name: 'a', meleeAttack: 10, meleeDefence: 5 }), 'attack');
  assert.equal(roles.unitCombatRole({ id: 2, name: 'b', rangeAttack: 30, meleeDefence: 30 }), 'attack', 'ties are attack units');
  assert.equal(roles.unitCombatRole({ id: 3, name: 'c', meleeAttack: 5, rangeDefence: 9 }), 'defense');
  assert.equal(roles.unitCombatRole({ id: 4, name: 'd', meleeAttack: 'x' }), 'attack', 'missing values count as 0');
  assert.equal(roles.isAttackUnit({ id: 5, name: 'e', meleeDefence: 1 }), false);
});

test('the troop picker uses the shared role rule instead of its own formula', async () => {
  const picker = await readFile(new URL('../src/components/TroopPickerModal.tsx', import.meta.url), 'utf8');
  assert.match(picker, /unitCombatRole\(item\)/);
  assert.doesNotMatch(picker, /attack >= defense/);
});
