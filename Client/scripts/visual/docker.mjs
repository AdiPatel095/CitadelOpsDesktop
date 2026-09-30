import { spawnSync } from 'node:child_process';
import { fileURLToPath } from 'node:url';

const root = fileURLToPath(new URL('../../', import.meta.url));
const result = spawnSync('docker', [
  'run', '--rm', '--platform', 'linux/amd64', '--ipc=host',
  '-e', 'PLAYWRIGHT_CONTAINER=1', '-v', `${root}:/work`, '-v', '/work/node_modules', '-w', '/work',
  'mcr.microsoft.com/playwright:v1.63.0-noble',
  'bash', '-lc', 'npm ci && npm run test:visual -- "$@"', 'visual', ...process.argv.slice(2),
], { stdio: 'inherit' });
if (result.error) console.error(result.error);
process.exitCode = result.status ?? 1;
