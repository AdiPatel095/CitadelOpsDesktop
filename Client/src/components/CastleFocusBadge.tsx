import React from 'react';
import { useAuth } from '../context/AuthContext';
import { useCastleFocus } from '../context/CastleFocusContext';
import CastleFocusHoverPopover from './CastleFocusHoverPopover';
import { castleFocusDisplayName } from '../types/castleFocusState.ts';

/** Header chip: focused castle name only; hover shows decoration names (CSS popover — native title is unreliable). */
const CastleFocusBadge: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { castleFocus } = useCastleFocus();

  if (!gameLoggedIn) {
    return null;
  }

  const label = castleFocusDisplayName(castleFocus);

  return (
    <CastleFocusHoverPopover castleFocus={castleFocus} className="max-w-[220px]">
      <div className="rounded-global flex w-full min-w-0 items-center gap-2 border border-teal-500/30 bg-teal-500/10 px-3 py-1.5">
        <span className="shrink-0 text-[9px] font-bold uppercase tracking-wider text-teal-400/90">Focus</span>
        <span className="min-w-0 truncate text-xs font-semibold text-teal-200">{label}</span>
      </div>
    </CastleFocusHoverPopover>
  );
};

export default CastleFocusBadge;
