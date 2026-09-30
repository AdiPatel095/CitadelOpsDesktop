import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

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
  const event = JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH, 'utf8'));
  const pr = event.pull_request;
  if (!pr) throw new Error('Baseline guard requires a pull_request event.');
  const files = execFileSync('git', ['diff', '--name-only', '-z', `${pr.base.sha}...${pr.head.sha}`], {
    encoding: 'utf8',
  }).split('\0').filter(Boolean);
  const errors = checkBaselineChanges(files, pr.body);
  for (const error of errors) console.error(error);
  process.exitCode = errors.length ? 1 : 0;
}
