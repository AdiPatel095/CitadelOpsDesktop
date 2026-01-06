import React, { useState } from 'react';
import { useAuth } from './context/AuthContext.tsx';
import { Providers } from './Providers';

import EquipmentView from './equipment/components/EquipmentView';
import SupportPage from './views/SupportPage';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import RegistrationPending from './components/RegistrationPending';
import InsufficientCreditsModal from './components/InsufficientCreditsModal';
import UpdateModal from './components/UpdateModal';
import LoginCredentialsModal from './components/LoginCredentialsModal';
import { Alerts } from './components/Alerts';
import { AutoBirdSettingsModal } from './settings/components/AutoBirdSettingsModal';


import { type ViewId } from './config/navigation';

const AppContent: React.FC = () => {
  const { isAuthenticated, isLoading, hardwareID, versionUpdate, isVersionBannerDismissed, dismissVersionBanner, startGame, storedUsername, storedServer } = useAuth();
  const [activeView, setActiveView] = useState<ViewId>('equipment');

  // Modal states
  const [isAutoBirdSettingsOpen, setIsAutoBirdSettingsOpen] = useState(false);
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
      case 'equipment':
        return <EquipmentView />;
      case 'support':
        return <SupportPage />;
      default:
        return <EquipmentView />;
    }
  };

  return (
    <div className="min-h-screen bg-bg-app text-text-main font-sans selection:bg-primary/30 flex flex-col transition-colors duration-300">
      <Header onOpenLoginModal={() => setIsLoginModalOpen(true)} />

      <Sidebar
        currentView={activeView}
        onViewChange={setActiveView}
        onOpenAutoBirdSettings={() => setIsAutoBirdSettingsOpen(true)}
        onOpenLoginModal={() => setIsLoginModalOpen(true)}
      />

      <main className="ml-64 min-h-screen transition-all duration-300 pt-16 relative">
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
