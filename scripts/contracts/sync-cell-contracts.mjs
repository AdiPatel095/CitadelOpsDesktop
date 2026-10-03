import { readFileSync, readdirSync, mkdirSync, copyFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { parseArgs } from 'node:util';
import { spawnSync } from 'node:child_process';

const { values } = parseArgs({ options: { backend: { type: 'string' }, check: { type: 'boolean', default: false } } });
const desktop = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const backend = resolve(values.backend ?? resolve(desktop, '../CitadelOpsBackend'));
const workerFixtures = 'Server/Accounts/testdata/contracts/worker';
const backendFixtures = 'hosted/testdata/contracts/backend';

function go(root, pkg, test, update = false) {
  const args = ['test', pkg, '-run', `^${test}$`, '-count=1', ...(update ? ['-update'] : [])];
  const result = spawnSync('go', args, { cwd: root, stdio: 'inherit' });
  if (result.error) throw result.error;
  if (result.status !== 0) throw new Error(`go ${args.join(' ')} failed in ${root}`);
}

try {
  // In check mode the generators compare in memory and never write files.
  go(desktop, './Server/Accounts', 'TestGenerateCellContracts', !values.check);
  go(backend, './hosted', 'TestGenerateCellContracts', !values.check);
  for (const [source, target] of [
    [resolve(desktop, workerFixtures), resolve(backend, 'hosted/testdata/contracts/worker')],
    [resolve(backend, backendFixtures), resolve(desktop, 'Server/Accounts/testdata/contracts/backend')],
  ]) {
    const files = readdirSync(source).sort();
    if (!values.check) mkdirSync(target, { recursive: true });
    const targetFiles = readdirSync(target).sort();
    if (targetFiles.some((file) => !files.includes(file))) throw new Error(`Unexpected copied fixture in ${target}`);
    for (const file of files) {
      if (values.check) {
        if (!readFileSync(resolve(source, file)).equals(readFileSync(resolve(target, file)))) throw new Error(`Contract mirror drift: ${file}`);
      } else copyFileSync(resolve(source, file), resolve(target, file));
    }
  }
  go(desktop, './Server/Accounts', 'TestReceiveCellContracts');
  go(backend, './hosted', 'TestReceiveCellContracts');
  console.log(`Cell contracts ${values.check ? 'checked' : 'synced'} successfully.`);
} catch (error) {
  console.error(error.message);
  process.exitCode = 1;
}
