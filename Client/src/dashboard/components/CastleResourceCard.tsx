import React from 'react';
import './CastleResourceCard.css';

import WoodIcon from '../../../assets/Wood.png';
import StoneIcon from '../../../assets/Stone.png';
import FoodIcon from '../../../assets/Food.png';
import CharcoalIcon from '../../../assets/Charcoal.png';
import OliveOilIcon from '../../../assets/OliveOil.png';
import GlassIcon from '../../../assets/Glass.png';
import IronOreIcon from '../../../assets/Iron_Ore.png';
import HoneyIcon from '../../../assets/Honey.png';
import MeadIcon from '../../../assets/Mead.png';
import BeefIcon from '../../../assets/Beef.png';

import {
    type CastleResourcesAmount,
    type CastleStorageMax,
    type CastleProductionTotal
} from '../models/PlayerCastleInfo';

interface CastleResourceCardProps {
    castleName: string,
    resources: CastleResourcesAmount,
    storage: CastleStorageMax,
    production: CastleProductionTotal,
    key?: never
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

const CastleResourceCard: React.FC<CastleResourceCardProps> = ({castleName, resources, storage, production}) => {
    return (
        <div className="castle-card">
            <h3 className="castle-name">{castleName}</h3>

            <div className="resource-list-view">
                {resourceKeys.map(key => {
                    const resourceBaseName = key.replace('_amount', '');
                    const amount = resources[key];
                    const max = storage[`${resourceBaseName}_max` as keyof CastleStorageMax];
                    const prod = production[`${resourceBaseName}_prod` as keyof CastleProductionTotal];
                    const percentage = max > 0 ? (amount / max) * 100 : 0;

                    return (
                        <div key={key} className="resource-list-item">
                            <img src={resourceIconMap[resourceBaseName]} alt={resourceBaseName}
                                 className="resource-icon"/>
                            <div className="resource-info">
                                <div className="resource-text">
                                    <span>{amount.toLocaleString()} / {max.toLocaleString()}</span>
                                    <span className="production-rate">(+{prod.toLocaleString()}/hr)</span>
                                </div>
                                <div className="progress-bar-container">
                                    <div className="progress-bar" style={{width: `${percentage}%`}}></div>
                                </div>
                            </div>
                        </div>
                    );
                })}
            </div>

            <div className="queues-section">
                <div className="queue">
                    <h4>Recruitment Queue</h4>
                    <div className="queue-items">
                        {[...Array(5)].map((_, i) => <div key={i} className="queue-item-placeholder"/>)}
                    </div>
                </div>
                <div className="queue">
                    <h4>Tool Queue</h4>
                    <div className="queue-items">
                        {[...Array(5)].map((_, i) => <div key={i} className="queue-item-placeholder"/>)}
                    </div>
                </div>
            </div>
        </div>
    );
};

export default CastleResourceCard;
