import { useSyncExternalStore } from 'react';

/**
 * The hosted account the command center is showing, as the portal reports it (CIT-19). The portal sets it before the
 * client renders; on desktop nothing ever sets it, so every reader sees `null` and behaves as before. It carries only
 * what the account record already shows (status, wait reason, the game's typed login failure); never a credential.
 */
export interface HostedAccountConnection {
  accountId: string;
  name: string;
  /** `HostedAccount.status`. */
  status: string;
  statusDetail?: string;
  waitUntil?: string;
  waitReason?: string;
  /** The account is switched on for a hosted runtime. */
  enabled: boolean;
  hasCredential: boolean;
  /** A hosted runtime was created for this account before (`HostedAccount.runtimeId`); it may not be running now. */
  runtimeCreated: boolean;
  /** `HostedAccount.observed`. */
  runtimePresent: boolean;
  loggedIn: boolean;
  loginFailure?: { class?: string; fatal?: boolean; suspendedUntil?: string };
  server?: string;
}

let current: HostedAccountConnection | null = null;
const listeners = new Set<() => void>();

export const setHostedAccountConnection = (next: HostedAccountConnection | null): void => {
  if (JSON.stringify(current) === JSON.stringify(next)) return;
  current = next;
  for (const listener of listeners) listener();
};

export const getHostedAccountConnection = (): HostedAccountConnection | null => current;

const subscribe = (listener: () => void) => { listeners.add(listener); return () => { listeners.delete(listener); }; };

export const useHostedAccountConnection = (): HostedAccountConnection | null => (
  useSyncExternalStore(subscribe, getHostedAccountConnection, getHostedAccountConnection)
);

let openAccountCenter: (() => void) | null = null;

/** The portal's way to open the Account Center with this account selected. Never set on desktop. */
export const setHostedAccountCenterOpener = (opener: (() => void) | null): void => { openAccountCenter = opener; };
export const getHostedAccountCenterOpener = (): (() => void) | null => openAccountCenter;
