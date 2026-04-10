import React from 'react';
import { Card, CardHeader, CardTitle, CardContent } from '../../components/ui';

// Asset paths from public directory
const WoodIcon = '/assets/Resources/Wood.png';
const StoneIcon = '/assets/Resources/Stone.png';
const FoodIcon = '/assets/Resources/Food.png';
const CharcoalIcon = '/assets/Resources/Charcoal.png';
const OliveOilIcon = '/assets/Resources/OliveOil.png';
const GlassIcon = '/assets/Resources/Glass.png';
const IronOreIcon = '/assets/Resources/Iron_Ore.png';
const HoneyIcon = '/assets/Resources/Honey.png';
const MeadIcon = '/assets/Resources/Mead.png';
const BeefIcon = '/assets/Resources/Beef.png';

import {
  type CastleResourcesAmount,
  type CastleStorageMax,
  type CastleProductionTotal
} from '../models/PlayerCastleInfo';

interface CastleResourceCardProps {
  title: string;
  resources: CastleResourcesAmount;
  storage: CastleStorageMax;
  production: CastleProductionTotal;
}

const resourceIconMap: { [key: string]: string } = {
  wood: WoodIcon,
  stone: StoneIcon,
  food: FoodIcon,
  coal: CharcoalIcon,
  oil: OliveOilIcon,
  glass: GlassIcon,
  iron: IronOreIcon,
  honey: HoneyIcon,
  mead: MeadIcon,
  beef: BeefIcon,
};

const resourceKeys: (keyof CastleResourcesAmount)[] = [
  'wood_amount', 'stone_amount', 'food_amount', 'coal_amount', 'oil_amount',
  'glass_amount', 'iron_amount', 'honey_amount', 'mead_amount', 'beef_amount'
];

const CastleResourceCard: React.FC<CastleResourceCardProps> = ({ title, resources, storage, production }) => {
  return (
    <Card className="flex flex-col min-h-0">
      <CardHeader className="pb-3">
        <CardTitle className="text-primary">{title}</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2 overflow-y-auto custom-scrollbar pt-3">
        {resourceKeys.map(key => {
          const resourceBaseName = key.replace('_amount', '');
          const amount = resources[key] as number;
          const max = storage[`${resourceBaseName}_max` as keyof CastleStorageMax] as number;
          let prod = production[`${resourceBaseName}_prod` as keyof CastleProductionTotal] ?? 0;

          // Deduct consumption for food/mead/beef
          if (resourceBaseName === 'food') {
            prod -= (production.food_consumption ?? 0);
          } else if (resourceBaseName === 'mead') {
            prod -= (production.mead_consumption ?? 0);
          } else if (resourceBaseName === 'beef') {
            prod -= (production.beef_consumption ?? 0);
          }

          const percentage = max > 0 ? (amount / max) * 100 : 0;
          const prodClass = prod < 0 ? "text-error font-semibold" : "text-success font-semibold";
          const prodPrefix = prod > 0 ? "+" : "";

          return (
            <div key={key} className="flex items-center gap-3 p-2.5 rounded-global bg-bg-card-hover border border-border-base transition-colors hover:border-primary/30">
              <img src={resourceIconMap[resourceBaseName]} alt={resourceBaseName} className="w-8 h-8 object-contain drop-shadow-sm shrink-0" />
              <div className="flex-1 flex flex-col gap-1.5 min-w-0">
                <div className="flex justify-between items-center text-xs font-medium text-text-main">
                  <span className="truncate mr-2">{amount.toLocaleString()} / {max.toLocaleString()}</span>
                  <span className={`${prodClass} shrink-0`}>({prodPrefix}{prod.toLocaleString()}/hr)</span>
                </div>
                <div className="w-full h-1.5 bg-bg-app rounded-full overflow-hidden border border-border-base/50">
                  <div className="h-full bg-primary transition-all duration-500 ease-out" style={{ width: `${Math.min(100, Math.max(0, percentage))}%` }}></div>
                </div>
              </div>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );
};

export default CastleResourceCard;
