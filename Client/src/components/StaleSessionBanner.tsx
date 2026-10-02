import React from 'react';
import { LocalizedText } from '../i18n/LocalizedText';
import { useCitadelAPI } from '../api/ApiContext';
import { useAuth } from '../context/AuthContext';
import { Banner, Button } from './ui';

/** Shows last known data while the game session is disconnected. */
const StaleSessionBanner: React.FC = () => {
  const { gameLoggedIn, startGame } = useAuth();
  const { state } = useCitadelAPI();
  const backgroundConnection = state?.session.mode === 'background';
  if (gameLoggedIn) return null;
  return (
    <Banner tone="warning" role="status"
      title={<LocalizedText messageKey="ui.components.staleSessionBanner.disconnected.last.known.data.166a8c99" />}
      action={<Button variant="ghost" size="sm" onClick={() => startGame()}><LocalizedText messageKey="bot.start" /></Button>}>
      <LocalizedText messageKey={backgroundConnection
        ? 'ui.components.staleSessionBanner.body.reconnect' : 'ui.components.staleSessionBanner.body.reloadTab'} />
    </Banner>
  );
};
export default StaleSessionBanner;
