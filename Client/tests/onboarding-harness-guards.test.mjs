import assert from 'node:assert/strict';
import { readdir, readFile } from 'node:fs/promises';
import { test } from 'node:test';

const HARNESS = new URL('./onboarding-browser/', import.meta.url);
const read = (name) => readFile(new URL(name, HARNESS), 'utf8');
const code = (text) => text.replace(/\/\*[\s\S]*?\*\//g, '').replace(/^\s*\/\/.*$/gm, '');
const harnessFiles = ['catalogs.ts', 'dock.tsx', 'fixtureServer.ts', 'fixtureSocket.ts', 'install.ts', 'main.tsx', 'networkGuard.ts', 'product.ts', 'realm.ts', 'scenario.ts', 'vite.config.ts', 'coverage.mjs'];

async function walk(dir) {
  const out = [];
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = new URL(`${entry.name}${entry.isDirectory() ? '/' : ''}`, dir);
    if (entry.isDirectory()) out.push(...await walk(path)); else if (/\.(ts|tsx)$/.test(entry.name)) out.push(path);
  }
  return out;
}

test('no harness module imports the real transport, the API context or a product write path', async () => {
  for (const file of harnessFiles) {
    const text = code(await read(file));
    assert.doesNotMatch(text, /api\/CitadelClient|from '.*\/CitadelClient'|RuntimeURL|api\/ApiContext/, `${file}: the real client and its transport stay untouched`);
    assert.doesNotMatch(text, /\bnew WebSocket\(|\bnew EventSource\(|\bnew XMLHttpRequest\(|\bimport\(['"]https?:/, `${file}: nothing opens a real connection`);
  }
  const server = code(await read('fixtureServer.ts'));
  assert.doesNotMatch(server, /\bfetch\(/, 'the fixture server never fetches');
});

test('the transport denies every network path it does not answer from a fixture', async () => {
  const guard = code(await read('networkGuard.ts'));
  assert.match(guard, /url\.origin !== window\.location\.origin[\s\S]*?record\(`fetch/, 'cross-origin fetch is blocked');
  for (const name of ['WebSocket', 'EventSource', 'XMLHttpRequest']) assert.match(guard, new RegExp(`window\\.${name} = new Proxy`), `${name} is replaced`);
  assert.match(guard, /navigator\.sendBeacon = /);
  const sockets = guard.slice(guard.indexOf('window.WebSocket = new Proxy'), guard.indexOf('if (typeof window.EventSource'));
  assert.match(sockets, /options\.socket\?\.\(url\)[\s\S]*?record\(`WebSocket/, 'a socket is answered only when the fixture takes it; any other is refused');
  const install = code(await read('install.ts'));
  assert.match(install, /url\.pathname\.startsWith\('\/api\/'\)[\s\S]*?server\.handle\(/, 'every /api/ request is answered by the fixture server');
  assert.match(install, /url\.includes\(EVENTS_PATH\) \? new FixtureSocket/, 'only the event stream is answered by a socket');
  const main = code(await read('main.tsx'));
  assert.ok(main.indexOf('installFixtureTransport(server)') < main.indexOf("import('../../src/App.tsx')"), 'the transport is installed before the production app loads');
});

test('simulation is labelled by the harness shell, not by product code', async () => {
  const dock = await read('dock.tsx');
  assert.match(dock, /SIMULATION_BANNER = 'Simulated preview: sample data only\. Nothing here reaches the game or an account\.'/);
  assert.match(dock, /Candidate \{candidate\}/);
  const html = await read('index.html');
  assert.ok(html.indexOf('data-fixture-banner') < html.indexOf('id="root"'), 'the banner sits outside the application root');
  assert.match(html, /data-fixture-dock/);
  const css = await read('fixture.css');
  assert.match(css, /#fixture-banner \{[^}]*position: fixed;[^}]*z-index: 2147483000/, 'persistent and above modal portals');
  assert.doesNotMatch(await read('main.tsx'), /banner[^\n]*(remove|dismiss|hidden)/i, 'the banner cannot be dismissed');
  assert.doesNotMatch(dock, /dismiss|close banner/i);
});

test('the harness needs no product change, no new dependency and one documented command', async () => {
  const files = await walk(new URL('../src/', import.meta.url));
  for (const file of files) {
    const text = await readFile(file, 'utf8');
    assert.doesNotMatch(text, /onboarding-browser|FixtureServer|__CANDIDATE_SHA__|fixture:\/\//, `${file.pathname}: product code does not know the harness`);
  }
  const pkg = JSON.parse(await readFile(new URL('../package.json', import.meta.url), 'utf8'));
  assert.equal(pkg.scripts['preview:onboarding'], 'vite --config tests/onboarding-browser/vite.config.ts');
  const config = code(await read('vite.config.ts'));
  assert.match(config, /host: '127\.0\.0\.1', port: 41734, strictPort: true/);
  assert.doesNotMatch(config, /proxy/, 'no proxy: there is no server behind the preview');
  assert.match(config, /publicDir: fileURLToPath\(new URL\('\.\.\/\.\.\/public', import\.meta\.url\)\)/, 'the game images (/game-data/**) are served from Client/public');
  assert.ok((await readdir(new URL('../public/game-data/', import.meta.url))).length > 0, 'and that directory holds them');
  const allowed = new Set(['react', 'react-dom/client', 'vite', '@vitejs/plugin-react', '@tailwindcss/vite', 'node:child_process', 'node:url']);
  for (const file of harnessFiles.filter((name) => /\.(ts|tsx)$/.test(name))) {
    for (const [, source] of code(await read(file)).matchAll(/from '([^']+)'/g)) {
      if (source.startsWith('.')) continue;
      assert.ok(allowed.has(source), `${file}: unexpected import ${source}`);
    }
  }
});

test('fixtures use fictional names only: the realm, the accounts and the worlds', async () => {
  const realm = await read('realm.ts');
  for (const name of ['Preview Commander', 'demo-world', 'Stonehaven', 'Ironwatch Pact']) assert.ok(realm.includes(name), name);
  assert.match(realm, /fixture:\/\/synthetic-world/);
});

test('the guard lets the dev server\'s own hot-reload sockets through and nothing else; reset clears only citadelops keys', async () => {
  const guard = code(await read('networkGuard.ts'));
  assert.match(guard, /protocols\.some\(\(protocol\) => protocol === 'vite-hmr' \|\| protocol === 'vite-ping'\)/, 'only Vite\'s two protocols');
  assert.match(guard, /new URL\(url, window\.location\.href\)\.host === window\.location\.host/, 'and only on this page\'s own host');
  assert.match(guard, /isDevServerSocket\(url, protocols\)\) return Reflect\.construct\(target, args\)/);
  const sockets = guard.slice(guard.indexOf('window.WebSocket = new Proxy'), guard.indexOf('if (typeof window.EventSource'));
  assert.ok(sockets.indexOf('options.socket?.(url)') < sockets.indexOf('isDevServerSocket(url, protocols)') && sockets.indexOf('isDevServerSocket(url, protocols)') < sockets.indexOf('record(`WebSocket'), 'the fixture socket first, then the dev server, then refusal');
  const main = code(await read('main.tsx'));
  assert.doesNotMatch(main, /sessionStorage\.clear\(\)/, 'reset never clears keys the preview did not write');
  assert.match(main, /Object\.keys\(window\.sessionStorage\)\) if \(key\.startsWith\('citadelops'\)\)/);
  const server = code(await read('fixtureServer.ts'));
  assert.match(server, /route\.split\('\?'\)\[0\] === '\/game-data\/localize'\) return this\.handleLocalize/, 'the localize POST is routed before non-GET requests are refused');
  assert.match(server, /\/automations\/auto-storm\/troop-cap-preview/);
});
