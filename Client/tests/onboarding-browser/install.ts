import { VISUAL_STATES_KEY } from './visualStates';
import type { LogEntry } from './fixtureServer';
import type { FixtureServer } from './fixtureServer';
import { FixtureSocket } from './fixtureSocket';
import { installNetworkGuard } from './networkGuard';

/**
 * Installs the fixture server as the desktop page's only network (CIT-22): every `/api/` request the production client
 * makes is answered by `FixtureServer`, its event stream by `FixtureSocket`, and everything else is denied by the
 * network guard.
 */
const EVENTS_PATH = '/api/v2/events';

export interface InstalledTransport {
  server: FixtureServer;
  blocked: LogEntry[];
}

export function installFixtureTransport(server: FixtureServer): InstalledTransport {
  const blocked: LogEntry[] = [];
  installNetworkGuard({
    record: (what) => {
      blocked.push({ at: Date.now(), kind: 'blocked', detail: what });
      server.log.push({ at: Date.now(), kind: 'blocked', detail: `${what} (blocked: the preview has no network)` });
    },
    answer: (url, method, body) => {
      if (!url.pathname.startsWith('/api/')) return null;
      // A short delay keeps loading states visible instead of flashing.
      return new Promise<Response>((resolve) => { window.setTimeout(() => { void server.handle(`${url.pathname}${url.search}`, method, body, localStorage.getItem(VISUAL_STATES_KEY)).then(resolve); }, 30); });
    },
    socket: (url) => (url.includes(EVENTS_PATH) ? new FixtureSocket(url, server) : null),
  });
  // Revisions must keep moving so successive snapshots are accepted by the client's stale-revision guard.
  window.setInterval(() => { server.revision += 1; }, 30_000);
  return { server, blocked };
}
