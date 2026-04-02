import React from 'react';

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
    /** Card heading (e.g. "Resources"). */
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
        <div className="castle-card">
            <h3 className="castle-name">{title}</h3>

            <div className="resource-list-view">
                {resourceKeys.map(key => {
                    const resourceBaseName = key.replace('_amount', '');
                    const amount = resources[key] as number;
                    const max = storage[`${resourceBaseName}_max` as keyof CastleStorageMax] as number;
                    let prod = production[`${resourceBaseName}_prod` as keyof CastleProductionTotal] ?? 0;

                    // Deduct consumption for food/mead/beef (calculated on backend)
                    if (resourceBaseName === 'food') {
                        prod -= (production.food_consumption ?? 0);
                    } else if (resourceBaseName === 'mead') {
                        prod -= (production.mead_consumption ?? 0);
                    } else if (resourceBaseName === 'beef') {
                        prod -= (production.beef_consumption ?? 0);
                    }

                    const percentage = max > 0 ? (amount / max) * 100 : 0;
                    const prodClass = prod < 0 ? "text-red-500 font-semibold" : "production-rate";
                    const prodPrefix = prod > 0 ? "+" : "";

                    return (
                        <div key={key} className="resource-list-item">
                            <img src={resourceIconMap[resourceBaseName]} alt={resourceBaseName}
                                className="resource-icon" />
                            <div className="resource-info">
                                <div className="resource-text">
                                    <span>{amount.toLocaleString()} / {max.toLocaleString()}</span>
                                    <span className={prodClass}>({prodPrefix}{prod.toLocaleString()}/hr)</span>
                                </div>
                                <div className="progress-bar-container">
                                    <div className="progress-bar" style={{ width: `${percentage}%` }}></div>
                                </div>
                            </div>
                        </div>
                    );
                })}
            </div>
        </div>
    );
};

export default CastleResourceCard;
