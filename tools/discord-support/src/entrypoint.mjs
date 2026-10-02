import { realpathSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
// Node canonicalizes module URLs while argv can retain the installed service
// symlink. Compare filesystem identities after decoding the file URL.
export function isDirectEntry(moduleUrl,entryScript = process.argv[1]) {
  if (!entryScript) return false;
  try { return realpathSync(fileURLToPath(moduleUrl)) === realpathSync(entryScript); }
  catch { return false; } // Imports and non-file/unresolvable argv are inactive.
}
