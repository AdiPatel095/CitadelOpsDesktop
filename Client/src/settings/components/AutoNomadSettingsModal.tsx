import { StopFooter } from '../../components/StopControl';
import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import { BookOpen, Clock3, Crosshair, Lock, RotateCcw, ShieldCheck, Target, TestTube2 } from 'lucide-react';
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
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Card, Input, Select, SettingsModal, Switch } from '../../components/ui';
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
  AUTO_NOMAD_SECTION,
  clampAutoNomadInteger,
  defaultAutoNomadClientState,
  parseAutoNomadClientState,
  type AutoNomadClientStateV5,
} from '../AutoNomadClientState';
import { eventDifficultyName, useEventDifficultyOptions } from '../EventDifficultyOptions';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import { DailyAttackLimitField } from './DailyAttackLimitField';
import { FeatureGuideModal } from './FeatureGuideModal';
import { AUTOMATION_ENABLED_KEYS } from '../disclosure/placement';
import { cooldownSkipLines, countCustomValues, rbcTrialLine, travelLine } from '../disclosure/summaries';
import { useSettingsDisclosure } from '../disclosure/useSettingsDisclosure';
import { AutomationRunStrip } from './AutomationRunStrip';
import { collapsedSettingNote, SettingsSection } from './SettingsSection';
import { englishGuidePack, useGuideLocale } from '../../config/useGuideLocale';
import { useDraftRecovery } from '../useDraftRecovery';

/** What the editor holds right after it loads a saved configuration: used by the load effect and by draft recovery. */
function nomadFromSections(sections: Record<string, unknown> | undefined) {
  const draft = parseAutoNomadClientState(sections?.[AUTO_NOMAD_SECTION]);
  const presets = parseAttackPresetDocument(sections?.[ATTACK_PRESETS_SECTION]);
  return {
    draft,
    nomadRef: attackSetupRef(draft.nomadPresetId, presets, AUTO_NOMAD_SECTION, 'nomad'),
    samuraiRef: attackSetupRef(draft.samuraiPresetId, presets, AUTO_NOMAD_SECTION, 'samurai'),
  };
}

interface AutoNomadSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenAutomationDuration?: (featureKey: string, featureLabel: string) => void;
}

const nomadDefaults = defaultAutoNomadClientState();

export const AutoNomadSettingsModal: React.FC<AutoNomadSettingsModalProps> = ({ isOpen, onClose, onOpenAutomationDuration }) => {
  const disclosure = useSettingsDisclosure('autoNomad');
  const { t: localizeStatic } = useStaticLocale();
  const { state } = useCitadelAPI();
  const setup = useSetupContext(AUTO_NOMAD_SECTION, useHostedRuntimePresence());
  const [commandersOpen, setCommandersOpen] = useState(false);
  const { troops, tools, unitsLoading, unitsError } = useMetadata();
  const draftSession = useConfigurationDraftSession({
    isOpen,
    section: AUTO_NOMAD_SECTION,
    configurationDependencies: [ATTACK_PRESETS_SECTION, COMMANDER_FEATURE_SECTION],
    sessionKey: setup.sessionKey,
  });
  const [draft, setDraft] = useState<AutoNomadClientStateV5>(defaultAutoNomadClientState);
  const [nomadRef, setNomadRef] = useState<AttackSetupRef>({ source: 'none' });
  const [samuraiRef, setSamuraiRef] = useState<AttackSetupRef>({ source: 'none' });
  const [saving, setSaving] = useState(false);
  const [isGuideOpen, setIsGuideOpen] = useState(false);
  const { locale: guideLocale, pack: guidePack } = useGuideLocale();
  const nomadGuidePack = guidePack.autoNomad ? guidePack : englishGuidePack;
  const nomadGuideLocale = nomadGuidePack === englishGuidePack ? 'en' : guideLocale;
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
  const difficultyCatalog = useEventDifficultyOptions(isOpen, [72, 80], completedAchievements);
  const nomadDifficulties = difficultyCatalog.optionsByEvent['72'] ?? [];
  const samuraiDifficulties = difficultyCatalog.optionsByEvent['80'] ?? [];
  const nomadSelectionAvailable = nomadDifficulties.some((option) => option.value === String(draft.nomadDifficultyId));
  const samuraiSelectionAvailable = samuraiDifficulties.some((option) => option.value === String(draft.samuraiDifficultyId));
  const sourceCastle = useMemo(() => {
    const castle = state?.castles?.[String(draft.sourceCastleId)];
    return castle && castle.kingdomId === 0 ? castle : null;
  }, [draft.sourceCastleId, state?.castles]);
  const recipePending = useMemo(() => pendingStarterReviews(), []);
  const nomadRecommendation = useMemo(
    () => recommendEventAttackSetup({ sourceCastle, observation, troops, tools, metadataReady, eventId: 72 }),
    [metadataReady, observation, sourceCastle, tools, troops],
  );
  const samuraiRecommendation = useMemo(
    () => recommendEventAttackSetup({ sourceCastle, observation, troops, tools, metadataReady, eventId: 80 }),
    [metadataReady, observation, sourceCastle, tools, troops],
  );

  useEffect(() => {
    if (!isOpen || !draftSession.initialSnapshot) return;
    const initial = nomadFromSections(draftSession.initialSections);
    setDraft(initial.draft);
    setNomadRef(initial.nomadRef);
    setSamuraiRef(initial.samuraiRef);
  }, [draftSession.initialSections, draftSession.openKey, draftSession.initialSnapshot, isOpen]);

  const nomadUsable = attackSetupRefUsable(nomadRef, presetDocument);
  const samuraiUsable = attackSetupRefUsable(samuraiRef, presetDocument);
  const trialReady = draft.rbcTest.enabled
    && Boolean(draft.rbcTest.runId)
    && nomadUsable
    && draft.skipCooldowns;
  const eventReady = !draft.rbcTest.enabled
    && nomadUsable
    && samuraiUsable
    && nomadSelectionAvailable
    && samuraiSelectionAvailable
    && draft.scoreTarget > 0;
  const canSave = draft.sourceCastleId > 0 && (trialReady || eventReady);
  const readiness = useMemo(() => evaluateEventAttackReadiness({
    featureId: 'autoNomad',
    draft: {
      sourceCastleId: draft.sourceCastleId,
      slots: draft.rbcTest.enabled
        ? [{ slot: 'nomad', ref: nomadRef }]
        : [{ slot: 'nomad', ref: nomadRef }, { slot: 'samurai', ref: samuraiRef }],
      scoreTarget: draft.rbcTest.enabled ? undefined : draft.scoreTarget,
      dailyAttackLimit: draft.dailyAttackLimit,
      horseTravelBoostId: draft.horseTravelBoostId,
    },
    state,
    document: presetDocument,
    troops,
    tools,
    metadataReady,
    observation,
    commanders: { assignments: commanderAssignments, movement: setup.movement, gameLoggedIn: setup.gameLoggedIn },
    difficulties: draft.rbcTest.enabled ? undefined : {
      selections: [
        { eventId: 72, available: nomadSelectionAvailable },
        { eventId: 80, available: samuraiSelectionAvailable },
      ],
      achievementsObserved,
      loading: difficultyCatalog.loading,
    },
  }), [
    observation,
    commanderAssignments, setup.gameLoggedIn, setup.movement,
    achievementsObserved, difficultyCatalog.loading, draft.dailyAttackLimit, draft.horseTravelBoostId, draft.rbcTest.enabled,
    draft.scoreTarget, draft.sourceCastleId, metadataReady, nomadRef, nomadSelectionAvailable, presetDocument, samuraiRef,
    samuraiSelectionAvailable, state, tools, troops,
  ]);
  const slotChecks = (slot: string) => readiness.checks.filter((check) => check.slot === slot);
  const fixReadiness = (check: ReadinessCheck) => {
    if (check.id === 'commanders' || check.id === 'commander-assignment') {
      setCommandersOpen(true);
      window.requestAnimationFrame(() => focusReadinessTarget('auto-nomad-commanders-heading'));
      return;
    }
    disclosure.fix(check);
  };
  const moduleLabel = localizeStatic('attackPresets.module.autoNomad');
  const saveAsPreset = (setup: Parameters<typeof saveInlineSetupAsUserPreset>[1], name: string) => (
    saveInlineSetupAsUserPreset(draftSession, setup, name)
  );

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
    const warnings: AppCreatedPresetSaveWarning[] = [];
    try {
      await saveModuleWithAppCreatedPresets({
        draftSession,
        section: AUTO_NOMAD_SECTION,
        slots: [
          { slot: 'nomad', ref: nomadRef, moduleLabel, slotLabel: localizeStatic('attackPresets.slot.nomad') },
          { slot: 'samurai', ref: samuraiRef, moduleLabel, slotLabel: localizeStatic('attackPresets.slot.samurai') },
        ],
        buildSectionValue: (ids) => ({ ...draft, nomadPresetId: ids.nomad, samuraiPresetId: ids.samurai }),
        formatPresetName: (module, slot) => localizeStatic('attackPresets.appCreatedName', { module, slot }),
        warnings,
      });
      Notifications.success('Auto Nomad/Samurai settings saved.');
      if (warnings.includes('cleanup-pending')) Notifications.warning(localizeStatic('attackPresets.cleanupPending'));
      onClose();
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not save Auto Nomad/Samurai settings.');
      if (warnings.includes('cleanup-pending')) Notifications.warning(localizeStatic('attackPresets.rollbackPending'));
    } finally {
      setSaving(false);
    }
  };

  const loadedNomad = nomadFromSections(draftSession.sections);
  const recovery = useDraftRecovery({ section: AUTO_NOMAD_SECTION, isOpen, draftSession, draft: draft, loaded: loadedNomad.draft, extras: { nomadRef, samuraiRef }, loadedExtras: { nomadRef: loadedNomad.nomadRef, samuraiRef: loadedNomad.samuraiRef } });
  useEffect(() => {
    const extras = draftSession.recoveredExtras?.value as { nomadRef?: AttackSetupRef; samuraiRef?: AttackSetupRef } | undefined;
    if (!extras) return;
    if (extras.nomadRef) setNomadRef(extras.nomadRef);
    if (extras.samuraiRef) setSamuraiRef(extras.samuraiRef);
  }, [draftSession.recoveredExtras]);

  return (<><SettingsModal
      footerLeading={<StopFooter featureId="autoNomad" />}
      isOpen={isOpen}
      onClose={() => { if (!saving) onClose(); }}
      maxWidth="3xl"
      titleTrailing={<Button variant="secondary" size="sm" className="shrink-0" onClick={() => setIsGuideOpen(true)} leftIcon={<BookOpen className="h-4 w-4" />}><span lang={nomadGuideLocale}>{nomadGuidePack.ui.guideButton}</span></Button>}
      title={localizeStatic("ui.settings.components.autoNomadSettingsModal.title.auto.nomad.samurai.13e95d24")}
      icon={<Crosshair className="h-5 w-5" />}
      description={localizeStatic("ui.settings.components.autoNomadSettingsModal.description.four.camp.leveling.and.locked.target.attack.2e4977f9")}
      onSave={() => void save()}
      isSaving={saving}
      saveDisabled={!canSave || !draftSession.ready}
      contentDisabled={!draftSession.ready}
      contentNotice={<>{recovery.banner}{draftSession.conflictNotice}</>}
    >
      <AutomationRunStrip
        featureId="autoNomad"
        onOpenDuration={onOpenAutomationDuration ? () => onOpenAutomationDuration(AUTOMATION_ENABLED_KEYS.autoNomad, 'Auto Nomad / Samurai') : undefined}
      />
      <div className="space-y-3">
        <SettingsSection disclosure={disclosure} section="setup">
        <Card variant="solid" className="">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="md:col-span-2">
              <CastleRequirementField
                id="auto-nomad-source"
                label={<LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.source.castle.86d5a48e" />}
                value={draft.sourceCastleId}
                onChange={(sourceCastleId) => setDraft((current) => ({ ...current, sourceCastleId }))}
                state={setup.state}
                purpose="source-great-empire"
                options={castles.map((castle) => ({ value: String(castle.id), label: `${castle.name} · ${castle.x}:${castle.y}` }))}
                placeholder={localizeStatic("ui.settings.components.autoNomadSettingsModal.placeholder.choose.a.great.empire.castle.8a81fec1")}
              />
            </div>

            <EventAttackSetupField
              id="auto-nomad-nomad"
              label={<LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.nomad.attack.preset.88ec98ef" />}
              section={AUTO_NOMAD_SECTION}
              slot="nomad"
              moduleLabel={moduleLabel}
              slotLabel={localizeStatic('attackPresets.slot.nomad')}
              value={nomadRef}
              onChange={setNomadRef}
              document={presetDocument}
              references={presetReferences}
              sourceCastle={sourceCastle}
              observation={observation}
              eventId={72}
              recommendation={nomadRecommendation}
              recipePending={recipePending}
              onSaveAsPreset={saveAsPreset}
              readinessChecks={slotChecks('nomad')}
              disabled={saving}
            />
            <EventAttackSetupField
              id="auto-nomad-samurai"
              label={<LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.samurai.attack.preset.31066b77" />}
              section={AUTO_NOMAD_SECTION}
              slot="samurai"
              moduleLabel={moduleLabel}
              slotLabel={localizeStatic('attackPresets.slot.samurai')}
              value={samuraiRef}
              onChange={setSamuraiRef}
              document={presetDocument}
              references={presetReferences}
              sourceCastle={sourceCastle}
              observation={observation}
              eventId={80}
              recommendation={samuraiRecommendation}
              recipePending={recipePending}
              onSaveAsPreset={saveAsPreset}
              readinessChecks={draft.rbcTest.enabled ? [] : slotChecks('samurai')}
              disabled={saving}
            />
          </div>
        </Card>
        </SettingsSection>

        <SettingsSection disclosure={disclosure} section="limits">
        <div id="auto-nomad-daily-limit" tabIndex={-1} className="outline-none">
          <DailyAttackLimitField
            value={draft.dailyAttackLimit}
            onChange={(dailyAttackLimit) => setDraft((current) => ({ ...current, dailyAttackLimit }))}
            serverState={state?.dailyAttacks}
          />
        </div>
        </SettingsSection>

        <SettingsSection disclosure={disclosure} section="event" className="space-y-3">
        <Card id="auto-nomad-difficulty" variant="solid" className="">
          <div className="mb-3 flex items-start justify-between gap-3">
            <div>
              <div className="flex items-center gap-2 text-body font-semibold text-text-main"><ShieldCheck className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.event.start.difficulty.d32020cb" /></div>
              <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.the.module.starts.the.active.event.with.b13c4375" /></p>
            </div>
            <Badge variant="outline">{achievementsObserved ? 'Achievements synced' : 'Syncing achievements'}</Badge>
          </div>
          <div className="grid gap-4 md:grid-cols-2">
            <label className="block">
              <span className="mb-1.5 flex items-center justify-between gap-2 text-caption font-semibold text-text-muted">
                Nomad
                <span className="normal-case text-primary">Through {eventDifficultyName(nomadDifficulties, Number(nomadDifficulties.at(-1)?.value))}</span>
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
              <span className="mb-1.5 flex items-center justify-between gap-2 text-caption font-semibold text-text-muted">
                Samurai
                <span className="normal-case text-primary">Through {eventDifficultyName(samuraiDifficulties, Number(samuraiDifficulties.at(-1)?.value))}</span>
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
          {difficultyCatalog.loading ? <p className="mt-3 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.loading.official.event.difficulties.8ddbd72d" /></p> : null}
          {difficultyCatalog.error ? <p className="mt-3 text-caption text-danger">{difficultyCatalog.error}</p> : null}
        </Card>

        <Card variant="solid" className="">
          <div className="grid items-start gap-4 md:grid-cols-2">
            <label id="auto-nomad-score" className="flex min-w-0 flex-col">
              <span className="mb-1.5 flex min-h-6 items-center gap-2 text-caption font-semibold text-text-muted"><Target className="h-3.5 w-3.5" /> <LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.stop.at.event.score.f1752bfd" /></span>
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
                <span className="flex min-w-0 items-center gap-2"><Clock3 className="h-3.5 w-3.5 shrink-0" /> <LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.stop.before.event.ends.96ca2172" /></span>
                <Badge variant="outline" className="shrink-0"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.30.min.recommended.61238251" /></Badge>
              </span>
              <Input
                type="number"
                min={0}
                max={1440}
                value={Math.round(draft.minimumRemainingSec / 60)}
                onChange={(event) => setDraft((current) => ({ ...current, minimumRemainingSec: clampAutoNomadInteger(event.target.value, 0, 1440, 30) * 60 }))}
                rightIcon={<span className="text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.min.1f6fa6f6" /></span>}
                className="font-mono"
              />
            </label>
          </div>
        </Card>
        </SettingsSection>

        <SettingsSection
          disclosure={disclosure}
          section="cooldown-skips"
          summary={cooldownSkipLines(draft.skipCooldowns, draft.timeSkipReserve)}
          customCount={countCustomValues(draft, nomadDefaults, ['skipCooldowns', 'timeSkipReserve'])}
        >
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-body font-semibold text-text-main"><RotateCcw className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.clear.each.landed.hit.cooldown.e51dba73" /></div>
              <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.after.every.confirmed.victory.refresh.the.target.755f02c1" /></p>
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
                    <span className="mb-1.5 block text-caption font-semibold text-text-muted">Keep {label}</span>
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
              <p className="mt-3 text-caption text-warning"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.each.server.command.uses.exactly.one.skip.ab37a717" /></p>
            </div>
          ) : null}
        </SettingsSection>

        <SettingsSection
          disclosure={disclosure}
          section="travel"
          summary={[travelLine(draft.horseTravelBoostId)]}
          customCount={countCustomValues(draft, nomadDefaults, ['horseTravelBoostId'])}
        >
          <HorseTravelBoostSelect
            className="block"
            value={draft.horseTravelBoostId}
            onChange={(horseTravelBoostId) => setDraft((current) => ({ ...current, horseTravelBoostId }))}
          />
        </SettingsSection>

        <SettingsSection
          disclosure={disclosure}
          section="rbc-trial"
          summary={[rbcTrialLine(draft.rbcTest)]}
          customCount={draft.rbcTest.enabled ? 1 : 0}
        >
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-body font-semibold text-text-main"><TestTube2 className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.temporary.rbc.end.to.end.trial.9da6b870" /></div>
              <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.use.the.nomad.preset.against.one.robber.89f8e101" /></p>
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
                <span className="mb-1.5 block text-caption font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.target.x.884ddd14" /></span>
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
                <span className="mb-1.5 block text-caption font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.target.y.aae5c7ef" /></span>
                <Input
                  type="number"
                  min={0}
                  max={2000}
                  value={draft.rbcTest.targetY}
                  onChange={(event) => setDraft((current) => ({ ...current, rbcTest: { ...current.rbcTest, targetY: clampAutoNomadInteger(event.target.value, 0, 2000, 0) } }))}
                  className="font-mono"
                />
              </label>
              <p className="sm:col-span-2 text-caption text-warning"><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.the.chain.uses.every.currently.available.selected.b2a29d49" /></p>
            </div>
          ) : null}
        </SettingsSection>

        <Card variant="solid" className="">
          <div className="flex items-center gap-2 text-body font-semibold text-text-main"><Lock className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.fixed.four.camp.flow.380f9acb" /></div>
          <div className="mt-3 grid gap-2 sm:grid-cols-3">
            <div className="rounded-xl border border-border-base bg-bg-app/45 p-3 text-caption text-text-muted"><Badge variant="outline" className="mb-2">1</Badge><div><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.advance.each.of.the.four.nearest.regular.45952900" /></div></div>
            <div className="rounded-xl border border-border-base bg-bg-app/45 p-3 text-caption text-text-muted"><Badge variant="outline" className="mb-2">2</Badge><div><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.rank.maxed.camps.by.defense.capacity.plus.f2d38ec8" /></div></div>
            <div className="rounded-xl border border-border-base bg-bg-app/45 p-3 text-caption text-text-muted"><Badge variant="outline" className="mb-2">3</Badge><div><LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.send.the.active.event.s.preset.only.3ad5b2bc" /></div></div>
          </div>
        </Card>

        <p className="rounded-global border border-border-base bg-bg-app/40 px-4 py-3 text-caption text-text-muted">
          <LocalizedText messageKey="ui.settings.components.autoNomadSettingsModal.adi.must.confirm.the.same.target.and.1c6467ca" /></p>

        <ReadinessPanel
          report={readiness}
          slotLabelKeys={{ nomad: 'attackPresets.slot.nomad', samurai: 'attackPresets.slot.samurai' }}
          onFix={fixReadiness}
          noteFor={collapsedSettingNote(disclosure)}
        />
        <CommanderAssignmentPanel
          id="auto-nomad-commanders"
          featureId="autoNomad"
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
    <FeatureGuideModal feature="autoNomad" isOpen={isOpen && isGuideOpen} onClose={() => setIsGuideOpen(false)} />
    </>
  );
};
