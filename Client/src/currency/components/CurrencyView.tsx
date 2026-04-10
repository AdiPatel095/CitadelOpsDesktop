import React from 'react';
import { useResources } from '../context/ResourceContext.tsx';
import { Card, CardHeader, CardTitle, CardContent } from '../../components/ui';

// Asset paths from public directory
const SceatIcon = '/assets/Resources/Sceat.png';
const DucatIcon = '/assets/Resources/ImperialDucat.png';
const RelicShardIcon = '/assets/Resources/Relic_Shards.png';
const ConstTokenIcon = '/assets/Resources/ConstructionToken.png';
const UpgrTokenIcon = '/assets/Resources/UpgradeToken.png';
const AfflTixIcon = '/assets/Resources/AffluenceTickets.png';
const PlasterIcon = '/assets/Resources/Plaster.png';
const DrgScaleIcon = '/assets/Resources/DragonScaleTiles.png';
const DrgSplIcon = '/assets/Resources/DragonScaleSplinters.png';
const MightPtIcon = '/assets/Resources/MightPoints.png';
const GloryPtIcon = '/assets/Resources/Glory.png';

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
        <div className="text-primary animate-pulse">Loading resources...</div>
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-8 pb-8">
      {Object.entries(categories).map(([categoryName, resourceKeys]) => (
        <Card key={categoryName} className="flex flex-col border-border-base bg-bg-app/20">
          <CardHeader className="bg-bg-card-hover/50 pb-4 border-b border-border-base rounded-t-[calc(var(--radius-global)-1px)]">
            <CardTitle className="text-xl text-primary">{categoryName}</CardTitle>
          </CardHeader>
          <CardContent className="p-6">
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
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
