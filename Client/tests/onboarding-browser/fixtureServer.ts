import type { ConfigurationSnapshot, GameStateV2, IntentReceipt } from './product';
import { catalogFor, catalogManifest, LOCALIZED } from './catalogs';
import { applyRuntimeStep, applySessionMode, buildScenario, mergePatch, type BuiltScenario, type ScenarioFile, type SessionMode } from './scenario';

/**
 * The fixture "server" of the onboarding preview (CIT-22): answers the production client's `/api/v2/*` requests from
 * one built scenario. It is pure (no window, no network): `handle()` maps a request to a Response, and every effect is
 * recorded in `log`. Writes are intercepted: a configuration write follows the real compare-and-set rules against the
 * fixture's own revision; an intent is recorded and answered with a synthetic receipt whose actor is `ui`, never
 * `automation:*`, so nothing the harness accepts can become an attributed first result.
 */
export type LogKind = 'intent' | 'config' | 'blocked' | 'scenario';
export interface LogEntry { at: number; kind: LogKind; detail: string }

export type ServerEvent = 'snapshot' | 'log';

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } });
const error = (status: number, code: string, message: string) => json({ error: { code, message } }, status);
const isRecord = (value: unknown): value is Record<string, unknown> => typeof value === 'object' && value !== null && !Array.isArray(value);
const sameJSON = (left: unknown, right: unknown) => JSON.stringify(left) === JSON.stringify(right);

export interface FixtureServerOptions {
  file: ScenarioFile;
  nowMs?: () => number;
  session?: SessionMode;
  /** `?fail=stop|start` (defaults to the scenario's own). */
  fail?: 'stop' | 'start';
  /** 1 or 2: the second account of "Switch account". */
  account?: 1 | 2;
}

export class FixtureServer {
  readonly file: ScenarioFile;
  built: BuiltScenario;
  sessionMode: SessionMode;
  account: 1 | 2;
  log: LogEntry[] = [];
  revision = 1;
  private readonly now: () => number;
  private failMode: 'stop' | 'start' | undefined;
  private failConsumed = false;
  private conflictFired = false;
  private step = 0;
  private listeners = new Set<(event: ServerEvent) => void>();

  constructor(options: FixtureServerOptions) {
    this.file = options.file;
    this.now = options.nowMs ?? Date.now;
    this.sessionMode = options.session ?? options.file.session ?? 'live';
    this.account = options.account ?? 1;
    this.failMode = options.fail ?? options.file.fail;
    this.built = buildScenario(options.file, { nowMs: this.now(), session: 'live' });
    if (this.account === 2 && options.file.alternate) this.built = { ...this.built, state: mergePatch(this.built.state, options.file.alternate) };
  }

  onEvent(listener: (event: ServerEvent) => void): () => void {
    this.listeners.add(listener);
    return () => { this.listeners.delete(listener); };
  }

  private emit(event: ServerEvent) { for (const listener of this.listeners) listener(event); }
  private record(kind: LogKind, detail: string) { this.log.push({ at: this.now(), kind, detail }); this.emit('log'); }

  /** The state as served: the scenario's live state under the current session mode. */
  state(): GameStateV2 {
    return { ...applySessionMode(this.built.state, this.sessionMode), revision: this.revision };
  }
  configuration(): ConfigurationSnapshot { return this.built.configuration; }
  operations(): IntentReceipt[] { return this.built.operations; }

  runtimeStepCount(): number { return this.file.runtime?.length ?? 0; }
  nextRuntimeLabel(): string | null { return this.file.runtime?.[this.step]?.label ?? null; }

  /** "Advance runtime": applies the scenario's next runtime step. Returns false when there is none. */
  advance(): boolean {
    const step = this.file.runtime?.[this.step];
    if (!step) return false;
    this.step += 1;
    this.revision += 1;
    this.built = applyRuntimeStep(this.built, step, this.now());
    this.record('scenario', `Runtime step: ${step.label}`);
    this.emit('snapshot');
    return true;
  }

  setSessionMode(mode: SessionMode) {
    this.sessionMode = mode;
    this.revision += 1;
    this.record('scenario', `Session mode: ${mode}`);
    this.emit('snapshot');
  }

  switchAccount() {
    if (!this.file.alternate) return;
    const next: 1 | 2 = this.account === 1 ? 2 : 1;
    const fresh = buildScenario(this.file, { nowMs: this.now(), session: 'live' });
    this.account = next;
    this.built = { ...this.built, state: next === 2 ? mergePatch(fresh.state, this.file.alternate) : fresh.state };
    this.revision += 1;
    this.record('scenario', `Switched to account ${next}`);
    this.emit('snapshot');
  }

  /** What Start Bot does here: the scenario is served connected; saved settings written so far stay. */
  private startBot() {
    if (this.file.startedPatch) {
      const started = buildScenario({ ...this.file, patch: { ...(this.file.patch ?? {}), ...this.file.startedPatch } }, { nowMs: this.now(), session: 'live' });
      this.built = { ...this.built, state: started.state };
    }
    this.sessionMode = 'live';
    this.revision += 1;
    this.emit('snapshot');
  }

  private saveConfiguration(sections: Record<string, unknown>): ConfigurationSnapshot {
    const next = { ...this.built.configuration, revision: this.built.configuration.revision + 1, updatedAt: new Date(this.now()).toISOString(), sections };
    this.built = { ...this.built, configuration: next };
    this.emit('snapshot');
    return next;
  }

  /** Answer for one request. `path` is the URL pathname (after the runtime prefix, if any). */
  async handle(path: string, method: string, body: unknown): Promise<Response> {
    const verb = method.toUpperCase();
    const at = path.indexOf('/api/v2/');
    const route = at >= 0 ? path.slice(at + '/api/v2'.length) : path;
    if (route === '/health') return json({ api: 2, status: 'ok', fixture: true });
    if (route === '/state' && verb === 'GET') return json(this.state());
    if (route === '/operations' && verb === 'GET') return json(this.operations());
    if (route === '/intents' && verb === 'GET') return json([]);
    if (route.startsWith('/config')) return this.handleConfiguration(route, verb, body);
    if (route.startsWith('/intents/') && verb === 'POST') return this.handleIntent(decodeURIComponent(route.slice('/intents/'.length)), body);
    if (/^\/operations\/[^/]+\/cancel$/.test(route) && verb === 'POST') {
      this.record('intent', `cancel ${route.split('/')[2]}`);
      return json({ id: route.split('/')[2], cancelled: false });
    }
    if (verb !== 'GET') {
      this.record('blocked', `${verb} ${route}: no fixture route (nothing was sent)`);
      return error(501, 'preview_unavailable', 'Simulated preview: this action is not available in the preview.');
    }
    return this.handleRead(route);
  }

  private handleRead(route: string): Response {
    const path = route.split('?')[0];
    if (path === '/game-data') return json(catalogManifest());
    if (path === '/game-data/localize') return json({ values: LOCALIZED, locale: { resolvedLocale: 'en' } });
    if (path.startsWith('/game-data/')) return json(catalogFor(decodeURIComponent(path.slice('/game-data/'.length)), this.built.catalogRows));
    if (path === '/locales') return json({ locales: [{ code: 'en' }] });
    if (path === '/update') return json({ currentVersion: '0.0.0-preview', latestVersion: '0.0.0-preview', available: false, installSupported: false, status: 'current', progress: 0, restartRequired: false, checkedAt: new Date(this.now()).toISOString() });
    if (path === '/diagnostics') return json({ applicationMemoryMb: 0, browserMemoryMb: 0, observedAt: new Date(this.now()).toISOString() });
    if (path === '/telemetry/attack-rates') return json({ observedAt: new Date(this.now()).toISOString(), windowMinutes: 60, launchesByFeature: {} });
    if (path === '/browsers') return json({ selected: null, current: null, available: [], restartRequired: false, selectionIntent: 'session.select_browser' });
    if (path === '/session/game-servers') return json({ version: 'fixture', source: 'fixture', updatedAt: new Date(this.now()).toISOString(), servers: [
      { code: 'DEMO1', label: 'Demo World', zone: 'demo-world', host: 'fixture.invalid', url: 'wss://fixture.invalid:443', international: false, instance: 1 },
      { code: 'DEMO2', label: 'Demo World 2', zone: 'demo-world-2', host: 'fixture.invalid', url: 'wss://fixture.invalid:443', international: false, instance: 2 },
    ] });
    if (path === '/session/background-login') return json({ configured: false, server: 'DEMO1', language: 'en' });
    if (path.startsWith('/projections/')) return json({ items: [] });
    if (path.startsWith('/alliance-targets')) return json({ observedAt: new Date(this.now()).toISOString(), targets: [], total: 0, page: 1, pageSize: 25 });
    if (path.startsWith('/history/')) return json({ reports: [], samples: [], rangeSeconds: 86_400 });
    if (path.startsWith('/world-intelligence/')) return json({ entries: [], runs: [], rows: [], datasets: [] });
    if (path.startsWith('/buildings/')) return error(501, 'preview_unavailable', 'Simulated preview: building capture and blueprint previews are not available.');
    this.record('blocked', `GET ${route}: no fixture route`);
    return error(404, 'not_found', 'Simulated preview: no fixture data for this request.');
  }

  private handleIntent(name: string, body: unknown): Response {
    const args = isRecord(body) && isRecord(body.arguments) ? body.arguments : {};
    this.record('intent', `${name}${Object.keys(args).length ? ` ${JSON.stringify(args)}` : ''} (recorded, not sent)`);
    if (name === 'session.start' || name === 'session.reconnect') this.startBot();
    else if (name === 'session.stop') this.setSessionMode('disconnected');
    const at = new Date(this.now()).toISOString();
    const requested = isRecord(body) && typeof body.actor === 'string' ? body.actor : 'ui';
    const receipt = {
      id: isRecord(body) && typeof body.id === 'string' && body.id ? body.id : `sim-${this.log.length}-${this.now().toString(36)}`,
      intent: name,
      // Never `automation:*`: a preview intent cannot become an attributed first result.
      actor: requested.startsWith('automation:') ? 'ui' : requested, priority: 0, status: 'succeeded', phase: 'completed',
      plan: { intent: name, effect: 'read', stateRevision: this.revision, steps: [], summary: `Simulated: ${name}` },
      completedStepIndexes: [], submittedAt: at, completedAt: at,
    };
    return json(receipt);
  }

  private handleConfiguration(route: string, verb: string, body: unknown): Response {
    if (route === '/config' && verb === 'GET') return json(this.built.configuration);
    if (route === '/config/export' && verb === 'GET') {
      return json({ format: 'citadelops-settings', formatVersion: 1, exportedAt: new Date(this.now()).toISOString(), appVersion: 'preview', configuration: { schemaVersion: this.built.configuration.schemaVersion, sections: this.built.configuration.sections } });
    }
    const match = /^\/config\/([^/?]+)$/.exec(route);
    if (!match || verb !== 'PUT') {
      this.record('blocked', `${verb} ${route}: no fixture route`);
      return error(501, 'preview_unavailable', 'Simulated preview: this action is not available in the preview.');
    }
    if (!isRecord(body) || !Object.prototype.hasOwnProperty.call(body, 'value')) return error(400, 'invalid_request', 'Configuration value is required.');
    const section = decodeURIComponent(match[1]);
    const sections = this.built.configuration.sections;
    if (section === 'automation.enabled' && this.failMode && !this.failConsumed && isRecord(body.value)) {
      const current = isRecord(sections[section]) ? sections[section] as Record<string, unknown> : {};
      const turnsOff = Object.entries(body.value).some(([key, value]) => current[key] === true && value === false);
      const turnsOn = Object.entries(body.value).some(([key, value]) => current[key] !== true && value === true);
      if ((this.failMode === 'stop' && turnsOff) || (this.failMode === 'start' && turnsOn)) {
        this.failConsumed = true;
        this.record('config', `${section}: rejected on purpose (?fail=${this.failMode}); the next write goes through`);
        return error(500, 'internal', 'The fixture rejected this write on purpose (fail).');
      }
    }
    if (this.file.conflictOnce && !this.conflictFired && section !== 'automation.enabled') {
      this.conflictFired = true;
      // As if another window saved first: the revision moves (and a destination may change) before this write is checked.
      let next = { ...sections };
      for (const [name, value] of Object.entries(this.file.conflictOnce.configPatch ?? {})) next[name] = mergePatch(next[name], value as never);
      next = { ...next };
      this.saveConfiguration(next);
      this.record('config', `${section}: rejected once with a configuration conflict (another window saved first)`);
      return error(409, 'configuration_conflict', 'Configuration changed before this update could be applied.');
    }
    if (typeof body.expectedRevision === 'number' && body.expectedRevision !== this.built.configuration.revision) {
      this.record('config', `${section}: configuration_conflict (expected revision ${body.expectedRevision}, fixture is at ${this.built.configuration.revision})`);
      return error(409, 'configuration_conflict', 'Configuration changed before this update could be applied.');
    }
    if (Object.prototype.hasOwnProperty.call(body, 'expectedValue') && !sameJSON(body.expectedValue, sections[section])) {
      this.record('config', `${section}: configuration_conflict (the saved section changed)`);
      return error(409, 'configuration_conflict', 'Configuration changed before this update could be applied.');
    }
    const saved = this.saveConfiguration({ ...sections, [section]: body.value });
    this.record('config', `${section}: saved in the fixture (revision ${saved.revision}); nothing was sent anywhere`);
    return json(saved);
  }
}
