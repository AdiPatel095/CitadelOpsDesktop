import assert from 'node:assert/strict';
import { test } from 'node:test';
import { mkdtempSync, mkdirSync, readFileSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { roles, mapWeight, nearestRoles, pixels, transform, run } from '../scripts/typography/type-scale.mjs';

function choose(source, file, selections) {
  const report = transform(source, { file });
  const decisions = {};
  for (const finding of report.unresolved.filter(item => item.kind === 'size')) {
    if (selections[finding.before]) decisions[finding.key] = { expected: finding.before, ...selections[finding.before] };
  }
  return transform(source, { file, decisions });
}

test('spec role sizes, line heights, weights, nearest ties and 12px floor', () => {
  assert.deepEqual([...new Set(Object.values(roles).map(role => role.size))].sort((a, b) => a - b), [12, 13, 14, 16, 20, 24, 32, 48, 64]);
  assert.deepEqual(Object.values(roles).map(role => role.line), [68, 52, 40, 32, 28, 24, 24, 20, 18, 16]);
  assert.deepEqual(nearestRoles(9), ['caption']);
  assert.deepEqual(nearestRoles(15), ['title-sm', 'body-lg', 'body']);
  assert.deepEqual(nearestRoles(16), ['title-sm', 'body-lg']);
  assert.deepEqual(nearestRoles(40), ['display', 'display-sm']);
  assert.equal(pixels('0.625rem'), null);
  assert.equal(pixels('0.625rem', 16), 10);
  for (const value of ['1em', '90%', 'clamp(12px, 1vw, 20px)', 'calc(12px + 1px)', '0', '-1px']) assert.equal(pixels(value, 16), null);
});

test('every plan weight maps; unspecified values remain unresolved', () => {
  for (const value of [400, 500, 600, 700]) assert.equal(mapWeight(value), value);
  assert.equal(mapWeight(520), 500);
  for (const value of [620, 630, 640, 650]) assert.equal(mapWeight(value), 600);
  for (const value of [720, 740, 750, 760, 780, 790, 800, 820, 830, 850, 900, 950, 1000]) assert.equal(mapWeight(value), 700);
  assert.equal(mapWeight('normal'), 400);
  assert.equal(mapWeight('bold'), 700);
  for (const value of [300, 550, 660, 1001, 'bolder', 'lighter', 'var(--weight)']) assert.equal(mapWeight(value), null);
});

test('CSS floor and weights preserve formatting, comments, important and unrelated properties', () => {
  const source = '/* font-size: 8px */\n.a { font-size: 10px !important; color: red; font-weight: 650; padding: 10px; }\n';
  const result = transform(source, { file: 'a.css' });
  assert.equal(result.output, '/* font-size: 8px */\n.a { font-size: var(--font-size-12) !important; color: red; font-weight: var(--font-weight-600); padding: 10px; }\n');
  assert.equal(result.unresolved.length, 0);
  assert.deepEqual(transform(result.output, { file: 'a.css' }).changes, []);
});

test('title and reading text at the same size require separate semantic decisions', () => {
  const source = '.title { font-size: 16px; } .reading { font-size: 16px; }';
  const dry = transform(source, { file: 'a.css' });
  assert.equal(dry.output, source);
  assert.equal(dry.unresolved.length, 2);
  const decisions = Object.fromEntries(dry.unresolved.map((item, index) => [item.key, { expected: item.before, role: index === 0 ? 'title-sm' : 'body-lg' }]));
  assert.equal(transform(source, { file: 'a.css', decisions }).unresolved.length, 0);
  const compact = choose('.page { font-size: 22px; }', 'a.css', { '22px': { role: 'headline', compact: true } });
  assert.match(compact.output, /var\(--font-size-20\)/);
});

test('CSS dynamic, relative, shorthand and value comments are reported intact', () => {
  const source = '.a { font-size: clamp(1rem, 3vw, 4rem); font-size: 1em; font: bold 10px sans-serif; font-weight: bolder; font-size: 10px /* retain */; font-size: var(--size); }';
  const result = transform(source, { file: 'a.css' });
  assert.equal(result.output, source);
  assert.equal(result.unresolved.length, 5);
  assert.equal(transform('.a {font-size: .625rem}', { file: 'a.css', remBase: 16 }).output, '.a {font-size: var(--font-size-12)}');
});

test('TSX scoped class replacement preserves variants, color, whitespace and UI strings', () => {
  const source = '// text-[9px]\nconst label = "text-[9px]"; const x = <p title="text-[9px]" className="md:hover:!text-[10px]  font-[520]! text-[#fff]">text-[9px]</p>;';
  const result = transform(source, { file: 'a.tsx' });
  assert.equal(result.output, source.replace('md:hover:!text-[10px]  font-[520]!', 'md:hover:!text-caption  font-medium!'));
  assert.equal(result.unresolved.length, 0);
  assert.deepEqual(transform(result.output, { file: 'a.tsx' }).changes, []);
});

test('static conditional/helper/array/object classes and legacy weight utilities', () => {
  const source = 'const x = <p className={cn(active ? "text-[9px]" : `font-[950]`, ["hover:font-black"], { "font-extrabold": active }, active && "text-[11px]")}/>;';
  const result = transform(source, { file: 'a.tsx' });
  assert.match(result.output, /active \? "text-caption" : `font-bold`/);
  assert.match(result.output, /"hover:font-bold"/);
  assert.match(result.output, /\{ "font-bold": active \}/);
  assert.match(result.output, /active && "text-caption"/);
  assert.equal(result.unresolved.length, 0);
});

test('role decisions on arbitrary and legacy text utilities preserve variant and importance', () => {
  const source = 'const x = <h2 className="lg:text-[16px]! sm:text-base font-semibold"/>;';
  const result = choose(source, 'a.tsx', { '16px': { role: 'title-sm' } });
  assert.equal(result.output, 'const x = <h2 className="lg:text-title-sm! sm:text-title-sm font-semibold"/>;');
  assert.equal(result.unresolved.length, 0);
});

test('dynamic classes, custom leading and unspecified light weights are reported', () => {
  const source = 'const x = <><p className={`text-[${size}px]`}/><p className="text-[10px]/4 font-light text-[calc(12px+1vw)]"/><p className={classes}/></>;';
  const result = transform(source, { file: 'a.tsx' });
  assert.equal(result.output, source);
  assert.equal(result.unresolved.length, 5);
  const legacyLeading = 'const x = <p className="md:text-sm/4"/>;';
  assert.equal(transform(legacyLeading, { file: 'a.tsx' }).unresolved.length, 1);
});

test('class object key collisions and spread keys never discard conditions', () => {
  const source = 'const x = <p className={cn({ "font-[800]": a, "font-[900]": b })}/>;';
  const result = transform(source, { file: 'a.tsx' });
  assert.equal(result.output, source);
  assert.equal(result.changes.length, 0);
  assert.match(result.unresolved[0].reason, /collide/);
  const spread = 'const x = <p className={cn({ ...classes, [key]: active })}/>;';
  assert.equal(transform(spread, { file: 'a.tsx' }).unresolved.length, 2);
});

test('inline styles and SVG attributes change only font values', () => {
  const source = 'const x = <><p style={{ fontSize: 10, fontWeight: 650, width: 10, color: "red" }}/><text fontSize="11px" fontWeight={950}>Hi</text></>;';
  const result = transform(source, { file: 'a.tsx' });
  assert.equal(result.output, 'const x = <><p style={{ fontSize: "var(--font-size-12)", fontWeight: "var(--font-weight-600)", width: 10, color: "red" }}/><text fontSize="var(--font-size-12)" fontWeight={"var(--font-weight-700)"}>Hi</text></>;');
  assert.deepEqual(transform(result.output, { file: 'a.tsx' }).changes, []);
  const dynamic = 'const x = <p style={{ fontSize: Math.max(9, size), fontWeight: weight }}/>;';
  assert.equal(transform(dynamic, { file: 'a.tsx' }).unresolved.length, 2);
});

test('stale/unknown/invalid role decisions and syntax fail closed', () => {
  const source = '.a { font-size: 16px; }';
  const key = transform(source, { file: 'a.css' }).unresolved[0].key;
  for (const decision of [null, { expected: '15px', role: 'title-sm' }, { expected: '16px', role: 'constructor' }, { expected: '16px', role: 'body', compact: true }, { expected: '16px', role: 'body', compact: 'yes' }]) {
    assert.throws(() => transform(source, { file: 'a.css', decisions: { [key]: decision } }), /stale or invalid/);
  }
  const tiny = '.a { font-size: 10px; }';
  const tinyKey = transform(tiny, { file: 'a.css' }).changes[0].key;
  assert.throws(() => transform(tiny, { file: 'a.css', decisions: { [tinyKey]: { expected: '10px', role: 'title' } } }), /must map to caption/);
  assert.throws(() => transform('.a {', { file: 'a.css' }));
  assert.throws(() => transform('const x = <p', { file: 'a.tsx' }), /invalid TSX/);
});

test('dry-run CLI and write preflight, path boundaries, deduplication and token exclusion', t => {
  const root = mkdtempSync(path.join(tmpdir(), 'cit-63-type-scale-'));
  t.after(() => rmSync(root, { recursive: true, force: true }));
  mkdirSync(path.join(root, 'src', 'styles'), { recursive: true });
  const a = 'const x = <p className="text-[9px] font-[950]"/>;';
  const b = '.title { font-size: 16px; }';
  writeFileSync(path.join(root, 'src/a.tsx'), a);
  writeFileSync(path.join(root, 'src/b.css'), b);
  writeFileSync(path.join(root, 'src/styles/tokens.css'), ':root { --font-size-12: 12px; font-size: 10px; }');
  const result = run({ root, files: ['src', 'src/a.tsx'] });
  assert.equal(result.files.length, 2);
  assert.equal(result.mode, 'dry-run');
  assert.equal(readFileSync(path.join(root, 'src/a.tsx'), 'utf8'), a);
  assert.throws(() => run({ root, files: ['src'], write: true }), /Refusing writes/);
  assert.equal(readFileSync(path.join(root, 'src/a.tsx'), 'utf8'), a);
  assert.throws(() => run({ root, files: ['../escape.css'] }), /inside root/);
  assert.throws(() => run({ root, files: ['src/a.tsx'], decisions: { stale: {} } }), /unused\/stale/);
  assert.throws(() => run({ root, files: ['src'], remBase: NaN }), /positive/);
  symlinkSync(path.join(root, 'src'), path.join(root, 'linked'));
  assert.throws(() => run({ root, files: ['linked/a.tsx'] }), /symlink/);
  const cli = spawnSync(process.execPath, [fileURLToPath(new URL('../scripts/typography/type-scale.mjs', import.meta.url)), '--root', root, 'src/a.tsx'], { encoding: 'utf8' });
  assert.equal(cli.status, 0, cli.stderr);
  assert.equal(JSON.parse(cli.stdout).mode, 'dry-run');
  assert.equal(readFileSync(path.join(root, 'src/a.tsx'), 'utf8'), a);
  run({ root, files: ['src/a.tsx'], write: true });
  assert.match(readFileSync(path.join(root, 'src/a.tsx'), 'utf8'), /text-caption font-bold/);
  assert.equal(run({ root, files: ['src/a.tsx'], write: true }).files[0].changes.length, 0);
});
