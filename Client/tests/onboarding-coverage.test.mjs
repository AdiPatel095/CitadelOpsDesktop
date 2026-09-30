import assert from 'node:assert/strict';
import { readdir, readFile, writeFile } from 'node:fs/promises';
import { after, test } from 'node:test';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { COLUMNS, FEATURE_MODULES, SHARED_MODULES, deriveRows, renderTable, support } from './onboarding-browser/coverage.mjs';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({ root: clientRoot, appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const { SETTINGS_PLACEMENT } = await vite.ssrLoadModule('/src/settings/disclosure/placement.ts');

after(async () => {
  await vite.close();
});

const DIR = new URL('./onboarding-browser/scenarios/', import.meta.url);
const scenarios = await Promise.all((await readdir(DIR)).filter((name) => name.endsWith('.json')).sort().map(async (name) => JSON.parse(await readFile(new URL(name, DIR), 'utf8'))));
const DOC = new URL('../../Docs/OnboardingCoverage.md', import.meta.url);
const counts = Object.fromEntries(Object.entries(SETTINGS_PLACEMENT).map(([id, sections]) => [id, sections.length]));
const START = '<!-- coverage-table:start -->';
const END = '<!-- coverage-table:end -->';

test('every module is covered in every column it supports, and every scenario id in the matrix exists', () => {
  const ids = new Set(scenarios.map((scenario) => scenario.id));
  assert.equal(FEATURE_MODULES.length, 19);
  assert.deepEqual(FEATURE_MODULES.map(([id]) => id).sort(), Object.keys(SETTINGS_PLACEMENT).sort(), 'the matrix lists exactly the catalog modules');
  for (const row of deriveRows(scenarios, counts)) {
    row.cells.forEach((cell, index) => {
      const [column, title] = COLUMNS[index];
      assert.notEqual(cell, 'MISSING', `${row.id}: no scenario covers "${title}" (${column}), a column this module supports`);
      if (support(row.id, column).yes) for (const match of cell.matchAll(/`([a-z0-9-]+)`/g)) assert.ok(ids.has(match[1]), `${row.id}: scenario ${match[1]} does not exist`);
      else assert.match(cell, /^n\/a: .{8,}/, `${row.id}/${column}: a not-applicable cell says why`);
    });
  }
  for (const scenario of scenarios) {
    for (const moduleId of scenario.modules) assert.ok([...FEATURE_MODULES, ...SHARED_MODULES].some(([id]) => id === moduleId), `${scenario.id}: unknown module ${moduleId}`);
    for (const column of scenario.columns) assert.ok(COLUMNS.some(([id]) => id === column), `${scenario.id}: unknown column ${column}`);
  }
});

test('the coverage document is the table derived from the scenario files', async () => {
  const table = renderTable(scenarios, counts);
  const text = await readFile(DOC, 'utf8').catch(() => '');
  if (process.env.UPDATE_COVERAGE === '1') {
    const start = text.indexOf(START);
    const end = text.indexOf(END);
    assert.ok(start >= 0 && end > start, 'the document carries the table markers');
    await writeFile(DOC, `${text.slice(0, start + START.length)}\n${table}\n${text.slice(end)}`);
    return;
  }
  const start = text.indexOf(START);
  const end = text.indexOf(END);
  assert.ok(start >= 0 && end > start, 'Docs/OnboardingCoverage.md carries the table markers');
  assert.equal(text.slice(start + START.length, end).trim(), table, 'the table is out of date: run UPDATE_COVERAGE=1 node --test tests/onboarding-coverage.test.mjs');
});

test('the document lists every scenario with its step and platforms, and names the platform differences', async () => {
  const text = await readFile(DOC, 'utf8');
  for (const scenario of scenarios) {
    const row = text.split('\n').find((line) => line.startsWith(`| \`${scenario.id}\` |`));
    assert.ok(row, `${scenario.id} has a row in the scenario table`);
    assert.ok(row.includes(`| ${scenario.step} |`), `${scenario.id}: walkthrough step ${scenario.step}`);
    assert.ok(row.includes(scenario.platforms.join(', ')), `${scenario.id}: platforms`);
    if (scenario.hostedOnly) assert.match(row, /hosted:/, `${scenario.id}: hosted note`);
    if (scenario.desktopOnly) assert.match(row, /desktop:/, `${scenario.id}: desktop note`);
  }
});

test('the runbook names only scenarios that exist, gives the exact command and URL, and mentions every label of the simulation', async () => {
  const runbook = await readFile(new URL('../../Docs/OnboardingPreview.md', import.meta.url), 'utf8');
  const ids = new Set(scenarios.map((scenario) => scenario.id));
  const walkthrough = runbook.slice(runbook.indexOf('## Walkthrough'), runbook.indexOf('## Evidence'));
  const mentioned = [...walkthrough.matchAll(/`([a-z0-9]+(?:-[a-z0-9]+)+)`/g)].map((match) => match[1]);
  assert.ok(mentioned.length > 30);
  for (const id of mentioned) assert.ok(ids.has(id), `the runbook names ${id}, which is not a scenario`);
  assert.match(runbook, /npm run preview:onboarding/);
  assert.match(runbook, /http:\/\/127\.0\.0\.1:41734\//);
  assert.match(runbook, /Simulated preview: sample data only\. Nothing here reaches the game or an account\./);
  assert.match(runbook, /Candidate <short SHA>/);
  assert.match(runbook, /Simulated: attack on tower 12/);
  assert.match(runbook, /Ctrl-C/);
  for (const parameter of ['scenario', 'session', 'fail', 'locale', 'account', 'draft', 'reset']) assert.ok(runbook.includes(`\`${parameter}=`) || runbook.includes(`?${parameter}=`), `parameter ${parameter}`);
  // The ten-minute path and the step order follow the accepted walkthrough.
  assert.match(runbook, /Steps 1, 2, 9, 6 and 13 below, in that order\./);
  assert.doesNotMatch(runbook.replace(/`[^`]*`/g, '').replace(/Advance runtime|runtime evidence|runtime proof|"runtime"/g, ''), /\bruntime\b/i, 'no "runtime" wording in the runbook outside the dock control name and statements that it is not runtime evidence');
});
