import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
import { parseArgs } from 'node:util';

export function checkBaselineChanges(files, body) {
  const changed = files.filter((file) => file.startsWith('Client/tests/visual/__screenshots__/'));
  if (changed.length === 0) return [];
  const section = /(?:^|\n)#{1,6}\s+Visual baseline changes[^\n]*\n([\s\S]*?)(?=\n#{1,6}\s|$)/i.exec(body ?? '')?.[1];
  if (!section) return ['Missing Visual baseline changes section.'];
  return changed.filter((file) => {
    const escaped = path.basename(file).replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    return !new RegExp(`(?:^|[\\s\x60/])${escaped}(?=[\\s\x60:—–-]|$)`, 'm').test(section);
  }).map((file) => `Baseline not named in Visual baseline changes: ${file}`);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const { values } = parseArgs({ options: { base: { type: 'string' }, 'body-file': { type: 'string' } } });
  if (!values.base || !values['body-file']) throw new Error('Usage: --base origin/develop --body-file <file>');
  const files = execFileSync('git', ['diff', '--name-only', '-z', `${values.base}...HEAD`], {
    encoding: 'utf8',
  }).split('\0').filter(Boolean);
  const errors = checkBaselineChanges(files, readFileSync(values['body-file'], 'utf8'));
  for (const error of errors) console.error(error);
  process.exitCode = errors.length ? 1 : 0;
}
