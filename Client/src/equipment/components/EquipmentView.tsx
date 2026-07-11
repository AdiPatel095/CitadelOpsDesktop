import { useEffect, useMemo, useState } from 'react';
import { Crown, Gem, RefreshCw, Shield, Swords } from 'lucide-react';
import { useCitadelAPI } from '../../api/ApiContext';
import type {
  CastellanStateV2,
  CommanderStateV2,
  EquipmentInstanceV2,
  GemInstanceV2,
} from '../../api/Contracts';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle, PillSelector } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';

type EquipmentMode = 'Commander' | 'Castellan';
type Leader = CommanderStateV2 | CastellanStateV2;

const slotNames: Record<string, string> = {
  '1': 'Armor',
  '2': 'Weapon',
  '3': 'Helmet',
  '4': 'Artifact',
  '6': 'Hero',
};

export default function EquipmentView() {
  const { state, submitIntent } = useCitadelAPI();
  const { getEffect, getEquipment, getGem } = useMetadata();
  const [mode, setMode] = useState<EquipmentMode>('Commander');
  const [selectedID, setSelectedID] = useState<number | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const [operationError, setOperationError] = useState('');

  const leaders = useMemo<Leader[]>(() => {
    const values = mode === 'Commander'
      ? Object.values(state?.commanders ?? {})
      : Object.values(state?.castellans ?? {});
    return values.sort((left, right) => leaderPosition(left) - leaderPosition(right) || left.id - right.id);
  }, [mode, state?.castellans, state?.commanders]);

  useEffect(() => {
    if (selectedID != null && leaders.some((leader) => leader.id === selectedID)) return;
    setSelectedID(leaders[0]?.id ?? null);
  }, [leaders, selectedID]);

  const selected = leaders.find((leader) => leader.id === selectedID) ?? null;
  const equipment = Object.entries(selected?.equipment ?? {})
    .map(([slot, instanceID]) => ({
      slot,
      item: state?.inventory.equipment[String(instanceID)],
      gem: selected?.gems[slot] != null
        ? state?.inventory.gems[String(selected.gems[slot])]
        : undefined,
    }))
    .filter((entry): entry is { slot: string; item: EquipmentInstanceV2; gem: GemInstanceV2 | undefined } => entry.item != null)
    .sort((left, right) => Number(left.slot) - Number(right.slot));

  const refresh = () => {
    setRefreshing(true);
    setOperationError('');
    void submitIntent('equipment.refresh')
      .catch((error) => setOperationError(error instanceof Error ? error.message : 'Could not refresh equipment'))
      .finally(() => setRefreshing(false));
  };

  return (
    <div className="equipment-view-shell flex flex-col gap-6">
      <StaleSessionBanner />

      <Card className="liquid-prominent-header-card">
        <CardHeader className="liquid-card-header-prominent flex-row flex-wrap items-center gap-3">
          <div className="flex min-w-0 flex-1 items-center gap-3">
            <div className="flex h-9 w-9 items-center justify-center rounded-lg bg-primary/10 text-primary">
              <Shield className="h-5 w-5" />
            </div>
            <div>
              <CardTitle className="text-lg">Equipment</CardTitle>
              <p className="mt-0.5 text-xs text-text-muted">
                Live loadouts normalized from game frames and official item definitions.
              </p>
            </div>
          </div>
          <PillSelector
            value={mode}
            options={['Commander', 'Castellan']}
            onChange={(value) => setMode(value as EquipmentMode)}
          />
          <Button
            variant="secondary"
            size="sm"
            disabled={!state?.session.loggedIn || refreshing}
            onClick={refresh}
            leftIcon={<RefreshCw className={`h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} />}
          >
            Refresh
          </Button>
        </CardHeader>
      </Card>

      {operationError && <p className="text-sm text-error">{operationError}</p>}

      <div className="grid min-h-0 gap-6 xl:grid-cols-[minmax(15rem,0.72fr)_minmax(0,1.28fr)]">
        <Card className="min-h-0">
          <CardHeader className="border-b border-border-base">
            <CardTitle className="flex items-center justify-between text-base">
              <span>{mode}s</span>
              <Badge variant="secondary">{leaders.length}</Badge>
            </CardTitle>
          </CardHeader>
          <CardContent className="max-h-[65vh] space-y-2 overflow-y-auto p-3 custom-scrollbar">
            {leaders.length === 0 ? (
              <p className="p-4 text-sm text-text-muted">No {mode.toLowerCase()} loadouts have been observed.</p>
            ) : leaders.map((leader) => {
              const active = leader.id === selectedID;
              return (
                <button
                  type="button"
                  key={leader.id}
                  onClick={() => setSelectedID(leader.id)}
                  className={`flex w-full items-center gap-3 rounded-global border px-3 py-3 text-left transition-colors ${
                    active
                      ? 'border-primary/50 bg-primary/10 text-primary'
                      : 'border-border-base bg-bg-card/45 text-text-main hover:border-primary/30 hover:bg-bg-card-hover/50'
                  }`}
                >
                  {mode === 'Commander' ? <Swords className="h-4 w-4 shrink-0" /> : <Crown className="h-4 w-4 shrink-0" />}
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-semibold">{leaderName(leader, mode)}</span>
                    <span className="block text-[11px] text-text-muted">ID {leader.id}</span>
                  </span>
                  {'available' in leader && (
                    <Badge variant={leader.available ? 'success' : 'secondary'}>
                      {leader.available ? 'Free' : 'Busy'}
                    </Badge>
                  )}
                </button>
              );
            })}
          </CardContent>
        </Card>

        <Card className="min-h-0">
          <CardHeader className="border-b border-border-base">
            <CardTitle className="text-base">
              {selected ? leaderName(selected, mode) : `Select a ${mode.toLowerCase()}`}
            </CardTitle>
          </CardHeader>
          <CardContent className="p-4">
            {!selected ? (
              <p className="py-10 text-center text-sm text-text-muted">No loadout selected.</p>
            ) : equipment.length === 0 ? (
              <p className="py-10 text-center text-sm text-text-muted">This loadout has no observed equipment.</p>
            ) : (
              <div className="grid gap-3 md:grid-cols-2">
                {equipment.map(({ slot, item, gem }) => {
                  const definition = getEquipment(item.definitionId);
                  const gemDefinition = gem ? getGem(gem.definitionId) : undefined;
                  return (
                    <div key={item.id} className="rounded-global border border-border-base bg-bg-card/55 p-4">
                      <div className="flex items-start justify-between gap-3">
                        <div className="min-w-0">
                          <p className="text-[10px] font-bold uppercase tracking-wider text-text-muted">
                            {slotNames[slot] ?? `Slot ${slot}`}
                          </p>
                          <h3 className="mt-1 truncate text-sm font-bold text-text-main">
                            {definition?.name ?? `Equipment ${item.definitionId}`}
                          </h3>
                          <p className="mt-1 text-[11px] text-text-muted">
                            Instance {item.id}
                            {item.level != null ? ` · Level ${item.level}` : ''}
                            {item.rarityId != null ? ` · Rarity ${item.rarityId}` : ''}
                          </p>
                        </div>
                        {item.setId ? <Badge variant="outline">Set {item.setId}</Badge> : null}
                      </div>

                      <EffectList effects={item.effects} getEffectName={(id) => getEffect(id)?.name} />

                      {gem && (
                        <div className="mt-3 rounded-lg border border-purple-500/25 bg-purple-500/5 p-3">
                          <div className="flex items-center gap-2 text-xs font-semibold text-purple-300">
                            <Gem className="h-3.5 w-3.5" />
                            {gemDefinition?.name ?? `Gem ${gem.definitionId}`}
                            {gem.level ? <span className="text-text-muted">Level {gem.level}</span> : null}
                          </div>
                          <EffectList effects={gem.effects} getEffectName={(id) => getEffect(id)?.name} compact />
                        </div>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

function EffectList({
  effects,
  getEffectName,
  compact = false,
}: {
  effects: Record<string, number[]>;
  getEffectName: (id: number) => string | undefined;
  compact?: boolean;
}) {
  const entries = Object.entries(effects);
  if (entries.length === 0) return null;
  return (
    <div className={`${compact ? 'mt-2' : 'mt-3'} space-y-1.5`}>
      {entries.map(([id, values]) => (
        <div key={id} className="flex items-start justify-between gap-3 text-xs">
          <span className="min-w-0 truncate text-text-muted">{getEffectName(Number(id)) ?? `Effect ${id}`}</span>
          <span className="shrink-0 font-mono font-semibold text-text-main">
            {values.map(formatEffectValue).join(' / ')}
          </span>
        </div>
      ))}
    </div>
  );
}

function leaderPosition(leader: Leader): number {
  return 'visiblePosition' in leader ? leader.visiblePosition ?? leader.id : leader.id;
}

function leaderName(leader: Leader, mode: EquipmentMode): string {
  if (leader.name) return leader.name;
  if ('castleId' in leader && leader.castleId) return `Castle ${leader.castleId}`;
  return `${mode} ${leaderPosition(leader)}`;
}

function formatEffectValue(value: number): string {
  return Number.isInteger(value) ? value.toLocaleString() : value.toLocaleString(undefined, { maximumFractionDigits: 1 });
}
