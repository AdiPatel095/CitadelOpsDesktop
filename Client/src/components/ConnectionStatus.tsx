import { LocalizedText } from '../i18n/LocalizedText';
import { useLocale } from '../i18n/LocaleContext';
import type { PlayerStatusDescription } from '../settings/readiness/playerStatus';
import { StatusBadge } from './ui/StatusBadge';
/** Native disclosure gives touch and keyboard users the same full reason as hover. */
export function ConnectionStatus({ value }: { value: PlayerStatusDescription }) {
  const { t } = useLocale();
  return <details className="player-connection-panel">
    <summary aria-label={t('playerStatus.connectionDetails')}><StatusBadge {...value} inline canRevealReason /></summary>
    <div className="player-connection-details"><strong><LocalizedText messageKey="playerStatus.connectionPanel" /></strong><StatusBadge {...value} /></div>
  </details>;
}
