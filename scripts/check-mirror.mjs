import { readFileSync, readdirSync, statSync } from 'node:fs';
import { resolve } from 'node:path';
import { createHash } from 'node:crypto';
import { parseArgs } from 'node:util';
import { fileURLToPath } from 'node:url';

const { values } = parseArgs({ options: { desktop: { type: 'string' }, portal: { type: 'string' } } });
const portalRoot = resolve(values.portal ?? '.');
const desktopRoot = values.desktop && resolve(values.desktop);
if (!desktopRoot) throw new Error('Usage: npm run check:mirror -- --desktop <desktop checkout> [--portal <portal checkout>]');
for (const [name, checkout, source] of [['desktop', desktopRoot, 'Client/src'], ['portal', portalRoot, 'src/commandCenter']]) {
  if (!statSync(resolve(checkout, source), { throwIfNoEntry: false })?.isDirectory()) {
    console.error(`Missing ${name} checkout: ${checkout}. Expected a sibling repository; use --${name} <repo> to select another checkout.`);
    process.exit(1);
  }
}
const sha = (bytes) => createHash('sha256').update(bytes).digest('hex');
const manifest = JSON.parse(readFileSync(fileURLToPath(new URL('../mirror-files.json', import.meta.url)), 'utf8'));
const pairKey = ({ portal, desktop }) => JSON.stringify([portal, desktop]);
const listed = new Set(manifest.files.map(pairKey));
const unmirrored = new Set();
for (const file of manifest.unmirrored) {
  if (typeof file.reason !== 'string' || !file.reason.trim() || /[\r\n]/.test(file.reason)) {
    throw new Error(`Unmirrored pair needs a one-line reason: ${file.portal} <-> ${file.desktop}`);
  }
  unmirrored.add(pairKey(file));
}
let drift = 0;
for (const file of manifest.files) {
  const portal = readFileSync(resolve(portalRoot, file.portal));
  const desktop = readFileSync(resolve(desktopRoot, file.desktop));
  if (!portal.equals(desktop)) {
    console.error(`Mirror drift: ${file.portal} (${sha(portal)}) != ${file.desktop} (${sha(desktop)})`);
    drift++;
  } else {
    console.log(`${file.portal} <-> ${file.desktop}: SHA-256 ${sha(portal)} (byte-identical)`);
  }
}

// Generated localization data is checked by the localization pipeline.
const excluded = ['i18n/catalogs/', 'i18n/officialBundled/', 'i18n/server/', 'i18n/backend/'];
function* filesUnder(root, relative = '', exclusions = []) {
  for (const entry of readdirSync(resolve(root, relative), { withFileTypes: true })) {
    const path = relative ? `${relative}/${entry.name}` : entry.name;
    if (exclusions.some((prefix) => `${path}/`.startsWith(prefix))) continue;
    if (entry.isDirectory()) yield* filesUnder(root, path, exclusions);
    else if (entry.isFile()) yield path;
  }
}

const candidates = [];
for (const [portalSource, desktopSource, exclusions] of [
  ['src/commandCenter', 'Client/src', excluded],
  ['src/commandCenter/mock/onboarding', 'Client/tests/onboarding-browser', []],
]) {
  for (const relative of filesUnder(resolve(desktopRoot, desktopSource), '', exclusions)) {
    const file = { portal: `${portalSource}/${relative}`, desktop: `${desktopSource}/${relative}` };
    if (statSync(resolve(portalRoot, file.portal), { throwIfNoEntry: false })?.isFile()) candidates.push(file);
  }
}
for (const name of ['OnboardingCoverage.md', 'OnboardingEvaluation.md']) {
  const path = `Docs/${name}`;
  if ([portalRoot, desktopRoot].every((root) => statSync(resolve(root, path), { throwIfNoEntry: false })?.isFile())) {
    candidates.push({ portal: path, desktop: path });
  }
}

let unlisted = 0;
for (const file of candidates.sort((a, b) => a.portal < b.portal ? -1 : a.portal > b.portal ? 1 : 0)) {
  if (listed.has(pairKey(file)) || unmirrored.has(pairKey(file))) continue;
  const portal = readFileSync(resolve(portalRoot, file.portal));
  const desktop = readFileSync(resolve(desktopRoot, file.desktop));
  if (portal.equals(desktop)) {
    console.error(`Mirror completeness: unlisted identical pair ${file.portal} <-> ${file.desktop}: SHA-256 ${sha(portal)}`);
    unlisted++;
  }
}
console.log(`${manifest.files.length} pairs, ${drift ? `${drift} with drift` : 'all identical'}, ${unlisted} unlisted`);
if (drift || unlisted) process.exit(1);
