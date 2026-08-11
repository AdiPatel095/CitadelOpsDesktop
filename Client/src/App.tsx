import React, { lazy, Suspense, useEffect, useRef, useState, type ComponentType } from 'react';
import { Providers } from './Providers';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import { Alerts } from './components/Alerts';
import { LoggerDock } from './components/LoggerDock';
import UpdateModal from './components/UpdateModal';
import { useAutoEquipmentCleanup } from './settings/AutoEquipmentCleanup';
import { type ViewId } from './config/Navigation';

type StaticViewId = Exclude<ViewId, 'automation'>;
type SettingsModalId =
  | 'recruit'
  | 'tool'
  | 'sceat'
  | 'food'
  | 'tci'
  | 'bird'
  | 'station'
  | 'hospital'
  | 'tower'
  | 'invasion'
  | 'nomad'
  | 'advisor'
  | 'buyer'
  | 'khan'
  | 'beri'
  | 'storm';

interface SettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule?: (id: string, label: string) => void;
}

function lazyNamed<P>(loader: () => Promise<unknown>, exportName: string) {
  return lazy(async () => {
    const loaded = await loader() as Record<string, ComponentType<P>>;
    return { default: loaded[exportName] };
  });
}

const viewComponents: Record<StaticViewId, React.LazyExoticComponent<ComponentType>> = {
  castle: lazy(() => import('./dashboard/components/CastleView')),
  events: lazy(() => import('./views/EventsView')),
  'attack-presets': lazy(() => import('./views/AttackPresetsView')),
  'defense-presets': lazy(() => import('./views/DefensePresetsView')),
  equipment: lazy(() => import('./equipment/components/EquipmentView')),
  movement: lazy(() => import('./Movement/components/MovementView')),
  'battle-stats': lazy(() => import('./battleStats/components/BattleStatsView')),
  'player-tracker': lazy(() => import('./playerTracker/components/PlayerTrackerView')),
  'alliance-targets': lazy(() => import('./allianceTargets/components/AllianceTargetsView')),
	'world-intelligence': lazy(() => import('./worldIntelligence/components/WorldIntelligenceView')),
  rift: lazy(() => import('./Rift/components/RiftView')),
  support: lazy(() => import('./views/SupportPage')),
  settings: lazy(() => import('./views/SettingsView')),
  'patch-notes': lazy(() => import('./views/PatchNotesView')),
};

const AutomationView = lazy(() => import('./views/AutomationView'));
const FeatureScheduleModal = lazyNamed<{
  isOpen: boolean;
  featureID: string | null;
  featureLabel: string;
  onClose: () => void;
}>(() => import('./settings/components/FeatureScheduleModal'), 'FeatureScheduleModal');
const AutomationDurationModal = lazyNamed<{
  isOpen: boolean;
  featureKey: string;
  featureLabel: string;
  onClose: () => void;
}>(() => import('./settings/components/AutomationDurationModal'), 'AutomationDurationModal');
const lazySettingsModal = (loader: () => Promise<unknown>, exportName: string) => (
  lazyNamed<SettingsModalProps>(loader, exportName)
);

const settingsModals: Record<SettingsModalId, React.LazyExoticComponent<ComponentType<SettingsModalProps>>> = {
  recruit: lazySettingsModal(() => import('./settings/components/RecruitTroopsSettingsModal'), 'RecruitTroopsSettingsModal'),
  tool: lazySettingsModal(() => import('./settings/components/AutoToolSettingsModal'), 'AutoToolSettingsModal'),
  sceat: lazySettingsModal(() => import('./settings/components/AutoSceatResSettingsModal'), 'AutoSceatResSettingsModal'),
  food: lazySettingsModal(() => import('./settings/components/AutoFoodBalanceSettingsModal'), 'AutoFoodBalanceSettingsModal'),
  tci: lazySettingsModal(() => import('./settings/components/AutoTCISettingsModal'), 'AutoTCISettingsModal'),
  bird: lazySettingsModal(() => import('./settings/components/AutoBirdSettingsModal'), 'AutoBirdSettingsModal'),
  station: lazySettingsModal(() => import('./settings/components/AutoStationSettingsModal'), 'AutoStationSettingsModal'),
  hospital: lazySettingsModal(() => import('./settings/components/AutoHospitalSettingsModal'), 'AutoHospitalSettingsModal'),
  tower: lazySettingsModal(() => import('./settings/components/AutoTowerSettingsModal'), 'AutoTowerSettingsModal'),
  invasion: lazySettingsModal(() => import('./settings/components/AutoInvasionSettingsModal'), 'AutoInvasionSettingsModal'),
  nomad: lazySettingsModal(() => import('./settings/components/AutoNomadSettingsModal'), 'AutoNomadSettingsModal'),
  advisor: lazySettingsModal(() => import('./settings/components/AutoAdvisorSettingsModal'), 'AutoAdvisorSettingsModal'),
  buyer: lazySettingsModal(() => import('./settings/components/AutoBuyerSettingsModal'), 'AutoBuyerSettingsModal'),
  khan: lazySettingsModal(() => import('./settings/components/AutoKhanSettingsModal'), 'AutoKhanSettingsModal'),
  beri: lazySettingsModal(() => import('./settings/components/AutoBeriWorldSettingsModal'), 'AutoBeriWorldSettingsModal'),
  storm: lazySettingsModal(() => import('./settings/components/AutoStormSettingsModal'), 'AutoStormSettingsModal'),
};

const WorkspaceFallback = () => (
  <div className="ui-workspace-loading" role="status">
    <span className="ui-workspace-loading-mark" aria-hidden="true" />
    Loading workspace…
  </div>
);

const AppContent: React.FC = () => {
  const [activeView, setActiveView] = useState<ViewId>('castle');
  const [activeSettingsModal, setActiveSettingsModal] = useState<SettingsModalId | null>(null);
  const [scheduleTarget, setScheduleTarget] = useState<{ id: string; label: string } | null>(null);
  const [durationTarget, setDurationTarget] = useState<{ key: string; label: string } | null>(null);
  const mainRef = useRef<HTMLElement>(null);
  const autoEquipmentCleanup = useAutoEquipmentCleanup();
  const openSettings = (id: SettingsModalId) => () => setActiveSettingsModal(id);
  const openSchedule = (id: string, label: string) => setScheduleTarget({ id, label });
  const openDuration = (key: string, label: string) => setDurationTarget({ key, label });
  const SettingsModal = activeSettingsModal ? settingsModals[activeSettingsModal] : null;

  useEffect(() => {
    if (mainRef.current) mainRef.current.scrollTop = 0;
  }, [activeView]);

  const workspace = activeView === 'automation' ? (
    <AutomationView
      onOpenAutoTCISettings={openSettings('tci')}
      onOpenAutoSceatResSettings={openSettings('sceat')}
      onOpenAutoFoodBalanceSettings={openSettings('food')}
      onOpenRecruitTroopsSettings={openSettings('recruit')}
      onOpenAutoToolSettings={openSettings('tool')}
      onOpenAutoHospitalSettings={openSettings('hospital')}
      onOpenAutoTowerSettings={openSettings('tower')}
      onOpenAutoInvasionSettings={openSettings('invasion')}
      onOpenAutoNomadSettings={openSettings('nomad')}
      onOpenAutoAdvisorSettings={openSettings('advisor')}
      onOpenAutoBuyerSettings={openSettings('buyer')}
      onOpenAutoKhanSettings={openSettings('khan')}
      onOpenAutoBeriWorldSettings={openSettings('beri')}
      onOpenAutoStormSettings={openSettings('storm')}
      autoEquipmentCleanup={autoEquipmentCleanup}
      onOpenFeatureSchedule={openSchedule}
      onOpenAutomationDuration={openDuration}
    />
  ) : React.createElement(viewComponents[activeView]);

  return (
    <div className="liquid-app flex flex-col text-text-main font-sans transition-colors duration-300">
      <div className="m3-expressive-backdrop" aria-hidden="true">
        <span className="m3-expressive-shape m3-expressive-shape-one" />
        <span className="m3-expressive-shape m3-expressive-shape-two" />
        <span className="m3-expressive-shape m3-expressive-shape-three" />
      </div>
      <Header
        onOpenAutoBirdSettings={openSettings('bird')}
        onOpenAutoStationSettings={openSettings('station')}
        onOpenAutomationDuration={openDuration}
      />
      <Sidebar currentView={activeView} onViewChange={setActiveView} />

      <main ref={mainRef} className="liquid-main custom-scrollbar">
        <div className="liquid-content animate-fade-in">
          <Suspense fallback={<WorkspaceFallback />}>{workspace}</Suspense>
        </div>
      </main>

      <Alerts />
      {SettingsModal && (
        <Suspense fallback={null}>
          <SettingsModal
            isOpen
            onClose={() => setActiveSettingsModal(null)}
            onOpenFeatureSchedule={openSchedule}
          />
        </Suspense>
      )}
      {scheduleTarget && (
        <Suspense fallback={null}>
          <FeatureScheduleModal
            isOpen
            featureID={scheduleTarget.id}
            featureLabel={scheduleTarget.label}
            onClose={() => setScheduleTarget(null)}
          />
        </Suspense>
      )}
      {durationTarget && (
        <Suspense fallback={null}>
          <AutomationDurationModal
            isOpen
            featureKey={durationTarget.key}
            featureLabel={durationTarget.label}
            onClose={() => setDurationTarget(null)}
          />
        </Suspense>
      )}

      <LoggerDock />
      <UpdateModal />
    </div>
  );
};

function App() {
  return <Providers><AppContent /></Providers>;
}

export default App;
