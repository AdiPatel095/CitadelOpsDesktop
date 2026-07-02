import React from 'react';
import { useAuth } from '../context/AuthContext';
import { useCastleFocus } from '../context/CastleFocusContext';
import { castleFocusDisplayName } from '../types/CastleFocusState.ts';

/** Header chip: focused castle name only. */
const CastleFocusBadge: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { castleFocus } = useCastleFocus();

  if (!gameLoggedIn) {
    return null;
  }

  const label = castleFocusDisplayName(castleFocus);

  return (
    <div className="liquid-glass-edge flex w-full min-w-0 items-center gap-2 rounded-full px-3 py-1.5 text-primary">
      <span className="shrink-0 text-[9px] font-bold uppercase text-primary/80">Focus</span>
      <span className="min-w-0 truncate text-xs font-semibold text-text-main">{label}</span>
    </div>
  );
};

export default CastleFocusBadge;
