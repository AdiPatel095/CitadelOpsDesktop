import { castleSettingsKey, castleSettingsEntry, normalizeStormKeys, stormEditorCastles } from '../stormRole';
import { StormSettingsRepair } from './StormSettingsRepair';
import { StopFooter } from '../../components/StopControl';
import { castleCandidates } from '../copy/candidates';
import { towersCopyDescriptor } from '../copy/features/towers';
import { copyReapplied, genericSaveError, useCastleCopyReplayRun, useCastleCopyReplayState } from '../copy/useCastleCopyReplay';
import { CastleCopyButton } from './CastleCopyDialog';
import { useLocale as useStaticLocale } from '../../i18n/useLocale';
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { BookOpen, Bot, Crosshair, FastForward, TicketCheck } from 'lucide-react';
import UnitImage from '../../components/UnitImage';
import { showTroopPicker } from '../../components/TroopPicker';
import { Button, Card, Input, SettingsModal, SettingsToggleRow, Switch } from '../../components/ui';
import { useCitadelAPI } from '../../api/useCitadelAPI';
import { castleOptionsFromState } from '../../api/Selectors';
import {
	AUTO_TOWER_MAXIMUM_DAILY_TIME_SKIPS,
	clampMapRefreshInterval,
	clampMaximumDailyTimeSkips,
	clampRadius,
  defaultAutoTowerCastleSettings,
  defaultAutoTowerClientState,
  parseAutoTowerClientState,
  type AutoTowerCastleSettings,
} from '../AutoTowerClientState';
import { AutoTowerGuideModal } from './AutoTowerGuideModal';
import { useGuideLocale } from '../../config/useGuideLocale';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import { DailyAttackLimitField } from './DailyAttackLimitField';
import type { HorseTravelBoostID } from '../HorseTravelBoost';
import { useConfigurationDraftSession } from '../ConfigurationDraftSession';
import { useMetadata } from '../../context/useMetadata';
import { COMMANDER_FEATURE_SECTION } from '../../Movement/types/CommanderFeatureAssignments';
import { savedCommanderAssignments } from '../requirements/commanderAssignmentDraft';
import { evaluateCommanderEligibility } from '../requirements/commanderEligibility';
import { evaluateTowerReadiness } from '../requirements/setupReadiness';
import { useSetupContext } from '../requirements/useSetupContext';
import { focusReadinessTarget } from '../readiness/focusReadinessTarget';
import type { ReadinessCheck } from '../readiness/Readiness';
import { CommanderAssignmentPanel } from './CommanderAssignmentPanel';
import { ReadinessCheckLine, ReadinessPanel } from './ReadinessPanel';
import { UnitStockList } from './UnitStockList';
import { AUTOMATION_ENABLED_KEYS } from '../disclosure/placement';
import { countCustomValues, towerAdvisorSummary, towerScanSummary, travelLine } from '../disclosure/summaries';
import { useSettingsDisclosure } from '../disclosure/useSettingsDisclosure';
import { AutomationRunStrip } from './AutomationRunStrip';
import { SettingsSection } from './SettingsSection';
import { collapsedSettingNote } from './collapsedSettingNote';
import { useDraftRecovery } from '../useDraftRecovery';

interface AutoTowerSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
  onOpenAutomationDuration?: (featureKey: string, featureLabel: string) => void;
}

const towerDefaults = defaultAutoTowerClientState();

export const AutoTowerSettingsModal: React.FC<AutoTowerSettingsModalProps> = ({ isOpen, onClose, onOpenFeatureSchedule, onOpenAutomationDuration }) => {
  const { locale: guideLocale, pack: guidePack } = useGuideLocale();
  const { t: localizeStatic } = useStaticLocale();
  const { state } = useCitadelAPI();
  const setup = useSetupContext('automation.autoTowers');
  const copyReplay = useCastleCopyReplayState();
  const draftSession = useConfigurationDraftSession({
    isOpen,
    section: 'automation.autoTowers',
    configurationDependencies: [COMMANDER_FEATURE_SECTION],
    sessionKey: setup.sessionKey,
    copyReplay: copyReplay.sessionOption,
  });
  const { troops, tools, unitsLoading, unitsError } = useMetadata();
  const [commandersOpen, setCommandersOpen] = useState(false);
  const disclosure = useSettingsDisclosure('autoTowers');
  const [settings, setSettings] = useState<Record<string, AutoTowerCastleSettings>>({});
  const castles = stormEditorCastles(castleOptionsFromState(state), Object.hasOwn(settings, 'storm'));
  const roleSettings = normalizeStormKeys(settings, state);
  const [mapRefreshIntervalSec, setMapRefreshIntervalSec] = useState(1800);
  const [dailyAttackLimit, setDailyAttackLimit] = useState(0);
  const [horseTravelBoostId, setHorseTravelBoostId] = useState<HorseTravelBoostID>(-1);
  const [useAdvisor, setUseAdvisor] = useState(false);
  const [autoActivateAdvisor, setAutoActivateAdvisor] = useState(false);
  const [maximumDailyTimeSkips, setMaximumDailyTimeSkips] = useState(0);
  const [isGuideOpen, setIsGuideOpen] = useState(false);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen) {
      setIsGuideOpen(false);
      setSaveError(null);
      return;
    }
    if (!draftSession.initialSnapshot) return;
    const current = parseAutoTowerClientState(
      draftSession.initialSections?.['automation.autoTowers'] ?? defaultAutoTowerClientState(),
    );
    setSettings(current.castles);
    setMapRefreshIntervalSec(current.mapRefreshIntervalSec);
    setDailyAttackLimit(current.dailyAttackLimit);
    setHorseTravelBoostId(current.horseTravelBoostId);
    setUseAdvisor(current.useAdvisor);
    setAutoActivateAdvisor(current.autoActivateAdvisor);
    setMaximumDailyTimeSkips(current.maximumDailyTimeSkips);
  }, [draftSession.initialSections, draftSession.openKey, draftSession.initialSnapshot, isOpen]);

  const commanderAssignments = useMemo(() => savedCommanderAssignments(draftSession.sections), [draftSession.sections]);
  const copyContext = useMemo(() => ({
    state, troops, tools, metadataReady: !unitsLoading && !unitsError, observation: setup.observation,
    candidates: castleCandidates(castles.filter((castle) => castle.id > 0), state, { keyFor: castleSettingsKey }),
  }), [castles, setup.observation, state, tools, troops, unitsError, unitsLoading]);
  const copyRun = useCastleCopyReplayRun(copyReplay, { descriptor: towersCopyDescriptor, draft: roleSettings, context: copyContext, featureLabel: 'Auto Towers', applyDraft: setSettings, isOpen });
  const readiness = useMemo(() => evaluateTowerReadiness({
    state,
    castles: settings,
    troops,
    tools,
    metadataReady: !unitsLoading && !unitsError,
    observation: setup.observation,
    commanders: evaluateCommanderEligibility({
      featureId: 'autoTowers',
      state,
      assignments: commanderAssignments,
      movement: setup.movement,
      gameLoggedIn: setup.gameLoggedIn,
      now: Date.now(),
    }),
  }), [commanderAssignments, settings, setup.gameLoggedIn, setup.movement, state, tools, troops, unitsError, unitsLoading, setup.observation]);
  const fixReadiness = (check: ReadinessCheck) => {
    if (check.id === 'commanders' || check.id === 'commander-assignment') {
      setCommandersOpen(true);
      window.requestAnimationFrame(() => focusReadinessTarget('auto-towers-commanders-heading'));
      return;
    }
    if (!disclosure.fix(check)) focusReadinessTarget('auto-towers-castles');
  };

  const settingsFor = useCallback((castleID: number): AutoTowerCastleSettings => (
    castleSettingsEntry(settings, castles.find((castle) => castle.id === castleID) ?? { id: castleID, kingdomId: 0 }) ?? defaultAutoTowerCastleSettings()
  ), [settings, castles]);

  const updateCastle = (castleID: number, update: Partial<AutoTowerCastleSettings>) => {
    const key = castleSettingsKey(castles.find((castle) => castle.id === castleID) ?? { id: castleID, kingdomId: 0 });
    setSettings((current) => ({ ...normalizeStormKeys(current, state), [key]: { ...(normalizeStormKeys(current, state)[key] ?? defaultAutoTowerCastleSettings()), ...update } }));
  };

  const chooseTroop = async (castleID: number) => {
    const castle = state?.castles[String(castleID)];
    const selected = settingsFor(castleID).unitId;
    const result = await showTroopPicker({
      mode: 'single',
      title: `Tower troop — ${castle?.name?.trim() || `Castle ${castleID}`}`,
      preselected: selected > 0 ? [selected] : [],
      stockQuantities: castle?.units.stationed,
      stockObservation: { castle: castle ?? null, observation: setup.observation },
    });
    if (typeof result === 'number' && result > 0) updateCastle(castleID, { unitId: result });
  };

  const save = async () => {
    if (isSaving) return;
    setIsSaving(true);
    setSaveError(null);
    const current = parseAutoTowerClientState(draftSession.sections?.['automation.autoTowers']);
    try {
      await draftSession.save({
        ...current,
        version: 4,
        mapRefreshIntervalSec,
        dailyAttackLimit,
        horseTravelBoostId,
        useAdvisor,
        autoActivateAdvisor,
        maximumDailyTimeSkips,
        castles: normalizeStormKeys(settings, state),
      });
      onClose();
    } catch (error) {
      setSaveError(genericSaveError(error, copyReplay, 'Could not save Auto Towers settings.'));
    } finally {
      setIsSaving(false);
    }
  };

  const handleClose = () => {
    if (!isSaving) onClose();
  };

  // What the editor holds once its load effect has applied the saved section (every field the draft lists is set from it).
  const loadedTowers = parseAutoTowerClientState(draftSession.sections?.['automation.autoTowers'] ?? defaultAutoTowerClientState());
  const recovery = useDraftRecovery({ section: 'automation.autoTowers', isOpen, draftSession, draft: { ...parseAutoTowerClientState(draftSession.sections?.['automation.autoTowers']), version: 4, mapRefreshIntervalSec, dailyAttackLimit, horseTravelBoostId, useAdvisor, autoActivateAdvisor, maximumDailyTimeSkips, castles: settings }, loaded: { ...loadedTowers, version: 4 }, copyReapplied: copyReapplied(copyReplay) });

  return (
    <>
    <SettingsModal
      footerLeading={<StopFooter featureId="autoTowers" />}
      isOpen={isOpen}
      onClose={handleClose}
      maxWidth="full"
      title={localizeStatic("ui.settings.components.autoTowerSettingsModal.title.auto.towers.247e8c64")}
      icon={<Crosshair className="h-5 w-5" />}
      description={localizeStatic("ui.settings.components.autoTowerSettingsModal.description.each.scan.saves.every.tower.observed.in.00c98e17")}
      titleTrailing={<Button variant="outline" size="sm" onClick={() => setIsGuideOpen(true)} leftIcon={<BookOpen className="h-4 w-4" />}><span lang={guideLocale}>{guidePack.ui.guideButton}</span></Button>}
      onSave={save}
      saveLabel="Save changes"
      isSaving={isSaving}
      saveDisabled={!draftSession.ready}
      contentDisabled={!draftSession.ready}
      contentNotice={<>{copyRun.status}{recovery.banner}{draftSession.conflictNotice}{copyRun.dialog}</>}
    >
      <AutomationRunStrip
        featureId="autoTowers"
        scheduleId="autoTowers"
        onOpenSchedule={() => onOpenFeatureSchedule('autoTowers', 'Auto Towers')}
        onOpenDuration={onOpenAutomationDuration ? () => onOpenAutomationDuration(AUTOMATION_ENABLED_KEYS.autoTowers, 'Auto Towers') : undefined}
      />
      {saveError && (
        <div className="mb-4 rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error" role="alert">
          {saveError}
        </div>
      )}

      <SettingsSection disclosure={disclosure} section="castles" className="mb-4">
        <StormSettingsRepair entries={settings} state={state} onChange={setSettings} />
        <div id="auto-towers-castles" tabIndex={-1} className="grid grid-cols-1 gap-4 outline-none sm:grid-cols-2 xl:grid-cols-3">
          {castles.map((castle) => {
            const plan = settingsFor(castle.id);
            const stock = state?.castles[String(castle.id)]?.units.stationed[String(plan.unitId)] ?? 0;
            const stockResult = plan.enabled ? readiness.stockByCastle[castleSettingsKey(castle)] : undefined;
            return (
              <Card key={castle.id} variant="solid" className="flex flex-col gap-4 bg-bg-card-hover/40 p-4 shadow-inner">
                <div className="flex items-start justify-between gap-3 border-b border-border-base pb-3">
                  <div className="min-w-0">
                    <h3 className="truncate text-sm font-bold text-primary">{castle.kingdomId === 4 ? <LocalizedText messageKey={castle.id === 0 ? "stormRole.idleLabel" : "stormRole.label"} /> : castle.name}</h3>
                    {castle.kingdomId === 4 && <Button variant="ghost" size="sm" onClick={() => setSettings((previous) => { const draft = normalizeStormKeys(previous, state); delete draft.storm; return draft; })}><LocalizedText messageKey="stormRole.remove" /></Button>}
                    <p className="mt-0.5 text-xs text-text-muted">{castle.kingdomId}:{castle.x}:{castle.y}</p>
                  </div>
                  <Switch
                    checked={plan.enabled}
                    onChange={() => updateCastle(castle.id, { enabled: !plan.enabled })}
                    ariaLabel={`Toggle Auto Towers for ${castle.name}`}
                  />
                </div>

                <p className="rounded-xl border border-border-base bg-bg-app/50 px-3 py-2.5 text-[11px] text-text-muted">
                  <LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.no.batch.cap.launch.every.eligible.target.373ba7c9" /></p>

                <button
                  type="button"
                  onClick={() => chooseTroop(castle.id)}
                  className="flex min-h-16 items-center gap-3 rounded-xl border border-dashed border-border-base bg-bg-app/60 p-3 text-left transition-colors hover:border-primary/50 hover:bg-primary/5"
                >
                  {plan.unitId > 0 ? <UnitImage unitId={plan.unitId} size={48} showLevel /> : <Crosshair className="h-7 w-7 text-text-muted" />}
                  <span className="min-w-0">
                    <span className="block text-xs font-bold text-text-main">{plan.unitId > 0 ? `Unit ${plan.unitId}` : 'Choose troop'}</span>
                    <span className="mt-0.5 block text-[11px] text-text-muted">
                      {plan.unitId > 0 ? `${stock.toLocaleString()} stationed` : 'Two full flanks use this unit'}
                    </span>
                  </span>
                </button>

                <label className="flex flex-col gap-1">
                  <span className="text-[10px] font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.radius.6fe0661c" /></span>
                  <Input
                    type="number"
                    min={1}
                    max={50}
                    value={plan.radius}
                    onChange={(event) => updateCastle(castle.id, { radius: clampRadius(event.target.value) })}
                    className="text-center font-mono"
                    rightIcon={<span className="text-[10px] text-text-muted"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.tiles.ad9243fa" /></span>}
                  />
                </label>

                {stockResult ? (
                  <div className="space-y-1.5 rounded-xl border border-border-base bg-bg-app/50 px-3 py-2.5">
                    <UnitStockList lines={stockResult.lines} freshness={stockResult.freshness} />
                    <ul><ReadinessCheckLine check={stockResult.check} /></ul>
                  </div>
                ) : null}

                <p className="text-[11px] text-text-muted">
                  <LocalizedText messageKey="settingsSummary.towerCastleScope" params={{ radius: plan.radius, maiden: plan.maidenOnly ? 'on' : 'off' }} />
                </p>
                <CastleCopyButton
                  descriptor={towersCopyDescriptor}
                  draft={settings}
                  sourceKey={castleSettingsKey(castle)}
                  context={copyContext}
                  featureLabel="Auto Towers"
                  onApply={(next, replay) => { setSettings(next); copyReplay.setReplay(replay); copyReplay.setStatus(false); }}
                  className="self-start"
                />
              </Card>
            );
          })}
        </div>
      </SettingsSection>

      <SettingsSection disclosure={disclosure} section="limits" className="mb-4">
        <DailyAttackLimitField
          zeroLabel="Attack count · 0 removes limit"
          value={dailyAttackLimit}
          onChange={setDailyAttackLimit}
          serverState={state?.dailyAttacks}
          description={localizeStatic("ui.settings.components.autoTowerSettingsModal.description.stop.auto.towers.when.the.server.s.7d443e02")}
        />
      </SettingsSection>

      <div className="mb-4 space-y-3">
        <SettingsSection
          disclosure={disclosure}
          section="advisor"
          summary={towerAdvisorSummary({ useAdvisor, autoActivateAdvisor, maximumDailyTimeSkips })}
          customCount={countCustomValues({ ...towerDefaults, useAdvisor, autoActivateAdvisor, maximumDailyTimeSkips }, towerDefaults, ['useAdvisor', 'autoActivateAdvisor', 'maximumDailyTimeSkips'])}
        >
          <div id="auto-towers-advisor" className="mb-3 flex items-start gap-3">
            <div className="rounded-xl bg-primary/10 p-2 text-primary" aria-hidden="true">
              <Bot className="h-5 w-5" />
            </div>
            <div>
              <h3 className="text-sm font-black text-text-main"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.robber.baron.advisor.04006eb4" /></h3>
              <p className="mt-0.5 text-[11px] leading-relaxed text-text-muted">
                <LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.advisor.mode.runs.native.same.tower.chains.046a6fbd" /></p>
            </div>
          </div>
          <div className="grid gap-2 lg:grid-cols-2">
            <SettingsToggleRow
              title={localizeStatic("ui.settings.components.autoTowerSettingsModal.title.use.advisor.with.time.skips.12bfd1b3")}
              description={localizeStatic("ui.settings.components.autoTowerSettingsModal.description.send.a.native.same.tower.advisor.chain.9b485b0e")}
              icon={<Bot className="h-4 w-4" />}
              checked={useAdvisor}
              onChange={setUseAdvisor}
              ariaLabel={localizeStatic("ui.settings.components.autoTowerSettingsModal.ariaLabel.use.robber.baron.advisor.mode.for.auto.7147e92c")}
            />
            <SettingsToggleRow
              title={localizeStatic("ui.settings.components.autoTowerSettingsModal.title.auto.activate.with.token.82178176")}
              description={localizeStatic("ui.settings.components.autoTowerSettingsModal.description.if.the.advisor.is.inactive.consume.one.69e81071")}
              icon={<TicketCheck className="h-4 w-4" />}
              checked={autoActivateAdvisor}
              onChange={setAutoActivateAdvisor}
              disabled={!useAdvisor}
              disabledReason="Enable Advisor mode first."
              ariaLabel={localizeStatic("ui.settings.components.autoTowerSettingsModal.ariaLabel.auto.activate.the.robber.baron.advisor.with.8bebd830")}
            />
          </div>
          <div className="mt-3 rounded-xl border border-border-base bg-bg-card/60 p-3">
            <div className="flex flex-wrap items-center justify-between gap-3">
              <div className="flex min-w-0 items-start gap-2.5">
                <FastForward className="mt-0.5 h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
                <div>
                  <label htmlFor="auto-tower-daily-time-skips" className="text-xs font-bold text-text-main"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.maximum.daily.time.skips.a3f214d9" /></label>
                  <p className="mt-0.5 text-[11px] leading-relaxed text-text-muted">
                    <LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.confirmed.advisor.chains.count.against.this.cap.0f5b3790" /></p>
                </div>
              </div>
              <div className="flex items-center gap-2">
                <Input
                  id="auto-tower-daily-time-skips"
                  type="number"
                  min={0}
                  max={AUTO_TOWER_MAXIMUM_DAILY_TIME_SKIPS}
                  value={maximumDailyTimeSkips}
                  disabled={!useAdvisor}
                  onChange={(event) => setMaximumDailyTimeSkips(clampMaximumDailyTimeSkips(event.target.value))}
                  className="w-28 text-center font-mono"
                />
                <span className="text-[11px] font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.skips.7932e297" /></span>
              </div>
            </div>
          </div>
        </SettingsSection>

        <SettingsSection
          disclosure={disclosure}
          section="scan"
          summary={towerScanSummary(mapRefreshIntervalSec, castles.map((castle) => settingsFor(castle.id)).filter((plan) => plan.enabled))}
          customCount={(mapRefreshIntervalSec !== towerDefaults.mapRefreshIntervalSec ? 1 : 0)
            + castles.map((castle) => settingsFor(castle.id)).filter((plan) => plan.maidenOnly).length}
        >
          <div className="mb-4 flex flex-wrap items-center justify-between gap-4 rounded-global border border-primary/20 bg-primary/5 p-4">
            <div className="min-w-0">
              <div className="text-sm font-bold text-text-main"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.authoritative.map.scan.dc172025" /></div>
              <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.fast.focus.switch.through.every.enabled.castle.86ab11a7" /></p>
            </div>
            <label className="flex items-center gap-2">
              <span className="text-xs font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.every.9b8617fd" /></span>
              <div className="w-24">
                <Input
                  type="number"
                  min={1800}
                  max={3600}
                  value={mapRefreshIntervalSec}
                  onChange={(event) => setMapRefreshIntervalSec(clampMapRefreshInterval(event.target.value))}
                  className="text-center font-mono"
                />
              </div>
              <span className="text-xs font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.sec.add93534" /></span>
            </label>
          </div>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 xl:grid-cols-3">
            {castles.map((castle) => {
              const plan = settingsFor(castle.id);
              return (
                <div key={castle.id} className="grid gap-3 rounded-xl border border-border-base bg-bg-app/50 p-3">
                  <div className="flex items-center justify-between gap-2">
                    <span className="truncate text-xs font-bold text-primary">{castle.kingdomId === 4 ? <LocalizedText messageKey={castle.id === 0 ? "stormRole.idleLabel" : "stormRole.label"} /> : castle.name}</span>
                    {plan.enabled ? null : <span className="text-[10px] font-semibold uppercase text-text-muted"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.castle.off.539be403" /></span>}
                  </div>
                  <div className="flex items-center justify-between gap-3">
                    <div className="min-w-0">
                      <div className="text-xs font-bold text-text-main"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.maiden.supported.only.1374eb47" /></div>
                      <p className="mt-0.5 text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.autoTowerSettingsModal.only.use.an.available.commander.with.the.d7a1c498" /></p>
                    </div>
                    <Switch
                      checked={plan.maidenOnly}
                      onChange={() => updateCastle(castle.id, { maidenOnly: !plan.maidenOnly })}
                      ariaLabel={`Require maiden-supported commander for ${castle.name}`}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        </SettingsSection>

        <SettingsSection
          disclosure={disclosure}
          section="travel"
          summary={[travelLine(horseTravelBoostId)]}
          customCount={horseTravelBoostId !== towerDefaults.horseTravelBoostId ? 1 : 0}
        >
          <HorseTravelBoostSelect
            value={horseTravelBoostId}
            onChange={setHorseTravelBoostId}
            description={useAdvisor
              ? 'Advisor chains repeat this explicitly selected travel option for each generated hit. Ruby tiers are used only when selected here.'
              : undefined}
          />
        </SettingsSection>
      </div>

      <div className="mt-4 space-y-4">
        <ReadinessPanel report={readiness.report} onFix={fixReadiness} noteFor={collapsedSettingNote(disclosure)} />
        <CommanderAssignmentPanel
          id="auto-towers-commanders"
          featureId="autoTowers"
          draftSession={draftSession}
          state={setup.state}
          movement={setup.movement}
          gameLoggedIn={setup.gameLoggedIn}
          expanded={commandersOpen}
          onExpandedChange={setCommandersOpen}
          disabled={isSaving}
        />
      </div>
    </SettingsModal>
    <AutoTowerGuideModal isOpen={isOpen && isGuideOpen} onClose={() => setIsGuideOpen(false)} />
    </>
  );
};
