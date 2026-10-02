import { StopFooter } from '../../components/StopControl';
import { castleCandidates } from '../copy/candidates';
import { stationCopyDescriptor } from '../copy/features/station';
import { copyReapplied, genericSaveError, useCastleCopyReplayRun, useCastleCopyReplayState } from '../copy/useCastleCopyReplay';
import { CastleCopyButton } from './CastleCopyDialog';
import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import { BookOpen, Plus, Shield } from 'lucide-react';
import { useGuideLocale } from '../../config/useGuideLocale';
import { FeatureGuideModal } from './FeatureGuideModal';
import { showTroopPicker, type UnitWithQuantity } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import {
  AddSlot,
  Button,
  Card,
  Input,
  QuantityAssetTile,
  SettingsModal,
  SettingsToggleRow,
} from '../../components/ui';
import {
  DEFAULT_AUTO_STATION_STATE,
  parseAutoStationClientState,
  type AutoStationClientStateV1,
} from '../AutoStationClientState';
import { useCitadelAPI } from '../../api/ApiContext';
import { castleOptionsFromState, type CastleOptionV2 } from '../../api/Selectors';
import { useConfigurationDraftSession } from '../ConfigurationDraftSession';
import { useMetadata } from '../../context/MetadataContext';
import { evaluateReserveReadiness } from '../requirements/setupReadiness';
import { useSetupContext } from '../requirements/useSetupContext';
import { focusReadinessTarget } from '../readiness/focusReadinessTarget';
import type { ReadinessCheck } from '../readiness/Readiness';
import { AUTOMATION_ENABLED_KEYS } from '../disclosure/placement';
import { countCustomValues, stationFiltersSummary } from '../disclosure/summaries';
import { useSettingsDisclosure } from '../disclosure/useSettingsDisclosure';
import { AutomationRunStrip } from './AutomationRunStrip';
import { ReadinessCheckLine, ReadinessPanel } from './ReadinessPanel';
import { collapsedSettingNote, SettingsSection } from './SettingsSection';
import { UnitStockList } from './UnitStockList';
import { useDraftRecovery } from '../useDraftRecovery';

interface AutoStationSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenAutomationDuration?: (featureKey: string, featureLabel: string) => void;
}

function clampMinutes(value: number): number {
  if (!Number.isFinite(value)) return 1;
  return Math.min(60, Math.max(1, Math.round(value)));
}

function clampDays(value: number): number {
  if (!Number.isFinite(value)) return 3;
  return Math.min(30, Math.max(0, Math.round(value)));
}

export const AutoStationSettingsModal: React.FC<AutoStationSettingsModalProps> = ({ isOpen, onClose, onOpenAutomationDuration }) => {
  const { locale: guideLocale, pack: guidePack } = useGuideLocale();
  const [isGuideOpen, setIsGuideOpen] = useState(false);
  useEffect(() => { if (!isOpen) setIsGuideOpen(false); }, [isOpen]);
  const { t: localizeStatic } = useStaticLocale();
  const { state: gameState } = useCitadelAPI();
  const setup = useSetupContext('automation.autoStation');
  const copyReplay = useCastleCopyReplayState();
  const draftSession = useConfigurationDraftSession({ isOpen, section: 'automation.autoStation', sessionKey: setup.sessionKey, copyReplay: copyReplay.sessionOption });
  const disclosure = useSettingsDisclosure('autoStation');
  const { troops, tools, unitsLoading, unitsError } = useMetadata();
  const castles = castleOptionsFromState(gameState);
  const [state, setState] = useState<AutoStationClientStateV1>(() => parseAutoStationClientState(null));
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  useEffect(() => {
    if (!isOpen) {
      setSaveError(null);
      return;
    }
    if (!draftSession.initialSnapshot) return;
    setState(parseAutoStationClientState(
      draftSession.initialSections?.['automation.autoStation'],
    ));
  }, [draftSession.initialSections, draftSession.openKey, draftSession.initialSnapshot, isOpen]);

  const copyContext = useMemo(() => ({
    state: gameState, troops, tools, metadataReady: !unitsLoading && !unitsError, observation: setup.observation,
    candidates: castleCandidates(castles, gameState),
  }), [castles, gameState, setup.observation, tools, troops, unitsError, unitsLoading]);
  const copyRun = useCastleCopyReplayRun(copyReplay, { descriptor: stationCopyDescriptor, draft: state.settings, context: copyContext, featureLabel: 'Auto Station', applyDraft: (next) => setState((previous) => ({ ...previous, settings: next })), isOpen });
  const readiness = useMemo(() => evaluateReserveReadiness({
    featureId: 'autoStation',
    state: gameState,
    reserves: state.settings,
    troops,
    tools,
    metadataReady: !unitsLoading && !unitsError,
    observation: setup.observation,
  }), [gameState, state.settings, tools, troops, unitsError, unitsLoading, setup.observation]);

  const fixReadiness = (check: ReadinessCheck) => {
    if (!disclosure.fix(check)) focusReadinessTarget('auto-station-castles');
  };

  const selectReserve = async (castle: CastleOptionV2) => {
    const castleID = String(castle.id);
    const current = state.settings[castleID] ?? [];
    const preselectedQuantities: Record<number, number> = {};
    current.forEach((troop) => {
      preselectedQuantities[troop.id] = troop.amount;
    });
    const result = await showTroopPicker({
      mode: 'multi',
      title: `Troops left to defend — ${castle.name}`,
      allowQuantity: true,
      preselected: current.map((troop) => troop.id),
      preselectedQuantities,
    });
    if (!Array.isArray(result)) return;
    const troops = (result as UnitWithQuantity[]).map((troop) => ({
      id: troop.unitId,
      amount: troop.quantity,
    }));
    setState((previous) => ({
      ...previous,
      settings: { ...previous.settings, [castleID]: troops },
    }));
  };

  const removeReserve = (castleID: string, unitID: number) => {
    setState((previous) => ({
      ...previous,
      settings: {
        ...previous.settings,
        [castleID]: (previous.settings[castleID] ?? []).filter((troop) => troop.id !== unitID),
      },
    }));
  };

  const save = async () => {
    if (isSaving) return;
    setIsSaving(true);
    setSaveError(null);
    try {
      await draftSession.save(parseAutoStationClientState(state));
      onClose();
    } catch (error) {
      setSaveError(genericSaveError(error, copyReplay, 'Could not save Auto Station settings.'));
    } finally {
      setIsSaving(false);
    }
  };

  const handleClose = () => {
    if (!isSaving) onClose();
  };

  const recovery = useDraftRecovery({ section: 'automation.autoStation', isOpen, draftSession, draft: parseAutoStationClientState(state), loaded: parseAutoStationClientState(parseAutoStationClientState(draftSession.sections?.['automation.autoStation'])), copyReapplied: copyReapplied(copyReplay) });

  return (
    <>
    <SettingsModal
      footerLeading={<StopFooter featureId="autoStation" />}
      isOpen={isOpen}
      onClose={handleClose}
      maxWidth="full"
      title={localizeStatic("ui.settings.components.autoStationSettingsModal.title.auto.station.settings.eb56c8a6")}
      icon={<Shield className="h-5 w-5" />}
      description={localizeStatic("ui.settings.components.autoStationSettingsModal.description.choose.the.exact.troops.that.stay.behind.a5d0c68a")}
      titleTrailing={<Button variant="secondary" size="sm" onClick={() => setIsGuideOpen(true)} leftIcon={<BookOpen className="h-4 w-4" />}><span lang={guideLocale}>{guidePack.ui.guideButton}</span></Button>}
      onSave={save}
      saveLabel="Save changes"
      isSaving={isSaving}
      saveDisabled={!draftSession.ready}
      contentDisabled={!draftSession.ready}
      contentNotice={<>{copyRun.status}{recovery.banner}{draftSession.conflictNotice}{copyRun.dialog}</>}
    >
      {saveError && (
        <div className="mb-4 rounded-global border border-error/30 bg-error/10 px-4 py-3 text-body font-semibold text-error" role="alert">
          {saveError}
        </div>
      )}
      <AutomationRunStrip
        featureId="autoStation"
        onOpenDuration={onOpenAutomationDuration ? () => onOpenAutomationDuration(AUTOMATION_ENABLED_KEYS.autoStation, 'Auto Station') : undefined}
      />
      <div className="flex w-full flex-col gap-6">
        <SettingsSection disclosure={disclosure} section="evacuation">
          <Card variant="solid" className="">
            <div className="grid gap-4 md:grid-cols-2">
              <label id="auto-station-lead-time" className="flex flex-col gap-1.5">
                <span className="text-caption font-semibold text-primary"><LocalizedText messageKey="ui.settings.components.autoStationSettingsModal.evacuate.at.621aebd7" /></span>
                <Input
                  type="number"
                  min={1}
                  max={60}
                  value={Math.round(state.leadTimeSec / 60)}
                  onChange={(event) => setState((previous) => ({
                    ...previous,
                    leadTimeSec: clampMinutes(Number(event.target.value)) * 60,
                  }))}
                  className="font-mono"
                  rightIcon={<span className="text-caption font-medium text-text-muted"><LocalizedText messageKey="ui.settings.components.autoStationSettingsModal.minutes.left.4703188b" /></span>}
                />
                <span className="text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoStationSettingsModal.troops.leave.this.many.minutes.before.the.3ef5b581" /></span>
              </label>
              <SettingsToggleRow
                title={localizeStatic("ui.settings.components.autoStationSettingsModal.title.recall.when.clear.553ed7f0")}
                description={localizeStatic("ui.settings.components.autoStationSettingsModal.bring.evacuated.troops.home.once.no.attack.b3b45068")}
                checked={state.recallWhenClear}
                onChange={(checked) => setState((previous) => ({ ...previous, recallWhenClear: checked }))}
              />
              <SettingsToggleRow
                title={localizeStatic("ui.settings.components.autoStationSettingsModal.title.open.gate.fallback.739eb349")}
                description={localizeStatic("ui.settings.components.autoStationSettingsModal.when.troops.cannot.be.evacuated.in.time.3bd33592")}
                tone={state.openGateFallback ? 'warning' : 'default'}
                checked={state.openGateFallback}
                onChange={(checked) => setState((previous) => ({ ...previous, openGateFallback: checked }))}
              />
            </div>
            <p className="mt-4 text-caption text-text-muted">
              <span lang={guideLocale} dir={guideLocale === 'ar' ? 'rtl' : 'ltr'}>{guidePack.autoStation.feature.helper}</span>
            </p>
          </Card>
        </SettingsSection>

        <SettingsSection
          disclosure={disclosure}
          section="filters"
          summary={stationFiltersSummary(state)}
          customCount={countCustomValues(state, DEFAULT_AUTO_STATION_STATE, ['minRPTDays'])}
        >
          <div className="grid gap-4 md:grid-cols-2">
            <label className="flex flex-col gap-1.5">
              <span className="text-caption font-semibold text-primary"><LocalizedText messageKey="ui.settings.components.autoStationSettingsModal.minimum.bird.days.on.target.71cbcd1f" /></span>
              <Input
                type="number"
                min={0}
                max={30}
                value={state.minRPTDays}
                onChange={(event) => setState((previous) => ({
                  ...previous,
                  minRPTDays: clampDays(Number(event.target.value)),
                }))}
                className="font-mono"
                rightIcon={<span className="text-caption font-medium text-text-muted"><LocalizedText messageKey="ui.settings.components.autoStationSettingsModal.days.e08c0aa8" /></span>}
              />
              <span className="text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoStationSettingsModal.troops.are.sent.only.to.alliance.members.faff0c89" /></span>
            </label>
          </div>
        </SettingsSection>

        <ReadinessPanel report={readiness.report} onFix={fixReadiness} noteFor={collapsedSettingNote(disclosure)} />

        <SettingsSection disclosure={disclosure} section="reserves">
        <div id="auto-station-castles" tabIndex={-1} className="custom-scrollbar min-h-0 flex-1 overflow-y-auto pr-1 outline-none">
          {castles.length === 0 && (
            <p className="py-8 text-center text-body text-text-muted"><LocalizedText messageKey="ui.settings.components.autoStationSettingsModal.loading.castles.37f1e3a3" /></p>
          )}
          <div className="grid grid-cols-1 gap-4 pb-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {castles.map((castle) => {
              const castleID = String(castle.id);
              const reserves = state.settings[castleID] ?? [];
              const stock = readiness.stockByCastle[castleID];
              return (
                <Card key={castle.id} variant="solid" className="flex flex-col">
                  <div className="mb-3 border-b border-border-base pb-2">
                    <h3 className="text-title-sm font-bold text-primary">{castle.name || `${castle.type} castle`}</h3>
                    <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.settings.components.autoStationSettingsModal.these.amounts.remain.in.the.castle.e33daec5" /></p>
                  </div>
                  {reserves.length === 0 ? (
                    <div className="flex flex-1 flex-col items-center justify-center gap-3 py-6">
                      <p className="text-center text-caption font-mediumr text-text-muted"><LocalizedText messageKey="ui.settings.components.autoStationSettingsModal.no.defense.reserve.b4ce10ce" /></p>
                      <Button variant="secondary" size="sm" onClick={() => selectReserve(castle)} leftIcon={<Plus className="h-4 w-4" />}>
                        <LocalizedText messageKey="ui.settings.components.autoStationSettingsModal.add.troops.5264f439" /></Button>
                    </div>
                  ) : (
                    <div className="flex flex-wrap justify-center gap-4">
                      {reserves.map((troop) => (
                        <QuantityAssetTile
                          key={troop.id}
                          visual={<UnitImage unitId={troop.id} size={76} showLevel className="rounded-xl" />}
                          quantity={troop.amount}
                          onRemove={() => removeReserve(castleID, troop.id)}
                          removeLabel="Remove defense troop"
                        />
                      ))}
                      <AddSlot
                        label={localizeStatic("ui.settings.components.autoStationSettingsModal.label.edit.defense.troops.f3c64913")}
                        layout="icon"
                        onClick={() => selectReserve(castle)}
                        className="h-[76px] w-[76px]"
                        icon={<Plus className="h-8 w-8" strokeWidth={1.5} />}
                      />
                    </div>
                  )}
                  <CastleCopyButton
                    descriptor={stationCopyDescriptor}
                    draft={state.settings}
                    sourceKey={castleID}
                    context={copyContext}
                    featureLabel="Auto Station"
                    onApply={(next, replay) => { setState((previous) => ({ ...previous, settings: next })); copyReplay.setReplay(replay); copyReplay.setStatus(false); }}
                    className="mt-3 self-start"
                  />
                  {stock ? (
                    <div className="mt-3 space-y-1.5 border-t border-border-base pt-2">
                      <UnitStockList lines={stock.lines} mode="reserve" freshness={stock.freshness} />
                      <ul><ReadinessCheckLine check={stock.check} /></ul>
                    </div>
                  ) : null}
                </Card>
              );
            })}
          </div>
        </div>
        </SettingsSection>
      </div>
    </SettingsModal>
    <FeatureGuideModal feature="autoStation" isOpen={isOpen && isGuideOpen} onClose={() => setIsGuideOpen(false)} />
    </>
  );
};
