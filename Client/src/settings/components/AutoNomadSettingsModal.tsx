import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import { Castle, Clock3, Crosshair, Lock, RotateCcw, ShieldCheck, Swords, Target, TestTube2 } from 'lucide-react';
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
  AUTO_NOMAD_SECTION,
  clampAutoNomadInteger,
  defaultAutoNomadClientState,
  parseAutoNomadClientState,
  type AutoNomadClientStateV5,
} from '../AutoNomadClientState';
import { eventDifficultyName, useEventDifficultyOptions } from '../EventDifficultyOptions';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import { DailyAttackLimitField } from './DailyAttackLimitField';

interface AutoNomadSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const AutoNomadSettingsModal: React.FC<AutoNomadSettingsModalProps> = ({ isOpen, onClose }) => {
  const { t: localizeStatic } = useStaticLocale();
  const { state, configuration, updateConfiguration } = useCitadelAPI();
  const [draft, setDraft] = useState<AutoNomadClientStateV5>(defaultAutoNomadClientState);
  const [saving, setSaving] = useState(false);
  const castles = useMemo(() => castleOptionsFromState(state).filter((castle) => castle.kingdomId === 0), [state]);
  const presetDocument = useMemo(
    () => parseAttackPresetDocument(configuration?.sections[ATTACK_PRESETS_SECTION]),
    [configuration?.sections],
  );
  const completedAchievements = state?.player.achievements?.completed ?? {};
  const achievementsObserved = Boolean(state?.player.achievements?.observedAt);
  const difficultyCatalog = useEventDifficultyOptions(isOpen, [72, 80], completedAchievements);
  const nomadDifficulties = difficultyCatalog.optionsByEvent['72'] ?? [];
  const samuraiDifficulties = difficultyCatalog.optionsByEvent['80'] ?? [];
  const nomadSelectionAvailable = nomadDifficulties.some((option) => option.value === String(draft.nomadDifficultyId));
  const samuraiSelectionAvailable = samuraiDifficulties.some((option) => option.value === String(draft.samuraiDifficultyId));
  const selectedNomadPreset = presetDocument.presets.find((preset) => preset.id === draft.nomadPresetId);
  const selectedSamuraiPreset = presetDocument.presets.find((preset) => preset.id === draft.samuraiPresetId);
  const nomadPresetSummary = selectedNomadPreset ? summarizeAttackPreset(selectedNomadPreset) : null;
  const samuraiPresetSummary = selectedSamuraiPreset ? summarizeAttackPreset(selectedSamuraiPreset) : null;

  useEffect(() => {
    if (!isOpen) return;
    setDraft(parseAutoNomadClientState(configuration?.sections[AUTO_NOMAD_SECTION]));
  }, [configuration?.sections, isOpen]);

  const trialReady = draft.rbcTest.enabled
    && Boolean(draft.rbcTest.runId)
    && Boolean(draft.nomadPresetId)
    && draft.skipCooldowns;
  const eventReady = !draft.rbcTest.enabled
    && Boolean(draft.nomadPresetId)
    && Boolean(draft.samuraiPresetId)
    && nomadSelectionAvailable
    && samuraiSelectionAvailable
    && draft.scoreTarget > 0;
  const canSave = draft.sourceCastleId > 0 && (trialReady || eventReady);

  const setTimeSkipReserve = (key: string, value: unknown) => {
    setDraft((current) => ({
      ...current,
      timeSkipReserve: {
        ...current.timeSkipReserve,
        [key]: clampAutoNomadInteger(value, 0, Number.MAX_SAFE_INTEGER, 0),
      },
    }));
  };

  const save = async () => {
    if (saving || !canSave) return;
    setSaving(true);
    try {
      await updateConfiguration(AUTO_NOMAD_SECTION, draft);
      Notifications.success('Auto Nomad/Samurai settings saved.');
      onClose();
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not save Auto Nomad/Samurai settings.');
    } finally {
      setSaving(false);
    }
  };

  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={() => { if (!saving) onClose(); }}
      maxWidth="3xl"
      title={localizeStatic("ui.settings.components.autoNomadSettingsModal.title.auto.nomad.samurai.13e95d24")}
      icon={<Crosshair className="h-5 w-5" />}
      description={localizeStatic("ui.settings.components.autoNomadSettingsModal.description.four.camp.leveling.and.locked.target.attack.2e4977f9")}
      onSave={() => void save()}
      isSaving={saving}
      saveDisabled={!canSave}
    >
      <div className="space-y-3">
        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block md:col-span-2">
              <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Castle className="h-3.5 w-3.5" /> Source castle</span>
              <Select
                value={draft.sourceCastleId > 0 ? String(draft.sourceCastleId) : ''}
                onChange={(value) => setDraft((current) => ({ ...current, sourceCastleId: Number(value) || 0 }))}
                options={castles.map((castle) => ({ value: String(castle.id), label: `${castle.name} · ${castle.x}:${castle.y}` }))}
                placeholder={localizeStatic("ui.settings.components.autoNomadSettingsModal.placeholder.choose.a.great.empire.castle.8a81fec1")}
                menuGrowToViewport
              />
            </label>

            <label className="block">
              <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Swords className="h-3.5 w-3.5" /> Nomad attack preset</span>
              <Select
                value={draft.nomadPresetId}
                onChange={(nomadPresetId) => setDraft((current) => ({ ...current, nomadPresetId }))}
                options={presetDocument.presets.map((preset) => ({ value: preset.id, label: preset.name }))}
                placeholder={presetDocument.presets.length > 0 ? 'Choose a CitadelOps preset' : 'Create an Attack Preset first'}
                disabled={presetDocument.presets.length === 0}
                menuGrowToViewport
              />
            </label>
            <label className="block">
              <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Swords className="h-3.5 w-3.5" /> Samurai attack preset</span>
              <Select
                value={draft.samuraiPresetId}
                onChange={(samuraiPresetId) => setDraft((current) => ({ ...current, samuraiPresetId }))}
                options={presetDocument.presets.map((preset) => ({ value: preset.id, label: preset.name }))}
                placeholder={presetDocument.presets.length > 0 ? 'Choose a CitadelOps preset' : 'Create an Attack Preset first'}
                disabled={presetDocument.presets.length === 0}
                menuGrowToViewport
              />
            </label>
            <HorseTravelBoostSelect
              className="block md:col-span-2"
              value={draft.horseTravelBoostId}
              onChange={(horseTravelBoostId) => setDraft((current) => ({ ...current, horseTravelBoostId }))}
            />
          </div>
          {nomadPresetSummary || samuraiPresetSummary ? (
            <div className="mt-3 grid gap-2 border-t border-border-base pt-3 md:grid-cols-2">
              {nomadPresetSummary ? (
                <div className="flex flex-wrap items-center gap-2">
                  <span className="mr-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.nomad.b156d00c" /></span>
                  <Badge variant="outline">{nomadPresetSummary.waves} waves</Badge>
                  <Badge variant="outline">{nomadPresetSummary.troops.toLocaleString()} troops</Badge>
                  <Badge variant="outline">{nomadPresetSummary.tools.toLocaleString()} tools</Badge>
                </div>
              ) : null}
              {samuraiPresetSummary ? (
                <div className="flex flex-wrap items-center gap-2">
                  <span className="mr-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.samurai.031cfd72" /></span>
                  <Badge variant="outline">{samuraiPresetSummary.waves} waves</Badge>
                  <Badge variant="outline">{samuraiPresetSummary.troops.toLocaleString()} troops</Badge>
                  <Badge variant="outline">{samuraiPresetSummary.tools.toLocaleString()} tools</Badge>
                </div>
              ) : null}
            </div>
          ) : null}
        </Card>

        <DailyAttackLimitField
          value={draft.dailyAttackLimit}
          onChange={(dailyAttackLimit) => setDraft((current) => ({ ...current, dailyAttackLimit }))}
          serverState={state?.dailyAttacks}
        />

        <Card variant="solid" className="p-4">
          <div className="mb-3 flex items-start justify-between gap-3">
            <div>
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><ShieldCheck className="h-4 w-4 text-primary" /> Event start difficulty</div>
              <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.the.module.starts.the.active.event.with.b13c4375" /></p>
            </div>
            <Badge variant="outline">{achievementsObserved ? 'Achievements synced' : 'Syncing achievements'}</Badge>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className="mb-1.5 flex items-center justify-between gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
                Nomad
                <span className="normal-case tracking-normal text-primary">Through {eventDifficultyName(nomadDifficulties, Number(nomadDifficulties.at(-1)?.value))}</span>
              </span>
              <Select
                value={nomadSelectionAvailable ? String(draft.nomadDifficultyId) : ''}
                onChange={(value) => setDraft((current) => ({ ...current, nomadDifficultyId: Number(value) || 0 }))}
                options={nomadDifficulties}
                placeholder={localizeStatic("ui.settings.components.autoNomadSettingsModal.placeholder.choose.unlocked.difficulty.a1bf5994")}
                menuGrowToViewport
              />
            </label>
            <label className="block">
              <span className="mb-1.5 flex items-center justify-between gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
                Samurai
                <span className="normal-case tracking-normal text-primary">Through {eventDifficultyName(samuraiDifficulties, Number(samuraiDifficulties.at(-1)?.value))}</span>
              </span>
              <Select
                value={samuraiSelectionAvailable ? String(draft.samuraiDifficultyId) : ''}
                onChange={(value) => setDraft((current) => ({ ...current, samuraiDifficultyId: Number(value) || 0 }))}
                options={samuraiDifficulties}
                placeholder={localizeStatic("ui.settings.components.autoNomadSettingsModal.placeholder.choose.unlocked.difficulty.a1bf5994")}
                menuGrowToViewport
              />
            </label>
          </div>
          {difficultyCatalog.loading ? <p className="mt-3 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.loading.official.event.difficulties.8ddbd72d" /></p> : null}
          {difficultyCatalog.error ? <p className="mt-3 text-xs text-danger">{difficultyCatalog.error}</p> : null}
        </Card>

        <Card variant="solid" className="p-4">
          <div className="grid items-start gap-4 md:grid-cols-2">
            <label className="flex min-w-0 flex-col">
              <span className="mb-1.5 flex min-h-6 items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Target className="h-3.5 w-3.5" /> Stop at event score</span>
              <Input
                type="text"
                inputMode="numeric"
                autoComplete="off"
                value={draft.scoreTarget > 0 ? draft.scoreTarget.toLocaleString() : ''}
                onChange={(event) => {
                  const digits = event.target.value.replace(/\D/g, '');
                  const scoreTarget = digits ? Math.min(Number.MAX_SAFE_INTEGER, Number.parseInt(digits, 10)) : 0;
                  setDraft((current) => ({ ...current, scoreTarget }));
                }}
                placeholder={`e.g. ${(250000).toLocaleString()}`}
                className="font-mono"
              />
            </label>
            <label className="flex min-w-0 flex-col">
              <span className="mb-1.5 flex min-h-6 items-center justify-between gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
                <span className="flex min-w-0 items-center gap-2"><Clock3 className="h-3.5 w-3.5 shrink-0" /> Stop before event ends</span>
                <Badge variant="outline" className="shrink-0"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.30.min.recommended.61238251" /></Badge>
              </span>
              <Input
                type="number"
                min={0}
                max={1440}
                value={Math.round(draft.minimumRemainingSec / 60)}
                onChange={(event) => setDraft((current) => ({ ...current, minimumRemainingSec: clampAutoNomadInteger(event.target.value, 0, 1440, 30) * 60 }))}
                rightIcon={<span className="text-[10px] text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.min.1f6fa6f6" /></span>}
                className="font-mono"
              />
            </label>
          </div>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><RotateCcw className="h-4 w-4 text-primary" /> Clear each landed-hit cooldown</div>
              <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.after.every.confirmed.victory.refresh.the.target.755f02c1" /></p>
            </div>
            <Switch
              checked={draft.skipCooldowns}
              onChange={(skipCooldowns) => setDraft((current) => ({ ...current, skipCooldowns }))}
              ariaLabel={localizeStatic("ui.settings.components.autoNomadSettingsModal.ariaLabel.clear.auto.nomad.and.samurai.camp.cooldowns.0795c132")}
            />
          </div>
          {draft.skipCooldowns ? (
            <div className="mt-3 border-t border-border-base pt-3">
              <div className="grid grid-cols-3 gap-2">
                {([
                  ['MS1', '1m'],
                  ['MS2', '5m'],
                  ['MS3', '10m'],
                  ['MS4', '30m'],
                  ['MS5', '60m'],
                  ['MS6', '5h'],
                  ['MS7', '24h'],
                ] as const).map(([key, label]) => (
                  <label key={key} className="block">
                    <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Keep {label}</span>
                    <Input
                      type="number"
                      min={0}
                      value={draft.timeSkipReserve[key] ?? 0}
                      onChange={(event) => setTimeSkipReserve(key, event.target.value)}
                      className="font-mono"
                    />
                  </label>
                ))}
              </div>
              <p className="mt-3 text-[11px] text-warning"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.each.server.command.uses.exactly.one.skip.ab37a717" /></p>
            </div>
          ) : null}
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><TestTube2 className="h-4 w-4 text-primary" /> Temporary RBC end-to-end trial</div>
              <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.use.the.nomad.preset.against.one.robber.89f8e101" /></p>
            </div>
            <Switch
              checked={draft.rbcTest.enabled}
              onChange={(enabled) => setDraft((current) => ({
                ...current,
                skipCooldowns: enabled ? true : current.skipCooldowns,
                rbcTest: {
                  ...current.rbcTest,
                  enabled,
                  runId: enabled ? (globalThis.crypto?.randomUUID?.() ?? `rbc-${Date.now()}`) : current.rbcTest.runId,
                },
              }))}
              ariaLabel={localizeStatic("ui.settings.components.autoNomadSettingsModal.ariaLabel.run.a.resource.sized.auto.camp.trial.07ab088e")}
            />
          </div>
          {draft.rbcTest.enabled ? (
            <div className="mt-3 grid gap-4 border-t border-border-base pt-3 sm:grid-cols-2">
              <label className="block">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.target.x.884ddd14" /></span>
                <Input
                  type="number"
                  min={0}
                  max={2000}
                  value={draft.rbcTest.targetX}
                  onChange={(event) => setDraft((current) => ({ ...current, rbcTest: { ...current.rbcTest, targetX: clampAutoNomadInteger(event.target.value, 0, 2000, 0) } }))}
                  className="font-mono"
                />
              </label>
              <label className="block">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.target.y.aae5c7ef" /></span>
                <Input
                  type="number"
                  min={0}
                  max={2000}
                  value={draft.rbcTest.targetY}
                  onChange={(event) => setDraft((current) => ({ ...current, rbcTest: { ...current.rbcTest, targetY: clampAutoNomadInteger(event.target.value, 0, 2000, 0) } }))}
                  className="font-mono"
                />
              </label>
              <p className="sm:col-span-2 text-[11px] text-warning"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.the.chain.uses.every.currently.available.selected.b2a29d49" /></p>
            </div>
          ) : null}
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-center gap-2 text-sm font-black text-text-main"><Lock className="h-4 w-4 text-primary" /> Fixed four-camp flow</div>
          <div className="mt-3 grid gap-2 sm:grid-cols-3">
            <div className="rounded-xl border border-border-base bg-bg-app/45 p-3 text-xs text-text-muted"><Badge variant="outline" className="mb-2">1</Badge><div><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.advance.each.of.the.four.nearest.regular.45952900" /></div></div>
            <div className="rounded-xl border border-border-base bg-bg-app/45 p-3 text-xs text-text-muted"><Badge variant="outline" className="mb-2">2</Badge><div><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.rank.maxed.camps.by.defense.capacity.plus.f2d38ec8" /></div></div>
            <div className="rounded-xl border border-border-base bg-bg-app/45 p-3 text-xs text-text-muted"><Badge variant="outline" className="mb-2">3</Badge><div><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.send.the.active.event.s.preset.only.3ad5b2bc" /></div></div>
          </div>
        </Card>

        <p className="rounded-global border border-border-base bg-bg-app/40 px-4 py-3 text-xs text-text-muted">
          <LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.adi.must.confirm.the.same.target.and.1c6467ca" /></p>
      </div>
    </SettingsModal>
  );
};
