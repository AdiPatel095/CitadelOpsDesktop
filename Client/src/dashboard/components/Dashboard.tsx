import React from 'react';
import { useCastleResources } from '../context/CastleResourceContext.tsx';
import CastleResourceCard from './CastleResourceCard.tsx';


const Dashboard: React.FC = () => {
  const { castleResources } = useCastleResources();
  const castlesList = Array.from(castleResources.entries());

  if (castleResources.size === 0) {
    return <div>Loading castle data...</div>;
  }

  return (
    <div className="dashboard">
      <div className="dashboard-grid">
        {castlesList.map(([castleId, castle]) => (
          <CastleResourceCard
            key={castleId}
            castleName={castle.castleName}
            resources={castle.amount}
            storage={castle.storage}
            production={castle.production}
          />
        ))}
      </div>
    </div>
  );
};

export default Dashboard;
