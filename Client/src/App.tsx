import React, { useState, useEffect } from 'react';
import { CastleResourceProvider } from './dashboard/context/CastleResourceContext.tsx';
import { EquipmentProvider } from './equipment/context/EquipmentContext.tsx';
import Dashboard from './dashboard/components/Dashboard.tsx';
import CurrencyView from './currency/components/CurrencyView.tsx';
import EquipmentView from './equipment/components/EquipmentView';
import Header from './components/Header';
import Sidebar from './components/Sidebar';
import LogsPanel from './components/LogsPanel';
import EventCountersPanel from './components/EventCountersPanel';
import './App.css';
import './Curtain.css';
import {ResourceProvider} from "./currency/context/ResourceContext.tsx";
import { FrontendWebsocket } from './websocket';

export type View = 'Dashboard' | 'Currency' | 'Equipment'; // Removed Troops and Schedule

const AppContent: React.FC = () => {
  const [isSidebarCollapsed, setSidebarCollapsed] = useState(false);
  const [activeView, setActiveView] = useState<View>('Dashboard');

  useEffect(() => {
    FrontendWebsocket.connect('ws://localhost:8080/ws');
  }, []);

  const toggleSidebar = () => {
    setSidebarCollapsed(!isSidebarCollapsed);
  };


  return (
    <div className={`app-layout ${isSidebarCollapsed ? 'sidebar-collapsed' : ''}`}>
      <Header />
      <Sidebar 
        isCollapsed={isSidebarCollapsed} 
        activeView={activeView} 
        setActiveView={setActiveView} 
        toggleSidebar={toggleSidebar} 
      />
      <main className="main-content">
        {activeView === 'Dashboard' && <Dashboard />}
        {activeView === 'Currency' && <CurrencyView />}
        {activeView === 'Equipment' && <EquipmentView />}
      </main>
      <LogsPanel />
      <EventCountersPanel />
    </div>
  );
};

function App() {
  return (
      <CastleResourceProvider>
          <ResourceProvider>
              <EquipmentProvider>
                  <AppContent />
              </EquipmentProvider>
          </ResourceProvider>
      </CastleResourceProvider>
  );
}

export default App;
