import { spawnSync } from 'node:child_process';

const [mode, ...args] = process.argv.slice(2);
const updating = mode === 'update' || args.some((arg) => arg.startsWith('--update-snapshots') || arg === '-u');
if (updating && (process.platform !== 'darwin' || process.arch !== 'arm64')) {
  console.error('Visual baselines must be generated on the studio Mac (macOS arm64).');
  process.exit(1);
}
const options = mode === 'update' ? ['--update-snapshots=changed', ...args] : args;
const result = spawnSync(process.execPath, ['node_modules/@playwright/test/cli.js', 'test', ...options], {
  stdio: 'inherit',
});
if (result.error) console.error(result.error);
process.exitCode = result.status ?? 1;
