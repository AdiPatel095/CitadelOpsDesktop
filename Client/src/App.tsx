import React, { useState } from 'react';
import { CastleResourceProvider } from './dashboard/context/CastleResourceContext.tsx';
import { EquipmentProvider } from './equipment/context/EquipmentContext.tsx';
import { AuthProvider, useAuth } from './context/AuthContext.tsx';

import EquipmentView from './equipment/components/EquipmentView';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import RegistrationPending from './components/RegistrationPending';

import { ResourceProvider } from "./currency/context/ResourceContext.tsx";
import { type ViewId } from './config/navigation';

const AppContent: React.FC = () => {
  const { isAuthenticated, isLoading, hardwareID } = useAuth();
  const [activeView, setActiveView] = useState<ViewId>('equipment');

  // Show loading state while waiting for registration status
  if (isLoading) {
    return (
      <div className="min-h-screen bg-dark-bg flex items-center justify-center">
        <div className="text-gray-400">Connecting to server...</div>
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
            <p className="text-gray-400">Settings view coming soon.</p>
          </div>
        );
      case 'support':
        return (
          <div className="glass-panel p-6">
            <h2 className="heading-1 mb-4">Support</h2>
            <p className="text-gray-400">Support view coming soon.</p>
          </div>
        );
      default:
        return <EquipmentView />;
    }
  };

  return (
    <div className="min-h-screen bg-dark-bg text-gray-100 font-sans selection:bg-primary/30">
      <Header />
      <Sidebar currentView={activeView} onViewChange={setActiveView} />

      <main className="ml-64 pt-16 min-h-screen transition-all duration-300">
        <div className="p-6 max-w-[1600px] mx-auto animate-fade-in">
          {renderView()}
        </div>
      </main>
    </div>
  );
};

function App() {
  return (
    <AuthProvider>
      <CastleResourceProvider>
        <ResourceProvider>
          <EquipmentProvider>
            <AppContent />
          </EquipmentProvider>
        </ResourceProvider>
      </CastleResourceProvider>
    </AuthProvider>
  );
}

export default App;

