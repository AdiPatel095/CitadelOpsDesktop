import React, { useState } from 'react';
import { useAuth } from './context/AuthContext.tsx';
import { Providers } from './Providers';

import EquipmentView from './equipment/components/EquipmentView';
import SupportPage from './views/SupportPage';
import CastleView from './dashboard/components/CastleView';
import CurrencyView from './currency/components/CurrencyView';
import RiftView from './Rift/components/RiftView';
import MovementView from './Movement/components/MovementView';
import BattleStatsView from './battleStats/components/BattleStatsView';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import UpdateModal from './components/UpdateModal';
import { Alerts } from './components/Alerts';
import { RecruitTroopsSettingsModal } from './settings/components/RecruitTroopsSettingsModal';
import { AutoToolSettingsModal } from './settings/components/AutoToolSettingsModal';
import { AutoTCISettingsModal } from './settings/components/AutoTCISettingsModal';
import { AutoBirdSettingsModal } from './settings/components/AutoBirdSettingsModal';
import { FeatureScheduleModal } from './settings/components/FeatureScheduleModal';
import SettingsView from './views/SettingsView';
import PatchNotesView from './views/PatchNotesView';
import { type ViewId } from './config/Navigation';
import { LoggerDock } from './components/LoggerDock';

const AppContent: React.FC = () => {
  const {
    versionUpdate,
    isVersionBannerDismissed,
    dismissVersionBanner,
  } = useAuth();

  const [activeView, setActiveView] = useState<ViewId>('castle');

  // Modal states
  const [isRecruitTroopsSettingsOpen, setIsRecruitTroopsSettingsOpen] = useState(false);
  const [isAutoToolSettingsOpen, setIsAutoToolSettingsOpen] = useState(false);
  const [isAutoTCISettingsOpen, setIsAutoTCISettingsOpen] = useState(false);
  const [isAutoBirdSettingsOpen, setIsAutoBirdSettingsOpen] = useState(false);
  const [scheduleTarget, setScheduleTarget] = useState<{ id: string; label: string } | null>(null);

  const renderView = () => {
    switch (activeView) {
      case 'castle':
        return <CastleView />;
      case 'equipment':
        return <EquipmentView />;
      case 'currency':
        return <CurrencyView />;
      case 'movement':
        return <MovementView />;
      case 'battle-stats':
        return <BattleStatsView />;
      case 'rift':
        return <RiftView />;
      case 'support':
        return <SupportPage />;
      case 'settings':
        return <SettingsView />;
      case 'patch-notes':
        return <PatchNotesView />;
      default:
        return <EquipmentView />;
    }
  };

  return (
    <div className="liquid-app flex flex-col text-text-main font-sans transition-colors duration-300">
      <Header
        onOpenAutoBirdSettings={() => setIsAutoBirdSettingsOpen(true)}
      />

      <Sidebar
        currentView={activeView}
        onViewChange={setActiveView}
        onOpenRecruitTroopsSettings={() => setIsRecruitTroopsSettingsOpen(true)}
        onOpenAutoToolSettings={() => setIsAutoToolSettingsOpen(true)}
        onOpenAutoTCISettings={() => setIsAutoTCISettingsOpen(true)}
      />

      <main className="liquid-main custom-scrollbar">
        <div className="liquid-content animate-fade-in">
          {renderView()}
        </div>
      </main>

      <Alerts />

      <RecruitTroopsSettingsModal
        isOpen={isRecruitTroopsSettingsOpen}
        onClose={() => setIsRecruitTroopsSettingsOpen(false)}
        onOpenFeatureSchedule={(id, label) => setScheduleTarget({ id, label })}
      />

      <AutoToolSettingsModal
        isOpen={isAutoToolSettingsOpen}
        onClose={() => setIsAutoToolSettingsOpen(false)}
        onOpenFeatureSchedule={(id, label) => setScheduleTarget({ id, label })}
      />

      <AutoTCISettingsModal
        isOpen={isAutoTCISettingsOpen}
        onClose={() => setIsAutoTCISettingsOpen(false)}
        onOpenFeatureSchedule={(id, label) => setScheduleTarget({ id, label })}
      />

      <AutoBirdSettingsModal
        isOpen={isAutoBirdSettingsOpen}
        onClose={() => setIsAutoBirdSettingsOpen(false)}
        onOpenFeatureSchedule={(id, label) => setScheduleTarget({ id, label })}
      />

      <FeatureScheduleModal
        isOpen={scheduleTarget != null}
        featureID={scheduleTarget?.id ?? null}
        featureLabel={scheduleTarget?.label ?? ''}
        onClose={() => setScheduleTarget(null)}
      />

      {/* Version Update Modal - displayed when new version available */}
      {versionUpdate && !isVersionBannerDismissed && (
        <UpdateModal
          newVersion={versionUpdate.newVersion}
          downloadUrl={versionUpdate.downloadUrl}
          onDismiss={dismissVersionBanner}
        />
      )}

      <LoggerDock />
    </div>
  );
};

function App() {
  return (
    <Providers>
      <AppContent />
    </Providers>
  );
}

export default App;
