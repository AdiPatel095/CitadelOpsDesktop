import React, { useEffect, useMemo, useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { useCastleFocus } from '../context/CastleFocusContext';
import { Select, type SelectOption } from './ui';

const CastleFocusSwitcher: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { castle, castles, selectCastle } = useCastleFocus();
  const [pendingCastleId, setPendingCastleId] = useState<number | null>(null);
  const currentCastleId = castle?.id ?? 0;

  useEffect(() => {
    if (pendingCastleId != null && currentCastleId === pendingCastleId) setPendingCastleId(null);
  }, [currentCastleId, pendingCastleId]);

  useEffect(() => {
    if (pendingCastleId == null) return;
    const t = window.setTimeout(() => setPendingCastleId(null), 12_000);
    return () => window.clearTimeout(t);
  }, [pendingCastleId]);

  if (castles.length === 0) return null;

  const busy = pendingCastleId != null && gameLoggedIn;

  const dropdownOptions: SelectOption[] = castles.map((candidate) => ({
    value: String(candidate.id),
    label: `${candidate.name?.trim() || `Castle ${candidate.id}`}${candidate.kingdomId !== 0 ? ` · K${candidate.kingdomId}` : ''}`,
  }));

  const placeholderText = currentCastleId > 0
    ? `${castle?.name?.trim() || `Castle ${currentCastleId}`}${gameLoggedIn ? ' (in-game)' : ' (last known)'}`
    : 'Select castle';

  return (
    <div className="flex items-center min-w-[14rem] sm:min-w-[18rem]">
      <Select
        value={currentCastleId > 0 ? String(currentCastleId) : ''}
        options={dropdownOptions}
        menuGrowToViewport
        onChange={(v) => {
          const castleId = Number(v);
          if (!Number.isFinite(castleId) || castleId <= 0) return;
          if (gameLoggedIn) setPendingCastleId(castleId);
          selectCastle(castleId);
        }}
        placeholder={placeholderText}
        disabled={busy}
        className="w-full"
      />
    </div>
  );
};

export default CastleFocusSwitcher;
