import type { LogEntry } from './fixtureServer';
import { FixtureServer } from './fixtureServer';

/**
 * Installs the fixture server as the page's only network (CIT-22). Everything the production client asks of `/api/v2/*`
 * is answered by `FixtureServer`; every other network path is refused (deny by default): cross-origin fetch, XMLHttpRequest,
 * WebSocket, EventSource and sendBeacon are blocked and recorded. Same-origin module and asset loading by Vite is the only
 * thing let through, and only for paths that are not `/api/`. Nothing here contacts a game, an account or a production API.
 */
const EVENTS_PATH = '/api/v2/events';

type SocketListener = ((event: MessageEvent) => void) | null;

class FixtureSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;
  readonly url: string;
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: SocketListener = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;
  private timer: number;
  private off: () => void;
  private readonly server: FixtureServer;

  constructor(url: string, server: FixtureServer) {
    this.server = server;
    this.url = url;
    this.timer = window.setTimeout(() => {
      this.readyState = FixtureSocket.OPEN;
      this.onopen?.();
      this.pushAll();
    }, 60);
    this.off = server.onEvent((event) => { if (event === 'snapshot' && this.readyState === FixtureSocket.OPEN) this.pushAll(); });
  }

  private push(type: string, payload: unknown) {
    this.onmessage?.(new MessageEvent('message', { data: JSON.stringify({ v: 2, type, revision: this.server.revision, payload }) }));
  }

  private pushAll() {
    this.push('state.snapshot', this.server.state());
    this.push('config.changed', this.server.configuration());
    this.push('operations.snapshot', this.server.operations());
  }

  send() { /* The client submits intents over REST; nothing is expected here. */ }
  addEventListener() { /* Handlers are assigned as properties by the client. */ }
  removeEventListener() { /* See addEventListener. */ }

  close() {
    window.clearTimeout(this.timer);
    this.off();
    this.readyState = FixtureSocket.CLOSED;
    this.onclose?.();
  }
}

export interface InstalledTransport {
  server: FixtureServer;
  blocked: LogEntry[];
}

export function installFixtureTransport(server: FixtureServer): InstalledTransport {
  const blocked: LogEntry[] = [];
  const block = (what: string) => {
    blocked.push({ at: Date.now(), kind: 'blocked', detail: what });
    server.log.push({ at: Date.now(), kind: 'blocked', detail: `${what} (blocked: the preview has no network)` });
  };
  const realFetch = window.fetch.bind(window);

  window.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url, window.location.href);
    if (url.origin !== window.location.origin) {
      block(`fetch ${url.origin}${url.pathname}`);
      return Promise.reject(new TypeError('Simulated preview: network access is disabled.'));
    }
    if (url.pathname.startsWith('/api/')) {
      const method = (init?.method ?? (typeof input === 'object' && 'method' in input ? input.method : 'GET')) || 'GET';
      let body: unknown = undefined;
      try { body = typeof init?.body === 'string' ? JSON.parse(init.body) : undefined; } catch { body = undefined; }
      // A short delay keeps loading states visible instead of flashing.
      return new Promise<Response>((resolve) => { window.setTimeout(() => { void server.handle(`${url.pathname}${url.search}`, method, body).then(resolve); }, 30); });
    }
    return realFetch(input as RequestInfo, init);
  }) as typeof window.fetch;

  window.WebSocket = new Proxy(window.WebSocket, {
    construct(_target, args: [string | URL, (string | string[])?]) {
      const url = String(args[0]);
      if (url.includes(EVENTS_PATH)) return new FixtureSocket(url, server) as unknown as WebSocket;
      block(`WebSocket ${url}`);
      throw new DOMException('Simulated preview: network access is disabled.', 'SecurityError');
    },
  });
  if (typeof window.EventSource === 'function') {
    window.EventSource = new Proxy(window.EventSource, {
      construct(_target, args: [string | URL]) {
        block(`EventSource ${String(args[0])}`);
        throw new DOMException('Simulated preview: network access is disabled.', 'SecurityError');
      },
    });
  }
  window.XMLHttpRequest = new Proxy(window.XMLHttpRequest, {
    construct() {
      block('XMLHttpRequest');
      throw new DOMException('Simulated preview: network access is disabled.', 'SecurityError');
    },
  });
  if (typeof navigator.sendBeacon === 'function') {
    navigator.sendBeacon = (url: string | URL) => { block(`sendBeacon ${String(url)}`); return false; };
  }
  // Revisions must keep moving so successive snapshots are accepted by the client's stale-revision guard.
  window.setInterval(() => { server.revision += 1; }, 30_000);
  return { server, blocked };
}
