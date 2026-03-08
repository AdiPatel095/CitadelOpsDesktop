import React from 'react';
import { useCastleResources } from '../context/CastleResourceContext';
import CastleUnitCard from './CastleUnitCard';

const UnitsDashboard: React.FC = () => {
    const { castleResources } = useCastleResources();

    // Get all castles that have troop data, using the castle name and troops directly
    const castlesWithTroops = Array.from(castleResources.entries())
        .filter(([_, castle]) => castle.troops?.troopsMixed && Object.keys(castle.troops.troopsMixed).length > 0);

    if (castlesWithTroops.length === 0) {
        return <div>Loading unit data...</div>;
    }

    return (
        <div className="dashboard">
            <div className="dashboard-grid">
                {castlesWithTroops.map(([castleId, castle]) => (
                    <CastleUnitCard
                        key={castleId}
                        castleName={castle.castleName}
                        troopsMixed={castle.troops.troopsMixed}
                        troopsI={castle.troops.troopsI}
                        troopsTU={castle.troops.troopsTU}
                    />
                ))}
            </div>
        </div>
    );
};

export default UnitsDashboard;
