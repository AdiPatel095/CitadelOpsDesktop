import { readFileSync, statSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const args = process.argv.slice(2);
const option = (name) => { const index = args.lastIndexOf(name); return index < 0 ? undefined : args[index + 1]; };
const desktop = option('--desktop');
const portal = option('--portal');
if (!desktop || !portal) throw new Error('Usage: check-mirror.mjs --desktop <repo> --portal <repo>');
for (const [name, checkout, source] of [['desktop', desktop, 'Client/src'], ['portal', portal, 'src/commandCenter']]) {
  if (!statSync(resolve(checkout, source), { throwIfNoEntry: false })?.isDirectory()) {
    console.error(`Missing ${name} checkout: ${resolve(checkout)}. Expected a sibling repository; use --${name} <repo> to select another checkout.`);
    process.exit(1);
  }
}
const files = JSON.parse(readFileSync(fileURLToPath(new URL('../mirror-files.json', import.meta.url)), 'utf8'));
for (const file of files) {
  const desktopBytes = readFileSync(resolve(desktop, 'Client/src', file));
  const portalBytes = readFileSync(resolve(portal, 'src/commandCenter', file));
  if (!desktopBytes.equals(portalBytes)) throw new Error(`Mirror mismatch: ${file}`);
}
console.log(`Mirror verified: ${files.length} shared file(s).`);
