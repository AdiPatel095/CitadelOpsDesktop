import React, { useState } from 'react';
import { useAuth } from './context/AuthContext.tsx';
import { Providers } from './Providers';

import EquipmentView from './equipment/components/EquipmentView';
import SupportPage from './views/SupportPage';
import CastleView from './dashboard/components/CastleView';
import CurrencyView from './currency/components/CurrencyView';
import EventModulesView from './EventModules/components/EventModulesView';
import RiftView from './Rift/components/RiftView';
import MovementView from './Movement/components/MovementView';
import BattleStatsView from './battleStats/components/BattleStatsView';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import UpdateModal from './components/UpdateModal';
import { Alerts } from './components/Alerts';
import { RecruitTroopsSettingsModal } from './settings/components/RecruitTroopsSettingsModal';
import { AutoTCISettingsModal } from './settings/components/AutoTCISettingsModal';
import { AutoBirdSettingsModal } from './settings/components/AutoBirdSettingsModal';
import { AutoBeriWorldSettingsModal } from './settings/components/AutoBeriWorldSettingsModal';
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
  const [isAutoTCISettingsOpen, setIsAutoTCISettingsOpen] = useState(false);
  const [isAutoBirdSettingsOpen, setIsAutoBirdSettingsOpen] = useState(false);
  const [isAutoBeriWorldSettingsOpen, setIsAutoBeriWorldSettingsOpen] = useState(false);

  const renderView = () => {
    switch (activeView) {
      case 'castle':
        return <CastleView />;
      case 'equipment':
        return <EquipmentView />;
      case 'event-modules':
        return <EventModulesView />;
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
    <div className="min-h-screen bg-bg-app text-text-main font-sans selection:bg-primary/30 flex flex-col transition-colors duration-300">
      <Header onOpenAutoBirdSettings={() => setIsAutoBirdSettingsOpen(true)} />

      <Sidebar
        currentView={activeView}
        onViewChange={setActiveView}
        onOpenRecruitTroopsSettings={() => setIsRecruitTroopsSettingsOpen(true)}
        onOpenAutoTCISettings={() => setIsAutoTCISettingsOpen(true)}
        onOpenAutoBeriWorldSettings={() => setIsAutoBeriWorldSettingsOpen(true)}
      />

      <main
        className="relative ml-64 h-screen overflow-y-auto transition-all duration-300 pt-16"
      >
        <div className="p-6 max-w-[1600px] mx-auto animate-fade-in relative z-10">
          {renderView()}
        </div>
      </main>

      <Alerts />

      <RecruitTroopsSettingsModal
        isOpen={isRecruitTroopsSettingsOpen}
        onClose={() => setIsRecruitTroopsSettingsOpen(false)}
      />

      <AutoTCISettingsModal isOpen={isAutoTCISettingsOpen} onClose={() => setIsAutoTCISettingsOpen(false)} />

      <AutoBirdSettingsModal isOpen={isAutoBirdSettingsOpen} onClose={() => setIsAutoBirdSettingsOpen(false)} />

      <AutoBeriWorldSettingsModal isOpen={isAutoBeriWorldSettingsOpen} onClose={() => setIsAutoBeriWorldSettingsOpen(false)} />

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
