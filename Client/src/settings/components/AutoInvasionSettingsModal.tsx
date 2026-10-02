import { StopFooter } from '../../components/StopControl';
import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import { BookOpen, Clock3, Crosshair, ShieldCheck, ShieldPlus, Target } from 'lucide-react';
import { useCitadelAPI } from '../../api/ApiContext';
import { castleOptionsFromState } from '../../api/Selectors';
import {
  ATTACK_PRESETS_SECTION,
  parseAttackPresetDocument,
} from '../../attackPresets/AttackPresetTypes';
import {
  attackSetupRef,
  attackSetupRefUsable,
  type AttackSetupRef,
} from '../../attackPresets/AppCreatedPresets';
import { attackPresetReferences } from '../../attackPresets/AttackPresetReferences';
import { Badge, Button, Card, Input, Select, SettingsModal, Switch } from '../../components/ui';
import { Notifications } from '../../components/Notifications';
import { useMetadata } from '../../context/MetadataContext';
import { useConfigurationDraftSession } from '../ConfigurationDraftSession';
import {
  saveInlineSetupAsUserPreset,
  saveModuleWithAppCreatedPresets,
  type AppCreatedPresetSaveWarning,
} from '../AppCreatedPresetSave';
import { recommendEventAttackSetup } from '../onboarding/EventAttackRecommendation';
import { pendingStarterReviews } from '../onboarding/StarterRecipes';
import { evaluateEventAttackReadiness } from '../readiness/eventAttackReadiness';
import { focusReadinessTarget } from '../readiness/focusReadinessTarget';
import type { ReadinessCheck } from '../readiness/Readiness';
import { EventAttackSetupField } from './EventAttackSetupField';
import { ReadinessPanel } from './ReadinessPanel';
import { CastleRequirementField } from './CastleRequirementField';
import { CommanderAssignmentPanel } from './CommanderAssignmentPanel';
import { COMMANDER_FEATURE_SECTION } from '../../Movement/types/CommanderFeatureAssignments';
import { savedCommanderAssignments } from '../requirements/commanderAssignmentDraft';
import { useHostedRuntimePresence } from '../../config/Deployment';
import { useSetupContext } from '../requirements/useSetupContext';
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
import { AUTOMATION_ENABLED_KEYS } from '../disclosure/placement';
import { countCustomValues, travelLine } from '../disclosure/summaries';
import { useSettingsDisclosure } from '../disclosure/useSettingsDisclosure';
import { AutomationRunStrip } from './AutomationRunStrip';
import { collapsedSettingNote, SettingsSection } from './SettingsSection';
import { useDraftRecovery } from '../useDraftRecovery';

/** What the editor holds right after it loads a saved configuration: used by the load effect and by draft recovery. */
function invasionFromSections(sections: Record<string, unknown> | undefined) {
  const draft = parseAutoInvasionClientState(sections?.[AUTO_INVASION_SECTION]);
  return {
    draft,
    attackRef: attackSetupRef(draft.presetId, parseAttackPresetDocument(sections?.[ATTACK_PRESETS_SECTION]), AUTO_INVASION_SECTION, 'attack'),
  };
}

interface AutoInvasionSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenAutomationDuration?: (featureKey: string, featureLabel: string) => void;
}

const invasionDefaults = defaultAutoInvasionClientState();

export const AutoInvasionSettingsModal: React.FC<AutoInvasionSettingsModalProps> = ({ isOpen, onClose, onOpenAutomationDuration }) => {
  const disclosure = useSettingsDisclosure('autoInvasion');
  const { t: localizeStatic } = useStaticLocale();
  const { state } = useCitadelAPI();
  const setup = useSetupContext(AUTO_INVASION_SECTION, useHostedRuntimePresence());
  const [commandersOpen, setCommandersOpen] = useState(false);
  const { troops, tools, unitsLoading, unitsError } = useMetadata();
  const draftSession = useConfigurationDraftSession({
    isOpen,
    section: AUTO_INVASION_SECTION,
    configurationDependencies: [ATTACK_PRESETS_SECTION, COMMANDER_FEATURE_SECTION],
    sessionKey: setup.sessionKey,
  });
  const [draft, setDraft] = useState<AutoInvasionClientStateV1>(defaultAutoInvasionClientState);
  const [attackRef, setAttackRef] = useState<AttackSetupRef>({ source: 'none' });
  const [saving, setSaving] = useState(false);
  const [isGuideOpen, setIsGuideOpen] = useState(false);
  const { locale: guideLocale, pack: guidePack } = useGuideLocale();
  const invasionGuidePack = guidePack.autoInvasion ? guidePack : englishGuidePack;
  const invasionGuideLocale = invasionGuidePack === englishGuidePack ? 'en' : guideLocale;
  useEffect(() => { if (!isOpen) setIsGuideOpen(false); }, [isOpen]);
  const castles = useMemo(() => castleOptionsFromState(state).filter((castle) => castle.kingdomId === 0), [state]);
  const presetDocument = useMemo(
    () => parseAttackPresetDocument(draftSession.sections?.[ATTACK_PRESETS_SECTION]),
    [draftSession.sections],
  );
  const presetReferences = useMemo(() => attackPresetReferences(draftSession.sections), [draftSession.sections]);
  const commanderAssignments = useMemo(() => savedCommanderAssignments(draftSession.sections), [draftSession.sections]);
  const metadataReady = !unitsLoading && !unitsError;
  // Unit counts are current only once this connection has its baseline (CIT-15 D1).
  const observation = setup.observation;
  const completedAchievements = state?.player.achievements?.completed ?? {};
  const achievementsObserved = Boolean(state?.player.achievements?.observedAt);
  const difficultyCatalog = useEventDifficultyOptions(isOpen, [71, 103], completedAchievements);
  const foreignLordsDifficulties = difficultyCatalog.optionsByEvent['71'] ?? [];
  const bloodcrowDifficulties = difficultyCatalog.optionsByEvent['103'] ?? [];
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
    if (!isOpen || !draftSession.initialSnapshot) return;
    const initial = invasionFromSections(draftSession.initialSections);
    setDraft(initial.draft);
    setAttackRef(initial.attackRef);
  }, [draftSession.initialSections, draftSession.openKey, draftSession.initialSnapshot, isOpen]);

  const sourceCastle = useMemo(() => {
    const castle = state?.castles?.[String(draft.sourceCastleId)];
    return castle && castle.kingdomId === 0 ? castle : null;
  }, [draft.sourceCastleId, state?.castles]);
  const recipePending = useMemo(() => pendingStarterReviews(), []);
  const recommendation = useMemo(
    () => recommendEventAttackSetup({ sourceCastle, observation, troops, tools, metadataReady, eventId: 71 }),
    [metadataReady, observation, sourceCastle, tools, troops],
  );
  const canSave = draft.sourceCastleId > 0
    && attackSetupRefUsable(attackRef, presetDocument)
    && foreignLordsSelectionAvailable
    && bloodcrowSelectionAvailable
    && draft.scoreTarget > 0;
  const readiness = useMemo(() => evaluateEventAttackReadiness({
    featureId: 'autoInvasion',
    draft: {
      sourceCastleId: draft.sourceCastleId,
      slots: [{ slot: 'attack', ref: attackRef }],
      scoreTarget: draft.scoreTarget,
      dailyAttackLimit: draft.dailyAttackLimit,
      horseTravelBoostId: draft.horseTravelBoostId,
      fortifyCurrency: draft.fortifyCurrency,
    },
    state,
    document: presetDocument,
    troops,
    tools,
    metadataReady,
    observation,
    commanders: { assignments: commanderAssignments, movement: setup.movement, gameLoggedIn: setup.gameLoggedIn },
    difficulties: {
      selections: [
        { eventId: 71, available: foreignLordsSelectionAvailable },
        { eventId: 103, available: bloodcrowSelectionAvailable },
      ],
      achievementsObserved,
      loading: difficultyCatalog.loading,
    },
  }), [
    observation,
    commanderAssignments, setup.gameLoggedIn, setup.movement,
    achievementsObserved, attackRef, bloodcrowSelectionAvailable, difficultyCatalog.loading, draft.dailyAttackLimit,
    draft.fortifyCurrency, draft.horseTravelBoostId, draft.scoreTarget, draft.sourceCastleId, foreignLordsSelectionAvailable,
    metadataReady, presetDocument, state, tools, troops,
  ]);
  const fixReadiness = (check: ReadinessCheck) => {
    if (check.id === 'commanders' || check.id === 'commander-assignment') {
      setCommandersOpen(true);
      window.requestAnimationFrame(() => focusReadinessTarget('auto-invasion-commanders-heading'));
      return;
    }
    disclosure.fix(check);
  };
  const moduleLabel = localizeStatic('attackPresets.module.autoInvasion');

  const save = async () => {
    if (saving || !canSave) return;
    setSaving(true);
    const warnings: AppCreatedPresetSaveWarning[] = [];
    try {
      await saveModuleWithAppCreatedPresets({
        draftSession,
        section: AUTO_INVASION_SECTION,
        slots: [{ slot: 'attack', ref: attackRef, moduleLabel, slotLabel: localizeStatic('attackPresets.slot.attack') }],
        buildSectionValue: (ids) => ({ ...draft, presetId: ids.attack }),
        formatPresetName: (module, slot) => localizeStatic('attackPresets.appCreatedName', { module, slot }),
        warnings,
      });
      Notifications.success('Auto Invasion settings saved.');
      if (warnings.includes('cleanup-pending')) Notifications.warning(localizeStatic('attackPresets.cleanupPending'));
      onClose();
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not save Auto Invasion settings.');
      if (warnings.includes('cleanup-pending')) Notifications.warning(localizeStatic('attackPresets.rollbackPending'));
    } finally {
      setSaving(false);
    }
  };

  const loadedInvasion = invasionFromSections(draftSession.sections);
  const recovery = useDraftRecovery({ section: AUTO_INVASION_SECTION, isOpen, draftSession, draft: draft, loaded: loadedInvasion.draft, extras: { attackRef }, loadedExtras: { attackRef: loadedInvasion.attackRef } });
  useEffect(() => {
    const extras = draftSession.recoveredExtras?.value as { attackRef?: AttackSetupRef } | undefined;
    if (extras?.attackRef) setAttackRef(extras.attackRef);
  }, [draftSession.recoveredExtras]);

  return (<>
    <SettingsModal
      footerLeading={<StopFooter featureId="autoInvasion" />}
      isOpen={isOpen}
      onClose={() => { if (!saving) onClose(); }}
      maxWidth="3xl"
      title={localizeStatic("ui.settings.components.autoInvasionSettingsModal.title.auto.invasion.d43e5a94")}
      icon={<Crosshair className="h-5 w-5" />}
      description={localizeStatic("ui.settings.components.autoInvasionSettingsModal.description.foreign.lords.and.bloodcrow.attack.plan.0ee8d04e")}
      titleTrailing={<Button variant="secondary" size="sm" className="shrink-0" onClick={() => setIsGuideOpen(true)} leftIcon={<BookOpen className="h-4 w-4" />}><span lang={invasionGuideLocale}>{invasionGuidePack.ui.guideButton}</span></Button>}
      onSave={() => void save()}
      isSaving={saving}
      saveDisabled={!canSave || !draftSession.ready}
      contentDisabled={!draftSession.ready}
      contentNotice={<>{recovery.banner}{draftSession.conflictNotice}</>}
    >
      <AutomationRunStrip
        featureId="autoInvasion"
        onOpenDuration={onOpenAutomationDuration ? () => onOpenAutomationDuration(AUTOMATION_ENABLED_KEYS.autoInvasion, 'Auto Invasion') : undefined}
      />
      <div className="space-y-3">
        <SettingsSection disclosure={disclosure} section="setup">
        <Card variant="solid" className="">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="md:col-span-2">
              <CastleRequirementField
                id="auto-invasion-source"
                label={<LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.source.castle.86d5a48e" />}
                value={draft.sourceCastleId}
                onChange={(sourceCastleId) => setDraft((current) => ({ ...current, sourceCastleId }))}
                state={setup.state}
                purpose="source-great-empire"
                options={castles.map((castle) => ({ value: String(castle.id), label: `${castle.name} · ${castle.x}:${castle.y}` }))}
                placeholder={localizeStatic("ui.settings.components.autoInvasionSettingsModal.placeholder.choose.a.great.empire.castle.8a81fec1")}
              />
            </div>

            <div className="md:col-span-2">
              <EventAttackSetupField
                id="auto-invasion-attack"
                label={<LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.attack.preset.407b93e9" />}
                section={AUTO_INVASION_SECTION}
                slot="attack"
                moduleLabel={moduleLabel}
                slotLabel={localizeStatic('attackPresets.slot.attack')}
                value={attackRef}
                onChange={setAttackRef}
                document={presetDocument}
                references={presetReferences}
                sourceCastle={sourceCastle}
                observation={observation}
                eventId={71}
                recommendation={recommendation}
                recipePending={recipePending}
                onSaveAsPreset={(setup, name) => saveInlineSetupAsUserPreset(draftSession, setup, name)}
                readinessChecks={readiness.checks.filter((check) => check.slot === 'attack')}
                disabled={saving}
              />
            </div>
          </div>
        </Card>
        </SettingsSection>

        <SettingsSection disclosure={disclosure} section="event" className="space-y-3">
        <Card id="auto-invasion-difficulty" variant="solid" className="">
          <div className="mb-3 flex items-start justify-between gap-3">
            <div>
              <div className="flex items-center gap-2 text-body font-semibold text-text-main"><ShieldCheck className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.event.difficulty.88766fcf" /></div>
              <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.only.levels.unlocked.by.this.player.s.15a0e9b5" /></p>
            </div>
            <Badge variant="outline">{achievementsObserved ? 'Achievements synced' : 'Syncing achievements'}</Badge>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className="mb-1.5 flex items-center justify-between gap-2 text-caption font-semibold text-text-muted">
                Foreign Lords
                <span className="normal-case text-primary">Through {eventDifficultyName(foreignLordsDifficulties, Number(foreignLordsDifficulties.at(-1)?.value))}</span>
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
              <span className="mb-1.5 flex items-center justify-between gap-2 text-caption font-semibold text-text-muted">
                Bloodcrow
                <span className="normal-case text-primary">Through {eventDifficultyName(bloodcrowDifficulties, Number(bloodcrowDifficulties.at(-1)?.value))}</span>
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
          {difficultyCatalog.loading ? <p className="mt-3 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.loading.official.event.difficulties.8ddbd72d" /></p> : null}
          {difficultyCatalog.error ? <p className="mt-3 text-caption text-danger">{difficultyCatalog.error}</p> : null}
          {!achievementsObserved ? <p className="mt-3 text-caption text-warning"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.achievement.data.is.still.syncing.base.difficulties.bd6eea97" /></p> : null}
        </Card>

        <Card variant="solid" className="">
          <div className="grid items-start gap-4 md:grid-cols-2">
            <label id="auto-invasion-score" className="flex min-w-0 flex-col">
              <span className="mb-1.5 flex min-h-6 items-center gap-2 text-caption font-semibold text-text-muted"><Target className="h-3.5 w-3.5" /> <LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.stop.at.event.score.f1752bfd" /></span>
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
              <span className="mb-1.5 flex min-h-6 items-center justify-between gap-2 text-caption font-semibold text-text-muted">
                <span className="flex min-w-0 items-center gap-2"><Clock3 className="h-3.5 w-3.5 shrink-0" /> <LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.stop.before.event.ends.96ca2172" /></span>
                <Badge variant="outline" className="shrink-0"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.30.min.recommended.61238251" /></Badge>
              </span>
              <Input
                type="number"
                min={0}
                max={1440}
                value={Math.round(draft.minimumRemainingSec / 60)}
                onChange={(event) => setDraft((current) => ({ ...current, minimumRemainingSec: clampAutoInvasionInteger(event.target.value, 0, 1440, 30) * 60 }))}
                rightIcon={<span className="text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.min.1f6fa6f6" /></span>}
                className="font-mono"
              />
            </label>
          </div>
        </Card>
        </SettingsSection>

		<SettingsSection disclosure={disclosure} section="fortify">
		<Card id="auto-invasion-fortify" variant="solid" className="">
			<div className="flex items-start justify-between gap-4">
				<div className="min-w-0">
					<div className="flex items-center gap-2 text-body font-semibold text-text-main"><ShieldPlus className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.fortify.each.target.418c29a2" /></div>
					<p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.optionally.strengthen.the.generated.castle.before.launching.ad463b71" /></p>
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
					<span className="mb-1.5 block text-caption font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.fortification.currency.36f10a4f" /></span>
					<Select
						value={draft.fortifyCurrency}
						onChange={(value) => setDraft((current) => ({ ...current, fortifyCurrency: value as AutoInvasionClientStateV1['fortifyCurrency'] }))}
						options={fortifyOptions}
						menuGrowToViewport
					/>
					<p className="mt-2 text-caption text-text-muted">Available choices come from the active event’s server response. The event-currency choice follows the server-supplied code automatically{eventFortifyCurrency ? ` (currently ${eventFortifyCurrency})` : ''}. The game determines each cumulative <span className="font-mono"><LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.rae.f585b8d9" /></span> price. Rubies are never selected by default.</p>
				</label>
			) : null}
		</Card>
		</SettingsSection>

        <SettingsSection disclosure={disclosure} section="limits">
        <div id="auto-invasion-daily-limit" tabIndex={-1} className="outline-none">
          <DailyAttackLimitField
            value={draft.dailyAttackLimit}
            onChange={(dailyAttackLimit) => setDraft((current) => ({ ...current, dailyAttackLimit }))}
            serverState={state?.dailyAttacks}
          />
        </div>
        </SettingsSection>

        <SettingsSection
          disclosure={disclosure}
          section="travel"
          summary={[travelLine(draft.horseTravelBoostId)]}
          customCount={countCustomValues(draft, invasionDefaults, ['horseTravelBoostId'])}
        >
          <HorseTravelBoostSelect
            className="block"
            value={draft.horseTravelBoostId}
            onChange={(horseTravelBoostId) => setDraft((current) => ({ ...current, horseTravelBoostId }))}
          />
        </SettingsSection>

        <p className="rounded-global border border-border-base bg-bg-app/40 px-4 py-3 text-caption text-text-muted">
			<LocalizedText messageKey="ui.settings.components.autoInvasionSettingsModal.troop.quantities.adapt.to.the.freshly.resolved.67bcfcf5" /></p>

        <ReadinessPanel report={readiness} slotLabelKeys={{ attack: 'attackPresets.slot.attack' }} onFix={fixReadiness} noteFor={collapsedSettingNote(disclosure)} />

        <CommanderAssignmentPanel
          id="auto-invasion-commanders"
          featureId="autoInvasion"
          draftSession={draftSession}
          state={setup.state}
          movement={setup.movement}
          gameLoggedIn={setup.gameLoggedIn}
          expanded={commandersOpen}
          onExpandedChange={setCommandersOpen}
          disabled={saving}
        />
      </div>
    </SettingsModal>
    <FeatureGuideModal feature="autoInvasion" isOpen={isOpen && isGuideOpen} onClose={() => setIsGuideOpen(false)} />
    </>
  );
};
