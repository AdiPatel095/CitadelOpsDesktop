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
const recipes = await vite.ssrLoadModule('/src/settings/onboarding/StarterRecipes.ts');
const types = await vite.ssrLoadModule('/src/attackPresets/AttackPresetTypes.ts');

after(async () => {
  await vite.close();
});

const SOURCES = new Set(['existing-default', 'official-catalog', 'account-data', 'proposed']);

test('every starter entry carries review metadata owned by Maya', () => {
  const entries = Object.entries(recipes.EVENT_ATTACK_STARTER_RECIPE);
  assert.ok(entries.length >= 6);
  for (const [name, entry] of entries) {
    assert.ok(SOURCES.has(entry.source), name);
    assert.ok(entry.rationale.trim().length > 0, name);
    assert.equal(entry.review.owner, 'Maya', name);
    assert.ok(['pending', 'accepted'].includes(entry.review.status), name);
    if (entry.source === 'existing-default' || entry.source === 'official-catalog') assert.ok(entry.citation?.trim(), `${name} needs a citation`);
  }
});

test('accepted entries require evidence', () => {
  for (const [name, entry] of Object.entries(recipes.EVENT_ATTACK_STARTER_RECIPE)) {
    if (entry.review.status === 'accepted') assert.ok(entry.review.evidence?.trim(), `${name} is accepted without evidence`);
  }
  const accepted = structuredClone(recipes.EVENT_ATTACK_STARTER_RECIPE);
  accepted.waveCount.review = { owner: 'Maya', status: 'accepted', evidence: 'test' };
  assert.equal(recipes.pendingStarterReviews(accepted).includes('waveCount'), false);
});

test('no starter value is approved by this story', () => {
  assert.deepEqual(
    recipes.pendingStarterReviews().sort(),
    Object.keys(recipes.EVENT_ATTACK_STARTER_RECIPE).sort(),
  );
});

test('existing-default values equal the cited defaults', () => {
  const minimal = types.parseAttackPreset({
    id: 'x',
    name: 'x',
    waves: [{ L: { troops: [], tools: [] }, M: { troops: [], tools: [] }, R: { troops: [], tools: [] } }],
  });
  assert.equal(recipes.EVENT_ATTACK_STARTER_RECIPE.targetType.value, minimal.targetType);
  assert.equal(recipes.EVENT_ATTACK_STARTER_RECIPE.useTroopFamilies.value, minimal.useTroopFamilies);
  for (const [name, entry] of Object.entries(recipes.EVENT_ATTACK_STARTER_RECIPE)) {
    if (entry.source === 'existing-default') assert.ok(['targetType', 'useTroopFamilies'].includes(name), `${name} has no verified default comparison`);
  }
});

test('proposed numeric values stay within runtime limits and invent nothing else', () => {
  const { waveCount } = recipes.EVENT_ATTACK_STARTER_RECIPE;
  assert.equal(waveCount.source, 'proposed');
  assert.ok(waveCount.value >= 1 && waveCount.value <= 30);
  const numeric = Object.entries(recipes.EVENT_ATTACK_STARTER_RECIPE).filter(([, entry]) => typeof entry.value === 'number');
  assert.deepEqual(numeric.map(([name]) => name), ['waveCount']);
});
