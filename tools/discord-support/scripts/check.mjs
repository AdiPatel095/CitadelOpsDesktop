import { readdirSync } from 'node:fs';
import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';
import { join } from 'node:path';
const base=fileURLToPath(new URL('../',import.meta.url));
for (const directory of ['src','scripts','test']) for (const file of readdirSync(join(base,directory)).filter(f => f.endsWith('.mjs'))) {
  const result=spawnSync(process.execPath,['--check',join(base,directory,file)],{stdio:'inherit'});
  if (result.status !== 0) process.exit(1);
}
process.stdout.write('Syntax checks passed\n');
