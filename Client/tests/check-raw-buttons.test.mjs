import assert from 'node:assert/strict';
import { mkdtempSync, mkdirSync, writeFileSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { test } from 'node:test';
import { BUTTON_PATTERNS, checkSource, checkRoot } from '../scripts/check-raw-buttons.mjs';

const fixtures = [
  ['raw button missing marker', '<button onClick={() => act()}>Go</button>', ['data-button-pattern']],
  ['invalid marker', '<button data-button-pattern="action" />', ['data-button-pattern']],
  ['dynamic marker', '<button data-button-pattern={pattern} />', ['data-button-pattern']],
  ['bare marker', '<button data-button-pattern />', ['data-button-pattern']],
  ['spread does not prove marker', '<button {...props} />', ['data-button-pattern']],
  ['literal expression marker', '<button data-button-pattern={"row"} />', []],
  ['template literal marker', '<button data-button-pattern={`tile`} />', []],
  ['danger missing leading icon', '<Button variant="danger">Delete</Button>', ['leftIcon=']],
  ['danger expression', '<Button variant={"danger"} />', ['leftIcon=']],
  ['bare icon does not satisfy prop', '<Button variant="danger" leftIcon />', ['leftIcon=']],
  ['danger with icon', '<Button variant="danger" leftIcon={<Trash />} />', []],
  ['secondary does not need icon', '<Button variant="secondary" />', []],
  ['iconOnly missing label', '<Button iconOnly />', ['aria-label=']],
  ['iconOnly true', '<Button iconOnly={true} />', ['aria-label=']],
  ['iconOnly dynamic', '<Button iconOnly={compact} />', ['aria-label=']],
  ['iconOnly false', '<Button iconOnly={false} />', []],
  ['iconOnly parenthesized false', '<Button iconOnly={(false)} />', []],
  ['iconOnly with label', '<Button iconOnly aria-label="Close" />', []],
  ['iconOnly dynamic label', '<Button iconOnly aria-label={label} />', []],
  ['spread does not prove label', '<Button iconOnly {...props} />', ['aria-label=']],
  ['danger iconOnly requires both', '<Button variant="danger" iconOnly />', ['leftIcon=', 'aria-label=']],
  ['legacy base class', '<div className="m3-button" />', ['m3-button classes']],
  ['legacy variant class', '<a className="extra m3-button-filled other" />', ['m3-button classes']],
  ['legacy class helper', '<div className={cx("m3-button-tonal", active && "active")} />', ['m3-button classes']],
  ['legacy class variable', 'const classes = "m3-button-text"; const view = <a className={classes} />;', ['m3-button classes']],
  ['legacy dynamic template', '<a className={`m3-button-${variant}`} />', ['m3-button classes']],
  ['legacy template tail', '<a className={`${extra} m3-button-filled`} />', ['m3-button classes']],
  ['legacy object key', '<div className={cx({"m3-button-danger": active})} />', ['m3-button classes']],
  ['similarly named class ignored', '<div className="not-m3-button m3-buttons" />', []],
  ['comments and text ignored', '// <button className="m3-button" />\nconst view = <div>m3-button &lt;button&gt; {/* <button /> */}</div>;', []],
  ['nested JSX attribute', '<Button leftIcon={<button />} />', ['data-button-pattern']],
  ['multiline and quoted angle brackets', '<button\n title="a > b"\n data-button-pattern="card"\n onClick={() => act("<button>")}\n />', []],
];

for (const [name, source, reasons] of fixtures) {
  test(name, () => {
    const findings = checkSource(source);
    assert.equal(findings.length, reasons.length, JSON.stringify(findings));
    for (const reason of reasons) assert.ok(findings.some((finding) => finding.reason.includes(reason)), reason);
  });
}

for (const pattern of BUTTON_PATTERNS) {
  test(`allowed raw-button pattern: ${pattern}`, () => assert.deepEqual(checkSource(`<button data-button-pattern="${pattern}" />`), []));
}

for (const file of ['components/ui/Button.tsx', 'commandCenter/components/ui/Switch.tsx', 'tests/example.tsx', '__tests__/example.tsx', 'View.test.tsx', 'View.spec.tsx', 'mock/Example.tsx', 'commandCenter/mock/Example.tsx']) {
  test(`documented exemption: ${file}`, () => assert.deepEqual(checkSource('<button className="m3-button" />', file), []));
}

for (const file of ['components/Header.tsx', 'commandCenter/components/Header.tsx']) {
  test(`CIT-69 exempts only Header raw-button markers: ${file}`, () => {
    assert.deepEqual(checkSource('<button />', file), []);
    assert.equal(checkSource('<Button variant="danger" />', file).length, 1);
    assert.equal(checkSource('<button className="m3-button" />', file).length, 1);
  });
}

test('other headers are checked and findings name their actual line', () => {
  assert.deepEqual(checkSource('\n\n<button />', 'views/Header.tsx').map(({ file, line }) => ({ file, line })), [{ file: 'views/Header.tsx', line: 3 }]);
});

test('malformed TSX is reported rather than silently skipped', () => {
  assert.ok(checkSource('<button').some(({ reason }) => reason.startsWith('TSX parse error:')));
});

function withFixture(run) {
  const root = mkdtempSync(join(tmpdir(), 'cit65-buttons-'));
  const files = {
    'View.tsx': '\n<button />',
    'nested/View.tsx': '<Button variant="danger" />',
    'Valid.tsx': '<button data-button-pattern="nav" />',
    'components/ui/Button.tsx': '<button />',
    'components/Header.tsx': '<button />',
    'mock/Example.tsx': '<button />',
    'View.test.tsx': '<button />',
    'tests/Example.tsx': '<button />',
    'Ignored.ts': 'const text = "<button />";',
    'node_modules/ThirdParty.tsx': '<button />',
  };
  try {
    for (const [file, source] of Object.entries(files)) {
      const path = join(root, file);
      mkdirSync(join(path, '..'), { recursive: true });
      writeFileSync(path, source);
    }
    run(root);
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

const script = fileURLToPath(new URL('../scripts/check-raw-buttons.mjs', import.meta.url));
const cli = (...args) => spawnSync(process.execPath, [script, ...args], { encoding: 'utf8' });

test('recursive scan checks only non-exempt TSX files', () => withFixture((root) => {
  const { files, findings } = checkRoot(root);
  assert.equal(files, 4);
  assert.deepEqual(findings.map(({ file, line }) => ({ file, line })), [{ file: 'nested/View.tsx', line: 1 }, { file: 'View.tsx', line: 2 }]);
}));

test('report mode emits file:line findings and succeeds without changing fixtures', () => withFixture((root) => {
  const before = checkRoot(root);
  const result = cli(root, '--report');
  assert.equal(result.status, 0, result.stderr);
  assert.match(result.stdout, /View\.tsx:2 <button> needs/);
  assert.match(result.stdout, /nested\/View\.tsx:1 <Button variant="danger"> needs leftIcon=/);
  assert.match(result.stdout, /Raw-button report: 2 violation\(s\) in 4 TSX file\(s\)/);
  assert.match(result.stdout, /components\/Header\.tsx raw-button markers \(CIT-69/);
  assert.deepEqual(checkRoot(root), before);
}));

test('enforcement mode fails on the same findings', () => withFixture((root) => {
  const result = cli(root);
  assert.equal(result.status, 1, result.stderr);
  assert.match(result.stdout, /Raw-button check: 2 violation/);
}));

test('a clean root succeeds in both modes', () => {
  const root = mkdtempSync(join(tmpdir(), 'cit65-buttons-clean-'));
  try {
    writeFileSync(join(root, 'Valid.tsx'), '<button data-button-pattern="row" />');
    for (const args of [[root], ['--report', root]]) {
      const result = cli(...args);
      assert.equal(result.status, 0, result.stderr);
      assert.match(result.stdout, /0 violation/);
    }
  } finally { rmSync(root, { recursive: true, force: true }); }
});

test('invalid arguments and nonexistent roots fail even in report mode', () => {
  for (const args of [[], ['--report'], ['--unknown'], ['one', 'two'], ['/nonexistent/cit65-fixture', '--report']]) {
    const result = cli(...args);
    assert.equal(result.status, 2);
    assert.match(result.stderr, /Usage:|Raw-button checker:/);
  }
});
