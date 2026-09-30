import type { ConfigurationSnapshot, GameStateV2, IntentReceipt } from '../../src/api/Contracts';
import { stableDigest } from '../../src/settings/DraftRecovery';
import { DEFAULT_CATALOG_ROWS } from './catalogs';
import { buildRealmState, realmConfiguration, setRealmClock } from './realm';

/**
 * Scenarios are DATA (CIT-22): a JSON file per scenario, layered over one fictional realm. A scenario states what it
 * seeds and what an observer should see (`expect`), and which "Simulated" labels must be visible. No product module
 * branches on a scenario; the harness only serves different data.
 */
export type ScenarioPlatform = 'desktop' | 'hosted';
export type CoverageColumn = 'inline' | 'readiness' | 'disclosure' | 'phases' | 'copy' | 'goal';
export type SessionMode = 'live' | 'disconnected' | 'awaiting-baseline' | 'checkpoint';

type Json = null | boolean | number | string | Json[] | { [key: string]: Json };
type JsonObject = { [key: string]: Json };
type Row = Record<string, unknown>;

export interface RuntimeStep {
  label: string;
  /** JSON merge patch applied to the served state. */
  state?: JsonObject;
  /** Section-level merge patch applied to the configuration (a saved-switch change the game reported). */
  config?: JsonObject;
  /** Receipts added to `operations` (labelled "Simulated: …"). */
  operations?: JsonObject[];
}

export interface ScenarioStorageSeed {
  /** `@accountKey` is replaced with `<uid>:<worldId>`. */
  key: string;
  value: Json;
}

export interface ScenarioFile {
  id: string;
  title: string;
  purpose: string;
  /** Walkthrough step id (see Docs/OnboardingPreview.md), or "-" for a support scenario. */
  step: string;
  platforms: ScenarioPlatform[];
  /** Coverage-matrix module ids or shared surfaces this scenario exercises. */
  modules: string[];
  /** Coverage-matrix columns this scenario covers for its modules. */
  columns: CoverageColumn[];
  /** Platform differences, stated per scenario. */
  hostedOnly?: string;
  desktopOnly?: string;
  /** `empty`: a new user with nothing saved (default). `rich`: the fictional existing account as authored. */
  base?: 'empty' | 'rich';
  session?: SessionMode;
  /** `?fail=` default for this scenario: one `automation.enabled` write is rejected per page load. */
  fail?: 'stop' | 'start';
  /** The first Save of a section is rejected once with a configuration conflict, as if another window saved first. */
  conflictOnce?: { configPatch?: JsonObject };
  /** A second account for "Switch account": a state patch (different account and world, other castles). */
  alternate?: JsonObject;
  /** What Start Bot does in this fixture: the scenario is served again as connected (this patch replaces `patch`). Saved settings written so far are kept. */
  startedPatch?: { state?: JsonObject };
  /** Locale and viewport the walkthrough uses for this scenario (the dock can change both). */
  locale?: 'en' | 'de' | 'ar';
  viewport?: { width: number; height: number };
  patch?: {
    state?: JsonObject;
    /** Section name to value; `null` removes the section; objects are merge-patched into the section. */
    config?: JsonObject;
    operations?: JsonObject[];
    catalogs?: Record<string, Row[]>;
  };
  storage?: ScenarioStorageSeed[];
  runtime?: RuntimeStep[];
  /** The "Simulated" labels that must be visible in this scenario. */
  simulated: string[];
  /** What must be visible / must not be, for the walkthrough and QA. */
  expect: string[];
}

export interface BuiltScenario {
  id: string;
  file: ScenarioFile;
  state: GameStateV2;
  configuration: ConfigurationSnapshot;
  operations: IntentReceipt[];
  catalogRows: Record<string, Row[]>;
  storage: Array<{ key: string; value: string }>;
  accountKey: string;
}

const SAVED_DIGEST = /^@savedDigest:(.+)$/;
function resolveDigests<T>(value: T, sections: Record<string, unknown>): T {
  if (typeof value === 'string') {
    const match = SAVED_DIGEST.exec(value);
    return (match ? stableDigest(sections[match[1]] ?? null) : value) as unknown as T;
  }
  if (Array.isArray(value)) return value.map((entry) => resolveDigests(entry, sections)) as unknown as T;
  if (isObject(value)) return Object.fromEntries(Object.entries(value).map(([key, entry]) => [key, resolveDigests(entry, sections)])) as unknown as T;
  return value;
}

const isObject = (value: unknown): value is JsonObject => typeof value === 'object' && value !== null && !Array.isArray(value);

/**
 * RFC 7396 JSON merge patch: `null` deletes, objects merge, everything else replaces. One extension: a key that starts
 * with `=` replaces that key's value wholesale instead of merging (for example `"=castles": {}` empties the map).
 */
export function mergePatch<T>(target: T, patch: Json): T {
  if (!isObject(patch)) return patch as unknown as T;
  const result: JsonObject = isObject(target) ? { ...(target as unknown as JsonObject) } : {};
  for (const [key, value] of Object.entries(patch)) {
    if (key.startsWith('=')) result[key.slice(1)] = value;
    else if (value === null) delete result[key];
    else result[key] = mergePatch(result[key], value);
  }
  return result as unknown as T;
}

const UNIT_MS: Record<string, number> = { s: 1000, m: 60_000, h: 3_600_000, d: 86_400_000 };

/** Replaces `@now`, `@now-90m`, `@now+25m` and `@accountKey` inside scenario data. */
export function resolveTokens<T>(value: T, nowMs: number, accountKey: string): T {
  if (typeof value === 'string') {
    const time = /^@now(?:([+-])(\d+)([smhd]))?$/.exec(value);
    if (time) return new Date(nowMs + (time[1] ? (time[1] === '-' ? -1 : 1) * Number(time[2]) * UNIT_MS[time[3]] : 0)).toISOString() as unknown as T;
    return value.replaceAll('@accountKey', accountKey) as unknown as T;
  }
  if (Array.isArray(value)) return value.map((entry) => resolveTokens(entry, nowMs, accountKey)) as unknown as T;
  if (isObject(value)) return Object.fromEntries(Object.entries(value).map(([key, entry]) => [key.replaceAll('@accountKey', accountKey), resolveTokens(entry, nowMs, accountKey)])) as unknown as T;
  return value;
}

/** A session mode changes only the served session block (same rules as the hosted fixture modes). */
export function applySessionMode(state: GameStateV2, mode: SessionMode): GameStateV2 {
  if (mode === 'live') return state;
  const generation = Math.max(1, Number(state.session.generation) || 1);
  const session = mode === 'awaiting-baseline'
    ? { ...state.session, loggedIn: true, socketReady: true, generation, baselineGeneration: generation - 1 }
    : { ...state.session, status: mode === 'disconnected' ? 'disconnected' : state.session.status, loggedIn: false, socketReady: false };
  return { ...state, session };
}

/** The realm as a brand-new user has it: nothing saved, no presets, nothing on, no automation status reported. */
function emptyBase(state: GameStateV2, configuration: ConfigurationSnapshot): { state: GameStateV2; configuration: ConfigurationSnapshot } {
  const sections: Record<string, unknown> = {};
  for (const [name, value] of Object.entries(configuration.sections)) {
    if (name.startsWith('automation.') || name === 'attacks.presets' || name === 'defense.presets') continue;
    sections[name] = value;
  }
  sections['automation.enabled'] = {};
  sections['attacks.presets'] = { version: 1, presets: [] };
  sections['defense.presets'] = { version: 1, presets: [] };
  return { state: { ...state, automations: {} }, configuration: { ...configuration, sections } };
}

export const ACCOUNT_KEY_FOR = (state: GameStateV2): string => `${state.account?.uid ?? ''}:${state.account?.worldId ?? ''}`;

export function buildScenario(file: ScenarioFile, options: { nowMs?: number; session?: SessionMode } = {}): BuiltScenario {
  const nowMs = options.nowMs ?? Date.now();
  setRealmClock(() => nowMs);
  const realm = buildRealmState(1);
  let state: GameStateV2 = realm;
  let configuration: ConfigurationSnapshot = realmConfiguration(1);
  if ((file.base ?? 'empty') === 'empty') ({ state, configuration } = emptyBase(state, configuration));
  const accountKey = ACCOUNT_KEY_FOR(state);
  const patch = resolveTokens(file.patch ?? {}, nowMs, accountKey);
  if (patch.state) state = mergePatch(state, patch.state);
  if (patch.config) {
    const sections = { ...configuration.sections };
    for (const [name, value] of Object.entries(patch.config)) {
      if (value === null) delete sections[name];
      else sections[name] = mergePatch(sections[name], value);
    }
    configuration = { ...configuration, sections };
  }
  state = applySessionMode(state, options.session ?? file.session ?? 'live');
  const catalogRows = Object.fromEntries(Object.entries({ ...DEFAULT_CATALOG_ROWS, ...(patch.catalogs ?? {}) })) as Record<string, Row[]>;
  return {
    id: file.id, file, state, configuration,
    operations: (patch.operations ?? []) as unknown as IntentReceipt[],
    catalogRows,
    storage: (file.storage ?? []).map((seed) => ({
      key: resolveTokens(seed.key, nowMs, accountKey),
      value: JSON.stringify(resolveDigests(resolveTokens(seed.value, nowMs, accountKey), configuration.sections)),
    })),
    accountKey,
  };
}

/** Applies the next runtime step to a built scenario (the dock's "Advance runtime"). */
export function applyRuntimeStep(built: BuiltScenario, step: RuntimeStep, nowMs: number): BuiltScenario {
  const resolved = resolveTokens(step, nowMs, built.accountKey);
  let configuration = built.configuration;
  if (resolved.config) {
    const sections = { ...configuration.sections };
    for (const [name, value] of Object.entries(resolved.config)) {
      if (value === null) delete sections[name];
      else sections[name] = mergePatch(sections[name], value);
    }
    configuration = { ...configuration, revision: configuration.revision + 1, updatedAt: new Date(nowMs).toISOString(), sections };
  }
  return {
    ...built,
    state: resolved.state ? mergePatch(built.state, resolved.state) : built.state,
    configuration,
    operations: resolved.operations ? [...(resolved.operations as unknown as IntentReceipt[]), ...built.operations] : built.operations,
  };
}

/** The scenario files, keyed by id (Vite bundles the JSON; tests load it the same way). */
export function loadScenarioFiles(): ScenarioFile[] {
  const modules = import.meta.glob('./scenarios/*.json', { eager: true, import: 'default' }) as Record<string, ScenarioFile>;
  return Object.values(modules).sort((left, right) => left.id.localeCompare(right.id));
}

const REQUIRED: Array<keyof ScenarioFile> = ['id', 'title', 'purpose', 'step', 'platforms', 'modules', 'columns', 'simulated', 'expect'];

/** Structural check used by the fixtures test and the harness at startup. Returns problems, empty when valid. */
export function validateScenarioFile(file: ScenarioFile): string[] {
  const problems: string[] = [];
  for (const key of REQUIRED) if (file[key] === undefined) problems.push(`${file.id ?? '?'}: missing ${key}`);
  if (typeof file.id === 'string' && !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(file.id)) problems.push(`${file.id}: id must be kebab-case`);
  if (Array.isArray(file.platforms) && file.platforms.some((platform) => platform !== 'desktop' && platform !== 'hosted')) problems.push(`${file.id}: unknown platform`);
  if (Array.isArray(file.simulated) && file.simulated.length === 0) problems.push(`${file.id}: names no Simulated labels`);
  if (Array.isArray(file.expect) && file.expect.length === 0) problems.push(`${file.id}: no expected observations`);
  return problems;
}
