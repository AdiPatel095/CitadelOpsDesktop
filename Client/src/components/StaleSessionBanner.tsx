import React from 'react';
import { useAuth } from '../context/AuthContext';

/**
 * Shown when the game websocket is disconnected: views may show last known or snapshot-backed data only.
 */
const StaleSessionBanner: React.FC = () => {
  const { gameLoggedIn, startGame } = useAuth();

  if (gameLoggedIn) {
    return null;
  }

  return (
    <div
      role="status"
      className="rounded-global border border-warning/40 bg-warning/10 px-4 py-3 text-sm text-text-main"
    >
      <p className="font-medium text-warning">Disconnected — last known data</p>
      <p className="mt-1 text-xs text-text-muted">
        Figures below may be out of date.{' '}
        <button
          type="button"
          onClick={() => startGame()}
          className="font-semibold text-primary underline underline-offset-2 hover:text-primary/90"
        >
          Start Bot
        </button>{' '}
        to reload the game tab and refresh live data.
      </p>
    </div>
  );
};

export default StaleSessionBanner;
