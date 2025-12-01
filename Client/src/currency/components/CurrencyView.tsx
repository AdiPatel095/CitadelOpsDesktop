import React from 'react';
import './CurrencyView.css';
import { useResources } from '../context/ResourceContext.tsx';

// Import all available icons
import SceatIcon from '../../../assets/Sceat.png';
import DucatIcon from '../../../assets/ImperialDucat.png';
import RelicShardIcon from '../../../assets/Relic_Shards.png';
import ConstTokenIcon from '../../../assets/ConstructionToken.png';
import UpgrTokenIcon from '../../../assets/UpgradeToken.png';
import AfflTixIcon from '../../../assets/AffluenceTickets.png';
import PlasterIcon from '../../../assets/Plaster.png';
import DrgScaleIcon from '../../../assets/DragonScaleTiles.png';
import DrgSplIcon from '../../../assets/DragonScaleSplinters.png';
import MightPtIcon from '../../../assets/MightPoints.png';
import GloryPtIcon from '../../../assets/Glory.png';

// Generic Speedup Icon
const SpeedupIcon = () => (
  <svg width="48" height="48" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
    <circle cx="12" cy="12" r="10" />
    <polyline points="12 6 12 12 16 14" />
  </svg>
);

const iconMap: { [key: string]: string | React.FC } = {
  relic_shard: RelicShardIcon,
  sceat: SceatIcon,
  ducat: DucatIcon,
  const_token: ConstTokenIcon,
  upgr_token: UpgrTokenIcon,
  affl_tix: AfflTixIcon,
  plaster: PlasterIcon,
  drg_scale: DrgScaleIcon,
  drg_spl: DrgSplIcon,
  might_pt: MightPtIcon,
  glory_pt: GloryPtIcon,
  gallan_pt: MightPtIcon, // Using Might as placeholder
  min1: SpeedupIcon, min5: SpeedupIcon, min10: SpeedupIcon, min30: SpeedupIcon, 
  hr1: SpeedupIcon, hr5: SpeedupIcon, hr24: SpeedupIcon,
};

const formatResourceName = (key: string): string => {
  return key
    .replace(/_/g, ' ')
    .replace('tix', 'Tickets')
    .replace('pt', 'Points')
    .replace('drg', 'Dragon')
    .replace('min', ' Min')
    .replace('hr', ' Hr')
    .replace(/\b\w/g, char => char.toUpperCase());
};

const categories = {
  Building: ['relic_shard', 'sceat', 'ducat', 'const_token', 'upgr_token', 'plaster'],
  Combat: ['affl_tix', 'drg_scale', 'drg_spl', 'might_pt', 'glory_pt', 'gallan_pt'],
  Speedups: ['min1', 'min5', 'min10', 'min30', 'hr1', 'hr5', 'hr24'],
};

const CurrencyView: React.FC = () => {
  const { resources: globalResources } = useResources();

  if (!globalResources) {
    return <div>Loading resources...</div>;
  }

  return (
    <div className="currency-view">
      {Object.entries(categories).map(([categoryName, resourceKeys]) => (
        <div key={categoryName} className="category-section">
          <h2 className="category-title">{categoryName}</h2>
          <div className="currency-grid">
            {resourceKeys.map(key => {
              const value = globalResources[key as keyof typeof globalResources];
              const IconComponent = iconMap[key];
              if (value === undefined || value === null) return null;

              return (
                <div key={key} className="currency-item">
                  <span className="currency-name">{formatResourceName(key)}</span>
                  {typeof IconComponent === 'string'
                    ? <img src={IconComponent} alt={key} className="currency-icon" />
                    : <IconComponent />}
                  <span className="currency-value">{Number(value).toLocaleString()}</span>
                </div>
              );
            })}
          </div>
        </div>
      ))}
    </div>
  );
};

export default CurrencyView;
