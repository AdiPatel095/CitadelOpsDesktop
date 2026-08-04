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
  autoNomadDifficultyName,
  autoNomadDifficultyOptions,
  clampAutoNomadInteger,
  defaultAutoNomadClientState,
  parseAutoNomadClientState,
  type AutoNomadClientStateV5,
} from '../AutoNomadClientState';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import { DailyAttackLimitField } from './DailyAttackLimitField';

interface AutoNomadSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const AutoNomadSettingsModal: React.FC<AutoNomadSettingsModalProps> = ({ isOpen, onClose }) => {
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
  const nomadDifficulties = useMemo(
    () => autoNomadDifficultyOptions(301, 1102, completedAchievements),
    [completedAchievements],
  );
  const samuraiDifficulties = useMemo(
    () => autoNomadDifficultyOptions(201, 1096, completedAchievements),
    [completedAchievements],
  );
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
      title="Auto Nomad / Samurai"
      icon={<Crosshair className="h-5 w-5" />}
      description="Four-camp leveling and locked-target attack chains"
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
                placeholder="Choose a Great Empire castle"
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
                  <span className="mr-1 text-xs text-text-muted">Nomad</span>
                  <Badge variant="outline">{nomadPresetSummary.waves} waves</Badge>
                  <Badge variant="outline">{nomadPresetSummary.troops.toLocaleString()} troops</Badge>
                  <Badge variant="outline">{nomadPresetSummary.tools.toLocaleString()} tools</Badge>
                </div>
              ) : null}
              {samuraiPresetSummary ? (
                <div className="flex flex-wrap items-center gap-2">
                  <span className="mr-1 text-xs text-text-muted">Samurai</span>
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
              <p className="mt-1 text-xs text-text-muted">The module starts the active event with this difficulty before scanning camps.</p>
            </div>
            <Badge variant="outline">{achievementsObserved ? 'Achievements synced' : 'Syncing achievements'}</Badge>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className="mb-1.5 flex items-center justify-between gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
                Nomad
                <span className="normal-case tracking-normal text-primary">Through {autoNomadDifficultyName(Number(nomadDifficulties.at(-1)?.value), 301)}</span>
              </span>
              <Select
                value={nomadSelectionAvailable ? String(draft.nomadDifficultyId) : ''}
                onChange={(value) => setDraft((current) => ({ ...current, nomadDifficultyId: Number(value) || 0 }))}
                options={nomadDifficulties}
                placeholder="Choose unlocked difficulty"
                menuGrowToViewport
              />
            </label>
            <label className="block">
              <span className="mb-1.5 flex items-center justify-between gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
                Samurai
                <span className="normal-case tracking-normal text-primary">Through {autoNomadDifficultyName(Number(samuraiDifficulties.at(-1)?.value), 201)}</span>
              </span>
              <Select
                value={samuraiSelectionAvailable ? String(draft.samuraiDifficultyId) : ''}
                onChange={(value) => setDraft((current) => ({ ...current, samuraiDifficultyId: Number(value) || 0 }))}
                options={samuraiDifficulties}
                placeholder="Choose unlocked difficulty"
                menuGrowToViewport
              />
            </label>
          </div>
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
                <Badge variant="outline" className="shrink-0">30 min recommended</Badge>
              </span>
              <Input
                type="number"
                min={0}
                max={1440}
                value={Math.round(draft.minimumRemainingSec / 60)}
                onChange={(event) => setDraft((current) => ({ ...current, minimumRemainingSec: clampAutoNomadInteger(event.target.value, 0, 1440, 30) * 60 }))}
                rightIcon={<span className="text-[10px] text-text-muted">min</span>}
                className="font-mono"
              />
            </label>
          </div>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><RotateCcw className="h-4 w-4 text-primary" /> Clear each landed-hit cooldown</div>
              <p className="mt-1 text-xs text-text-muted">After every confirmed victory, refresh the target and spend an inventory time skip before the next chained march arrives.</p>
            </div>
            <Switch
              checked={draft.skipCooldowns}
              onChange={(skipCooldowns) => setDraft((current) => ({ ...current, skipCooldowns }))}
              ariaLabel="Clear Auto Nomad and Samurai camp cooldowns with time skips"
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
              <p className="mt-3 text-[11px] text-warning">Each server command uses exactly one skip. When no single skip covers the cooldown, smaller available skips are repeated one at a time after each confirmed response.</p>
            </div>
          ) : null}
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><TestTube2 className="h-4 w-4 text-primary" /> Temporary RBC end-to-end trial</div>
              <p className="mt-1 text-xs text-text-muted">Use the Nomad preset against one robber-baron castle, size the chain from live resources, then prove every victory is followed by an immediate time-skip reset.</p>
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
              ariaLabel="Run a resource-sized Auto Camp trial against an RBC"
            />
          </div>
          {draft.rbcTest.enabled ? (
            <div className="mt-3 grid gap-4 border-t border-border-base pt-3 sm:grid-cols-2">
              <label className="block">
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Target X</span>
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
                <span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Target Y</span>
                <Input
                  type="number"
                  min={0}
                  max={2000}
                  value={draft.rbcTest.targetY}
                  onChange={(event) => setDraft((current) => ({ ...current, rbcTest: { ...current.rbcTest, targetY: clampAutoNomadInteger(event.target.value, 0, 2000, 0) } }))}
                  className="font-mono"
                />
              </label>
              <p className="sm:col-span-2 text-[11px] text-warning">The chain uses every currently available selected commander supported by stationed preset copies and uncommitted response-gated RBC cooldown sequences. Any commander or troops returning from any attack can be launched on the next reevaluation without waiting for older chain marches. This mode bypasses event start and four-camp selection only for the explicit run ID created by this switch. Disable it after the trial before enabling the real Nomad/Samurai flow.</p>
            </div>
          ) : null}
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-center gap-2 text-sm font-black text-text-main"><Lock className="h-4 w-4 text-primary" /> Fixed four-camp flow</div>
          <div className="mt-3 grid gap-2 sm:grid-cols-3">
            <div className="rounded-xl border border-border-base bg-bg-app/45 p-3 text-xs text-text-muted"><Badge variant="outline" className="mb-2">1</Badge><div>Advance each of the four nearest regular camps to the terminal victory count defined for the active difficulty.</div></div>
            <div className="rounded-xl border border-border-base bg-bg-app/45 p-3 text-xs text-text-muted"><Badge variant="outline" className="mb-2">2</Badge><div>Rank maxed camps by defense capacity plus wall, gate, and moat values, then lock the weakest.</div></div>
            <div className="rounded-xl border border-border-base bg-bg-app/45 p-3 text-xs text-text-muted"><Badge variant="outline" className="mb-2">3</Badge><div>Send the active event’s preset only to the lock, then clear the returned cooldown between every ordered arrival.</div></div>
          </div>
        </Card>

        <p className="rounded-global border border-border-base bg-bg-app/40 px-4 py-3 text-xs text-text-muted">
          ADI must confirm the same target and zero cooldown before launch. Faster commanders are sent first, and every later CRA is sent immediately after the previous response. Batch size comes from available selected commanders, complete copies of the active event’s preset, and usable cooldown skips. Server-returned arrivals are checked for ordering, then each landed victory is cleared through the target cooldown controller. In-flight attacks can carry the final score beyond the configured threshold.
        </p>
      </div>
    </SettingsModal>
  );
};
