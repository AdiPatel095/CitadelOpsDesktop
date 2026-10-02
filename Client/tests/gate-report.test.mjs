import assert from 'node:assert/strict';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { test } from 'node:test';
import { areaFor, buildReport, generateReport, toMarkdown } from '../scripts/visual/gate-report.mjs';

const finding = { rule: 'pageOverflow', element: 'html', detail: 'Width exceeds viewport' };
const fixture = (view, width, violations = [finding], suite = 'layout') => ({ suite, view, width, violations });

test('groups fixture findings by rule, view and area with deterministic width counts', () => {
  const input = [fixture('automation', 768), fixture('automation', 390, [finding, finding]), fixture('castle', 390), fixture('landing', 390), fixture('attack-presets', 1440), fixture('settings-auto-towers', 390, [], 'keyboard')];
  const result = buildReport(input);
  assert.equal(result.reportCount, 6);
  assert.equal(result.violationCount, 6);
  assert.deepEqual(result.groups.find(group => group.view === 'automation').widths, [390, 768]);
  assert.equal(result.groups.find(group => group.view === 'automation').count, 3);
  assert.deepEqual(result, buildReport([...input].reverse()));
  assert.equal(areaFor('settings-auto-towers-focus'), 'd system');
  assert.equal(areaFor('events'), 'b core');
  assert.equal(areaFor('player-tracker'), 'c analytics');
  assert.equal(areaFor('account-center'), 'a public/account');
});

test('invalid input cannot become a clean report', () => {
  assert.throws(() => buildReport([{ view: 'castle' }]), /Invalid gate report/);
  assert.throws(() => buildReport([fixture('unknown', 390)]), /Unknown gate view/);
  assert.throws(() => buildReport([fixture('castle', 390, [{}])]), /Invalid violation/);
});

test('Markdown keeps table cells intact and clean reports explicit', () => {
  const report = buildReport([fixture('castle', 390, [{ ...finding, rule: 'rule|<x>\nnext' }])]);
  assert.match(toMarkdown(report), /rule&#124;&lt;x> next/);
  assert.match(toMarkdown(buildReport([fixture('castle', 390, [])])), /No findings/);
});

test('reads fixture JSON, writes both artifacts, and rejects missing or malformed evidence', async () => {
  const root = await mkdtemp(join(tmpdir(), 'cit74-gate-report-'));
  try {
    const input = join(root, 'inputs');
    await mkdir(input);
    await assert.rejects(generateReport(input, root), /No gate JSON/);
    await writeFile(join(input, 'layout-castle-390.json'), JSON.stringify(fixture('castle', 390)));
    const result = await generateReport(input, root);
    assert.deepEqual(JSON.parse(await readFile(join(root, 'gate-report.json'), 'utf8')), result);
    assert.equal(await readFile(join(root, 'gate-report.md'), 'utf8'), toMarkdown(result));
    await writeFile(join(input, 'broken.json'), '{');
    await assert.rejects(generateReport(input, root), /Cannot read gate input broken.json/);
  } finally { await rm(root, { recursive: true, force: true }); }
});


test('axe findings and language/theme axes remain distinct and header cases have owners', () => {
  const result = buildReport([
    { ...fixture('header-panel', 390, [finding], 'a11y'), locale: 'ar', theme: 'light' },
    { ...fixture('header-panel', 390, [finding], 'a11y'), locale: 'en', theme: 'dark' },
  ]);
  assert.equal(result.violationCount, 2);
  assert.deepEqual(new Set(result.groups[0].findings.map(item => `${item.locale}/${item.theme}`)), new Set(['ar/light', 'en/dark']));
  assert.equal(areaFor('header-panel'), 'b core');
  assert.equal(areaFor('stale-session'), 'b core');
  assert.equal(areaFor('avatar-menu'), 'd system');
  assert.equal(areaFor('feature-stats-loading'), 'b core');
  assert.equal(areaFor('world-intel-error'), 'b core');
  assert.equal(areaFor('auth'), 'a public/account');
});
