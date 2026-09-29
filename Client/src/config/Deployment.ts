/**
 * Desktop counterpart of the hosted `config/Deployment.ts` presence API
 * (CIT-16). The desktop client always talks to a live local runtime, so the
 * presence is constant; the hook exists so shared modal code is identical.
 */
export interface HostedRuntimePresence {
  mode: 'live' | 'checkpoint';
  checkpointObservedAt?: string;
  retryAt?: string;
}

const LIVE: HostedRuntimePresence = { mode: 'live' };

export function useHostedRuntimePresence(): HostedRuntimePresence {
  return LIVE;
}
