import React, { useEffect, useMemo, useState } from 'react';
import { ChevronDown } from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import { useCastleFocus } from '../context/CastleFocusContext';
import type { PlayerCastleOption } from '../types/castleFocusState.ts';

function optionKey(c: Pick<PlayerCastleOption, 'aid' | 'kingdomID'>): string {
  return `${c.aid}|${c.kingdomID}`;
}

function aidFromOptionKey(key: string | null): number {
  if (!key) return 0;
  const n = Number(key.split('|')[0]);
  return Number.isFinite(n) ? n : 0;
}

/**
 * Global control: castle list comes from every `castleFocus` payload (GCL / GameState),
 * not from Units or other views. Rendered from App below the header so it is not squeezed
 * by header chrome and works the same on every screen.
 */
const CastleFocusSwitcher: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { castleFocus, requestPlayerCastleFocus } = useCastleFocus();
  const [pendingKey, setPendingKey] = useState<string | null>(null);

  const options = useMemo(() => castleFocus?.playerCastles ?? [], [castleFocus?.playerCastles]);

  const currentAid = castleFocus?.aid && castleFocus.aid > 0 ? castleFocus.aid : 0;
  const currentKey =
    currentAid > 0 ? `${currentAid}|${castleFocus?.kingdomID ?? 0}` : '';

  const exactMatched = useMemo(
    () => options.some((c) => optionKey(c) === currentKey),
    [options, currentKey]
  );

  const sameAidOptions = useMemo(
    () => options.filter((c) => c.aid === currentAid),
    [options, currentAid]
  );

  /** JAA KID can differ from GCL kingdom on the same castle (e.g. desert/ice); still one row per aid in practice. */
  const matched = exactMatched || (currentAid > 0 && sameAidOptions.length === 1);

  const selectValue = exactMatched
    ? currentKey
    : sameAidOptions.length === 1
      ? optionKey(sameAidOptions[0])
      : '';

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

  if (!gameLoggedIn || options.length === 0) {
    return null;
  }

  const busy = pendingKey != null;

  return (
    <div className="flex items-center">
      <label htmlFor="castle-focus-select" className="sr-only">
        Focus castle
      </label>
      <div className="relative">
        <select
          id="castle-focus-select"
          disabled={busy}
          value={selectValue}
          onChange={(e) => {
            const v = e.target.value;
            if (!v) return;
            const c = options.find((o) => optionKey(o) === v);
            if (!c) return;
            setPendingKey(v);
            requestPlayerCastleFocus({
              castleId: c.aid,
              kingdomId: c.kingdomID,
              mapX: c.mapX,
              mapY: c.mapY,
            });
          }}
          className="max-w-[14rem] cursor-pointer appearance-none rounded-global border border-border-base bg-bg-app/80 py-1.5 pl-2.5 pr-8 text-xs font-semibold text-text-main shadow-sm transition-colors hover:border-primary/40 hover:bg-bg-input focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary/40 disabled:cursor-wait disabled:opacity-60 sm:max-w-[18rem]"
        >
          {!matched && (
            <option value="" disabled>
              {currentAid > 0
                ? `${castleFocus?.castleName?.trim() || 'Castle'} (in-game)`
                : 'Select castle'}
            </option>
          )}
          {options.map((c) => (
            <option key={optionKey(c)} value={optionKey(c)}>
              {c.name}
              {c.kingdomID !== 0 ? ` · K${c.kingdomID}` : ''}
            </option>
          ))}
        </select>
        <ChevronDown
          className="pointer-events-none absolute right-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-text-muted"
          aria-hidden
          strokeWidth={2.5}
        />
      </div>
    </div>
  );
};

export default CastleFocusSwitcher;
