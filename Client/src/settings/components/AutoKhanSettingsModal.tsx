import React, { useEffect, useMemo, useState } from 'react';
import {
  Castle,
  Clock3,
  Crosshair,
  LockKeyhole,
  RotateCcw,
  ShieldAlert,
  ShieldCheck,
  ShoppingCart,
  Swords,
} from 'lucide-react';
import { useCitadelAPI } from '../../api/ApiContext';
import { castleOptionsFromState } from '../../api/Selectors';
import {
  ATTACK_PRESETS_SECTION,
  parseAttackPresetDocument,
  summarizeAttackPreset,
} from '../../attackPresets/AttackPresetTypes';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Card, Input, Select, SettingsModal, Switch } from '../../components/ui';
import {
  DEFENSE_PRESETS_SECTION,
  parseDefensePresetDocument,
  summarizeDefensePreset,
} from '../../defensePresets/DefensePresetTypes';
import {
  AUTO_KHAN_SECTION,
  clampAutoKhanInteger,
  defaultAutoKhanClientState,
  parseAutoKhanClientState,
  type AutoKhanClientStateV1,
} from '../AutoKhanClientState';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import { DailyAttackLimitField } from './DailyAttackLimitField';

interface AutoKhanSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const AutoKhanSettingsModal: React.FC<AutoKhanSettingsModalProps> = ({ isOpen, onClose }) => {
  const { state, configuration, updateConfiguration } = useCitadelAPI();
  const [draft, setDraft] = useState<AutoKhanClientStateV1>(defaultAutoKhanClientState);
  const [saving, setSaving] = useState(false);
  const castles = useMemo(() => castleOptionsFromState(state).filter((castle) => castle.kingdomId === 0), [state]);
  const mainCastle = useMemo(
    () => Object.values(state?.castles ?? {}).find((castle) => castle.kingdomId === 0 && castle.slotType === 1),
    [state],
  );
  const attackDocument = useMemo(
    () => parseAttackPresetDocument(configuration?.sections[ATTACK_PRESETS_SECTION]),
    [configuration?.sections],
  );
  const defenseDocument = useMemo(
    () => parseDefensePresetDocument(configuration?.sections[DEFENSE_PRESETS_SECTION]),
    [configuration?.sections],
  );
  const selectedSource = castles.find((castle) => castle.id === draft.sourceCastleId);
  const selectedAttackPreset = attackDocument.presets.find((preset) => preset.id === draft.attackPresetId);
  const selectedDefensePreset = defenseDocument.presets.find((preset) => preset.id === draft.defensePresetId);
  const attackSummary = selectedAttackPreset ? summarizeAttackPreset(selectedAttackPreset) : null;
  const defenseSummary = selectedDefensePreset ? summarizeDefensePreset(selectedDefensePreset) : null;
  const sourceIsMain = mainCastle != null && draft.sourceCastleId === mainCastle.id;
  const protection = state?.khan?.protection;

  useEffect(() => {
    if (!isOpen) return;
    setDraft(parseAutoKhanClientState(configuration?.sections[AUTO_KHAN_SECTION]));
  }, [configuration?.sections, isOpen]);

  const canSave = draft.sourceCastleId > 0
    && Boolean(mainCastle)
    && Boolean(selectedAttackPreset)
    && Boolean(selectedDefensePreset)
    && draft.skipCooldowns
    && (!sourceIsMain || !draft.openGateProtection || draft.offensiveUnitThreshold > 0);

  const setTimeSkipReserve = (key: string, value: unknown) => {
    setDraft((current) => ({
      ...current,
      timeSkipReserve: {
        ...current.timeSkipReserve,
        [key]: clampAutoKhanInteger(value, 0, Number.MAX_SAFE_INTEGER, 0),
      },
    }));
  };

  const save = async () => {
    if (saving || !canSave) return;
    setSaving(true);
    try {
      await updateConfiguration(AUTO_KHAN_SECTION, {
        ...draft,
        openGateProtection: sourceIsMain && draft.openGateProtection,
      });
      Notifications.success('Auto Khan settings saved.');
      onClose();
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not save Auto Khan settings.');
    } finally {
      setSaving(false);
    }
  };

  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={() => { if (!saving) onClose(); }}
      maxWidth="3xl"
      title="Auto Khan"
      icon={<Crosshair className="h-5 w-5" />}
      description="Chained camp attacks, Khan taunts, and main-castle defense"
      onSave={() => void save()}
      isSaving={saving}
      saveDisabled={!canSave}
    >
      <div className="space-y-3">
        {protection?.active ? (
          <div className="rounded-global border border-warning/30 bg-warning/10 p-4">
            <div className="flex items-center gap-2 text-sm font-black text-warning"><LockKeyhole className="h-4 w-4" /> Auto Khan is safety-locked</div>
            <p className="mt-1 text-xs text-text-main">{protection.reason || 'Add defense units before the Khan chain can continue.'}</p>
            <div className="mt-2 flex flex-wrap gap-2">
              <Badge variant="warning">{(protection.offensiveWallUnits ?? 0).toLocaleString()} offensive wall units</Badge>
              <Badge variant="outline">Threshold {(protection.offensiveUnitThreshold ?? 0).toLocaleString()}</Badge>
            </div>
          </div>
        ) : null}

        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Castle className="h-3.5 w-3.5" /> Attack from</span>
              <Select
                value={draft.sourceCastleId > 0 ? String(draft.sourceCastleId) : ''}
                onChange={(value) => {
                  const sourceCastleId = Number(value) || 0;
                  setDraft((current) => ({
                    ...current,
                    sourceCastleId,
                    openGateProtection: mainCastle != null && sourceCastleId === mainCastle.id,
                  }));
                }}
                options={castles.map((castle) => ({
                  value: String(castle.id),
                  label: `${castle.name}${castle.id === mainCastle?.id ? ' · Main' : ' · Outpost'} · ${castle.x}:${castle.y}`,
                }))}
                placeholder="Choose a Great Empire castle"
                menuGrowToViewport
              />
            </label>

            <div>
              <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><ShieldCheck className="h-3.5 w-3.5" /> Defend at</span>
              <div className="flex min-h-[42px] items-center rounded-global border border-border-base bg-bg-input/70 px-4 text-sm text-text-main">
                {mainCastle ? `${mainCastle.name?.trim() || `Castle ${mainCastle.id}`} · Main · ${mainCastle.x}:${mainCastle.y}` : 'Great Empire main castle not found'}
              </div>
            </div>
          </div>
          <p className="mt-3 border-t border-border-base pt-3 text-xs text-text-muted">
            Auto Station has precedence. Any incoming player attack pauses new Khan attacks, cooldown skips, and defense changes while stationing runs. Khan taunts do not count as player attacks.
          </p>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><LockKeyhole className="h-4 w-4 text-primary" /> Nomad points stop</div>
              <p className="mt-1 text-xs text-text-muted">At the limit, Auto Khan stops launching, recalls its active outgoing attacks, and opens the main-castle gates so later taunts cannot consume the defense preset.</p>
              <label className="mt-3 block max-w-xs">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Stop at Nomad points · 0 disables</span>
                <Input
                  type="text"
                  inputMode="numeric"
                  autoComplete="off"
                  value={draft.nomadPointThreshold.toLocaleString()}
                  onChange={(event) => {
                    const digits = event.target.value.replace(/\D/g, '');
                    const nomadPointThreshold = clampAutoKhanInteger(digits, 0, Number.MAX_SAFE_INTEGER, 0);
                    setDraft((current) => ({ ...current, nomadPointThreshold }));
                  }}
                  className="font-mono"
                />
              </label>
              {draft.nomadPointThreshold > 0 ? (
                <p className="mt-2 text-xs text-warning">Reaching this limit uses the game&apos;s current ruby cost for the six-hour open-gate option.</p>
              ) : null}
            </div>

            <div className="border-t border-border-base pt-4 md:border-l md:border-t-0 md:pl-4 md:pt-0">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-sm font-black text-text-main"><ShoppingCart className="h-4 w-4 text-primary" /> Replenish defense tools</div>
                  <p className="mt-1 text-xs text-text-muted">Every 30 seconds, replace preset shortages from currently active coin, Nomad/Khan, event-token, or Aquamarine shop packages.</p>
                </div>
                <Switch
                  checked={draft.replenishDefenseTools}
                  onChange={(replenishDefenseTools) => setDraft((current) => ({ ...current, replenishDefenseTools }))}
                  ariaLabel="Replenish Auto Khan defense tools"
                />
              </div>
              {draft.replenishDefenseTools ? (
                <div className="mt-3 rounded-global border border-success/30 bg-success/10 p-3 text-xs text-text-main">
                  Ruby-priced packages are rejected. Auto Khan buys only the exact missing preset tool from a package currently advertised by the server, or the captured Luna table, and only when its non-premium balance can cover the purchase.
                </div>
              ) : null}
            </div>
          </div>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Swords className="h-3.5 w-3.5" /> Camp attack preset</span>
              <Select
                value={draft.attackPresetId}
                onChange={(attackPresetId) => setDraft((current) => ({ ...current, attackPresetId }))}
                options={attackDocument.presets.map((preset) => ({ value: preset.id, label: preset.name }))}
                placeholder={attackDocument.presets.length > 0 ? 'Choose an Attack Preset' : 'Create an Attack Preset first'}
                disabled={attackDocument.presets.length === 0}
                menuGrowToViewport
              />
              {attackSummary ? (
                <div className="mt-2 flex flex-wrap gap-2">
                  <Badge variant="outline">{attackSummary.waves} waves</Badge>
                  <Badge variant="outline">{attackSummary.troops.toLocaleString()} troops</Badge>
                </div>
              ) : null}
            </label>

            <label className="block">
              <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><ShieldCheck className="h-3.5 w-3.5" /> Main defense preset</span>
              <Select
                value={draft.defensePresetId}
                onChange={(defensePresetId) => setDraft((current) => ({ ...current, defensePresetId }))}
                options={defenseDocument.presets.map((preset) => ({ value: preset.id, label: preset.name }))}
                placeholder={defenseDocument.presets.length > 0 ? 'Choose a Defense Preset' : 'Create a Defense Preset first'}
                disabled={defenseDocument.presets.length === 0}
                menuGrowToViewport
              />
              {defenseSummary ? (
                <div className="mt-2 flex flex-wrap gap-2">
                  <Badge variant="outline">{defenseSummary.toolTypes.length} tool types</Badge>
                  <Badge variant="outline">{defenseSummary.toolAmount.toLocaleString()} tools</Badge>
                </div>
              ) : null}
            </label>
            <HorseTravelBoostSelect
              className="block md:col-span-2"
              value={draft.horseTravelBoostId}
              onChange={(horseTravelBoostId) => setDraft((current) => ({ ...current, horseTravelBoostId }))}
            />
          </div>
          <p className="mt-3 border-t border-border-base pt-3 text-xs text-text-muted">The selected defense preset is re-applied to the Great Empire main castle before the attack chain continues.</p>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><RotateCcw className="h-4 w-4 text-primary" /> Skip every Khan camp cooldown</div>
              <p className="mt-1 text-xs text-text-muted">Each launched hit reserves enough combined skip time. Every skip command uses one item, then waits for confirmation before applying another.</p>
            </div>
            <Switch
              checked={draft.skipCooldowns}
              onChange={(skipCooldowns) => setDraft((current) => ({ ...current, skipCooldowns }))}
              ariaLabel="Skip every Khan camp cooldown"
            />
          </div>
          <div className="mt-3 grid gap-3 border-t border-border-base pt-3 sm:grid-cols-4">
            {([
              ['MS1', 'Keep 1m'],
              ['MS2', 'Keep 5m'],
              ['MS3', 'Keep 10m'],
              ['MS4', 'Keep 30m'],
              ['MS5', 'Keep 1h'],
              ['MS6', 'Keep 5h'],
              ['MS7', 'Keep 24h'],
            ] as const).map(([key, label]) => (
              <label key={key} className="block">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">{label}</span>
                <Input
                  type="number"
                  min={0}
                  value={draft.timeSkipReserve[key] ?? 0}
                  onChange={(event) => setTimeSkipReserve(key, event.target.value)}
                  className="font-mono"
                />
              </label>
            ))}
            <label className="block">
              <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Clock3 className="h-3.5 w-3.5" /> Stop before event ends</span>
              <Input
                type="number"
                min={0}
                max={1440}
                value={Math.round(draft.minimumRemainingSec / 60)}
                onChange={(event) => setDraft((current) => ({
                  ...current,
                  minimumRemainingSec: clampAutoKhanInteger(event.target.value, 0, 1440, 5) * 60,
                }))}
                rightIcon={<span className="text-[10px] text-text-muted">min</span>}
                className="font-mono"
              />
            </label>
          </div>
          {!draft.skipCooldowns ? <p className="mt-3 text-xs text-warning">Cooldown skipping is required before these chained attacks can be saved and run.</p> : null}
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><ShieldAlert className="h-4 w-4 text-primary" /> Protect offense on the main castle wall</div>
              <p className="mt-1 text-xs text-text-muted">
                {sourceIsMain
                  ? 'Use this when the main castle holds both the attacking army and the defense.'
                  : selectedSource
                    ? 'Not needed: the attacking army is isolated in an outpost while the main castle defends.'
                    : 'Choose the main castle as the attack source to configure this safeguard.'}
              </p>
            </div>
            <Switch
              checked={sourceIsMain && draft.openGateProtection}
              onChange={(openGateProtection) => setDraft((current) => ({ ...current, openGateProtection }))}
              disabled={!sourceIsMain}
              ariaLabel="Open gates if offensive troops would defend the main castle"
            />
          </div>
          {sourceIsMain && draft.openGateProtection ? (
            <div className="mt-3 border-t border-border-base pt-3">
              <label className="block max-w-xs">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Offensive wall-unit threshold</span>
                <Input
                  type="text"
                  inputMode="numeric"
                  autoComplete="off"
                  value={draft.offensiveUnitThreshold.toLocaleString()}
                  onChange={(event) => {
                    const digits = event.target.value.replace(/\D/g, '');
                    const offensiveUnitThreshold = digits ? Number.parseInt(digits, 10) : 0;
                    setDraft((current) => ({ ...current, offensiveUnitThreshold }));
                  }}
                  className="font-mono"
                />
              </label>
              <div className="mt-3 rounded-global border border-warning/30 bg-warning/10 p-3 text-xs text-text-main">
                At or above this threshold, Auto Khan opens the main castle gates once for six hours and immediately stops attacks, cooldown skips, and new taunts. The feature stays soft-locked without changing your settings; after the gate expires, it refreshes defense and resumes only when the projected offensive wall count is below the threshold.
                This uses the game&apos;s current ruby cost for the six-hour open-gate option.
              </div>
            </div>
          ) : null}
        </Card>

        <DailyAttackLimitField
          value={draft.dailyAttackLimit}
          onChange={(dailyAttackLimit) => setDraft((current) => ({ ...current, dailyAttackLimit }))}
          serverState={state?.dailyAttacks}
        />
      </div>
    </SettingsModal>
  );
};
