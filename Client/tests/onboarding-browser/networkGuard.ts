/**
 * Deny by default (CIT-22). Installs the preview's only network policy on the page: every cross-origin fetch,
 * XMLHttpRequest, WebSocket, EventSource and sendBeacon is refused and recorded; a same-origin request is answered by the
 * fixture when `answer` takes it, refused when `refuse` names it, and otherwise passed on (the dev server's own module
 * and asset loading). Nothing here contacts a game, an account or a production API.
 */
export interface NetworkGuardOptions {
  /** Called with a short description of everything refused. */
  record: (what: string) => void;
  /** Same-origin requests the fixture answers. Return null to let the request fall through. */
  answer?: (url: URL, method: string, body: unknown) => Promise<Response> | null;
  /** Same-origin paths that are refused because they would reach a backend. */
  refuse?: (url: URL) => boolean;
  /** WebSocket URLs the fixture answers; anything else is refused. Return null to refuse. */
  socket?: (url: string) => object | null;
}

const REFUSED = 'Simulated preview: network access is disabled.';

export function installNetworkGuard(options: NetworkGuardOptions): void {
  const { record } = options;
  const realFetch = window.fetch.bind(window);

  window.fetch = ((input: RequestInfo | URL, init?: RequestInit) => {
    const url = new URL(typeof input === 'string' ? input : input instanceof URL ? input.toString() : input.url, window.location.href);
    if (url.origin !== window.location.origin) {
      record(`fetch ${url.origin}${url.pathname}`);
      return Promise.reject(new TypeError(REFUSED));
    }
    const method = (init?.method ?? (typeof input === 'object' && 'method' in input ? input.method : 'GET')) || 'GET';
    let body: unknown = undefined;
    try { body = typeof init?.body === 'string' ? JSON.parse(init.body) : undefined; } catch { body = undefined; }
    const answered = options.answer?.(url, method, body) ?? null;
    if (answered) return answered;
    if (options.refuse?.(url)) {
      record(`fetch ${url.pathname}`);
      return Promise.reject(new TypeError(REFUSED));
    }
    return realFetch(input as RequestInfo, init);
  }) as typeof window.fetch;

  window.WebSocket = new Proxy(window.WebSocket, {
    construct(_target, args: [string | URL, (string | string[])?]) {
      const url = String(args[0]);
      const answered = options.socket?.(url) ?? null;
      if (answered) return answered as WebSocket;
      record(`WebSocket ${url}`);
      throw new DOMException(REFUSED, 'SecurityError');
    },
  });
  if (typeof window.EventSource === 'function') {
    window.EventSource = new Proxy(window.EventSource, {
      construct(_target, args: [string | URL]) {
        record(`EventSource ${String(args[0])}`);
        throw new DOMException(REFUSED, 'SecurityError');
      },
    });
  }
  window.XMLHttpRequest = new Proxy(window.XMLHttpRequest, {
    construct() {
      record('XMLHttpRequest');
      throw new DOMException(REFUSED, 'SecurityError');
    },
  });
  if (typeof navigator.sendBeacon === 'function') {
    navigator.sendBeacon = (url: string | URL) => { record(`sendBeacon ${String(url)}`); return false; };
  }
}
