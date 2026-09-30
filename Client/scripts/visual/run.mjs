import { spawnSync } from 'node:child_process';

const [mode, ...args] = process.argv.slice(2);
const env = { ...process.env };
if (mode === 'host') env.VISUAL_HOST = '1';
const options = mode === 'update' ? ['--update-snapshots=changed', ...args] : args;
const result = spawnSync(process.execPath, ['node_modules/@playwright/test/cli.js', 'test', ...options], {
  stdio: 'inherit', env,
});
if (result.error) console.error(result.error);
process.exitCode = result.status ?? 1;
