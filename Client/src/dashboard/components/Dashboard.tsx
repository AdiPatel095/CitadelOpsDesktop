import React from 'react';
import { useCastleResources } from '../context/CastleResourceContext.tsx';
import CastleResourceCard from './CastleResourceCard.tsx';
import './Dashboard.css';

const Dashboard: React.FC = () => {
  const { castles } = useCastleResources();

  if (!castles) {
    return <div>Loading castle data...</div>;
  }

  const castleKeys = [
    'main_castle', 'outpost_1', 'outpost_2', 'outpost_3', 
    'ice_castle', 'desert_castle', 'dungeon_castle', 'storm_castle'
  ];

  const castleList = castleKeys.map(key => {
    const name = castles[`${key}_name` as keyof typeof castles];
    const resources = castles[`${key}_amount` as keyof typeof castles];
    const storage = castles[`${key}_storage` as keyof typeof castles];
    const production = castles[`${key}_production` as keyof typeof castles];

    if (name && resources && storage && production) {
      return { name, resources, storage, production };
    }
    return null;
  }).filter(Boolean);

  return (
    <div className="dashboard">
      <div className="dashboard-grid">
        {castleList.map(castle => (
          castle && (
            <CastleResourceCard
              key={castle.name}
              castleName={castle.name}
              resources={castle.resources}
              storage={castle.storage}
              production={castle.production}
            />
          )
        ))}
      </div>
    </div>
  );
};

export default Dashboard;
