import { useNow } from '../useNow';
import React, { useEffect, useMemo, useState } from 'react';
import { useCitadelAPI } from '../api/useCitadelAPI';
import { CitadelAPI } from '../api/CitadelClient';
import type { BackgroundLoginStatus, GameServerEntry } from '../api/Contracts';
import { useAuth } from '../context/useAuth';
import { LocalizedText } from '../i18n/LocalizedText';
import { messageLanguageAttributes } from '../i18n/messageLanguage';
import { useLocalizedMessage } from '../i18n/useLocalizedMessage';
import { configurationSection } from '../settings/Configuration';
import { backgroundLoginNeedsReauthorization, connectionControlAvailability, reauthorizeSavedLogin } from '../settings/connection/connectionControls';
import { describeSavedWorld, explainConnection, type RepairAction } from '../settings/connection/connectionExplain';
import { Button } from './ui/Button';
import { Modal } from './ui/Modal';

/**
 * Focused connection repair for the desktop app (CIT-19). It opens over whatever the player was doing (an open editor
 * stays mounted with its draft), says what the game reports, shows the saved world for checking, and offers only the
 * controls that already exist: Start Bot, Reconnect now, Re-enable saved login and Open connection settings. It never
 * collects a password and never guesses: an untyped rejection lists both things to check.
 */
export const ConnectionRepairDialog: React.FC<{ onClose: () => void; onOpenSettings: () => void }> = ({ onClose, onOpenSettings }) => {
  const now = useNow(1000);
  const { state, configuration, submitIntent } = useCitadelAPI();
  const auth = useAuth();
  const [saved, setSaved] = useState<BackgroundLoginStatus | null>(null);
  const [directory, setDirectory] = useState<GameServerEntry[]>([]);
  const [actionError, setActionError] = useState('');
  const [busy, setBusy] = useState(false);
  const session = state?.session ?? null;
  const mode: 'full' | 'background' = session?.mode === 'background' ? 'background' : 'full';

  useEffect(() => {
    let active = true;
    // Existing read-only endpoints: the saved background server (never the password) and the official world directory.
    void CitadelAPI.getBackgroundLoginStatus().then((status) => { if (active) setSaved(status); }).catch(() => undefined);
    void CitadelAPI.getGameServers().then((catalog) => { if (active) setDirectory(catalog.servers ?? []); }).catch(() => undefined);
    return () => { active = false; };
  }, []);

  const configuredServer = typeof configurationSection(configuration, 'session.connection').server === 'string'
    ? String(configurationSection(configuration, 'session.connection').server)
    : undefined;
  const world = useMemo(() => describeSavedWorld(saved?.server ?? configuredServer, directory), [configuredServer, directory, saved?.server]);
  const explanation = explainConnection({
    surface: 'desktop', mode,
    status: session?.status ?? 'disconnected',
    loggedIn: auth.gameLoggedIn,
    dashboard: auth.dashboardConnectionStatus,
    retryAt: session?.retryAt, cooldownUntil: session?.cooldownUntil,
    loginFailure: session?.loginFailure,
    savedWorld: mode === 'background' && world.kind !== 'none' ? world.short : undefined,
    now,
  });
  const detail = useLocalizedMessage(undefined, session?.detail ?? '');
  const controls = connectionControlAvailability(auth.gameConnectionState, auth.hasGameConnectionStatus);
  const configuredMode: 'full' | 'background' = configurationSection(configuration, 'session.connection').mode === 'background' ? 'background' : 'full';
  const canReenable = backgroundLoginNeedsReauthorization(configuredMode, session);
  const wants = new Set<RepairAction>(explanation.actions);

  const run = (action: () => Promise<unknown> | void) => {
    setActionError('');
    setBusy(true);
    void Promise.resolve()
      .then(action)
      .catch((error) => setActionError(error instanceof Error ? error.message : String(error)))
      .finally(() => setBusy(false));
  };

  return (
    <Modal
      isOpen
      onClose={onClose}
      maxWidth="lg"
      title={<LocalizedText messageKey="connectionRepair.title" />}
      footer={<Button variant="primary" onClick={onClose}><LocalizedText messageKey="connectionRepair.action.done" /></Button>}
    >
      <div className="space-y-4 text-sm text-text-main" data-connection-repair="desktop">
        <p className="text-xs text-text-muted"><LocalizedText messageKey="connectionRepair.intro" /></p>

        <section aria-labelledby="repair-status" className="space-y-1.5">
          <h3 id="repair-status" className="text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="connectionRepair.statusHeading" /></h3>
          <p className="font-semibold" data-repair-state={auth.gameConnectionState}>
            <LocalizedText messageKey="connectionRepair.state" params={{ state: auth.gameConnectionState }} />
            {' · '}
            <LocalizedText messageKey="connectionRepair.mode" params={{ mode }} />
          </p>
          {session?.browserName && mode === 'full' ? (
            <p className="text-xs text-text-muted"><LocalizedText messageKey="connectionRepair.browser" params={{ name: session.browserName }} /></p>
          ) : null}
          <p data-repair-explanation={explanation.typedClass ?? 'untyped'}><LocalizedText messageKey={explanation.messageKey} params={explanation.params} /></p>
          {explanation.showDetail && session?.detail ? (
            <p className="text-xs text-text-muted">
              <LocalizedText messageKey="connectionRepair.reported" />{' '}
              <span className="text-text-main" data-repair-detail {...messageLanguageAttributes(detail)}>{session.detail}</span>
            </p>
          ) : null}
        </section>

        <section aria-labelledby="repair-world" className="space-y-1.5">
          <h3 id="repair-world" className="text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="connectionRepair.worldHeading" /></h3>
          {mode === 'full' ? (
            <p><LocalizedText messageKey="connectionRepair.world.full" /></p>
          ) : world.kind === 'none' ? (
            <p><LocalizedText messageKey="connectionRepair.world.none" /></p>
          ) : world.kind === 'not-listed' ? (
            <p data-repair-world="not-listed"><LocalizedText messageKey="connectionRepair.world.notListed" params={{ code: world.code ?? '' }} /></p>
          ) : (
            <p data-repair-world="listed"><LocalizedText messageKey="connectionRepair.world.saved" params={{ world: world.long ?? '' }} /></p>
          )}
          {state?.account?.worldId ? (
            <p className="text-xs text-text-muted"><LocalizedText messageKey="connectionRepair.world.observed" params={{ world: state.account.worldId }} /></p>
          ) : null}
        </section>

        <section aria-labelledby="repair-actions" className="space-y-2">
          <h3 id="repair-actions" className="text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="connectionRepair.actionsHeading" /></h3>
          <div className="flex flex-wrap gap-2">
            {controls.canStart ? (
              <Button variant="primary" size="sm" disabled={busy || auth.dashboardConnectionStatus !== 'Connected' || auth.gameConnectionState === 'starting'} onClick={() => run(() => auth.startGame())}>
                <LocalizedText messageKey="connectionRepair.action.start" />
              </Button>
            ) : null}
            {controls.canReconnect || wants.has('reconnect') ? (
              <Button variant="outline" size="sm" disabled={busy || auth.dashboardConnectionStatus !== 'Connected'} onClick={() => run(() => auth.reconnectGame())}>
                <LocalizedText messageKey="connectionRepair.action.reconnect" />
              </Button>
            ) : null}
            {canReenable ? (
              <Button variant="outline" size="sm" disabled={busy} onClick={() => run(() => reauthorizeSavedLogin(submitIntent))}>
                <LocalizedText messageKey="connectionRepair.action.reenable" />
              </Button>
            ) : null}
            <Button variant="outline" size="sm" onClick={onOpenSettings}>
              <LocalizedText messageKey="connectionRepair.action.openSettings" />
            </Button>
          </div>
          {actionError ? <p role="alert" className="text-xs font-medium text-error"><LocalizedText messageKey="connectionRepair.actionFailed" params={{ reason: actionError }} /></p> : null}
        </section>
      </div>
    </Modal>
  );
};

export default ConnectionRepairDialog;
