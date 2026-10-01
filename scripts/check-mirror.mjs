import { readFileSync, statSync } from 'node:fs';
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
for (const file of JSON.parse(readFileSync(fileURLToPath(new URL('../mirror-files.json', import.meta.url)), 'utf8'))) {
  const portal = readFileSync(resolve(portalRoot, file.portal));
  const desktop = readFileSync(resolve(desktopRoot, file.desktop));
  if (!portal.equals(desktop)) { console.error(`Mirror drift: ${file.portal} (${sha(portal)}) != ${file.desktop} (${sha(desktop)})`); process.exit(1); }
  console.log(`${file.portal}: SHA-256 ${sha(portal)} (byte-identical)`);
}
