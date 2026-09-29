import assert from 'node:assert/strict';
import { readFile, readdir } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const { SETTINGS_PLACEMENT, AUTOMATION_ENABLED_KEYS } = await vite.ssrLoadModule('/src/settings/disclosure/placement.ts');

after(async () => {
  await vite.close();
});

const SOURCE_ROOT = new URL('../src/', import.meta.url);
const MATRIX = new URL('../../Docs/Features/SettingsPlacement.md', import.meta.url);

/** `feature -> Set('section:tier')` as declared in placement.ts. */
function declared() {
  const result = new Map();
  for (const [featureId, sections] of Object.entries(SETTINGS_PLACEMENT)) {
    result.set(featureId, new Set(sections.map((section) => `${section.id}:${section.tier}`)));
  }
  return result;
}

/** `feature -> Set(section)` actually rendered by the modals through <SettingsSection>. */
async function rendered() {
  const files = [
    ...(await readdir(new URL('settings/components/', SOURCE_ROOT))).filter((name) => name.endsWith('.tsx')).map((name) => `settings/components/${name}`),
    'views/AutomationView.tsx',
  ];
  const result = new Map();
  for (const file of files) {
    const text = await readFile(new URL(file, SOURCE_ROOT), 'utf8');
    const disclosures = new Map();
    for (const [, variable, literal, expression] of text.matchAll(/const (\w+) = useSettingsDisclosure\((?:'(\w+)'|([\w.]+))\)/g)) {
      disclosures.set(variable, literal ? [literal] : expression === 'definition.featureID' ? ['autoRecruit', 'autoTool'] : []);
    }
    for (const [, variable, section] of text.matchAll(/<SettingsSection\s+disclosure=\{(\w+)\}\s+section="([^"]+)"/g)) {
      const features = disclosures.get(variable);
      assert.ok(features && features.length > 0, `${file}: SettingsSection uses an unknown disclosure ${variable}`);
      for (const featureId of features) {
        if (!result.has(featureId)) result.set(featureId, new Set());
        result.get(featureId).add(section);
      }
    }
  }
  return result;
}

async function matrixRows() {
  const text = await readFile(MATRIX, 'utf8');
  const rows = new Map();
  for (const line of text.split('\n')) {
    const cells = line.split('|').map((cell) => cell.trim());
    if (cells.length < 7 || !/^auto[A-Z]/.test(cells[1])) continue;
    const [, featureId, section, tier] = cells;
    if (!rows.has(featureId)) rows.set(featureId, new Set());
    rows.get(featureId).add(`${section}:${tier}`);
  }
  return rows;
}

test('every placed section is rendered by its modal, and every rendered section is placed', async () => {
  const code = await rendered();
  for (const [featureId, sections] of declared()) {
    const ids = new Set([...sections].map((entry) => entry.split(':')[0]));
    assert.deepEqual([...(code.get(featureId) ?? [])].sort(), [...ids].sort(), featureId);
  }
  for (const featureId of code.keys()) assert.ok(SETTINGS_PLACEMENT[featureId], `${featureId} rendered but not placed`);
});

test('the reviewed placement matrix matches the code, row for row', async () => {
  const rows = await matrixRows();
  const code = declared();
  assert.deepEqual([...rows.keys()].sort(), [...code.keys()].sort());
  for (const [featureId, sections] of code) {
    assert.deepEqual([...(rows.get(featureId) ?? [])].sort(), [...sections].sort(), featureId);
  }
});

test('every feature in the matrix has an Essentials view and a reachable Stop', () => {
  for (const [featureId, sections] of Object.entries(SETTINGS_PLACEMENT)) {
    assert.ok(sections.some((section) => section.tier === 'essentials'), featureId);
    assert.match(AUTOMATION_ENABLED_KEYS[featureId], /^[a-z_]+$/, featureId);
  }
});

test('the module coverage inventory is complete (CIT-17 matrix families)', () => {
  const expected = [
    'autoNomad', 'autoInvasion', 'autoKhan', 'autoBeriWorld',
    'autoTowers', 'autoFortress', 'autoStorm',
    'autoFoodBalance', 'autoStation', 'autoBird',
    'autoRecruit', 'autoTool', 'autoHospital',
    'autoTCI', 'autoSceatRes', 'autoBooster', 'autoBuyer', 'autoAdvisor', 'autoEquipmentCleanup',
  ];
  assert.deepEqual(Object.keys(SETTINGS_PLACEMENT).sort(), [...expected].sort());
});

test('Maya matrix review adjustments (2026-09-29) are placed as decided', () => {
  const tier = (featureId, sectionId) => SETTINGS_PLACEMENT[featureId].find((section) => section.id === sectionId)?.tier;
  assert.equal(tier('autoTowers', 'castles'), 'essentials');
  assert.equal(tier('autoTowers', 'scan'), 'advanced');
  assert.equal(tier('autoKhan', 'policy'), 'essentials');
  assert.equal(tier('autoKhan', 'stop-limits'), 'advanced');
  assert.equal(tier('autoBeriWorld', 'building'), 'essentials');
  assert.equal(tier('autoBeriWorld', 'building-options'), 'advanced');
  assert.equal(tier('autoStorm', 'shop'), 'essentials');
  assert.equal(tier('autoStorm', 'donors'), 'essentials');
  assert.equal(tier('autoStorm', 'construction'), 'advanced');
  assert.equal(tier('autoStorm', 'import-tuning'), undefined, 'import sizing is folded into the Essentials donors group');
  assert.equal(tier('autoStation', 'evacuation'), 'essentials');
  assert.equal(tier('autoStation', 'filters'), 'advanced');
  assert.equal(tier('autoAdvisor', 'run-sizing'), 'essentials');
});

test('the matrix records Maya\'s review evidence and only the CIT-21 copy rows are pending', async () => {
  const text = await readFile(MATRIX, 'utf8');
  assert.match(text, /Maya matrix review 2026-09-29, Desktop 1c2fa93 \/ Hosted 745eef2, Product\/Simpler automation setup\.md § CIT-17 placement matrix review/);
  for (const line of text.split('\n')) {
    const cells = line.split('|').map((cell) => cell.trim());
    if (cells.length < 7 || !/^auto[A-Z]/.test(cells[1])) continue;
    // CIT-21 rows that gained "Copy to other castles" wait for Maya's review of that addition.
    const status = cells[cells.length - 2];
    if (status === 'pending') assert.match(cells[4], /Copy to other castles/, `${cells[1]}/${cells[2]}: only the CIT-21 copy rows may be pending`);
    else assert.match(status, /^accepted( \(adjusted\))?$/, `${cells[1]}/${cells[2]}`);
  }
});

test('the Storm shop group heading renders the placement title (Aquamarine and Ruby spending)', async () => {
  const shop = SETTINGS_PLACEMENT.autoStorm.find((section) => section.id === 'shop');
  const { messages } = await vite.ssrLoadModule('/src/i18n/messages.ts');
  assert.equal(messages[shop.titleKey], 'Aquamarine and Ruby spending');
  const source = await readFile(new URL('settings/components/AutoStormSettingsModal.tsx', SOURCE_ROOT), 'utf8');
  const shopBlock = source.slice(source.indexOf('section="shop"'), source.indexOf('section="limits"'));
  assert.ok(shopBlock.includes(`title={localizeStatic("${shop.titleKey}")}`), 'the shop heading uses the placement title key');
  assert.ok(!source.includes('title.aquamarine.spending'), 'the older heading key is gone');
});
