import { readFileSync, readdirSync } from 'node:fs';
import { relative, resolve } from 'node:path';
import { parseArgs } from 'node:util';
import { fileURLToPath } from 'node:url';

const rules = {
  'cit-67': ['liquid-prominent-header', 'liquid-card-header-prominent', 'm3-section-card', 'm3-card', 'md-expressive-shape-card-alt'],
  'cit-73': ['liquid-toggle', 'liquid-pill-selector', 'm3-empty-state'],
};
let enforced;
try {
  const { values } = parseArgs({ options: { enforce: { type: 'string' } } });
  enforced = values.enforce === undefined ? [] : values.enforce.split(',');
  if (enforced.some((rule) => !Object.hasOwn(rules, rule))) throw new Error('Unknown rule');
} catch {
  console.error('Usage: node scripts/check-retired-classes.mjs [--enforce cit-67[,cit-73]]');
  process.exit(2);
}

const root = fileURLToPath(new URL('..', import.meta.url));
const counts = Object.fromEntries(Object.keys(rules).map((rule) => [rule, 0]));
function scan(directory) {
  for (const entry of readdirSync(directory, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
    const path = resolve(directory, entry.name);
    if (entry.isDirectory()) {
      if (!['test', 'tests', '__tests__', 'node_modules'].includes(entry.name)) scan(path);
      continue;
    }
    const file = relative(root, path).replaceAll('\\', '/');
    if (!entry.isFile() || !/\.(ts|tsx|css)$/.test(file) || /\.(test|spec)\.(ts|tsx|css)$/.test(file) || file.endsWith('/styles/tokens.css')) continue;
    const text = readFileSync(path, 'utf8');
    for (const [rule, prefixes] of Object.entries(rules)) {
      const pattern = new RegExp(`(?<![A-Za-z0-9_])(?:${prefixes.join('|')})`, 'g');
      for (const match of text.matchAll(pattern)) {
        const line = text.slice(0, match.index).split('\n').length;
        console.log(`${file}:${line} ${rule} ${match[0]}`);
        counts[rule] += 1;
      }
    }
  }
}
scan(resolve(root, 'src'));
for (const [rule, count] of Object.entries(counts)) console.log(`${rule}: ${count} matches`);
if (enforced.some((rule) => counts[rule] > 0)) process.exitCode = 1;
