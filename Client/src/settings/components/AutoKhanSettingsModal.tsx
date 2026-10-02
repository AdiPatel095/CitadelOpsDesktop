import { StopFooter } from '../../components/StopControl';
import {useLocalizedMessage} from '../../i18n/useLocalizedMessage';
import {parseMessageDescriptor} from '../../i18n/messageDescriptor';
import {messageLanguageAttributes} from '../../i18n/messageLanguage';
import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import {
  BookOpen,
  Clock3,
  Crosshair,
  Flame,
  LockKeyhole,
  RotateCcw,
  ShieldAlert,
  ShieldCheck,
  ShoppingCart,
  Zap,
} from 'lucide-react';
import { castleOptionsFromState } from '../../api/Selectors';
import {
  ATTACK_PRESETS_SECTION,
  parseAttackPresetDocument,
} from '../../attackPresets/AttackPresetTypes';
import { attackSetupRef, attackSetupRefUsable, type AttackSetupRef } from '../../attackPresets/AppCreatedPresets';
import { attackPresetReferences, defensePresetReferences } from '../../attackPresets/AttackPresetReferences';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Card, Input, SettingsModal, Switch } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';
import {
  DEFENSE_PRESETS_SECTION,
  parseDefensePresetDocument,
} from '../../defensePresets/DefensePresetTypes';
import { defenseSetupRef, defenseSetupRefUsable, type DefenseSetupRef } from '../../defensePresets/AppCreatedDefensePresets';
import { COMMANDER_FEATURE_SECTION } from '../../Movement/types/CommanderFeatureAssignments';
import { useConfigurationDraftSession } from '../ConfigurationDraftSession';
import {
  saveInlineDefenseAsUserPreset,
  saveInlineSetupAsUserPreset,
  saveModuleWithAppCreatedPresets,
  type AppCreatedPresetSaveWarning,
} from '../AppCreatedPresetSave';
import { recommendEventAttackSetup } from '../onboarding/EventAttackRecommendation';
import { khanDefenseStarter } from '../onboarding/KhanDefenseStarter';
import { pendingStarterReviews } from '../onboarding/StarterRecipes';
import { evaluateKhanReadiness } from '../readiness/khanReadiness';
import { focusReadinessTarget } from '../readiness/focusReadinessTarget';
import type { ReadinessCheck } from '../readiness/Readiness';
import { savedCommanderAssignments } from '../requirements/commanderAssignmentDraft';
import { evaluateCommanderEligibility } from '../requirements/commanderEligibility';
import { greatEmpireMainCastle } from '../requirements/setupReadiness';
import { useHostedRuntimePresence } from '../../config/Deployment';
import { useSetupContext } from '../requirements/useSetupContext';
import { CastleRequirementField } from './CastleRequirementField';
import { CommanderAssignmentPanel } from './CommanderAssignmentPanel';
import { DefenseSetupField } from './DefenseSetupField';
import { EventAttackSetupField } from './EventAttackSetupField';
import { ReadinessPanel } from './ReadinessPanel';
import {
  AUTO_KHAN_SECTION,
  clampAutoKhanInteger,
  defaultAutoKhanClientState,
  parseAutoKhanClientState,
  type AutoKhanClientStateV1,
} from '../AutoKhanClientState';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import { DailyAttackLimitField } from './DailyAttackLimitField';
import { FeatureGuideModal } from './FeatureGuideModal';
import { AUTOMATION_ENABLED_KEYS } from '../disclosure/placement';
import { countCustomValues, khanStopLimitsSummary, travelLine } from '../disclosure/summaries';
import { useSettingsDisclosure } from '../disclosure/useSettingsDisclosure';
import { AutomationRunStrip } from './AutomationRunStrip';
import { collapsedSettingNote, SettingsSection } from './SettingsSection';
import { englishGuidePack, useGuideLocale } from '../../config/useGuideLocale';
import { useDraftRecovery } from '../useDraftRecovery';

/** What the editor holds right after it loads a saved configuration: used by the load effect and by draft recovery. */
function khanFromSections(sections: Record<string, unknown> | undefined) {
  const draft = parseAutoKhanClientState(sections?.[AUTO_KHAN_SECTION]);
  return {
    draft,
    attackRef: attackSetupRef(draft.attackPresetId, parseAttackPresetDocument(sections?.[ATTACK_PRESETS_SECTION]), AUTO_KHAN_SECTION, 'attack'),
    defenseRef: defenseSetupRef(draft.defensePresetId, parseDefensePresetDocument(sections?.[DEFENSE_PRESETS_SECTION]), AUTO_KHAN_SECTION, 'defense'),
  };
}

interface AutoKhanSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenAutomationDuration?: (featureKey: string, featureLabel: string) => void;
}

const khanDefaults = defaultAutoKhanClientState();

export const AutoKhanSettingsModal: React.FC<AutoKhanSettingsModalProps> = ({ isOpen, onClose, onOpenAutomationDuration }) => {
  const disclosure = useSettingsDisclosure('autoKhan');
  const { t: localizeStatic } = useStaticLocale();
  const setup = useSetupContext(AUTO_KHAN_SECTION, useHostedRuntimePresence());
  const state = setup.state;
  const { troops, tools, unitsLoading, unitsError } = useMetadata();
  const draftSession = useConfigurationDraftSession({
    isOpen,
    section: AUTO_KHAN_SECTION,
    configurationDependencies: [ATTACK_PRESETS_SECTION, DEFENSE_PRESETS_SECTION, COMMANDER_FEATURE_SECTION],
    sessionKey: setup.sessionKey,
  });
  const [draft, setDraft] = useState<AutoKhanClientStateV1>(defaultAutoKhanClientState);
  const [attackRef, setAttackRef] = useState<AttackSetupRef>({ source: 'none' });
  const [defenseRef, setDefenseRef] = useState<DefenseSetupRef>({ source: 'none' });
  const [commandersOpen, setCommandersOpen] = useState(false);
  const [saving, setSaving] = useState(false);
  const [isGuideOpen, setIsGuideOpen] = useState(false);
  const { locale: guideLocale, pack: guidePack } = useGuideLocale();
  const khanGuidePack = guidePack.autoKhan ? guidePack : englishGuidePack;
  const khanGuideLocale = khanGuidePack === englishGuidePack ? 'en' : guideLocale;
  useEffect(() => { if (!isOpen) setIsGuideOpen(false); }, [isOpen]);
  const castles = useMemo(() => castleOptionsFromState(state).filter((castle) => castle.kingdomId === 0), [state]);
  const mainCastle = useMemo(() => greatEmpireMainCastle(state) ?? undefined, [state]);
  const attackDocument = useMemo(
    () => parseAttackPresetDocument(draftSession.sections?.[ATTACK_PRESETS_SECTION]),
    [draftSession.sections],
  );
  const defenseDocument = useMemo(
    () => parseDefensePresetDocument(draftSession.sections?.[DEFENSE_PRESETS_SECTION]),
    [draftSession.sections],
  );
  const attackReferences = useMemo(() => attackPresetReferences(draftSession.sections), [draftSession.sections]);
  const defenseReferences = useMemo(() => defensePresetReferences(draftSession.sections), [draftSession.sections]);
  const commanderAssignments = useMemo(() => savedCommanderAssignments(draftSession.sections), [draftSession.sections]);
  const metadataReady = !unitsLoading && !unitsError;
  const observation = setup.observation;
  const selectedSource = castles.find((castle) => castle.id === draft.sourceCastleId);
  const sourceCastle = useMemo(() => {
    const castle = state?.castles?.[String(draft.sourceCastleId)];
    return castle && castle.kingdomId === 0 ? castle : null;
  }, [draft.sourceCastleId, state?.castles]);
  const recipePending = useMemo(() => pendingStarterReviews(), []);
  const recommendation = useMemo(
    () => recommendEventAttackSetup({ sourceCastle, observation, troops, tools, metadataReady, eventId: 0 }),
    [metadataReady, observation, sourceCastle, tools, troops],
  );
  const defenseStarter = useMemo(() => khanDefenseStarter({ mainCastle: mainCastle ?? null, observation }), [mainCastle, observation]);
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
      ? localizeStatic('khanRageBooster.none')
      : 'Waiting for the first authoritative boi booster snapshot';

  useEffect(() => {
    if (!isOpen || !draftSession.initialSnapshot) return;
    const initial = khanFromSections(draftSession.initialSections);
    setDraft(initial.draft);
    setAttackRef(initial.attackRef);
    setDefenseRef(initial.defenseRef);
  }, [draftSession.initialSections, draftSession.openKey, draftSession.initialSnapshot, isOpen]);

  const canSave = draft.sourceCastleId > 0
    && Boolean(mainCastle)
    && attackSetupRefUsable(attackRef, attackDocument)
    && defenseSetupRefUsable(defenseRef, defenseDocument)
    && draft.skipCooldowns
    && (!sourceIsMain || !draft.openGateProtection || draft.offensiveUnitThreshold > 0);
  const readiness = useMemo(() => evaluateKhanReadiness({
    draft,
    attack: attackRef,
    defense: defenseRef,
    state,
    attackDocument,
    defenseDocument,
    troops,
    tools,
    metadataReady,
    observation,
    commanders: evaluateCommanderEligibility({
      featureId: 'autoKhan',
      state,
      assignments: commanderAssignments,
      movement: setup.movement,
      gameLoggedIn: setup.gameLoggedIn,
      now: Date.now(),
    }),
  }), [attackDocument, attackRef, commanderAssignments, defenseDocument, defenseRef, draft, metadataReady, observation, setup.gameLoggedIn, setup.movement, state, tools, troops]);
  const fixReadiness = (check: ReadinessCheck) => {
    if (check.id === 'commanders' || check.id === 'commander-assignment') {
      setCommandersOpen(true);
      window.requestAnimationFrame(() => focusReadinessTarget('auto-khan-commanders-heading'));
      return;
    }
    disclosure.fix(check);
  };
  const khanCollapsedValues = {
    'auto-khan-rage': localizeStatic(draft.requireActiveRageBooster ? 'ui.settings.components.autoKhanSettingsModal.required.4850b174' : 'ui.settings.components.autoKhanSettingsModal.not.required.5fe2851c'),
  };
  const moduleLabel = localizeStatic('attackPresets.module.autoKhan');

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
    const warnings: AppCreatedPresetSaveWarning[] = [];
    try {
      // Ordered save: attack presets, defense presets, then Auto Khan (CIT-16).
      await saveModuleWithAppCreatedPresets({
        draftSession,
        section: AUTO_KHAN_SECTION,
        slots: [
          { slot: 'attack', ref: attackRef, moduleLabel, slotLabel: localizeStatic('attackPresets.slot.attack') },
          { document: DEFENSE_PRESETS_SECTION, slot: 'defense', ref: defenseRef, moduleLabel, slotLabel: localizeStatic('attackPresets.slot.defense') },
        ],
        buildSectionValue: (ids) => ({
          ...draft,
          attackPresetId: ids.attack,
          defensePresetId: ids.defense,
          openGateProtection: sourceIsMain && draft.openGateProtection,
        }),
        formatPresetName: (module, slot) => localizeStatic('attackPresets.appCreatedName', { module, slot }),
        warnings,
      });
      Notifications.success('Auto Khan settings saved.');
      if (warnings.includes('cleanup-pending')) Notifications.warning(localizeStatic('attackPresets.cleanupPending'));
      onClose();
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not save Auto Khan settings.');
      if (warnings.includes('cleanup-pending')) Notifications.warning(localizeStatic('attackPresets.rollbackPending'));
    } finally {
      setSaving(false);
    }
  };

  const loadedKhan = khanFromSections(draftSession.sections);
  const recovery = useDraftRecovery({ section: AUTO_KHAN_SECTION, isOpen, draftSession, draft: draft, loaded: loadedKhan.draft, extras: { attackRef, defenseRef }, loadedExtras: { attackRef: loadedKhan.attackRef, defenseRef: loadedKhan.defenseRef } });
  useEffect(() => {
    const extras = draftSession.recoveredExtras?.value as { attackRef?: AttackSetupRef; defenseRef?: DefenseSetupRef } | undefined;
    if (!extras) return;
    if (extras.attackRef) setAttackRef(extras.attackRef);
    if (extras.defenseRef) setDefenseRef(extras.defenseRef);
  }, [draftSession.recoveredExtras]);

  return (
    <>
    <SettingsModal
      footerLeading={<StopFooter featureId="autoKhan" />}
      isOpen={isOpen}
      onClose={() => { if (!saving) onClose(); }}
      maxWidth="3xl"
      titleTrailing={<Button variant="outline" size="sm" className="shrink-0" onClick={() => setIsGuideOpen(true)} leftIcon={<BookOpen className="h-4 w-4" />}><span lang={khanGuideLocale}>{khanGuidePack.ui.guideButton}</span></Button>}
      title={localizeStatic("ui.settings.components.autoKhanSettingsModal.title.auto.khan.24bcea17")}
      icon={<Crosshair className="h-5 w-5" />}
      description={localizeStatic("ui.settings.components.autoKhanSettingsModal.description.chained.camp.attacks.khan.taunts.and.main.339ad3f1")}
      onSave={() => void save()}
      isSaving={saving}
      saveDisabled={!canSave || !draftSession.ready}
      contentDisabled={!draftSession.ready}
      contentNotice={<>{recovery.banner}{draftSession.conflictNotice}</>}
    >
      <AutomationRunStrip
        featureId="autoKhan"
        onOpenDuration={onOpenAutomationDuration ? () => onOpenAutomationDuration(AUTOMATION_ENABLED_KEYS.autoKhan, 'Auto Khan') : undefined}
      />
      <div className="space-y-3">
        {protection?.active ? (
          <div className="rounded-global border border-warning/30 bg-warning/10 p-4">
            <div className="flex items-center gap-2 text-body font-semibold text-warning"><LockKeyhole className="h-4 w-4" /> <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.auto.khan.is.safety.locked.e454bd5b" /></div>
            <p className="mt-1 text-caption text-text-main" {...messageLanguageAttributes(protectionReason)}>{protectionReason.text}</p>
            <div className="mt-2 flex flex-wrap gap-2">
              <Badge variant="warning">{(protection.offensiveWallUnits ?? 0).toLocaleString()} offensive wall units</Badge>
              <Badge variant="outline">Threshold {(protection.offensiveUnitThreshold ?? 0).toLocaleString()}</Badge>
            </div>
          </div>
        ) : null}

        <SettingsSection disclosure={disclosure} section="setup" className="space-y-3">
        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <CastleRequirementField
              id="auto-khan-source"
              label={<LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.attack.from.2ef61f36" />}
              value={draft.sourceCastleId}
              onChange={(sourceCastleId) => setDraft((current) => ({
                ...current,
                sourceCastleId,
                openGateProtection: mainCastle != null && sourceCastleId === mainCastle.id,
              }))}
              state={state}
              purpose="source-great-empire"
              options={castles.map((castle) => ({
                value: String(castle.id),
                label: `${castle.name}${castle.id === mainCastle?.id ? ' · Main' : ' · Outpost'} · ${castle.x}:${castle.y}`,
              }))}
              placeholder={localizeStatic("ui.settings.components.autoKhanSettingsModal.placeholder.choose.a.great.empire.castle.8a81fec1")}
            />

            <div>
              <span className="mb-1.5 flex items-center gap-2 text-caption font-semibold text-text-muted"><ShieldCheck className="h-3.5 w-3.5" /> <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.defend.at.a5f2a10a" /></span>
              <div className="flex min-h-[42px] items-center rounded-global border border-border-base bg-bg-input/70 px-4 text-body text-text-main">
                {mainCastle ? `${mainCastle.name?.trim() || `Castle ${mainCastle.id}`} · Main · ${mainCastle.x}:${mainCastle.y}` : 'Great Empire main castle not found'}
              </div>
            </div>
          </div>
          <p className="mt-3 border-t border-border-base pt-3 text-caption text-text-muted">
            <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.auto.station.has.precedence.any.incoming.player.19ee005a" /></p>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <EventAttackSetupField
              id="auto-khan-attack"
              label={<LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.camp.attack.preset.7f63bfed" />}
              section={AUTO_KHAN_SECTION}
              slot="attack"
              moduleLabel={moduleLabel}
              slotLabel={localizeStatic('attackPresets.slot.attack')}
              value={attackRef}
              onChange={setAttackRef}
              document={attackDocument}
              references={attackReferences}
              sourceCastle={sourceCastle}
              observation={observation}
              eventId={0}
              recommendation={recommendation}
              recipePending={recipePending}
              onSaveAsPreset={(inline, name) => saveInlineSetupAsUserPreset(draftSession, inline, name)}
              readinessChecks={readiness.checks.filter((check) => check.slot === 'attack')}
              disabled={saving}
            />
            <DefenseSetupField
              id="auto-khan-defense"
              label={<LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.main.defense.preset.75e96539" />}
              section={AUTO_KHAN_SECTION}
              slot="defense"
              moduleLabel={moduleLabel}
              slotLabel={localizeStatic('attackPresets.slot.defense')}
              value={defenseRef}
              onChange={setDefenseRef}
              document={defenseDocument}
              references={defenseReferences}
              starter={defenseStarter}
              starterCastleName={mainCastle ? (mainCastle.name?.trim() || `#${mainCastle.id}`) : undefined}
              onSaveAsPreset={(inline, name) => saveInlineDefenseAsUserPreset(draftSession, inline, name)}
              readinessChecks={readiness.checks.filter((check) => check.slot === 'defense')}
              disabled={saving}
            />
          </div>
          <p className="mt-3 border-t border-border-base pt-3 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.the.selected.defense.preset.is.re.applied.e0cc99f5" /></p>
        </Card>

        </SettingsSection>

        <SettingsSection disclosure={disclosure} section="policy" className="space-y-3">
        <Card variant="solid" className="p-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div className="flex items-start justify-between gap-4">
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-body font-semibold text-text-main"><LockKeyhole className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.lock.automatic.khan.attacks.2b670e17" /></div>
                <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.stops.only.auto.khan.s.own.attack.284cf413" /></p>
              </div>
              <Switch
                checked={!draft.attackLaunchesEnabled}
                onChange={(locked) => setDraft((current) => ({ ...current, attackLaunchesEnabled: !locked }))}
                ariaLabel={localizeStatic("ui.settings.components.autoKhanSettingsModal.ariaLabel.lock.automatic.khan.attacks.2b670e17")}
              />
            </div>

            <div className="flex items-start justify-between gap-4 border-t border-border-base pt-4 md:border-l md:border-t-0 md:pl-4 md:pt-0">
              <div className="min-w-0">
                <div className="flex items-center gap-2 text-body font-semibold text-text-main"><Flame className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.trigger.khan.at.full.rage.6e8b370e" /></div>
                <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.turn.this.off.to.keep.attacking.and.e1a20f3a" /></p>
              </div>
              <Switch
                checked={draft.triggerRage}
                onChange={(triggerRage) => setDraft((current) => ({ ...current, triggerRage }))}
                ariaLabel={localizeStatic("ui.settings.components.autoKhanSettingsModal.ariaLabel.trigger.khan.retaliation.at.full.rage.a2d62469")}
              />
            </div>
          </div>
        </Card>

        <Card id="auto-khan-purchase-policy" variant="solid" className="p-4">
            <div>
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-body font-semibold text-text-main"><ShoppingCart className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.replenish.defense.tools.04cc1c22" /></div>
                  <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.every.30.seconds.replace.preset.shortages.from.b001d2e4" /></p>
                </div>
                <Switch
                  checked={draft.replenishDefenseTools}
                  onChange={(replenishDefenseTools) => setDraft((current) => ({ ...current, replenishDefenseTools }))}
                  ariaLabel={localizeStatic("ui.settings.components.autoKhanSettingsModal.ariaLabel.replenish.auto.khan.defense.tools.2db26fd2")}
                />
              </div>
              {draft.replenishDefenseTools ? (
                <div className="mt-3 rounded-global border border-success/30 bg-success/10 p-3 text-caption text-text-main">
                  <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.ruby.priced.packages.are.rejected.auto.khan.8c84936b" /></div>
              ) : null}
            </div>
        </Card>

        <Card variant="solid" className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-body font-semibold text-text-main"><ShieldAlert className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.protect.offense.on.the.main.castle.wall.e3913e51" /></div>
              <p className="mt-1 text-caption text-text-muted">
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
                <span className="mb-1.5 block text-caption font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.offensive.wall.unit.threshold.c94cc3f9" /></span>
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
              <div className="mt-3 rounded-global border border-warning/30 bg-warning/10 p-3 text-caption text-text-main">
                <LocalizedText messageKey="copy.khanPause" /></div>
            </div>
          ) : null}
        </Card>

        <Card id="auto-khan-nomad-points" variant="solid" className="p-4">
          <div>
            <div className="flex items-center gap-2 text-body font-semibold text-text-main"><LockKeyhole className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.nomad.points.stop.727b7bc2" /></div>
            <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.at.the.limit.auto.khan.stops.launching.8f630fc2" /></p>
            <label className="mt-3 block max-w-xs">
              <span className="mb-1.5 block text-caption font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.stop.at.nomad.points.0.disables.81362bed" /></span>
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
              <p className="mt-2 text-caption text-warning"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.reaching.this.limit.uses.the.game.s.10d3a9bb" /></p>
            ) : null}
          </div>
        </Card>

        </SettingsSection>

        <SettingsSection disclosure={disclosure} section="skips">
        <div id="auto-khan-skips" tabIndex={-1} className="outline-none">
        <Card variant="solid" className="p-4">
          <div className="flex items-start justify-between gap-4">
            <div className="min-w-0">
              <div className="flex items-center gap-2 text-body font-semibold text-text-main"><RotateCcw className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.skip.every.khan.camp.cooldown.c1c19442" /></div>
              <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.each.launched.hit.reserves.enough.combined.skip.10c1e007" /></p>
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
                <span className="mb-1.5 block text-caption font-semibold text-text-muted">{label}</span>
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
              <span className="mb-1.5 flex items-center gap-2 text-caption font-semibold text-text-muted"><Clock3 className="h-3.5 w-3.5" /> <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.stop.before.event.ends.96ca2172" /></span>
              <Input
                type="number"
                min={0}
                max={1440}
                value={Math.round(draft.minimumRemainingSec / 60)}
                onChange={(event) => setDraft((current) => ({
                  ...current,
                  minimumRemainingSec: clampAutoKhanInteger(event.target.value, 0, 1440, 5) * 60,
                }))}
                rightIcon={<span className="text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.min.1f6fa6f6" /></span>}
                className="font-mono"
              />
            </label>
          </div>
          {!draft.skipCooldowns ? <p className="mt-3 text-caption text-warning"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.cooldown.skipping.is.required.before.these.chained.0dfd850e" /></p> : null}
        </Card>
        </div>

        </SettingsSection>

        <SettingsSection disclosure={disclosure} section="limits">
        <div id="auto-khan-daily-limit" tabIndex={-1} className="outline-none">
          <DailyAttackLimitField
            value={draft.dailyAttackLimit}
            onChange={(dailyAttackLimit) => setDraft((current) => ({ ...current, dailyAttackLimit }))}
            serverState={state?.dailyAttacks}
          />
        </div>

        </SettingsSection>

        <SettingsSection
          disclosure={disclosure}
          section="stop-limits"
          summary={khanStopLimitsSummary(draft)}
          customCount={countCustomValues(draft, khanDefaults, ['maxRageChain', 'requireActiveRageBooster'])}
        >
        <div className="space-y-4">
          <div className="grid gap-4 md:grid-cols-2">
            <div>
              <div className="flex items-center gap-2 text-body font-semibold text-text-main"><Flame className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.rage.chain.limit.907ed6bc" /></div>
              <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.once.this.many.accepted.khan.retaliations.are.fa686d33" /></p>
              <label className="mt-3 block max-w-xs">
                <span className="mb-1.5 block text-caption font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.max.rage.chain.0.disables.limit.a652d0a7" /></span>
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

            <div id="auto-khan-rage" tabIndex={-1} className="border-t border-border-base pt-4 outline-none md:border-l md:border-t-0 md:pl-4 md:pt-0">
              <div className="flex items-start justify-between gap-4">
                <div className="min-w-0">
                  <div className="flex items-center gap-2 text-body font-semibold text-text-main"><Zap className="h-4 w-4 text-primary" /> <LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.require.rage.points.booster.ad17ec97" /></div>
                  <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.gate.new.automatic.camp.attacks.unless.the.a6408062" /></p>
                  <p className={`mt-1 text-caption font-semibold ${rageBoosterActive ? 'text-success' : 'text-text-muted'}`}>{rageBoosterStatus}</p>
                </div>
                <Switch
                  checked={draft.requireActiveRageBooster}
                  onChange={(requireActiveRageBooster) => setDraft((current) => ({ ...current, requireActiveRageBooster }))}
                  ariaLabel={localizeStatic("ui.settings.components.autoKhanSettingsModal.ariaLabel.require.an.active.khan.rage.points.booster.2bf5fdd6")}
                />
              </div>
              <p className="mt-3 rounded-global border border-border-base bg-bg-input/50 p-3 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoKhanSettingsModal.this.is.the.timed.rage.points.booster.4049d4e5" /></p>
            </div>
          </div>
        </div>
        </SettingsSection>

        <SettingsSection
          disclosure={disclosure}
          section="travel"
          summary={[travelLine(draft.horseTravelBoostId)]}
          customCount={countCustomValues(draft, khanDefaults, ['horseTravelBoostId'])}
        >
          <HorseTravelBoostSelect
            className="block"
            value={draft.horseTravelBoostId}
            onChange={(horseTravelBoostId) => setDraft((current) => ({ ...current, horseTravelBoostId }))}
          />
        </SettingsSection>

        <ReadinessPanel
          report={readiness}
          slotLabelKeys={{ attack: 'attackPresets.slot.attack', defense: 'attackPresets.slot.defense' }}
          onFix={fixReadiness}
          noteFor={collapsedSettingNote(disclosure, khanCollapsedValues)}
        />
        <CommanderAssignmentPanel
          id="auto-khan-commanders"
          featureId="autoKhan"
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
    <FeatureGuideModal feature="autoKhan" isOpen={isOpen && isGuideOpen} onClose={() => setIsGuideOpen(false)} />
    </>
  );
};

function formatBoosterRemaining(milliseconds: number): string {
  const totalMinutes = Math.max(1, Math.ceil(milliseconds / 60_000));
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (hours > 0) return `${hours}h${minutes > 0 ? ` ${minutes}m` : ''}`;
  return `${minutes}m`;
}
