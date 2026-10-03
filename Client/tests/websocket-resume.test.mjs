import assert from 'node:assert/strict';
import { after, afterEach, test } from 'node:test';
import { readFileSync } from 'node:fs';
import { createRequire } from 'node:module';
import { compileFunction } from 'node:vm';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

// Exercise the real client and the real APIProvider subscription callbacks.
// Only React's hook storage and the notification surface are stubbed: no
// hand-maintained copy of the resume protocol or patch application is used.
const SRC = '../src/';
const vite = await createServer({ configFile: false, root: fileURLToPath(new URL('..', import.meta.url)), appType: 'custom', logLevel: 'silent', server: { middlewareMode: true } });
const load = path => vite.ssrLoadModule(fileURLToPath(new URL(`${SRC}${path}.ts`, import.meta.url)));
const clientModule = await load('api/CitadelClient');
const transport = await load('api/RuntimeURL');
const { CitadelClient } = clientModule;
const require = createRequire(import.meta.url);
const ts = require('typescript');
const React = require('react');
const dependencies = {
  react: React,
  'react/jsx-runtime': require('react/jsx-runtime'),
  './CitadelClient': clientModule,
  './RuntimeURL': transport,
  './useCitadelAPI': await load('api/useCitadelAPI'),
  '../components/Notifications': { Notifications: { publish() {}, error() {} } },
};
const providerSource = readFileSync(new URL(`${SRC}api/ApiContext.tsx`, import.meta.url), 'utf8').replace(/import\.meta\.env/g, '({ DEV: false })');
for (const name of ['configurationErrorToast', 'OperationNotifications', 'StateResync', 'ConfigurationSync', 'VisibleTimers']) {
  if (providerSource.includes(`'./${name}'`)) dependencies[`./${name}`] = await load(`api/${name}`);
}
const stateNames = [...providerSource.matchAll(/const\s+\[\s*(\w+)\s*,[^\]]+\]\s*=\s*useState/g)].map(match => match[1]);
const providerCode = ts.transpileModule(providerSource, { compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 } }).outputText;
const original = { WebSocket: globalThis.WebSocket, window: globalThis.window, fetch: globalThis.fetch };
const clients = [];
let providerCleanup;
afterEach(() => {
  providerCleanup?.(); providerCleanup = null;
  for (const client of clients.splice(0)) client.disconnect();
  globalThis.WebSocket = original.WebSocket;
  globalThis.fetch = original.fetch;
  if (original.window === undefined) delete globalThis.window; else globalThis.window = original.window;
  transport.setConfigurationBasePath(null);
  transport.setRuntimeRouteResolver?.(null);
  transport.setRuntimeBasePath?.('');
});
after(() => vite.close());

function sockets() {
  const list = [];
  class Socket {
    static OPEN = 1; static CONNECTING = 0;
    readyState = 0;
    constructor(url, protocols) { this.url = url; this.protocols = protocols; list.push(this); }
    close() { this.readyState = 3; }
    send() { assert.fail('resume must not request a state snapshot'); }
    open() { this.readyState = 1; this.onopen(); }
    receive(message) { this.onmessage({ data: JSON.stringify({ v: 2, ...message }) }); }
  }
  globalThis.WebSocket = Socket;
  globalThis.window = { location: { origin: 'http://127.0.0.1', pathname: '/' } };
  return list;
}
const flush = async () => { for (let n = 0; n < 12; n++) await Promise.resolve(); };
const gameState = (revision, level = revision) => ({ revision, schemaVersion: 1, session: {}, player: { level }, castles: {}, movements: {}, map: {} });
const configuration = revision => ({ revision, schemaVersion: 1, sections: {} });
const manifest = { metadata: { digestSha256: 'catalog-a' }, catalogs: [] };
const receipt = { id: 'op-held', status: 'succeeded', intent: 'test' };

function mountProvider() {
  const states = [], effects = [];
  const hooks = {
    ...React,
    useState(initial) { const index = states.length; states.push(initial); return [initial, value => { states[index] = typeof value === 'function' ? value(states[index]) : value; }]; },
    useRef(value) { return { current: value }; },
    useCallback(fn) { return fn; },
    useMemo(fn) { return fn(); },
    useEffect(fn) { effects.push(fn); },
  };
  const client = new CitadelClient(); clients.push(client);
  const modules = { ...dependencies, react: hooks, './CitadelClient': { ...clientModule, CitadelAPI: client } };
  const module = { exports: {} };
  compileFunction(providerCode, ['require', 'module', 'exports'])(name => {
    assert.ok(name in modules, `unexpected provider dependency ${name}`); return modules[name];
  }, module, module.exports);
  globalThis.fetch = async input => {
    const path = new URL(String(input), 'http://127.0.0.1').pathname;
    if (path.endsWith('/update')) return Response.json({ currentVersion: 'test', status: 'idle' });
    assert.fail(`unexpected REST fallback: ${path}`);
  };
  module.exports.APIProvider({ children: null });
  // Run the production socket effect; other effects own unrelated polling or UI notifications.
  providerCleanup = effects.find(effect => effect.toString().includes('setResumeCursorProvider'))();
  return { client, states, state: () => states[stateNames.indexOf('state')], operations: () => states[stateNames.indexOf('operations')] };
}

function greet(socket, { instance = 'instance-a', revision = 8, sequence = 3 } = {}) {
  socket.receive({ type: 'state.snapshot', ...(instance ? { instance } : {}), payload: gameState(revision) });
  socket.receive({ type: 'operations.snapshot', sequence, payload: [receipt] });
  socket.receive({ type: 'config.changed', payload: configuration(5) });
  socket.receive({ type: 'catalog.changed', payload: manifest });
}

function cursor(socket) { return Object.fromEntries(new URL(socket.url).searchParams); }

test('CIT-37 context supplies no cursor before state, then a fresh applied cursor on every reconnect', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'], now: 0 });
  const list = sockets(); const dashboard = mountProvider();
  assert.deepEqual(cursor(list[0]), {}); list[0].open(); greet(list[0]);
  list[0].receive({ type: 'state.changed', baseRevision: 8, payload: { components: ['player'], revision: 10, patch: { updatedAt: '2026-10-02T00:00:00Z', revision: 10, schemaVersion: 1, player: { level: 10 } } } });
  list[0].receive({ type: 'operation.changed', sequence: 7, payload: { ...receipt, id: 'op-new' } });
  // A late duplicate must not move the operation cursor backward.
  list[0].receive({ type: 'operation.changed', sequence: 4, payload: receipt });
  list[0].onclose(); t.mock.timers.tick(1_000);
  assert.deepEqual(cursor(list[1]), { resume: '1', instance: 'instance-a', since: '10', ops: '7', config: '5', catalog: 'catalog-a' });
  assert.equal(dashboard.state().revision, 10);
  list[1].open(); list[1].receive({ type: 'state.resumed', instance: 'instance-a', payload: { instance: 'instance-a', from: 10, revision: 12, ops: 7 } });
  // The greeting's H is not an applied state revision until its patch lands.
  assert.equal(dashboard.state().revision, 10);
  list[1].receive({ type: 'state.changed', revision: 12, baseRevision: 10, gap: true, payload: { components: ['player'], revision: 12, patch: { updatedAt: '2026-10-02T00:00:00Z', schemaVersion: 1, revision: 12, player: { level: 12 } } } });
  assert.equal(dashboard.state().revision, 12); assert.equal(dashboard.operations()['op-held'].id, 'op-held');
  list[1].receive({ type: 'state.changed', revision: 11, baseRevision: 10, payload: { components: ['player'], revision: 11, patch: { updatedAt: '2026-10-02T00:00:00Z', schemaVersion: 1, revision: 11, player: { level: 11 } } } });
  assert.equal(dashboard.state().player.level, 12);
  list[1].onclose(); t.mock.timers.tick(1_000);
  assert.equal(cursor(list[2]).since, '12');
  await flush();
});

test('CIT-37 an instance change accepts a lower snapshot and resets the operation sequence', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'], now: 0 });
  const list = sockets(); const dashboard = mountProvider(); list[0].open(); greet(list[0], { revision: 100, sequence: 70 });
  list[0].onclose(); t.mock.timers.tick(1_000); list[1].open();
  greet(list[1], { instance: 'instance-b', revision: 2, sequence: 1 });
  assert.equal(dashboard.state().revision, 2); assert.equal(dashboard.state().player.level, 2);
  list[1].onclose(); t.mock.timers.tick(1_000);
  assert.equal(cursor(list[2]).instance, 'instance-b'); assert.equal(cursor(list[2]).since, '2'); assert.equal(cursor(list[2]).ops, '1');
  await flush();
});

test('CIT-37 older workers with no instance keep full greetings and never send a resume cursor', async t => {
  t.mock.timers.enable({ apis: ['setTimeout'], now: 0 });
  const list = sockets(); const dashboard = mountProvider(); list[0].open(); greet(list[0], { instance: null });
  list[0].onclose(); t.mock.timers.tick(1_000);
  assert.deepEqual(cursor(list[1]), {}); list[1].open(); list[1].receive({ type: 'state.snapshot', payload: gameState(10) });
  assert.equal(dashboard.state().revision, 10); assert.ok(dashboard.operations()['op-held']);
  await flush();
});

test('CIT-37 an interrupted greeting cannot skip operation or configuration snapshots it never received', t => {
  t.mock.timers.enable({ apis: ['setTimeout'], now: 0 });
  const list = sockets(); mountProvider(); list[0].open(); list[0].receive({ type: 'state.snapshot', instance: 'instance-a', payload: gameState(8) });
  list[0].onclose(); t.mock.timers.tick(1_000);
  assert.equal(cursor(list[1]).ops, '-1'); assert.equal(cursor(list[1]).config, '-1');
});

test('CIT-37 provider cleanup cannot clear a newer cursor provider', () => {
  const client = new CitadelClient(); clients.push(client); const list = sockets();
  const clearOld = client.setResumeCursorProvider(() => null);
  client.setResumeCursorProvider(() => ({ instance: 'new', since: 3, ops: 4, config: 5, catalog: 'a' }));
  clearOld(); client.connect(); assert.equal(cursor(list[0]).instance, 'new');
});

if (transport.routedSocketTarget) test('CIT-37 direct route preserves the complete logical query and keeps the token in protocols', async () => {
  transport.setRuntimeRouteResolver({ ready: async () => ({ base: 'https://cell.example/accounts/a', token: 'secret-token', generation: 1 }) });
  const logical = 'wss://gateway.example/accounts/a/api/v2/events?resume=1&instance=a&since=12&ops=3&config=5&catalog=abc';
  const direct = await transport.routedSocketTarget('https://gateway.example/accounts/a', logical);
  assert.equal(new URL(direct.url).search, new URL(logical).search);
  assert.equal(direct.url.includes('secret-token'), false);
  assert.deepEqual(direct.protocols, ['citadelops.v2', 'citadelops.session.secret-token']);
});
