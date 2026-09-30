import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

/**
 * Justifies the first-result attribution rule against the producers (CIT-20): the coordinator names every
 * automated operation `automation:<policy actor id>`, where the actor id is the policy's ActorID() or its
 * own ID(). Desktop only: it reads `Server/Automation`.
 */
const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const attribution = await vite.ssrLoadModule('/src/settings/readiness/automationAttribution.ts');
after(async () => {
  await vite.close();
});

const AUTOMATION = new URL('../../Server/Automation/', import.meta.url);

async function producers() {
  const files = (await readdir(AUTOMATION)).filter((name) => name.endsWith('.go') && !name.endsWith('_test.go'));
  const ids = new Map();
  const actors = new Map();
  for (const file of files) {
    const text = await readFile(new URL(file, AUTOMATION), 'utf8');
    for (const [, receiver, id] of text.matchAll(/func \(\*?(\w*)\) ID\(\) string\s*\{\s*return "([^"]+)"/g)) ids.set(receiver, id);
    for (const [, receiver, actor] of text.matchAll(/func \(\*?(\w*)\) ActorID\(\) string\s*\{\s*return "([^"]+)"/g)) actors.set(receiver, actor);
  }
  return { ids, actors };
}

test('the coordinator submits automated operations as automation:<policy actor id>', async () => {
  const coordinator = await readFile(new URL('Coordinator.go', AUTOMATION), 'utf8');
  assert.match(coordinator, /request\.Actor = "automation:" \+ policyActorID\(policy\)/);
  assert.match(coordinator, /copy\.Actor = "automation:" \+ policyActorID\(policy\)/);
  assert.match(coordinator, /func policyActorID\(policy Policy\) string[\s\S]*ActorID\(\)[\s\S]*return policy\.ID\(\)/);
});

test('every attributable feature is the actor id of at least one policy, and lane policies share their feature actor', async () => {
  const { ids, actors } = await producers();
  const actorIds = new Set([...ids.entries()].map(([receiver, id]) => actors.get(receiver) ?? id));
  for (const feature of ['autoNomad', 'autoInvasion', 'autoKhan', 'autoBeriWorld', 'autoTowers', 'autoFortress', 'autoStorm', 'autoFoodBalance', 'autoStation', 'autoBird', 'autoTCI', 'autoSceatRes', 'autoBooster', 'autoBuyer', 'autoAdvisor', 'autoEquipmentCleanup', 'autoHospital']) {
    assert.ok(actorIds.has(feature), `${feature}: no policy submits as automation:${feature}`);
    assert.equal(attribution.automationActor(feature), `automation:${feature}`);
  }
  // Lane policies keep the feature actor while their own status id differs.
  const byReceiver = (receiver) => actors.get(receiver);
  assert.equal(byReceiver('AutoStormShopPolicy'), 'autoStorm');
  assert.equal(byReceiver('AutoStormBuildPolicy'), 'autoStorm');
  assert.equal(byReceiver('BeriAttackPolicy'), 'autoBeriWorld');
  assert.equal(byReceiver('AutoKhanCooldownPolicy'), 'autoKhan');
  assert.equal(byReceiver('AutoKhanDefensePolicy'), 'autoKhan');
});

test('Recruit and Tool policies report under their own ids, matching the feature ids', async () => {
  const production = await readFile(new URL('ProductionPolicy.go', AUTOMATION), 'utf8');
  assert.match(production, /id: "autoRecruit", enabledKey: "recruit_troops"/);
  assert.match(production, /id: "autoTool", enabledKey: "auto_tool"/);
});
