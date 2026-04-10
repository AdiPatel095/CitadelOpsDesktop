import React, { useEffect, useMemo, useState } from 'react';
import { useAuth } from '../context/AuthContext';
import { useCastleFocus } from '../context/CastleFocusContext';
import type { PlayerCastleOption } from '../types/castleFocusState.ts';
import { Select, type SelectOption } from './ui';

function optionKey(c: Pick<PlayerCastleOption, 'aid' | 'kingdomID'>): string {
  return `${c.aid}|${c.kingdomID}`;
}

function aidFromOptionKey(key: string | null): number {
  if (!key) return 0;
  const n = Number(key.split('|')[0]);
  return Number.isFinite(n) ? n : 0;
}

/**
 * Global control: castle list comes from every `castleFocus` payload (GCL / GameState), or from snapshot when disconnected.
 * Rendered in the Header next to the castle focus badge.
 */
const CastleFocusSwitcher: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const {
    castleFocus,
    requestPlayerCastleFocus,
    setOfflineCastleFocusKey,
    offlineCastleFocusKey,
  } = useCastleFocus();
  const [pendingKey, setPendingKey] = useState<string | null>(null);

  const rawOptions = useMemo(() => castleFocus?.playerCastles ?? [], [castleFocus?.playerCastles]);

  const currentAid = castleFocus?.aid && castleFocus.aid > 0 ? castleFocus.aid : 0;
  const currentKey =
    currentAid > 0 ? `${currentAid}|${castleFocus?.kingdomID ?? 0}` : '';

  const exactMatched = useMemo(
    () => rawOptions.some((c) => optionKey(c) === currentKey),
    [rawOptions, currentKey]
  );

  const sameAidOptions = useMemo(
    () => rawOptions.filter((c) => c.aid === currentAid),
    [rawOptions, currentAid]
  );

  /** JAA KID can differ from GCL kingdom on the same castle (e.g. desert/ice); still one row per aid in practice. */
  const matched = exactMatched || (currentAid > 0 && sameAidOptions.length === 1);

  const selectValue = useMemo(() => {
    if (
      !gameLoggedIn &&
      offlineCastleFocusKey &&
      rawOptions.some((c) => optionKey(c) === offlineCastleFocusKey)
    ) {
      return offlineCastleFocusKey;
    }
    if (exactMatched) return currentKey;
    if (sameAidOptions.length === 1) return optionKey(sameAidOptions[0]);
    return '';
  }, [
    gameLoggedIn,
    offlineCastleFocusKey,
    rawOptions,
    exactMatched,
    currentKey,
    sameAidOptions,
  ]);

  useEffect(() => {
    if (pendingKey == null) return;
    if (currentKey === pendingKey) {
      setPendingKey(null);
      return;
    }
    const pendingAid = aidFromOptionKey(pendingKey);
    if (pendingAid > 0 && currentAid === pendingAid) {
      setPendingKey(null);
    }
  }, [currentKey, pendingKey, currentAid]);

  useEffect(() => {
    if (pendingKey == null) return;
    const t = window.setTimeout(() => setPendingKey(null), 12_000);
    return () => window.clearTimeout(t);
  }, [pendingKey]);

  if (rawOptions.length === 0) {
    return null;
  }

  const busy = pendingKey != null && gameLoggedIn;

  const dropdownOptions: SelectOption[] = rawOptions.map((c) => ({
    value: optionKey(c),
    label: `${c.name}${c.kingdomID !== 0 ? ` · K${c.kingdomID}` : ''}`
  }));

  const placeholderText = currentAid > 0
    ? `${castleFocus?.castleName?.trim() || 'Castle'}${gameLoggedIn ? ' (in-game)' : ' (last known)'}`
    : 'Select castle';

  return (
    <div className="flex items-center min-w-[14rem] sm:min-w-[18rem]">
      <Select
        value={selectValue}
        options={dropdownOptions}
        onChange={(v) => {
          const c = rawOptions.find((o) => optionKey(o) === v);
          if (!c) return;
          if (!gameLoggedIn) {
            setOfflineCastleFocusKey(v);
            return;
          }
          setPendingKey(v);
          requestPlayerCastleFocus({
            castleId: c.aid,
            kingdomId: c.kingdomID,
            mapX: c.mapX,
            mapY: c.mapY,
          });
        }}
        placeholder={placeholderText}
        disabled={busy}
        className="w-full"
      />
    </div>
  );
};

export default CastleFocusSwitcher;
