import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import { BookOpen, Castle, Clock3, Crosshair, ShieldCheck, ShieldPlus, Swords, Target } from 'lucide-react';
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
  clampAutoInvasionInteger,
  defaultAutoInvasionClientState,
  parseAutoInvasionClientState,
  type AutoInvasionClientStateV1,
} from '../AutoInvasionClientState';
import { eventDifficultyName, useEventDifficultyOptions } from '../EventDifficultyOptions';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import { FeatureGuideModal } from './FeatureGuideModal';
import { englishGuidePack, useGuideLocale } from '../../config/useGuideLocale';
import { DailyAttackLimitField } from './DailyAttackLimitField';

interface AutoInvasionSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export const AutoInvasionSettingsModal: React.FC<AutoInvasionSettingsModalProps> = ({ isOpen, onClose }) => {
  const { t: localizeStatic } = useStaticLocale();
  const { state, configuration, updateConfiguration } = useCitadelAPI();
  const [draft, setDraft] = useState<AutoInvasionClientStateV1>(defaultAutoInvasionClientState);
  const [saving, setSaving] = useState(false);
  const [isGuideOpen, setIsGuideOpen] = useState(false);
  const { locale: guideLocale, pack: guidePack } = useGuideLocale();
  const invasionGuidePack = guidePack.autoInvasion ? guidePack : englishGuidePack;
  const invasionGuideLocale = invasionGuidePack === englishGuidePack ? 'en' : guideLocale;
  useEffect(() => { if (!isOpen) setIsGuideOpen(false); }, [isOpen]);
  const castles = useMemo(() => castleOptionsFromState(state).filter((castle) => castle.kingdomId === 0), [state]);
  const presetDocument = useMemo(
    () => parseAttackPresetDocument(configuration?.sections[ATTACK_PRESETS_SECTION]),
    [configuration?.sections],
  );
  const completedAchievements = state?.player.achievements?.completed ?? {};
  const achievementsObserved = Boolean(state?.player.achievements?.observedAt);
  const difficultyCatalog = useEventDifficultyOptions(isOpen, [71, 103], completedAchievements);
  const foreignLordsDifficulties = difficultyCatalog.optionsByEvent['71'] ?? [];
  const bloodcrowDifficulties = difficultyCatalog.optionsByEvent['103'] ?? [];
  const selectedPreset = presetDocument.presets.find((preset) => preset.id === draft.presetId);
  const presetSummary = selectedPreset ? summarizeAttackPreset(selectedPreset) : null;
  const foreignLordsSelectionAvailable = foreignLordsDifficulties.some((option) => option.value === String(draft.foreignLordsDifficultyId));
	const bloodcrowSelectionAvailable = bloodcrowDifficulties.some((option) => option.value === String(draft.bloodcrowDifficultyId));
	const liveFortifyCurrencies = useMemo(() => Array.from(new Set(
		(state?.invasion.fortifyCurrencies ?? []).map((currency) => currency.trim().toUpperCase()).filter(Boolean),
	)), [state?.invasion.fortifyCurrencies]);
	const eventFortifyCurrency = liveFortifyCurrencies.find((currency) => currency !== 'GTO' && currency !== 'STO' && currency !== 'C2') ?? '';
	const fortifyOptions = useMemo(() => {
		if (liveFortifyCurrencies.length === 0) {
			return [
				{ value: 'GTO', label: 'Gold tokens' },
				{ value: 'STO', label: 'Silver tokens' },
				{ value: 'MEDALS', label: 'Event currency · detected automatically' },
				{ value: 'C2', label: 'Rubies' },
			];
		}
		const labels: Record<string, string> = {
			GTO: 'Gold tokens', STO: 'Silver tokens', KM: 'Khan medals', ST: 'Samurai tokens', KT: 'Khan tablets', C2: 'Rubies',
		};
		const options: Array<{ value: AutoInvasionClientStateV1['fortifyCurrency']; label: string }> = [];
		for (const currency of liveFortifyCurrencies) {
			const value = currency === 'GTO' || currency === 'STO' || currency === 'C2' ? currency : 'MEDALS';
			if (options.some((option) => option.value === value)) continue;
			options.push({
				value,
				label: value === 'MEDALS'
					? `Event currency · ${labels[currency] ?? currency} (${currency})`
					: `${labels[currency] ?? currency} (${currency})`,
			});
		}
		if (!options.some((option) => option.value === 'C2')) options.push({ value: 'C2', label: 'Rubies (C2)' });
		return options;
	}, [liveFortifyCurrencies]);

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

  return (<>
    <SettingsModal
      isOpen={isOpen}
      onClose={() => { if (!saving) onClose(); }}
      maxWidth="3xl"
      title={localizeStatic("ui.settings.components.autoInvasionSettingsModal.title.auto.invasion.d43e5a94")}
      icon={<Crosshair className="h-5 w-5" />}
      description={localizeStatic("ui.settings.components.autoInvasionSettingsModal.description.foreign.lords.and.bloodcrow.attack.plan.0ee8d04e")}
      titleTrailing={<Button variant="outline" size="sm" className="shrink-0" onClick={() => setIsGuideOpen(true)} leftIcon={<BookOpen className="h-4 w-4" />}><span lang={invasionGuideLocale}>{invasionGuidePack.ui.guideButton}</span></Button>}
      onSave={() => void save()}
      isSaving={saving}
      saveDisabled={!canSave}
    >
      <div className="space-y-3">
        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Castle className="h-3.5 w-3.5" /> <LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.source.castle.86d5a48e" /></span>
              <Select
                value={draft.sourceCastleId > 0 ? String(draft.sourceCastleId) : ''}
                onChange={(value) => setDraft((current) => ({ ...current, sourceCastleId: Number(value) || 0 }))}
                options={castles.map((castle) => ({ value: String(castle.id), label: `${castle.name} · ${castle.x}:${castle.y}` }))}
                placeholder={localizeStatic("ui.settings.components.autoInvasionSettingsModal.placeholder.choose.a.great.empire.castle.8a81fec1")}
                menuGrowToViewport
              />
            </label>

            <label className="block">
              <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Swords className="h-3.5 w-3.5" /> <LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.attack.preset.407b93e9" /></span>
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
              <span className="mr-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.preset.loadout.4c1e2d30" /></span>
              <Badge variant="outline">{presetSummary.waves} waves</Badge>
              <Badge variant="outline">{presetSummary.troops.toLocaleString()} troops</Badge>
              <Badge variant="outline">{presetSummary.tools.toLocaleString()} tools</Badge>
            </div>
          ) : null}
        </Card>

        <Card variant="solid" className="p-4">
          <div className="mb-3 flex items-start justify-between gap-3">
            <div>
              <div className="flex items-center gap-2 text-sm font-black text-text-main"><ShieldCheck className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.event.difficulty.88766fcf" /></div>
              <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.only.levels.unlocked.by.this.player.s.15a0e9b5" /></p>
            </div>
            <Badge variant="outline">{achievementsObserved ? 'Achievements synced' : 'Syncing achievements'}</Badge>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className="mb-1.5 flex items-center justify-between gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
                Foreign Lords
                <span className="normal-case tracking-normal text-primary">Through {eventDifficultyName(foreignLordsDifficulties, Number(foreignLordsDifficulties.at(-1)?.value))}</span>
              </span>
              <Select
                value={foreignLordsSelectionAvailable ? String(draft.foreignLordsDifficultyId) : ''}
                onChange={(value) => setDraft((current) => ({ ...current, foreignLordsDifficultyId: Number(value) || 0 }))}
                options={foreignLordsDifficulties}
                placeholder={localizeStatic("ui.settings.components.autoInvasionSettingsModal.placeholder.choose.unlocked.difficulty.a1bf5994")}
                menuGrowToViewport
              />
            </label>
            <label className="block">
              <span className="mb-1.5 flex items-center justify-between gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
                Bloodcrow
                <span className="normal-case tracking-normal text-primary">Through {eventDifficultyName(bloodcrowDifficulties, Number(bloodcrowDifficulties.at(-1)?.value))}</span>
              </span>
              <Select
                value={bloodcrowSelectionAvailable ? String(draft.bloodcrowDifficultyId) : ''}
                onChange={(value) => setDraft((current) => ({ ...current, bloodcrowDifficultyId: Number(value) || 0 }))}
                options={bloodcrowDifficulties}
                placeholder={localizeStatic("ui.settings.components.autoInvasionSettingsModal.placeholder.choose.unlocked.difficulty.a1bf5994")}
                menuGrowToViewport
              />
            </label>
          </div>
          {difficultyCatalog.loading ? <p className="mt-3 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.loading.official.event.difficulties.8ddbd72d" /></p> : null}
          {difficultyCatalog.error ? <p className="mt-3 text-xs text-danger">{difficultyCatalog.error}</p> : null}
          {!achievementsObserved ? <p className="mt-3 text-xs text-warning"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.achievement.data.is.still.syncing.base.difficulties.bd6eea97" /></p> : null}
        </Card>

        <Card variant="solid" className="p-4">
          <div className="grid items-start gap-4 md:grid-cols-2">
            <label className="flex min-w-0 flex-col">
              <span className="mb-1.5 flex min-h-6 items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted"><Target className="h-3.5 w-3.5" /> <LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.stop.at.event.score.f1752bfd" /></span>
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
                <span className="flex min-w-0 items-center gap-2"><Clock3 className="h-3.5 w-3.5 shrink-0" /> <LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.stop.before.event.ends.96ca2172" /></span>
                <Badge variant="outline" className="shrink-0"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.30.min.recommended.61238251" /></Badge>
              </span>
              <Input
                type="number"
                min={0}
                max={1440}
                value={Math.round(draft.minimumRemainingSec / 60)}
                onChange={(event) => setDraft((current) => ({ ...current, minimumRemainingSec: clampAutoInvasionInteger(event.target.value, 0, 1440, 30) * 60 }))}
                rightIcon={<span className="text-[10px] text-text-muted"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.min.1f6fa6f6" /></span>}
                className="font-mono"
              />
            </label>
          </div>
        </Card>

		<Card variant="solid" className="p-4">
			<div className="flex items-start justify-between gap-4">
				<div className="min-w-0">
					<div className="flex items-center gap-2 text-sm font-black text-text-main"><ShieldPlus className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.fortify.each.target.418c29a2" /></div>
					<p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.optionally.strengthen.the.generated.castle.before.launching.ad463b71" /></p>
				</div>
				<Switch
					checked={draft.fortifyCurrency !== ''}
					onChange={(checked) => setDraft((current) => ({
						...current,
						fortifyCurrency: checked ? (current.fortifyCurrency || (eventFortifyCurrency ? 'MEDALS' : 'GTO')) : '',
					}))}
					ariaLabel={localizeStatic("ui.settings.components.autoInvasionSettingsModal.ariaLabel.fortify.each.auto.invasion.target.f16c4716")}
				/>
			</div>
			{draft.fortifyCurrency !== '' ? (
				<label className="mt-3 block border-t border-border-base pt-3">
					<span className="mb-1.5 block text-[10px] font-black uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.fortification.currency.36f10a4f" /></span>
					<Select
						value={draft.fortifyCurrency}
						onChange={(value) => setDraft((current) => ({ ...current, fortifyCurrency: value as AutoInvasionClientStateV1['fortifyCurrency'] }))}
						options={fortifyOptions}
						menuGrowToViewport
					/>
					<p className="mt-2 text-[11px] text-text-muted">Available choices come from the active event’s server response. The event-currency choice follows the server-supplied code automatically{eventFortifyCurrency ? ` (currently ${eventFortifyCurrency})` : ''}. The game determines each cumulative <span className="font-mono"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.rae.f585b8d9" /></span> price. Rubies are never selected by default.</p>
				</label>
			) : null}
		</Card>

        <DailyAttackLimitField
          value={draft.dailyAttackLimit}
          onChange={(dailyAttackLimit) => setDraft((current) => ({ ...current, dailyAttackLimit }))}
          serverState={state?.dailyAttacks}
        />

        <p className="rounded-global border border-border-base bg-bg-app/40 px-4 py-3 text-xs text-text-muted">
			<LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.troop.quantities.adapt.to.the.freshly.resolved.67bcfcf5" /></p>
      </div>
    </SettingsModal>
    <FeatureGuideModal feature="autoInvasion" isOpen={isOpen && isGuideOpen} onClose={() => setIsGuideOpen(false)} />
    </>
  );
};
