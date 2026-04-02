import React, { useState } from 'react';
import { useAuth } from './context/AuthContext.tsx';
import { useCastleFocus } from './context/CastleFocusContext';
import { Providers } from './Providers';

import EquipmentView from './equipment/components/EquipmentView';
import SupportPage from './views/SupportPage';
import CastleView from './dashboard/components/CastleView';
import CurrencyView from './currency/components/CurrencyView';
import EventModulesView from './event-modules/components/EventModulesView';
import Header from './components/Header';
import CastleFocusSwitcher from './components/CastleFocusSwitcher';
import Sidebar from './components/Sidebar';
import RegistrationPending from './components/RegistrationPending';
import InsufficientCreditsModal from './components/InsufficientCreditsModal';
import UpdateModal from './components/UpdateModal';
import LoginCredentialsModal from './components/LoginCredentialsModal';
import { Alerts } from './components/Alerts';
import { AutoBirdSettingsModal } from './settings/components/AutoBirdSettingsModal';
import { RecruitTroopsSettingsModal } from './settings/components/RecruitTroopsSettingsModal';
import SettingsView from './views/SettingsView';
import { type ViewId } from './config/navigation';

const AppContent: React.FC = () => {
  const {
    isAuthenticated,
    isLoading,
    hardwareID,
    versionUpdate,
    isVersionBannerDismissed,
    dismissVersionBanner,
    startGame,
    storedUsername,
    storedServer,
    gameLoggedIn,
  } = useAuth();
  const { castleFocus } = useCastleFocus();

  const showCastleFocusBar =
    gameLoggedIn && Array.isArray(castleFocus?.playerCastles) && castleFocus.playerCastles.length > 0;
  const [activeView, setActiveView] = useState<ViewId>('equipment');

  // Modal states
  const [isAutoBirdSettingsOpen, setIsAutoBirdSettingsOpen] = useState(false);
  const [isRecruitTroopsSettingsOpen, setIsRecruitTroopsSettingsOpen] = useState(false);
  const [isLoginModalOpen, setIsLoginModalOpen] = useState(false);

  const handleSaveCredentials = (username: string, password: string, server: string) => {
    setIsLoginModalOpen(false);
    startGame({ username, password, server });
  };

  // Show loading state while waiting for registration status
  if (isLoading) {
    return (
      <div className="min-h-screen bg-bg-app flex items-center justify-center">
        <div className="text-text-muted">Connecting to server...</div>
      </div>
    );
  }

  // Show registration pending if not authenticated
  if (!isAuthenticated) {
    return <RegistrationPending hardwareID={hardwareID} />;
  }

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
      case 'support':
        return <SupportPage />;
      case 'settings':
        return <SettingsView />;
      default:
        return <EquipmentView />;
    }
  };

  return (
    <div className="min-h-screen bg-bg-app text-text-main font-sans selection:bg-primary/30 flex flex-col transition-colors duration-300">
      <Header onOpenAutoBirdSettings={() => setIsAutoBirdSettingsOpen(true)} />

      {showCastleFocusBar && (
        <div
          className="fixed top-16 left-64 right-0 z-[45] flex items-center justify-end gap-3 border-b border-border-base bg-bg-app/95 px-6 py-2 backdrop-blur-md transition-colors duration-300"
          role="region"
          aria-label="Castle focus"
        >
          <span className="hidden text-[10px] font-bold uppercase tracking-wider text-text-muted sm:inline">
            Focus castle
          </span>
          <CastleFocusSwitcher />
        </div>
      )}

      <Sidebar
        currentView={activeView}
        onViewChange={setActiveView}
        onOpenRecruitTroopsSettings={() => setIsRecruitTroopsSettingsOpen(true)}
      />

      <main
        className={`relative ml-64 h-screen overflow-y-auto transition-all duration-300 ${showCastleFocusBar ? 'pt-[6.75rem]' : 'pt-16'}`}
      >
        <div className="p-6 max-w-[1600px] mx-auto animate-fade-in relative z-10">
          {renderView()}
        </div>
      </main>

      <InsufficientCreditsModal />
      <Alerts />

      {/* Settings Modals */}
      <AutoBirdSettingsModal
        isOpen={isAutoBirdSettingsOpen}
        onClose={() => setIsAutoBirdSettingsOpen(false)}
      />

      <RecruitTroopsSettingsModal
        isOpen={isRecruitTroopsSettingsOpen}
        onClose={() => setIsRecruitTroopsSettingsOpen(false)}
      />

      {/* Version Update Modal - displayed when new version available */}
      {versionUpdate && !isVersionBannerDismissed && (
        <UpdateModal
          newVersion={versionUpdate.newVersion}
          downloadUrl={versionUpdate.downloadUrl}
          onDismiss={dismissVersionBanner}
        />
      )}

      {/* Login Credentials Modal */}
      <LoginCredentialsModal
        isOpen={isLoginModalOpen}
        onClose={() => setIsLoginModalOpen(false)}
        onSave={handleSaveCredentials}
        initialUsername={storedUsername || ''}
        initialServer={storedServer || 'United States'}
      />
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
