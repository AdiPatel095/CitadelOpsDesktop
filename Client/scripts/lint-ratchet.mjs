import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { dirname, relative, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const compilerRules = new Set([
  'react-hooks/preserve-manual-memoization',
  'react-hooks/use-memo',
  'react-hooks/immutability',
  'react-hooks/refs',
  'react-hooks/purity',
  'react-hooks/set-state-in-effect',
]);

export function evaluateLint(results, baseline, root = clientRoot) {
  if (baseline.version !== 1 || !baseline.files || typeof baseline.files !== 'object' || Array.isArray(baseline.files)) {
    throw new Error('Invalid lint baseline');
  }
  for (const [file, rules] of Object.entries(baseline.files)) {
    if (file !== relative(root, resolve(root, file)).split(sep).join('/') || file.startsWith('../') || !rules || typeof rules !== 'object' || Array.isArray(rules)) {
      throw new Error(`Invalid baseline file: ${file}`);
    }
    for (const [rule, count] of Object.entries(rules)) {
      if (!compilerRules.has(rule) || !Number.isInteger(count) || count <= 0) {
        throw new Error(`Invalid baseline count: ${file}: ${rule}`);
      }
    }
  }
  const files = {};
  const failures = [];
  let errors = 0;
  let warnings = 0;
  for (const result of results) {
    const file = relative(root, result.filePath).split(sep).join('/');
    for (const message of result.messages) {
      if (message.severity === 1) warnings += 1;
      if (message.severity !== 2) continue;
      errors += 1;
      if (message.fatal || !compilerRules.has(message.ruleId)) {
        failures.push(`${file}:${message.line ?? 0}: ${message.ruleId ?? 'fatal'}: ${message.message}`);
        continue;
      }
      files[file] ??= {};
      files[file][message.ruleId] = (files[file][message.ruleId] ?? 0) + 1;
    }
  }
  for (const [file, rules] of Object.entries(files)) {
    for (const [rule, count] of Object.entries(rules)) {
      const allowed = baseline.files[file]?.[rule] ?? 0;
      if (count > allowed) failures.push(`${file}: ${rule}: ${count} errors exceeds baseline ${allowed}`);
    }
  }
  return { errors, warnings, failures };
}

function main() {
  const require = createRequire(import.meta.url);
  const eslint = resolve(dirname(require.resolve('eslint/package.json')), 'bin/eslint.js');
  let output;
  try {
    output = execFileSync(process.execPath, [eslint, '.', '--format', 'json'], {
      cwd: clientRoot, encoding: 'utf8', maxBuffer: 32 * 1024 * 1024,
    });
  } catch (error) {
    // ESLint exits 1 for lint errors; exit 2 and execution failures must fail closed.
    if (error.status !== 1 || !error.stdout) throw error;
    output = error.stdout;
  }
  const baseline = JSON.parse(readFileSync(new URL('../lint-baseline.json', import.meta.url), 'utf8'));
  const { errors, warnings, failures } = evaluateLint(JSON.parse(output), baseline);
  console.log(`Full lint: ${errors} errors, ${warnings} warnings. Gate: ${failures.length} violations.`);
  for (const failure of failures) console.error(failure);
  process.exitCode = failures.length ? 1 : 0;
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  try { main(); } catch (error) {
    console.error(`Lint gate could not run: ${error.message}`);
    process.exitCode = 1;
  }
}
