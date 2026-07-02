import React from 'react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { useResources } from '../context/ResourceContext.tsx';
import { Card, CardHeader, CardTitle, CardContent } from '../../components/ui';

const SceatIcon = '/game-data/resources/images/Sceat.webp';
const DucatIcon = '/game-data/resources/images/ImperialDucat.webp';
const RelicShardIcon = '/game-data/resources/images/Relic_Shards.webp';
const ConstTokenIcon = '/game-data/resources/images/ConstructionToken.webp';
const UpgrTokenIcon = '/game-data/resources/images/UpgradeToken.webp';
const AfflTixIcon = '/game-data/resources/images/AffluenceTickets.webp';
const PlasterIcon = '/game-data/resources/images/Plaster.webp';
const DrgScaleIcon = '/game-data/resources/images/DragonScaleTiles.webp';
const DrgSplIcon = '/game-data/resources/images/DragonScaleSplinters.webp';
const MightPtIcon = '/game-data/resources/images/MightPoints.webp';
const GloryPtIcon = '/game-data/resources/images/Glory.webp';

// Generic Speedup Icon
const SpeedupIcon = () => (
  <svg className="w-12 h-12 text-primary drop-shadow-md" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.5">
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
    .replace('min', 'Min ')
    .replace('hr', 'Hr ')
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
    return (
      <div className="flex flex-col gap-6 h-full items-center justify-center">
        <StaleSessionBanner />
        <div className="text-primary animate-pulse">Loading resources...</div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-8 pb-8">
      <StaleSessionBanner />
      {Object.entries(categories).map(([categoryName, resourceKeys]) => (
        <Card key={categoryName} className="liquid-prominent-header-card flex flex-col">
          <CardHeader className="liquid-card-header-prominent">
            <CardTitle className="text-xl text-primary">{categoryName}</CardTitle>
          </CardHeader>
          <CardContent className="liquid-prominent-header-content p-6">
            <div className="currency-responsive-grid">
              {resourceKeys.map(key => {
                const value = globalResources[key as keyof typeof globalResources];
                const IconComponent = iconMap[key];
                if (value === undefined || value === null) return null;

                return (
                  <div key={key} className="flex flex-col items-center justify-center p-4 gap-3 bg-bg-card border border-border-base rounded-global shadow-sm hover:border-primary/50 hover:bg-bg-card-hover transition-colors">
                    <span className="text-xs font-bold text-text-muted uppercase tracking-wider text-center h-8 flex items-center justify-center">
                      {formatResourceName(key)}
                    </span>
                    <div className="h-14 flex items-center justify-center">
                      {typeof IconComponent === 'string'
                        ? <img src={IconComponent} alt={key} className="w-12 h-12 object-contain drop-shadow-md" />
                        : <IconComponent />}
                    </div>
                    <span className="text-lg font-bold text-text-main font-mono mt-1">
                      {Number(value).toLocaleString()}
                    </span>
                  </div>
                );
              })}
            </div>
          </CardContent>
        </Card>
      ))}
    </div>
  );
};

export default CurrencyView;
