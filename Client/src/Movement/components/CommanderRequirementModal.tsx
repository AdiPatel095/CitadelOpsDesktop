import React, { useEffect, useMemo, useState } from 'react';
import { CheckCircle2, Filter, Search, Trash2, UsersRound } from 'lucide-react';
import UnitImage from '../../components/UnitImage';
import { Badge, Button, Input, Modal } from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import { useMetadata, type MetadataItem } from '../../context/MetadataContext';
import type { GameStateV2 } from '../../api/Contracts';
import type {
  CommanderEquipmentEffectRequirement,
  CommanderFeatureID,
} from '../types/CommanderFeatureAssignments';

interface CommanderRequirementModalProps {
  isOpen: boolean;
  featureID: CommanderFeatureID | null;
  featureLabel: string;
  commanderIDs: readonly number[];
  requirement: CommanderEquipmentEffectRequirement | null;
  onClose: () => void;
  onApply: (requirement: CommanderEquipmentEffectRequirement) => void;
  onClear: () => void;
}

interface BonusTroopStat {
  key: string;
  effectDefinitionId: number;
  unitId?: number;
  effectName: string;
  unitName: string;
  commanderCount: number;
  minimumObserved: number | null;
  maximumObserved: number | null;
  areaTypeIDs: number[];
  scope: string;
}

interface MutableBonusTroopStat extends Omit<BonusTroopStat, 'commanderCount' | 'minimumObserved' | 'maximumObserved'> {
  valuesByCommander: Map<number, number>;
}

const CommanderRequirementModal: React.FC<CommanderRequirementModalProps> = ({
  isOpen,
  featureID,
  featureLabel,
  commanderIDs,
  requirement,
  onClose,
  onApply,
  onClear,
}) => {
  const { state } = useCitadelAPI();
  const { effects, troops } = useMetadata();
  const stats = useMemo(
    () => observedBonusTroopStats(state, commanderIDs, effects, troops, requirement),
    [commanderIDs, effects, requirement, state, troops],
  );
  const [search, setSearch] = useState('');
  const [selectedKey, setSelectedKey] = useState('');
  const [minimumValue, setMinimumValue] = useState('');
  const [maximumValue, setMaximumValue] = useState('');

  useEffect(() => {
    if (!isOpen) return;
    setSearch('');
    setSelectedKey(requirement ? statKey(requirement.effectDefinitionId, requirement.unitId) : '');
    setMinimumValue(requirement?.minimumValue != null ? formatNumber(requirement.minimumValue) : '');
    setMaximumValue(requirement?.maximumValue != null ? formatNumber(requirement.maximumValue) : '');
  }, [featureID, isOpen, requirement]);

  const filteredStats = useMemo(() => {
    const query = search.trim().toLowerCase();
    if (!query) return stats;
    return stats.filter((stat) => (
      `${stat.effectName} ${stat.unitName} ${stat.effectDefinitionId} ${stat.unitId ?? ''} ${stat.areaTypeIDs.join(' ')}`
        .toLowerCase()
        .includes(query)
    ));
  }, [search, stats]);
  const selected = stats.find((stat) => stat.key === selectedKey) ?? null;
  const minimum = Number(minimumValue);
  const maximum = maximumValue.trim() ? Number(maximumValue) : null;
  const validationError = !selected
    ? 'Select an equipped bonus-troop stat.'
    : !minimumValue.trim() || !Number.isFinite(minimum) || minimum < 0
      ? 'Enter a non-negative minimum.'
      : maximum != null && (!Number.isFinite(maximum) || maximum < minimum)
        ? 'Maximum must be greater than or equal to the minimum.'
        : null;

  const selectStat = (stat: BonusTroopStat) => {
    setSelectedKey(stat.key);
    if (stat.key !== selectedKey) {
      setMinimumValue(formatNumber(stat.minimumObserved ?? 0));
      setMaximumValue('');
    }
  };

  const apply = () => {
    if (!selected || validationError) return;
    onApply({
      kind: 'equipmentEffect',
      effectDefinitionId: selected.effectDefinitionId,
      ...(selected.unitId != null ? { unitId: selected.unitId } : {}),
      minimumValue: minimum,
      ...(maximum != null ? { maximumValue: maximum } : {}),
    });
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      maxWidth="3xl"
      title={`${featureLabel} commander equipment requirement`}
      footer={(
        <>
          <Button
            variant="danger"
            disabled={!requirement}
            onClick={onClear}
            leftIcon={<Trash2 className="h-4 w-4" />}
          >
            Clear requirement
          </Button>
          <div className="ml-auto flex gap-2">
            <Button variant="ghost" onClick={onClose}>Cancel</Button>
            <Button
              variant="primary"
              disabled={validationError != null}
              onClick={apply}
              leftIcon={<CheckCircle2 className="h-4 w-4" />}
            >
              Apply requirement
            </Button>
          </div>
        </>
      )}
    >
      <div className="flex flex-col gap-4">
        <div className="rounded-global border border-border-light bg-bg-card/45 px-4 py-3">
          <div className="flex items-center gap-2 text-sm font-semibold text-text-main">
            <Filter className="h-4 w-4 text-primary" />
            Require an equipped bonus-troop stat
          </div>
          <p className="mt-1 text-xs leading-relaxed text-text-muted">
            Only commanders whose currently equipped gear meets this limit can launch this function.
            Stats are discovered from live commander equipment and resolved by official effect and unit IDs.
            Event-scoped effects remain distinct and show their official target areas.
          </p>
        </div>

        <Input
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Search bonus troops, effects, unit IDs, or target areas…"
          aria-label="Search commander bonus troop stats"
          leftIcon={<Search className="h-4 w-4" />}
        />

        <div className="max-h-80 overflow-y-auto rounded-global border border-border-base bg-bg-app/55 p-2 custom-scrollbar">
          {filteredStats.length === 0 ? (
            <p className="px-3 py-8 text-center text-sm text-text-muted">
              No matching bonus-troop stats were found on the current commander equipment.
            </p>
          ) : (
            <div className="grid gap-2 sm:grid-cols-2">
              {filteredStats.map((stat) => {
                const selectedStat = stat.key === selectedKey;
                return (
                  <button
                    key={stat.key}
                    type="button"
                    aria-pressed={selectedStat}
                    aria-label={`Select ${stat.unitName} from ${stat.effectName}`}
                    onClick={() => selectStat(stat)}
                    className={`flex min-w-0 items-center gap-3 rounded-global border p-3 text-left transition-all ${
                      selectedStat
                        ? 'border-primary/55 bg-primary/10 shadow-[0_0_18px_color-mix(in_srgb,var(--color-primary)_12%,transparent)]'
                        : 'border-border-light bg-bg-card/40 hover:border-primary/30 hover:bg-bg-card-hover/60'
                    }`}
                  >
                    {stat.unitId != null ? (
                      <UnitImage unitId={stat.unitId} size={42} />
                    ) : (
                      <span className="flex h-[42px] w-[42px] shrink-0 items-center justify-center rounded-full border border-primary/25 bg-primary/10 text-primary">
                        <UsersRound className="h-5 w-5" />
                      </span>
                    )}
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-semibold text-text-main">{stat.unitName}</span>
                      <span className="block truncate text-[11px] text-text-muted">{stat.effectName}</span>
                      <span className="block truncate font-mono text-[10px] text-text-muted/80">
                        {stat.unitId != null ? `Unit ${stat.unitId} · ` : ''}Effect {stat.effectDefinitionId}
                      </span>
                      <span className="mt-1 flex flex-wrap gap-1">
                        <Badge variant="secondary" className="normal-case tracking-normal">
                          {stat.commanderCount} commander{stat.commanderCount === 1 ? '' : 's'}
                        </Badge>
                        {stat.minimumObserved != null && stat.maximumObserved != null ? (
                          <Badge variant="outline" className="normal-case tracking-normal">
                            {formatNumber(stat.minimumObserved)}–{formatNumber(stat.maximumObserved)} observed
                          </Badge>
                        ) : null}
                        {stat.scope && stat.scope !== 'generic' ? (
                          <Badge variant="warning" className="normal-case tracking-normal">{stat.scope.toUpperCase()}</Badge>
                        ) : null}
                        {stat.areaTypeIDs.map((areaTypeID) => (
                          <Badge key={areaTypeID} variant="primary" className="normal-case tracking-normal">
                            Area {areaTypeID}
                          </Badge>
                        ))}
                      </span>
                    </span>
                  </button>
                );
              })}
            </div>
          )}
        </div>

        <div className="grid gap-3 sm:grid-cols-2">
          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-semibold uppercase tracking-wider text-text-muted">Minimum bonus troops</span>
            <Input
              type="number"
              min={0}
              step={1}
              value={minimumValue}
              onChange={(event) => setMinimumValue(event.target.value)}
              aria-label="Minimum bonus troops"
            />
          </label>
          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-semibold uppercase tracking-wider text-text-muted">Maximum bonus troops · optional</span>
            <Input
              type="number"
              min={0}
              step={1}
              value={maximumValue}
              onChange={(event) => setMaximumValue(event.target.value)}
              aria-label="Maximum bonus troops"
              placeholder="No maximum"
            />
          </label>
        </div>
        {validationError ? <p className="text-xs font-medium text-error">{validationError}</p> : null}
      </div>
    </Modal>
  );
};

function observedBonusTroopStats(
  state: GameStateV2 | null,
  commanderIDs: readonly number[],
  effects: Record<number, MetadataItem>,
  troops: Record<number, MetadataItem>,
  current: CommanderEquipmentEffectRequirement | null,
): BonusTroopStat[] {
  const mutable = new Map<string, MutableBonusTroopStat>();
  if (state) {
    for (const commanderID of commanderIDs) {
      const commander = state.commanders[String(commanderID)];
      if (!commander) continue;
      for (const equipmentID of Object.values(commander.equipment)) {
        const equipment = state.inventory.equipment[String(equipmentID)];
        if (!equipment) continue;
        for (const effect of Array.isArray(equipment.effects) ? equipment.effects : []) {
          const metadata = effects[effect.definitionId];
          if (!isBonusTroopEffect(metadata)) continue;
          const troopValues = pairedTroopValues(effect.values, troops);
          const values = troopValues.length > 0
            ? troopValues
            : scalarEffectValue(effect.values).map((value) => ({ value }));
          for (const entry of values) {
            const key = statKey(effect.definitionId, entry.unitId);
            let stat = mutable.get(key);
            if (!stat) {
              stat = {
                key,
                effectDefinitionId: effect.definitionId,
                ...(entry.unitId != null ? { unitId: entry.unitId } : {}),
                effectName: metadata?.name?.trim() || `Effect ${effect.definitionId}`,
                unitName: entry.unitId != null
                  ? troops[entry.unitId]?.name?.trim() || `Unit ${entry.unitId}`
                  : 'Support troops',
                areaTypeIDs: metadataIntegerList(metadata?.areaTypeIds ?? metadata?.areaTypeID),
                scope: typeof metadata?.scope === 'string' ? metadata.scope : 'generic',
                valuesByCommander: new Map<number, number>(),
              };
              mutable.set(key, stat);
            }
            stat.valuesByCommander.set(commanderID, (stat.valuesByCommander.get(commanderID) ?? 0) + entry.value);
          }
        }
      }
    }
  }

  if (current) {
    const key = statKey(current.effectDefinitionId, current.unitId);
    if (!mutable.has(key)) {
      const metadata = effects[current.effectDefinitionId];
      const unitID = current.unitId;
      mutable.set(key, {
        key,
        effectDefinitionId: current.effectDefinitionId,
        ...(unitID != null ? { unitId: unitID } : {}),
        effectName: metadata?.name?.trim() || `Effect ${current.effectDefinitionId}`,
        unitName: unitID != null
          ? troops[unitID]?.name?.trim() || `Unit ${unitID}`
          : 'Support troops',
        areaTypeIDs: metadataIntegerList(metadata?.areaTypeIds ?? metadata?.areaTypeID),
        scope: typeof metadata?.scope === 'string' ? metadata.scope : 'generic',
        valuesByCommander: new Map<number, number>(),
      });
    }
  }

  return Array.from(mutable.values(), (stat): BonusTroopStat => {
    const values = Array.from(stat.valuesByCommander.values()).sort((left, right) => left - right);
    return {
      ...stat,
      commanderCount: values.length,
      minimumObserved: values[0] ?? null,
      maximumObserved: values.at(-1) ?? null,
    };
  }).sort((left, right) => (
    right.commanderCount - left.commanderCount
    || left.unitName.localeCompare(right.unitName)
    || left.effectName.localeCompare(right.effectName)
  ));
}

function isBonusTroopEffect(metadata: MetadataItem | undefined): boolean {
  if (!metadata) return false;
  const effectTypeName = typeof metadata.effectTypeName === 'string'
    ? metadata.effectTypeName.replace(/[_\s-]+/g, '').toLowerCase()
    : '';
  return effectTypeName === 'attacksupportunits';
}

function pairedTroopValues(
  values: readonly number[],
  troops: Record<number, MetadataItem>,
): Array<{ unitId: number; value: number }> {
  if (values.length < 2 || values.length % 2 !== 0) return [];
  const result: Array<{ unitId: number; value: number }> = [];
  for (let index = 0; index + 1 < values.length; index += 2) {
    const unitId = Math.trunc(Number(values[index]));
    const value = Number(values[index + 1]);
    if (unitId <= 0 || !troops[unitId] || !Number.isFinite(value)) return [];
    result.push({ unitId, value });
  }
  return result;
}

function scalarEffectValue(values: readonly number[]): number[] {
  const value = Number(values.at(-1));
  return Number.isFinite(value) ? [value] : [];
}

function metadataIntegerList(value: unknown): number[] {
  const values = Array.isArray(value) ? value : typeof value === 'string' ? value.split(',') : [value];
  return Array.from(new Set(values.flatMap((candidate) => {
    const parsed = Number(candidate);
    return Number.isFinite(parsed) && parsed > 0 ? [Math.trunc(parsed)] : [];
  }))).sort((left, right) => left - right);
}

function statKey(effectDefinitionId: number, unitId?: number): string {
  return `${effectDefinitionId}:${unitId ?? 0}`;
}

function formatNumber(value: number): string {
  return Number.isInteger(value) ? String(value) : value.toFixed(2).replace(/\.?0+$/, '');
}

export default CommanderRequirementModal;
