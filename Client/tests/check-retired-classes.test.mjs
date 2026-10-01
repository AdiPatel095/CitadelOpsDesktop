import assert from 'node:assert/strict';
import { copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { spawnSync } from 'node:child_process';
import test from 'node:test';

function fixture(run) {
  const root = mkdtempSync(join(tmpdir(), 'cit-retired-classes-'));
  mkdirSync(join(root, 'scripts'));
  mkdirSync(join(root, 'src'));
  copyFileSync(new URL('../scripts/check-retired-classes.mjs', import.meta.url), join(root, 'scripts/check-retired-classes.mjs'));
  const write = (path, content) => {
    const full = join(root, 'src', path);
    mkdirSync(join(full, '..'), { recursive: true });
    writeFileSync(full, content);
  };
  const check = (...args) => spawnSync(process.execPath, [join(root, 'scripts/check-retired-classes.mjs'), ...args], { cwd: tmpdir(), encoding: 'utf8' });
  try {
    run({ write, check });
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

test('report mode finds class strings, templates, CSS selectors and legacy variables', () => fixture(({ write, check }) => {
  write('Cards.tsx', '<div className="liquid-prominent-header-card m3-card-interactive liquid-card-header-prominent" />\nconst classes = `m3-section-card liquid-toggle-selected`;\n');
  write('cards.css', '.liquid-pill-selector { border-radius: var(--md-expressive-shape-card-alt); }\n.m3-empty-state {}\n');
  const result = check();
  assert.equal(result.status, 0, result.stderr);
  for (const expected of [
    'src/Cards.tsx:1 cit-67 liquid-prominent-header',
    'src/Cards.tsx:1 cit-67 m3-card',
    'src/Cards.tsx:1 cit-67 liquid-card-header-prominent',
    'src/Cards.tsx:2 cit-67 m3-section-card',
    'src/Cards.tsx:2 cit-73 liquid-toggle',
    'src/cards.css:1 cit-67 md-expressive-shape-card-alt',
    'src/cards.css:1 cit-73 liquid-pill-selector',
    'src/cards.css:2 cit-73 m3-empty-state',
    'cit-67: 5 matches', 'cit-73: 3 matches',
  ]) assert.ok(result.stdout.includes(expected), expected);
  assert.equal(check('--enforce', 'cit-67').status, 1);
  assert.equal(check('--enforce', 'cit-73').status, 1);
  assert.equal(check('--enforce', 'cit-67,cit-73').status, 1);
}));

test('token aliases, tests, unsupported extensions and non-boundary prefixes are skipped', () => fixture(({ write, check }) => {
  for (const path of ['styles/tokens.css', 'commandCenter/styles/tokens.css', 'card.test.tsx', 'card.spec.ts', 'tests/card.tsx', '__tests__/card.css', 'test/card.ts', 'card.js', 'card.md']) write(path, 'm3-card liquid-toggle');
  write('safe.ts', 'const value = "xm3-card _m3-card 2m3-card aliqliquid-toggle _liquid-toggle 9liquid-toggle";');
  const result = check('--enforce', 'cit-67,cit-73');
  assert.equal(result.status, 0, result.stderr);
  assert.equal(result.stdout, 'cit-67: 0 matches\ncit-73: 0 matches\n');
}));

test('enforcement applies only to selected rules and rejects unknown rules', () => fixture(({ write, check }) => {
  write('state.tsx', '<div className="m3-empty-state" />');
  assert.equal(check('--enforce', 'cit-67').status, 0);
  assert.equal(check('--enforce', 'cit-73').status, 1);
  for (const args of [['--enforce', 'unknown'], ['--enforce', 'cit-67,unknown'], ['--enforce'], ['--invalid'], ['--enforce', '']]) {
    assert.equal(check(...args).status, 2, args.join(' '));
  }
}));
