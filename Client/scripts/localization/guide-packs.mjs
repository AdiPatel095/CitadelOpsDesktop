#!/usr/bin/env node
import { createHash } from 'node:crypto';
import { readFile, rename, writeFile } from 'node:fs/promises';
import { resolve, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

export const registry = {
  autoTower: { source: 'autoTowerGuide.json', ui: 'tower', panels: 'tower' },
  autoBird: { source: 'autoBirdGuide.json', ui: 'bird', panels: 'bird' },
  autoStation: { source: 'autoStationGuide.json', ui: 'station', panels: 'station' },
  autoFortress: { source: 'autoFortressGuide.json', ui: 'fortress', panels: 'fortress' },
  autoInvasion: { source: 'autoInvasionGuide.json', ui: 'invasion', panels: 'invasion' },
  autoNomad: { source: 'autoNomadGuide.json', ui: 'nomad', panels: 'nomad' },
  autoAdvisor: { source: 'autoAdvisorGuide.json', ui: 'advisor', panels: 'advisor' },
  autoKhan: { source: 'autoKhanGuide.json', ui: 'khan', panels: 'khan' },
  autoBeri: { source: 'autoBeriGuide.json', ui: 'beri', panels: 'beri' },
  autoStorm: { source: 'autoStormGuide.json', ui: 'storm', panels: 'storm' },
};
const defaultRoot = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const hash = value => createHash('sha256').update(value).digest('hex');
const json = value => JSON.stringify(value, null, 2) + '\n';
const read = async path => JSON.parse(await readFile(path, 'utf8'));
const paths = root => ({ config: join(root, 'src/config'), locales: join(root, 'src/config/guideLocales') });
const packPath = (root, locale) => join(paths(root).locales, `${locale}.json`);
const progressPath = root => join(paths(root).locales, 'progress.json');
const provenancePath = root => join(paths(root).locales, 'provenance.json');
const get = (tree, path) => path.split('.').reduce((node, part) => node?.[part], tree);
const set = (tree, path, value) => {
  const parts = path.split('.');
  let node = tree;
  for (const part of parts.slice(0, -1)) node = node[part] ??= {};
  node[parts.at(-1)] = value;
};
const flatten = (tree, prefix, result = {}) => {
  if (typeof tree === 'string') { result[prefix] = tree; return result; }
  if (!tree || typeof tree !== 'object' || Array.isArray(tree)) throw Error(`Non-string guide field: ${prefix}`);
  for (const [key, value] of Object.entries(tree)) flatten(value, prefix ? `${prefix}.${key}` : key, result);
  return result;
};
const selectedEntries = (pack, feature) => {
  const spec = registry[feature];
  const entries = pack[feature] ? flatten(pack[feature], feature) : {};
  for (const [key, value] of Object.entries(pack.ui)) if (key.startsWith(spec.ui)) entries[`ui.${key}`] = value;
  for (const [key, value] of Object.entries(pack.panels)) if (key.startsWith(spec.panels)) flatten(value, `panels.${key}`, entries);
  return Object.fromEntries(Object.entries(entries).sort(([a], [b]) => a.localeCompare(b)));
};
const localeCodes = async root => {
  const text = await readFile(join(root, 'src/i18n/locales.ts'), 'utf8');
  const match = text.match(/localeCodes\s*=\s*\[([^\]]+)\]/);
  if (!match) throw Error('Configured localeCodes list is missing');
  return [...match[1].matchAll(/'([^']+)'/g)].map(item => item[1]);
};
const sourceContract = async (root, feature) => {
  if (!registry[feature]) throw Error(`Unknown guide feature: ${feature}`);
  const source = await read(join(paths(root).config, registry[feature].source));
  const english = await read(packPath(root, 'en'));
  const translated = english[feature];
  if (!translated || translated.recommendationIntro !== source.recommendationIntro ||
      JSON.stringify(Object.keys(translated.steps)) !== JSON.stringify(source.steps.map(step => step.id))) throw Error(`English ${feature} source/pack mismatch`);
  for (const step of source.steps) {
    const content = translated.steps[step.id];
    if (content.title !== step.title || JSON.stringify(Object.keys(content.items)) !== JSON.stringify(step.items.map(item => item.id))) throw Error(`English ${feature}.${step.id} mismatch`);
    for (const item of step.items) for (const field of ['label', 'description', 'recommendation']) {
      if ((item[field] ?? undefined) !== (content.items[item.id][field] ?? undefined)) throw Error(`English ${feature}.${step.id}.${item.id}.${field} mismatch`);
    }
    if (JSON.stringify(content.image ?? null) !== JSON.stringify(step.image ? { alt: step.image.alt, caption: step.image.caption } : null)) throw Error(`English ${feature}.${step.id}.image mismatch`);
  }
  const entries = selectedEntries(english, feature);
  const sourceSha256 = hash(JSON.stringify({ source, entries }));
  return { feature, sourceSha256, entries };
};
const validateTranslation = (contract, translation) => {
  if (translation.feature !== contract.feature || translation.sourceSha256 !== contract.sourceSha256) throw Error('Guide source hash or feature changed; export a fresh template');
  if (!translation.entries || typeof translation.entries !== 'object' || Array.isArray(translation.entries)) throw Error('Translation entries must be a flat object');
  const expected = Object.keys(contract.entries), actual = Object.keys(translation.entries);
  const missing = expected.filter(key => !actual.includes(key)), extra = actual.filter(key => !expected.includes(key));
  if (missing.length || extra.length) throw Error(`Guide paths missing: ${missing.join(', ') || 'none'}; extra: ${extra.join(', ') || 'none'}`);
  for (const [key, value] of Object.entries(translation.entries)) if (typeof value !== 'string' || !value.trim()) throw Error(`Blank or invalid guide translation: ${key}`);
  if (actual.every(key => translation.entries[key] === contract.entries[key])) throw Error('Untranslated English copy cannot be imported as a locale');
};
const loadProgress = async root => { try { return await read(progressPath(root)); } catch (error) { if (error.code === 'ENOENT') return { features: {} }; throw error; } };
const loadProvenance = async root => read(provenancePath(root));
const writeChanged = async (path, content) => {
  let old = null;
  try { old = await readFile(path, 'utf8'); } catch (error) { if (error.code !== 'ENOENT') throw error; }
  if (old === content) return false;
  const temporary = `${path}.guide-${process.pid}.tmp`;
  await writeFile(temporary, content);
  await rename(temporary, path);
  return true;
};
const contentHash = entries => hash(JSON.stringify(Object.fromEntries(Object.entries(entries).sort(([a], [b]) => a.localeCompare(b)))));
const withoutFeature = (pack, feature) => {
  const clone = structuredClone(pack);
  for (const key of Object.keys(selectedEntries({ ...clone, [feature]: clone[feature] ?? {} }, feature))) {
    const parts = key.split('.');
    let node = clone;
    for (const part of parts.slice(0, -1)) node = node?.[part];
    if (node) delete node[parts.at(-1)];
  }
  return clone;
};
export async function exportGuide(root, feature) { return sourceContract(root, feature); }
export async function importGuide(root, feature, locale, translation, mirror) {
  const codes = await localeCodes(root);
  if (locale === 'en' || !codes.includes(locale)) throw Error(`Unsupported non-English locale: ${locale}`);
  const contract = await sourceContract(root, feature);
  validateTranslation(contract, translation);
  const roots = mirror ? [root, mirror] : [root];
  const planned = [];
  for (const target of roots) {
    const own = await sourceContract(target, feature);
    if (own.sourceSha256 !== contract.sourceSha256 || JSON.stringify(own.entries) !== JSON.stringify(contract.entries)) throw Error('Mirror English guide source differs');
    const pack = await read(packPath(target, locale));
    for (const [key, value] of Object.entries(translation.entries)) set(pack, key, value);
    const progress = await loadProgress(target), provenance = await loadProvenance(target);
    progress.features ??= {};
    progress.features[feature] ??= { locales: {} };
    progress.features[feature].locales[locale] = { sourceSha256: contract.sourceSha256, contentSha256: contentHash(translation.entries) };
    provenance.locales[locale] = hash(json(pack));
    planned.push({ target, pack, progress, provenance });
  }
  if (planned.length === 2 && JSON.stringify(withoutFeature(planned[0].pack, feature)) !== JSON.stringify(withoutFeature(planned[1].pack, feature))) throw Error('Mirror has unrelated guide-pack differences');
  for (const item of planned) {
    await writeChanged(packPath(item.target, locale), json(item.pack));
    await writeChanged(progressPath(item.target), json(item.progress));
    await writeChanged(provenancePath(item.target), json(item.provenance));
  }
  return { locale, feature, sourceSha256: contract.sourceSha256, mirrors: planned.length };
}
export async function statusGuide(root, feature, mirror) {
  const contract = await sourceContract(root, feature), progress = await loadProgress(root);
  if (mirror && (await sourceContract(mirror, feature)).sourceSha256 !== contract.sourceSha256) throw Error('Mirror English guide source differs');
  const lines = [];
  for (const locale of (await localeCodes(root)).filter(code => code !== 'en')) {
    const pack = await read(packPath(root, locale));
    const entries = selectedEntries(pack, feature);
    const missing = Object.keys(contract.entries).filter(key => typeof entries[key] !== 'string' || !entries[key].trim());
    const saved = progress.features?.[feature]?.locales?.[locale];
    let state = missing.length ? 'missing' : !saved || saved.sourceSha256 !== contract.sourceSha256 || saved.contentSha256 !== contentHash(entries) ? 'stale' : 'complete';
    if (mirror && state === 'complete') {
      const other = await read(packPath(mirror, locale));
      if (JSON.stringify(pack) !== JSON.stringify(other)) state = 'stale';
    }
    lines.push({ locale, state, missing: missing.length });
  }
  return { feature, sourceSha256: contract.sourceSha256, entries: Object.keys(contract.entries).length, locales: lines };
}
export async function recordEnglish(root, feature, mirror) {
  const targets = mirror ? [root, mirror] : [root], source = await sourceContract(root, feature);
  for (const target of targets) {
    if ((await sourceContract(target, feature)).sourceSha256 !== source.sourceSha256) throw Error('Mirror English guide source differs');
    const provenance = await loadProvenance(target), progress = await loadProgress(target);
    provenance.source = 'autoTowerGuide.json, autoBirdGuide.json, autoStationGuide.json, autoFortressGuide.json and autoInvasionGuide.json, plus autoNomadGuide.json, autoAdvisorGuide.json, autoKhanGuide.json, autoBeriGuide.json and autoStormGuide.json; en.json fixes the semantic key contract.';
    provenance.scope = 'Auto Towers, Auto Bird, Auto Station, Auto Fortress, Auto Invasion, Auto Nomad/Samurai, Auto Advisor, Auto Khan, Auto Beri, and Auto Storm guides, feature-card and illustrative panel strings only; not whole-application localization.';
    provenance.englishSourceSha256 = provenance.locales.en = hash(await readFile(packPath(target, 'en')));
    progress.features ??= {};
    progress.features[feature] ??= { locales: {} };
    progress.features[feature].locales.en = { sourceSha256: source.sourceSha256, contentSha256: contentHash(source.entries) };
    await writeChanged(provenancePath(target), json(provenance));
    await writeChanged(progressPath(target), json(progress));
  }
  return { feature, sourceSha256: source.sourceSha256 };
}
async function main() {
  const [command, feature, ...options] = process.argv.slice(2);
  const flag = name => { const index = options.indexOf(name); return index < 0 ? undefined : options[index + 1]; };
  const root = resolve(flag('--root') ?? defaultRoot), mirror = flag('--mirror') ? resolve(flag('--mirror')) : undefined;
  if (!registry[feature]) throw Error(`Usage: guide-packs.mjs export|status|import|check|provenance autoTower|autoBird|autoStation|autoFortress|autoInvasion|autoNomad|autoAdvisor|autoKhan|autoBeri|autoStorm [locale file] [--root client] [--mirror client]`);
  if (command === 'export') { console.log(json(await exportGuide(root, feature)).trimEnd()); return; }
  if (command === 'status' || command === 'check') {
    const report = await statusGuide(root, feature, mirror);
    console.log(json(report).trimEnd());
    if (command === 'check' && report.locales.some(item => item.state !== 'complete')) process.exitCode = 1;
    return;
  }
  if (command === 'provenance') { console.log(json(await recordEnglish(root, feature, mirror)).trimEnd()); return; }
  if (command === 'import') {
    const [locale, file] = options;
    if (!file || file.startsWith('--')) throw Error('Import requires locale and translation JSON path');
    console.log(json(await importGuide(root, feature, locale, await read(resolve(file)), mirror)).trimEnd());
    return;
  }
  throw Error(`Unknown guide command: ${command}`);
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) main().catch(error => { console.error(error.message); process.exitCode = 1; });
