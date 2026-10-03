import { execSync } from 'node:child_process';
import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import tailwindcss from '@tailwindcss/vite';

const harnessRoot = fileURLToPath(new URL('.', import.meta.url));
const clientRoot = fileURLToPath(new URL('../..', import.meta.url));

const candidate = (() => {
  try { return execSync('git rev-parse --short HEAD', { cwd: clientRoot }).toString().trim(); } catch { return 'unknown'; }
})();

/**
 * Onboarding preview harness (CIT-22): the production UI with fixture data and no Go server. It needs no proxy and no
 * environment: the harness page installs a fixture transport that answers every `/api/v2/*` request in the browser.
 * Run it with `npm run preview:onboarding` (port 41734).
 */
export default defineConfig({
  root: harnessRoot,
  cacheDir: fileURLToPath(new URL('../../.visual/vite-cache', import.meta.url)),
  // The harness page lives outside `Client/`, so the game's static images (`/game-data/**`) must be served explicitly.
  publicDir: fileURLToPath(new URL('../../public', import.meta.url)),
  plugins: [react(), tailwindcss()],
  define: { __CANDIDATE_SHA__: JSON.stringify(candidate) },
  server: { host: '127.0.0.1', port: 41734, strictPort: true, fs: { allow: [clientRoot] } },
  build: { outDir: fileURLToPath(new URL('./dist', import.meta.url)), emptyOutDir: true, target: 'es2022' },
});
