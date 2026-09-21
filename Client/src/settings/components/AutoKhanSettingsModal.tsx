import {useLocalizedMessage} from '../../i18n/useLocalizedMessage';
import {parseMessageDescriptor} from '../../i18n/messageDescriptor';
import {messageLanguageAttributes} from '../../i18n/messageLanguage';
import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import {
  Castle,
  Clock3,
  Crosshair,
  Flame,
  LockKeyhole,
  RotateCcw,
  ShieldAlert,
  ShieldCheck,
  ShoppingCart,
  Swords,
  Zap,
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
  const { t: localizeStatic } = useStaticLocale();
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
  const protectionReason = useLocalizedMessage(parseMessageDescriptor(protection?.reasonDescriptor),protection?.reason || 'Add defense units before the Khan chain can continue.');
  const rageBooster = state?.market?.boosters?.['27'];
  const rageBoosterExpiresAt = rageBooster?.expiresAt ? Date.parse(rageBooster.expiresAt) : 0;
  const rageBoosterActive = rageBooster?.permanent === true
    || (Number.isFinite(rageBoosterExpiresAt) && rageBoosterExpiresAt > Date.now());
  const rageBoosterStatus = rageBoosterActive
    ? `${rageBooster?.bonusPercent ? `+${rageBooster.bonusPercent}% · ` : ''}${rageBooster?.permanent
      ? 'active'
      : `${formatBoosterRemaining(rageBoosterExpiresAt - Date.now())} left`}`
    : state?.market?.boostersObservedAt
      ? 'No active boi ID 27 booster detected'
      : 'Waiting for the first authoritative boi booster snapshot';

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
      title={localizeStatic("ui.settings.components.autoKhanSettingsModal.title.auto.khan.24bcea17")}
      icon={<Crosshair className="h-5 w-5" />}
      description={localizeStatic("ui.settings.components.autoKhanSettingsModal.description.chained.camp.attacks.khan.taunts.and.main.339ad3f1")}
      onSave={() => void save()}
      isSaving={saving}
      saveDisabled={!canSave}
    >
      <div className="space-y-3">
        {protection?.active ? (
          <div className="rounded-global border border-warning/30 bg-warning/10 p-4">
            <div className="flex items-center gap-2 text-sm font-black text-warning"><LockKeyhole className="h-4 w-4" /> Auto Khan is safety-locked</div>
            <p className="mt-1 text-xs text-text-main" {...messageLanguageAttributes(protectionReason)}>{protectionReason.text}</p>
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
                placeholder={localizeStatic("ui.settings.components.autoKhanSettingsModal.placeholder.choose.a.great.empire.castle.8a81fec1")}
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
            <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.auto.station.has.precedence.any.incoming.player.19ee005a" /></p>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-sm font-black text-text-main"><LockKeyhole className="h-4 w-4 text-primary" /> Lock automatic Khan attacks</div>
                <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.stops.only.auto.khan.s.own.attack.284cf413" /></p>
              </div>
              <Switch
                checked={!draft.attackLaunchesEnabled}
                onChange={(locked) => setDraft((current) => ({ ...current, attackLaunchesEnabled: !locked }))}
                ariaLabel={localizeStatic("ui.settings.components.autoKhanSettingsModal.ariaLabel.lock.automatic.khan.attacks.2b670e17")}
              />
            </div>

            <div className="flex items-start justify-between gap-4 border-t border-border-base pt-4 md:border-l md:border-t-0 md:pl-4 md:pt-0">
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-sm font-black text-text-main"><Flame className="h-4 w-4 text-primary" /> Trigger Khan at full rage</div>
                <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.turn.this.off.to.keep.attacking.and.e1a20f3a" /></p>
              </div>
              <Switch
                checked={draft.triggerRage}
                onChange={(triggerRage) => setDraft((current) => ({ ...current, triggerRage }))}
                ariaLabel={localizeStatic("ui.settings.components.autoKhanSettingsModal.ariaLabel.trigger.khan.retaliation.at.full.rage.a2d62469")}
              />
            </div>
          </div>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><Flame className="h-4 w-4 text-primary" /> Rage chain limit</div>
              <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.once.this.many.accepted.khan.retaliations.are.fa686d33" /></p>
              <label className="mt-3 block max-w-xs">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.max.rage.chain.0.disables.limit.a652d0a7" /></span>
                <Input
                  type="text"
                  inputMode="numeric"
                  autoComplete="off"
                  value={draft.maxRageChain.toLocaleString()}
                  onChange={(event) => {
                    const digits = event.target.value.replace(/\D/g, '');
                    const maxRageChain = clampAutoKhanInteger(digits, 0, Number.MAX_SAFE_INTEGER, 0);
                    setDraft((current) => ({ ...current, maxRageChain }));
                  }}
                  className="font-mono"
                />
              </label>
            </div>

            <div className="border-t border-border-base pt-4 md:border-l md:border-t-0 md:pl-4 md:pt-0">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-sm font-black text-text-main"><Zap className="h-4 w-4 text-primary" /> Require Rage points booster</div>
                  <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.gate.only.new.automatic.camp.attacks.unless.2ea26f61" /></p>
                  <p className={`mt-1 text-xs font-bold ${rageBoosterActive ? 'text-success' : 'text-text-muted'}`}>{rageBoosterStatus}</p>
                </div>
                <Switch
                  checked={draft.requireActiveRageBooster}
                  onChange={(requireActiveRageBooster) => setDraft((current) => ({ ...current, requireActiveRageBooster }))}
                  ariaLabel={localizeStatic("ui.settings.components.autoKhanSettingsModal.ariaLabel.require.an.active.khan.rage.points.booster.2bf5fdd6")}
                />
              </div>
              <p className="mt-3 rounded-global border border-border-base bg-bg-input/50 p-3 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.this.is.the.timed.rage.points.booster.4049d4e5" /></p>
            </div>
          </div>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><LockKeyhole className="h-4 w-4 text-primary" /> Nomad points stop</div>
              <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.at.the.limit.auto.khan.stops.launching.8f630fc2" /></p>
              <label className="mt-3 block max-w-xs">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.stop.at.nomad.points.0.disables.81362bed" /></span>
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
                <p className="mt-2 text-xs text-warning"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.reaching.this.limit.uses.the.game.s.10d3a9bb" /></p>
              ) : null}
            </div>

            <div className="border-t border-border-base pt-4 md:border-l md:border-t-0 md:pl-4 md:pt-0">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-sm font-black text-text-main"><ShoppingCart className="h-4 w-4 text-primary" /> Replenish defense tools</div>
                  <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.every.30.seconds.replace.preset.shortages.from.b001d2e4" /></p>
                </div>
                <Switch
                  checked={draft.replenishDefenseTools}
                  onChange={(replenishDefenseTools) => setDraft((current) => ({ ...current, replenishDefenseTools }))}
                  ariaLabel={localizeStatic("ui.settings.components.autoKhanSettingsModal.ariaLabel.replenish.auto.khan.defense.tools.2db26fd2")}
                />
              </div>
              {draft.replenishDefenseTools ? (
                <div className="mt-3 rounded-global border border-success/30 bg-success/10 p-3 text-xs text-text-main">
                  <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.ruby.priced.packages.are.rejected.auto.khan.8c84936b" /></div>
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
          <p className="mt-3 border-t border-border-base pt-3 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.the.selected.defense.preset.is.re.applied.e0cc99f5" /></p>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><RotateCcw className="h-4 w-4 text-primary" /> Skip every Khan camp cooldown</div>
              <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.each.launched.hit.reserves.enough.combined.skip.10c1e007" /></p>
            </div>
            <Switch
              checked={draft.skipCooldowns}
              onChange={(skipCooldowns) => setDraft((current) => ({ ...current, skipCooldowns }))}
              ariaLabel={localizeStatic("ui.settings.components.autoKhanSettingsModal.ariaLabel.skip.every.khan.camp.cooldown.c1c19442")}
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
                rightIcon={<span className="text-[10px] text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.min.1f6fa6f6" /></span>}
                className="font-mono"
              />
            </label>
          </div>
          {!draft.skipCooldowns ? <p className="mt-3 text-xs text-warning"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.cooldown.skipping.is.required.before.these.chained.0dfd850e" /></p> : null}
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
              ariaLabel={localizeStatic("ui.settings.components.autoKhanSettingsModal.ariaLabel.open.gates.if.offensive.troops.would.defend.14754cac")}
            />
          </div>
          {sourceIsMain && draft.openGateProtection ? (
            <div className="mt-3 border-t border-border-base pt-3">
              <label className="block max-w-xs">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.offensive.wall.unit.threshold.c94cc3f9" /></span>
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
                <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.at.or.above.this.threshold.auto.khan.e1d0e5a1" /></div>
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

function formatBoosterRemaining(milliseconds: number): string {
  const totalMinutes = Math.max(1, Math.ceil(milliseconds / 60_000));
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (hours > 0) return `${hours}h${minutes > 0 ? ` ${minutes}m` : ''}`;
  return `${minutes}m`;
}
