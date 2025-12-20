import React, { useState } from 'react';
import { useAuth } from './context/AuthContext.tsx';
import { Providers } from './Providers';

import EquipmentView from './equipment/components/EquipmentView';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import RegistrationPending from './components/RegistrationPending';
import InsufficientCreditsModal from './components/InsufficientCreditsModal';
import UpdateModal from './components/UpdateModal';
import { Alerts } from './components/Alerts';

import { type ViewId } from './config/navigation';

const AppContent: React.FC = () => {
  const { isAuthenticated, isLoading, hardwareID, versionUpdate, isVersionBannerDismissed, dismissVersionBanner } = useAuth();
  const [activeView, setActiveView] = useState<ViewId>('equipment');

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
      case 'settings':
        return (
          <div className="glass-panel p-6">
            <h2 className="heading-1 mb-4">Settings</h2>
            <p className="text-text-muted">Settings view coming soon.</p>
          </div>
        );
      case 'support':
        return (
          <div className="glass-panel p-6">
            <h2 className="heading-1 mb-4">Support</h2>
            <p className="text-text-muted">Support view coming soon.</p>
          </div>
        );
      default:
        return <EquipmentView />;
    }
  };

  return (
    <div className="min-h-screen bg-bg-app text-text-main font-sans selection:bg-primary/30 flex flex-col transition-colors duration-300">
      <Header />

      <Sidebar currentView={activeView} onViewChange={setActiveView} />

      <main className="ml-64 min-h-screen transition-all duration-300 pt-16 relative">
        <div className="p-6 max-w-[1600px] mx-auto animate-fade-in relative z-10">
          {renderView()}
        </div>
      </main>

      <InsufficientCreditsModal />
      <Alerts />

      {/* Version Update Modal - displayed when new version available */}
      {versionUpdate && !isVersionBannerDismissed && (
        <UpdateModal
          newVersion={versionUpdate.newVersion}
          downloadUrl={versionUpdate.downloadUrl}
          onDismiss={dismissVersionBanner}
        />
      )}
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
