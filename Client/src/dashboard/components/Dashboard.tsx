import React from 'react';
import { useCastleResources } from '../context/CastleResourceContext.tsx';
import CastleResourceCard from './CastleResourceCard.tsx';


const Dashboard: React.FC = () => {
  const { castleResources } = useCastleResources();
  const castlesList = Array.from(castleResources.values());

  if (castleResources.size === 0) {
    return <div>Loading castle data...</div>;
  }



  return (
    <div className="dashboard">
      <div className="dashboard-grid">
        {castlesList.map((castle) => (
          <CastleResourceCard
            key={castle.castleName}
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
