import { execFileSync } from 'node:child_process';
import { existsSync, mkdirSync, writeFileSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';
import ts from 'typescript';

const git = (root, ...args) => execFileSync('git', ['-C', root, ...args], { encoding: 'utf8', maxBuffer: 16 * 1024 * 1024 });

/** Read literal source assignments without evaluating imports or repository code. */
export function sourceAssignments(text, name) {
  const file = ts.createSourceFile('catalog.ts', text, ts.ScriptTarget.Latest, true);
  if (file.parseDiagnostics.length) throw new Error(`${name}: invalid source syntax`);
  let result;
  function visit(node) {
    if (ts.isVariableDeclaration(node) && node.name.getText(file) === name) {
      let value = node.initializer;
      while (value && (ts.isAsExpression(value) || ts.isSatisfiesExpression(value) || ts.isParenthesizedExpression(value))) value = value.expression;
      if (!value || !ts.isObjectLiteralExpression(value)) throw new Error(`${name}: expected a literal catalog`);
      result = {};
      for (const property of value.properties) {
        if (ts.isSpreadAssignment(property)) continue; // sourceMessages is read separately.
        if (!ts.isPropertyAssignment(property) || !ts.isStringLiteralLike(property.initializer)) throw new Error(`${name}: expected literal message values`);
        const key = property.name.text;
        if (typeof key !== 'string') throw new Error(`${name}: expected a literal message key`);
        result[key] = property.initializer.text;
      }
    }
    ts.forEachChild(node, visit);
  }
  visit(file);
  if (!result) throw new Error(`Catalog declaration not found: ${name}`);
  return result;
}

export function changedIncompleteKeys(before, after, catalogs) {
  return Object.keys(after).filter(key => before[key] !== after[key]
    && catalogs.some(catalog => typeof catalog[key] !== 'string' || !catalog[key].trim())).sort();
}

function at(root, revision, file) {
  if (!git(root, 'ls-tree', '--name-only', revision, '--', file).trim()) return undefined;
  return git(root, 'show', `${revision}:${file}`);
}

function source(root, revision, files) {
  return Object.assign({}, ...files.map(([file, declaration]) => {
    const text = at(root, revision, file);
    return text === undefined ? {} : declaration ? sourceAssignments(text, declaration) : JSON.parse(text);
  }));
}

function family(root, cut, localeCodes, files, catalogDirectory, repository) {
  const before = source(root, cut.since, files);
  const after = source(root, cut.until, files);
  const catalogs = localeCodes.map(locale => {
    const text = at(root, cut.until, `${catalogDirectory}/${locale}.json`);
    return text === undefined ? {} : JSON.parse(text);
  });
  return { repository, sources: files.map(([file]) => file), catalogDirectory,
    keys: changedIncompleteKeys(before, after, catalogs), allowEnglish: {} };
}

export function generateBatch({ batch, repositories, command, families }) {
  families ??= [...(repositories.desktop ? ['shared', 'runtime'] : []), ...(repositories.portal ? ['portal'] : [])];
  if (!families.length || families.some(name => !['shared', 'runtime', 'portal'].includes(name))) throw new Error('Unknown or empty family selection');
  if (families.some(name => !(name === 'portal' ? repositories.portal : repositories.desktop))) throw new Error('Selected family requires its source repository; supply --peer-repo and both peer cuts');
  const fixture = { schema: 1, batch, cuts: {}, command, families: {} };
  const resolved = {};
  for (const [name, repo] of Object.entries(repositories)) {
    const root = git(repo.root, 'rev-parse', '--show-toplevel').trim();
    const cut = Object.fromEntries(['since', 'until'].map(key => [key, git(root, 'rev-parse', '--verify', `${repo[key]}^{commit}`).trim()]));
    git(root, 'merge-base', '--is-ancestor', cut.since, cut.until);
    fixture.cuts[name] = cut;
    resolved[name] = { root, cut };
  }
  const desktop = resolved.desktop;
  const portal = resolved.portal;
  const canonical = desktop ?? portal;
  const base = desktop ? 'Client/src' : 'src/commandCenter';
  const localeSource = at(canonical.root, canonical.cut.until, `${base}/i18n/locales.ts`);
  const match = localeSource?.match(/localeCodes\s*=\s*\[([^\]]+)\]/);
  if (!match) throw new Error('Supported locale list not found');
  const locales = [...match[1].matchAll(/['"]([^'"]+)['"]/g)].map(value => value[1]).filter(code => code !== 'en');
  if (desktop && families.includes('shared')) {
    fixture.families.shared = family(desktop.root, desktop.cut, locales,
      [['Client/src/i18n/messages.ts', 'messages'], ['Client/src/i18n/sourceMessages.ts', 'sourceMessages']],
      'Client/src/i18n/catalogs', 'desktop');
  }
  if (desktop && families.includes('runtime')) {
    fixture.families.runtime = family(desktop.root, desktop.cut, locales,
      [['Server/Localization/en.json']], 'Server/Localization/locales', 'desktop');
  }
  if (portal && families.includes('portal')) fixture.families.portal = family(portal.root, portal.cut, locales,
    [['src/i18n/portal/en.json']], 'src/i18n/portal', 'portal');
  return fixture;
}

function main() {
  const { values } = parseArgs({ options: Object.fromEntries(
    ['batch', 'since', 'until', 'peer-repo', 'peer-since', 'peer-until', 'families', 'output'].map(key => [key, { type: 'string' }])) });
  if (!/^[1-9]\d*$/.test(values.batch ?? '') || !values.since || !values.until) throw new Error('Required: --batch N --since <previous cut> --until <this cut>');
  const root = git(fileURLToPath(new URL('../..', import.meta.url)), 'rev-parse', '--show-toplevel').trim();
  const name = existsSync(path.join(root, 'Client/package.json')) ? 'desktop' : 'portal';
  const repositories = { [name]: { root, since: values.since, until: values.until } };
  if (values['peer-repo']) {
    if (!values['peer-since'] || !values['peer-until']) throw new Error('Peer repository requires both peer cuts');
    repositories[name === 'desktop' ? 'portal' : 'desktop'] = {
      root: values['peer-repo'], since: values['peer-since'], until: values['peer-until'],
    };
  } else if (values['peer-since'] || values['peer-until']) throw new Error('Peer cuts require --peer-repo');
  const fixture = generateBatch({ batch: Number(values.batch), repositories, families: values.families?.split(','),
    command: ['node', 'scripts/localization/translation-batch.mjs', ...process.argv.slice(2)] });
  const client = name === 'desktop' ? path.join(root, 'Client') : root;
  const output = values.output ?? path.join(client, `tests/fixtures/translation-batches/batch-${values.batch}.json`);
  mkdirSync(path.dirname(output), { recursive: true });
  writeFileSync(output, `${JSON.stringify(fixture, null, 2)}\n`);
  console.log(`Batch ${values.batch}: ${Object.entries(fixture.families).map(([name, family]) => `${name}=${family.keys.length}`).join(', ')} → ${output}`);
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main();
