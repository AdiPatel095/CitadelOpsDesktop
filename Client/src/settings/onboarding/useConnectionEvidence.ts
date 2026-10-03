import { useMemo } from 'react';
import { useCitadelAPI } from '../../api/useCitadelAPI';
import { useHostedAccountConnection } from '../../config/HostedAccountConnection';
import { useHostedRuntimePresence } from '../../config/Deployment';
import { useAuth } from '../../context/useAuth';
import type { ConnectionEvidence } from './checklist';

/**
 * The connection evidence the checklist reads (CIT-19): the dashboard link, the game session, and, only in the hosted
 * product, the runtime presence and the account record the portal reports. Read-only.
 */
export function useConnectionEvidence(): ConnectionEvidence {
  const { state } = useCitadelAPI();
  const { gameLoggedIn, dashboardConnectionStatus } = useAuth();
  const presence = useHostedRuntimePresence();
  const account = useHostedAccountConnection();
  const session = state?.session ?? null;
  return useMemo<ConnectionEvidence>(() => ({
    dashboard: dashboardConnectionStatus,
    session: session ? {
      status: session.status, loggedIn: session.loggedIn, socketReady: session.socketReady,
      generation: session.generation, baselineGeneration: session.baselineGeneration,
      detail: session.detail, retryAt: session.retryAt, cooldownUntil: session.cooldownUntil,
      loginFailure: session.loginFailure,
    } : null,
    gameLoggedIn,
    ...(account || presence.mode === 'checkpoint' ? {
      hosted: {
        presence: presence.mode,
        ...(account ? {
          accountStatus: account.status, observedLoggedIn: account.loggedIn, runtimePresent: account.runtimePresent,
          loginFailure: account.loginFailure,
        } : {}),
      },
    } : {}),
    ...(session?.changedAt ? { observedAt: session.changedAt } : {}),
  }), [account, dashboardConnectionStatus, gameLoggedIn, presence.mode, session]);
}
