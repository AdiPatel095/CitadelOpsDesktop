import { spawn } from 'node:child_process';

const port = process.argv[2] ?? '41736';
if (!/^\d+$/.test(port)) throw new Error('Usage: serve.mjs [port]');

const server = spawn(process.execPath, [
  'node_modules/vite/bin/vite.js', '--config', 'tests/onboarding-browser/vite.config.ts',
  '--host', '127.0.0.1', '--port', port, '--strictPort',
], { stdio: 'inherit' });
for (const signal of ['SIGINT', 'SIGTERM']) process.on(signal, () => server.kill(signal));
server.on('error', (error) => { console.error(error); process.exitCode = 1; });
server.on('exit', (code) => { process.exitCode = code ?? 1; });
