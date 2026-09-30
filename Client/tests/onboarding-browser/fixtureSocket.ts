import type { FixtureServer } from './fixtureServer';

type SocketListener = ((event: MessageEvent) => void) | null;

/**
 * The event stream the production client opens (`/api/v2/events`), answered by the fixture server (CIT-22). It carries the
 * same three envelopes a real stream opens with and repeats them whenever the fixture's state changes. It never opens a
 * connection: there is nothing behind it.
 */
export class FixtureSocket {
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
