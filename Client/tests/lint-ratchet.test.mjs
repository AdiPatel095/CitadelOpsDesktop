import assert from 'node:assert/strict';
import { test } from 'node:test';
import { evaluateLint } from '../scripts/lint-ratchet.mjs';

const root = '/fixture/Client';
const rule = 'react-hooks/set-state-in-effect';
const baseline = { version: 1, files: { 'src/Existing.tsx': { [rule]: 2 } } };
const result = (file, ruleId = rule, count = 1, severity = 2) => ({
  filePath: `${root}/${file}`,
  messages: Array.from({ length: count }, () => ({ ruleId, severity, line: 10, message: 'fixture error' })),
});
const evaluate = (results) => evaluateLint(results, baseline, root);

test('existing compiler errors and warnings pass; reductions also pass', () => {
  assert.deepEqual(evaluate([result('src/Existing.tsx', rule, 2), result('src/Existing.tsx', 'react-hooks/exhaustive-deps', 3, 1)]), {
    errors: 2, warnings: 3, failures: [],
  });
  assert.equal(evaluate([result('src/Existing.tsx')]).failures.length, 0);
  assert.equal(evaluate([]).failures.length, 0);
});

test('new file and per-file increases cannot consume another file allowance', () => {
  assert.equal(evaluate([result('src/Existing.tsx', rule, 3)]).failures.length, 1);
  assert.equal(evaluate([result('src/Existing.tsx'), result('src/New.tsx')]).failures.length, 1);
});

test('per-rule increases cannot consume another rule allowance', () => {
  assert.equal(evaluate([result('src/Existing.tsx', 'react-hooks/purity')]).failures.length, 1);
});

test('non-compiler errors, other hooks rules, and fatal parse errors always block', () => {
  for (const otherRule of ['prefer-const', 'react-hooks/rules-of-hooks', null]) {
    assert.equal(evaluate([result('src/Existing.tsx', otherRule)]).failures.length, 1);
  }
  const parsed = result('src/Existing.tsx');
  parsed.messages[0].fatal = true;
  assert.equal(evaluate([parsed]).failures.length, 1);
});

test('malformed or expanded baseline fails closed', () => {
  for (const invalid of [
    {}, { version: 1, files: [] },
    { version: 1, files: { '../Elsewhere.tsx': { [rule]: 2 } } },
    { version: 1, files: { 'src/A.tsx': { 'prefer-const': 1 } } },
    { version: 1, files: { 'src/A.tsx': { [rule]: -1 } } },
  ]) assert.throws(() => evaluateLint([], invalid, root), /Invalid/);
});
