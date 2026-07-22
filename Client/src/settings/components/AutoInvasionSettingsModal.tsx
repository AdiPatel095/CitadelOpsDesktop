import React, { useEffect, useMemo, useState } from 'react';
import { Castle, Clock3, Crosshair, ShieldCheck, ShieldPlus, Swords, Target } from 'lucide-react';
import { useCitadelAPI } from '../../api/ApiContext';
import { castleOptionsFromState } from '../../api/Selectors';
import {
  ATTACK_PRESETS_SECTION,
  parseAttackPresetDocument,
  summarizeAttackPreset,
} from '../../attackPresets/AttackPresetTypes';
import { Badge, Button, Card, Input, Select, SettingsModal, Switch } from '../../components/ui';
import { Notifications } from '../../components/Notifications';
import {
  AUTO_INVASION_SECTION,
  autoInvasionDifficultyName,
  autoInvasionDifficultyOptions,
  clampAutoInvasionInteger,
  defaultAutoInvasionClientState,
  parseAutoInvasionClientState,
  type AutoInvasionClientStateV1,
} from '../AutoInvasionClientState';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import { DailyAttackLimitField } from './DailyAttackLimitField';

interface AutoInvasionSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const AutoInvasionSettingsModal: React.FC<AutoInvasionSettingsModalProps> = ({ isOpen, onClose }) => {
  const { state, configuration, updateConfiguration } = useCitadelAPI();
  const [draft, setDraft] = useState<AutoInvasionClientStateV1>(defaultAutoInvasionClientState);
  const [saving, setSaving] = useState(false);
  const castles = useMemo(() => castleOptionsFromState(state).filter((castle) => castle.kingdomId === 0), [state]);
  const presetDocument = useMemo(
    () => parseAttackPresetDocument(configuration?.sections[ATTACK_PRESETS_SECTION]),
    [configuration?.sections],
  );
  const completedAchievements = state?.player.achievements?.completed ?? {};
  const achievementsObserved = Boolean(state?.player.achievements?.observedAt);
  const foreignLordsDifficulties = useMemo(
    () => autoInvasionDifficultyOptions(1, completedAchievements),
    [completedAchievements],
  );
  const bloodcrowDifficulties = useMemo(
    () => autoInvasionDifficultyOptions(101, completedAchievements),
    [completedAchievements],
  );
  const selectedPreset = presetDocument.presets.find((preset) => preset.id === draft.presetId);
  const presetSummary = selectedPreset ? summarizeAttackPreset(selectedPreset) : null;
  const foreignLordsSelectionAvailable = foreignLordsDifficulties.some((option) => option.value === String(draft.foreignLordsDifficultyId));
  const bloodcrowSelectionAvailable = bloodcrowDifficulties.some((option) => option.value === String(draft.bloodcrowDifficultyId));
	const fortifyOptions = [
		{ value: 'GTO', label: 'Gold tokens' },
		{ value: 'STO', label: 'Silver tokens' },
		{ value: 'MEDALS', label: 'Event medals' },
		{ value: 'C2', label: 'Rubies' },
	];

  useEffect(() => {
    if (!isOpen) return;
    setDraft(parseAutoInvasionClientState(configuration?.sections[AUTO_INVASION_SECTION]));
  }, [configuration?.sections, isOpen]);

  const canSave = draft.sourceCastleId > 0
    && Boolean(draft.presetId)
    && foreignLordsSelectionAvailable
    && bloodcrowSelectionAvailable
    && draft.scoreTarget > 0;

  const save = async () => {
    if (saving || !canSave) return;
    setSaving(true);
    try {
      await updateConfiguration(AUTO_INVASION_SECTION, draft);
      Notifications.success('Auto Invasion settings saved.');
      onClose();
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not save Auto Invasion settings.');
    } finally {
      setSaving(false);
    }
  };

  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={() => { if (!saving) onClose(); }}
      maxWidth="3xl"
      title="Auto Invasion"
      icon={<Crosshair className="h-5 w-5" />}
      description="Foreign Lords and Bloodcrow attack plan"
      onSave={() => void save()}
      isSaving={saving}
      saveDisabled={!canSave}
    >
      <div className="space-y-3">
        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
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
              <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Swords className="h-3.5 w-3.5" /> Attack preset</span>
              <Select
                value={draft.presetId}
                onChange={(presetId) => setDraft((current) => ({ ...current, presetId }))}
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
          {presetSummary ? (
            <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-border-base pt-3">
              <span className="mr-1 text-xs text-text-muted">Preset loadout</span>
              <Badge variant="outline">{presetSummary.waves} waves</Badge>
              <Badge variant="outline">{presetSummary.troops.toLocaleString()} troops</Badge>
              <Badge variant="outline">{presetSummary.tools.toLocaleString()} tools</Badge>
            </div>
          ) : null}
        </Card>

        <Card variant="solid" className="p-4">
          <div className="mb-3 flex items-start justify-between gap-3">
            <div>
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><ShieldCheck className="h-4 w-4 text-primary" /> Event difficulty</div>
              <p className="mt-1 text-xs text-text-muted">Only levels unlocked by this player’s completed achievements are available.</p>
            </div>
            <Badge variant="outline">{achievementsObserved ? 'Achievements synced' : 'Syncing achievements'}</Badge>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className="mb-1.5 flex items-center justify-between gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
                Foreign Lords
                <span className="normal-case tracking-normal text-primary">Through {autoInvasionDifficultyName(Number(foreignLordsDifficulties.at(-1)?.value), 1)}</span>
              </span>
              <Select
                value={foreignLordsSelectionAvailable ? String(draft.foreignLordsDifficultyId) : ''}
                onChange={(value) => setDraft((current) => ({ ...current, foreignLordsDifficultyId: Number(value) || 0 }))}
                options={foreignLordsDifficulties}
                placeholder="Choose unlocked difficulty"
                menuGrowToViewport
              />
            </label>
            <label className="block">
              <span className="mb-1.5 flex items-center justify-between gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
                Bloodcrow
                <span className="normal-case tracking-normal text-primary">Through {autoInvasionDifficultyName(Number(bloodcrowDifficulties.at(-1)?.value), 101)}</span>
              </span>
              <Select
                value={bloodcrowSelectionAvailable ? String(draft.bloodcrowDifficultyId) : ''}
                onChange={(value) => setDraft((current) => ({ ...current, bloodcrowDifficultyId: Number(value) || 0 }))}
                options={bloodcrowDifficulties}
                placeholder="Choose unlocked difficulty"
                menuGrowToViewport
              />
            </label>
          </div>
          {!achievementsObserved ? <p className="mt-3 text-xs text-warning">Achievement data is still syncing; base difficulties are available now.</p> : null}
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
                onChange={(event) => setDraft((current) => ({ ...current, minimumRemainingSec: clampAutoInvasionInteger(event.target.value, 0, 1440, 30) * 60 }))}
                rightIcon={<span className="text-[10px] text-text-muted">min</span>}
                className="font-mono"
              />
            </label>
          </div>
        </Card>

		<Card variant="solid" className="p-4">
			<div className="flex items-start justify-between gap-4">
				<div className="min-w-0">
					<div className="flex items-center gap-2 text-sm font-black text-text-main"><ShieldPlus className="h-4 w-4 text-primary" /> Fortify each target</div>
					<p className="mt-1 text-xs text-text-muted">Optionally strengthen the generated castle before launching its attack. This spends the selected currency once per target.</p>
				</div>
				<Switch
					checked={draft.fortifyCurrency !== ''}
					onChange={(checked) => setDraft((current) => ({ ...current, fortifyCurrency: checked ? (current.fortifyCurrency || 'GTO') : '' }))}
					ariaLabel="Fortify each Auto Invasion target"
				/>
			</div>
			{draft.fortifyCurrency !== '' ? (
				<label className="mt-3 block border-t border-border-base pt-3">
					<span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted">Fortification currency</span>
					<Select
						value={draft.fortifyCurrency}
						onChange={(value) => setDraft((current) => ({ ...current, fortifyCurrency: value as AutoInvasionClientStateV1['fortifyCurrency'] }))}
						options={fortifyOptions}
						menuGrowToViewport
					/>
					<p className="mt-2 text-[11px] text-text-muted">Event medals use Khan medals for Foreign Lords and Samurai tokens for Bloodcrows. The game determines each cumulative <span className="font-mono">rae</span> price. Rubies are never selected by default.</p>
				</label>
			) : null}
		</Card>

        <DailyAttackLimitField
          value={draft.dailyAttackLimit}
          onChange={(dailyAttackLimit) => setDraft((current) => ({ ...current, dailyAttackLimit }))}
          serverState={state?.dailyAttacks}
        />

        <p className="rounded-global border border-border-base bg-bg-app/40 px-4 py-3 text-xs text-text-muted">
			Troop quantities adapt to the freshly resolved left, front, and right limits for each commander and target. Fortification is optional and never spends currency unless enabled above.
        </p>
      </div>
    </SettingsModal>
  );
};
