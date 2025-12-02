import React, { useState, useEffect } from 'react';
import { CastleResourceProvider } from './dashboard/context/CastleResourceContext.tsx';
import { EquipmentProvider } from './equipment/context/EquipmentContext.tsx';


import EquipmentView from './equipment/components/EquipmentView';
import Header from './components/Header';
import Sidebar from './components/Sidebar';

import './App.css';
import './Curtain.css';
import { ResourceProvider } from "./currency/context/ResourceContext.tsx";
import { FrontendWebsocket } from './websocket';

export type View = 'Equipment'; // Removed Troops, Schedule, Currency, and Dashboard

const AppContent: React.FC = () => {
  const [activeView, setActiveView] = useState<View>('Equipment');

  useEffect(() => {
    FrontendWebsocket.connect('ws://localhost:8080/ws');
  }, []);


  return (
    <div className="app-layout">
      <Header />
      <Sidebar
        activeView={activeView}
        setActiveView={setActiveView}
      />
      <main className="main-content">


        {activeView === 'Equipment' && <EquipmentView />}
      </main>

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
