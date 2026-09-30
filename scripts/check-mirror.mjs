import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

const args = process.argv.slice(2);
const option = (name) => { const index = args.indexOf(name); return index < 0 ? undefined : args[index + 1]; };
const desktop = option('--desktop');
const portal = option('--portal');
if (!desktop || !portal) throw new Error('Usage: check-mirror.mjs --desktop <repo> --portal <repo>');
const files = JSON.parse(readFileSync(fileURLToPath(new URL('../mirror-files.json', import.meta.url)), 'utf8'));
for (const file of files) {
  const desktopBytes = readFileSync(resolve(desktop, 'Client/src', file));
  const portalBytes = readFileSync(resolve(portal, 'src/commandCenter', file));
  if (!desktopBytes.equals(portalBytes)) throw new Error(`Mirror mismatch: ${file}`);
}
console.log(`Mirror verified: ${files.length} shared file(s).`);
