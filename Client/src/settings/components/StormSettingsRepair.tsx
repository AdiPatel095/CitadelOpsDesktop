import type { GameStateV2 } from '../../api/Contracts';
import { LocalizedText } from '../../i18n/LocalizedText';
import { Button } from '../../components/ui';
import { legacyStormRepairKeys, stormRepairDraft } from '../stormRole';

export function StormSettingsRepair<T>({ entries, state, onChange, stormLegacyKey }: {
  stormLegacyKey?: string; entries: Readonly<Record<string, T>>; state: GameStateV2 | null; onChange: (draft: Record<string, T>) => void;
}) {
  const stale = Object.keys(entries).filter((key) => key !== stormLegacyKey && /^\d+$/.test(key) && state && Object.keys(state.castles).length > 0 && !state.castles[key]);
  const repairable = legacyStormRepairKeys(entries, state, stormLegacyKey);
  return <>{stale.map((key) => <div key={key} className="flex flex-wrap items-center gap-2 text-sm text-text-muted">
    <LocalizedText messageKey="castleRequirement.missingOption" params={{ id: key }} />
    {repairable.includes(key) && <Button variant="ghost" size="sm" onClick={() => onChange(stormRepairDraft(entries, key, state, stormLegacyKey))}>
      <LocalizedText messageKey="stormRole.useLegacy" />
    </Button>}
    <Button variant="ghost" size="sm" onClick={() => { const draft = { ...entries }; delete draft[key]; onChange(draft); }}>
      <LocalizedText messageKey="stormRole.remove" />
    </Button>
  </div>)}</>;
}
