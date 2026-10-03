import { useState, useSyncExternalStore } from 'react';
import type { FixtureServer } from './fixtureServer';
import type { ScenarioFile, SessionMode } from './scenario';

export const SIMULATION_BANNER = 'Simulated preview: sample data only. Nothing here reaches the game or an account.';

/**
 * The control dock (CIT-22). It never calls product code: it only asks the fixture server to change what it serves
 * (session mode, the next runtime step, the second account) or reloads the page for a scenario / a reset.
 */
export function Dock({ server, scenarios, candidate, onReset, onScenario }: {
  server: FixtureServer;
  scenarios: readonly ScenarioFile[];
  candidate: string;
  onReset: () => void;
  onScenario: (id: string) => void;
}) {
  useSyncExternalStore((listener) => server.onEvent(listener), () => `${server.revision}:${server.log.length}:${server.sessionMode}:${server.account}`);
  const [showLog, setShowLog] = useState(false);
  const file = server.file;
  const next = server.nextRuntimeLabel();
  const blocked = server.log.filter((entry) => entry.kind === 'blocked').length;
  const writes = server.log.filter((entry) => entry.kind === 'intent' || entry.kind === 'config').length;
  return (
    <div>
      <div className="dock-row">
        <label>Scenario{' '}
          <select value={file.id} onChange={(event) => onScenario(event.target.value)} aria-label="Scenario">
            {scenarios.map((entry) => <option key={entry.id} value={entry.id}>{entry.id}</option>)}
          </select>
        </label>
        <label>Session{' '}
          <select value={server.sessionMode} onChange={(event) => server.setSessionMode(event.target.value as SessionMode)} aria-label="Session mode">
            {(['live', 'disconnected', 'awaiting-baseline', 'checkpoint'] as const).map((mode) => <option key={mode} value={mode}>{mode}</option>)}
          </select>
        </label>
        <button type="button" onClick={() => server.setSessionMode(server.sessionMode === 'live' ? 'disconnected' : 'live')}>Toggle connection</button>
        <button type="button" disabled={next === null} onClick={() => server.advance()} title={next ?? 'This scenario has no more runtime steps'}>
          Advance runtime{next ? `: ${next}` : ''}
        </button>
        {file.alternate ? <button type="button" onClick={() => server.switchAccount()}>Switch account ({server.account === 1 ? 'to 2' : 'to 1'})</button> : null}
        <button type="button" onClick={onReset}>Reset scenario</button>
        <button type="button" onClick={() => setShowLog((current) => !current)} aria-expanded={showLog}>
          Intercepted: {writes} write{writes === 1 ? '' : 's'}{blocked ? `, ${blocked} blocked request${blocked === 1 ? '' : 's'}` : ''}
        </button>
        {file.viewport ? <span>Suggested width: {file.viewport.width}px</span> : null}
        <span className="dock-sha" data-candidate>Candidate {candidate}</span>
      </div>
      {showLog ? (
        <div className="dock-log" role="log" aria-label="Intercepted requests">
          {server.log.length === 0 ? <div>Nothing intercepted yet.</div> : server.log.slice(-40).map((entry, index) => (
            <div key={`${entry.at}:${index}`} className={entry.kind === 'blocked' ? 'dock-blocked' : undefined}>
              {new Date(entry.at).toLocaleTimeString()} · {entry.kind} · {entry.detail}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}
