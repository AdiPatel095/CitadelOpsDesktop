import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const { SETTINGS_FIX_TARGETS, fixTargetFor } = await vite.ssrLoadModule('/src/settings/disclosure/fixTargets.ts');
const { SETTINGS_PLACEMENT } = await vite.ssrLoadModule('/src/settings/disclosure/placement.ts');
const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');

after(async () => {
  await vite.close();
});

const source = (path) => readFile(new URL(`../src/${path}`, import.meta.url), 'utf8');

/** Body of a top-level function, from its declaration to the next top-level function. */
function functionBody(text, name) {
  const start = text.search(new RegExp(`^(export )?function ${name}\\(`, 'm'));
  assert.ok(start >= 0, `function ${name}`);
  const rest = text.slice(start + 1);
  const next = rest.search(/^(export )?function /m);
  return next < 0 ? text.slice(start) : text.slice(start, start + 1 + next);
}

/** Check ids pushed as checks; plan lines are informational and never need a fix. */
function checkIds(text) {
  let stripped = text;
  for (let index = stripped.indexOf('plan.push('); index >= 0; index = stripped.indexOf('plan.push(', index + 1)) {
    let depth = 0;
    let end = index + 'plan.push'.length;
    for (; end < stripped.length; end += 1) {
      if (stripped[end] === '(') depth += 1;
      if (stripped[end] === ')') { depth -= 1; if (depth === 0) break; }
    }
    stripped = stripped.slice(0, index) + ' '.repeat(end - index + 1) + stripped.slice(end + 1);
  }
  return new Set([...stripped.matchAll(/\bid: '([a-z0-9-]+)'/g)].map((match) => match[1]));
}

const event = await source('settings/readiness/eventAttackReadiness.ts');
const khan = await source('settings/readiness/khanReadiness.ts');
const storm = await source('settings/readiness/stormReadiness.ts');
const setup = await source('settings/requirements/setupReadiness.ts');
const commanders = await source('settings/requirements/commanderEligibility.ts');
const slotHelpers = ['attackSlotReadiness', 'dailyLimitCheck', 'travelBoostCheck', 'inventoryCheck', 'compositionCheck']
  .filter((name) => new RegExp(`^(export )?function ${name}\\(`, 'm').test(event))
  .map((name) => functionBody(event, name)).join('\n');

const EVALUATORS = {
  autoNomad: [event, commanders],
  autoInvasion: [event, commanders],
  autoBeriWorld: [event, commanders],
  autoKhan: [khan, slotHelpers, commanders, "id: 'source-castle'"],
  autoStorm: [storm, slotHelpers, commanders],
  autoTowers: [functionBody(setup, 'evaluateTowerReadiness'), commanders],
  autoFortress: [functionBody(setup, 'evaluateFortressReadiness'), commanders],
  autoFoodBalance: [functionBody(setup, 'evaluateFoodBalanceReadiness')],
  // Static ID extraction cannot follow the Auto Bird-only branch.
  autoStation: [functionBody(setup, 'evaluateReserveReadiness').replace("id: 'storm-reserve'", '')],
  autoBird: [functionBody(setup, 'evaluateReserveReadiness')],
};

const MODALS = {
  autoNomad: 'AutoNomadSettingsModal', autoInvasion: 'AutoInvasionSettingsModal', autoBeriWorld: 'AutoBeriWorldSettingsModal',
  autoKhan: 'AutoKhanSettingsModal', autoStorm: 'AutoStormSettingsModal', autoTowers: 'AutoTowerSettingsModal',
  autoFortress: 'AutoFortressSettingsModal', autoFoodBalance: 'AutoFoodBalanceSettingsModal', autoStation: 'AutoStationSettingsModal',
  autoBird: 'AutoBirdSettingsModal',
};

test('every check id a feature evaluator can emit has a fix-target entry (null when there is no settings control)', () => {
  for (const [featureId, texts] of Object.entries(EVALUATORS)) {
    const ids = new Set(texts.flatMap((text) => [...checkIds(text)]));
    assert.ok(ids.size > 0, featureId);
    const table = SETTINGS_FIX_TARGETS[featureId];
    for (const id of ids) {
      const covered = Object.hasOwn(table, id) || Object.keys(table).some((key) => key.startsWith(`${id}:`));
      assert.ok(covered, `${featureId}: check "${id}" has no fix target`);
    }
  }
});

test('fix targets point at placed sections and at controls that exist in the modal', async () => {
  for (const [featureId, table] of Object.entries(SETTINGS_FIX_TARGETS)) {
    const sections = new Set(SETTINGS_PLACEMENT[featureId].map((section) => section.id));
    const modal = MODALS[featureId] ? await source(`settings/components/${MODALS[featureId]}.tsx`) : '';
    for (const [checkId, target] of Object.entries(table)) {
      if (target === null) continue;
      assert.ok(sections.has(target.section), `${featureId}/${checkId}: section ${target.section}`);
      assert.ok(modal.includes(`"${target.control}"`) || modal.includes(`'${target.control}'`), `${featureId}/${checkId}: control ${target.control} in ${MODALS[featureId]}`);
      if (target.labelKey) assert.ok(messages[target.labelKey], target.labelKey);
    }
  }
});

test('slot-specific targets win over the plain check id', () => {
  assert.equal(fixTargetFor('autoStorm', { id: 'composition', slot: 'islands' }).control, 'auto-storm-islands');
  assert.equal(fixTargetFor('autoStorm', { id: 'composition' }).control, 'auto-storm-branches');
  assert.equal(fixTargetFor('autoKhan', { id: 'rage-booster' }).section, 'stop-limits', 'a collapsed Advanced control is revealed before focus');
  assert.equal(fixTargetFor('autoKhan', { id: 'protection' }), null);
  assert.equal(fixTargetFor('autoTowers', { id: 'unknown-check' }), null);
});

test('Storm reserve fix reveals and focuses the Storm castle card', () => {
  assert.deepEqual(fixTargetFor('autoBird', { id: 'storm-reserve', slot: 'storm' }), { section: 'castles', control: 'auto-bird-castle-storm' });
  assert.equal(fixTargetFor('autoStation', { id: 'storm-reserve', slot: 'storm' }), null);
});
