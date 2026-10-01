import { readFileSync, readdirSync } from 'node:fs';
import { execFileSync } from 'node:child_process';
import { join } from 'node:path';
import { parseArgs } from 'node:util';
import { pathToFileURL } from 'node:url';

export function cssMetrics(source) {
  const css = source.replace(/\/\*[\s\S]*?\*\//g, '');
  const declarations = [...css.matchAll(/([\w-]+)\s*:\s*([^;{}]+)(?:;|(?=\}))/g)];
  const values = (pattern) => [...new Set(declarations.filter((d) => pattern.test(d[1])).map((d) => d[2].trim()))].sort();
  return {
    lines: source.split('\n').length - Number(source.endsWith('\n')),
    important: (css.match(/!important\b/gi) ?? []).length,
    hex: [...new Set((css.match(/#[\da-f]{3,8}\b/gi) ?? []).map((s) => s.toLowerCase()))].sort(),
    rgb: [...new Set((css.match(/rgba?\([^)]*\)/gi) ?? []).map((s) => s.replace(/\s+/g, ' ').toLowerCase()))].sort(),
    radii: values(/^(?:border(?:-[\w-]+)?-radius|--radius[\w-]*)$/),
    fontSizes: values(/^(?:font-size|--font-size[\w-]*)$/),
    fontWeights: values(/^(?:font-weight|--font-weight[\w-]*)$/),
  };
}
function cssFiles(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => entry.isDirectory() ? cssFiles(join(dir, entry.name)) : entry.name.endsWith('.css') ? [join(dir, entry.name)] : []);
}
if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const { values } = parseArgs({ options: { ref: { type: 'string' }, json: { type: 'boolean' } } });
  const prefix = execFileSync('git', ['rev-parse', '--show-prefix'], { encoding: 'utf8' }).trim();
  const gitRoot = execFileSync('git', ['rev-parse', '--show-toplevel'], { encoding: 'utf8' }).trim();
  const files = values.ref ? execFileSync('git', ['ls-tree', '-r', '--name-only', values.ref, '--', `${prefix}src`], { encoding: 'utf8', cwd: gitRoot }).trim().split('\n').filter((f) => f.endsWith('.css')).map((f) => f.slice(prefix.length)) : cssFiles('src');
  const metrics = Object.fromEntries(files.sort().map((file) => [file, cssMetrics(values.ref ? execFileSync('git', ['show', `${values.ref}:${prefix}${file}`], { encoding: 'utf8' }) : readFileSync(file, 'utf8'))]));
  if (values.json) console.log(JSON.stringify(metrics, null, 2));
  else {
    console.log('| File | Lines | !important | Hex | RGB | Radii | Font sizes | Font weights |\n|---|---:|---:|---:|---:|---:|---:|---:|');
    for (const [file, m] of Object.entries(metrics)) console.log(`| ${file} | ${m.lines} | ${m.important} | ${m.hex.length} | ${m.rgb.length} | ${m.radii.length} | ${m.fontSizes.length} | ${m.fontWeights.length} |`);
  }
}
