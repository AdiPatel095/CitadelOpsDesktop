import React from 'react';
import { TROOP_DEFINITIONS } from '../../config/constants';
import UnitImage from '../../components/UnitImage';

interface CastleUnitCardProps {
    /** Card heading (e.g. "Units"). */
    title: string;
    troopsMixed: { [unitID: string]: number };
    troopsI: { [unitID: string]: number };
    troopsTU: { [unitID: string]: number };
}

const CastleUnitCard: React.FC<CastleUnitCardProps> = ({ title, troopsMixed, troopsI, troopsTU }) => {
    // Go serializes map[int]int with string keys, so Object.keys gives strings
    const sortedUnitIds = Object.keys(troopsMixed)
        .map(Number)
        .filter(id => !isNaN(id) && troopsMixed[id] > 0)
        .sort((a, b) => (troopsMixed[b] || 0) - (troopsMixed[a] || 0));

    return (
        <div className="castle-card">
            <h3 className="castle-name">{title}</h3>

            <div className="flex-1 overflow-y-auto p-4 custom-scrollbar">
                {sortedUnitIds.length === 0 ? (
                    <div className="text-center py-8 text-text-muted">
                        <p className="text-sm">No units found</p>
                    </div>
                ) : (
                    <div className="grid grid-cols-3 sm:grid-cols-4 md:grid-cols-5 gap-3">
                        {sortedUnitIds.map(unitId => {
                            const name = TROOP_DEFINITIONS[unitId] || `Unit ${unitId}`;
                            const total = troopsMixed[unitId] || 0;
                            const inCastle = troopsI[unitId] || 0;
                            const travelling = troopsTU[unitId] || 0;

                            return (
                                <div
                                    key={unitId}
                                    className="relative flex flex-col items-center p-2 rounded-xl transition-all duration-200 border-2 border-border-base bg-bg-card hover:border-primary/50 hover:bg-bg-card-hover"
                                >
                                    {/* Unit Image  */}
                                    <div className="w-full aspect-square flex items-center justify-center p-1">
                                        <UnitImage unitId={unitId} size={60} showLevel={true} />
                                    </div>

                                    {/* Total and separation */}
                                    <span className="mt-2 text-sm font-bold text-text-main text-center">
                                        {total.toLocaleString()}
                                    </span>
                                    <span className="text-[10px] text-text-muted text-center mt-0.5">
                                        ({inCastle.toLocaleString()} / {travelling.toLocaleString()})
                                    </span>

                                    <span className="mt-1 pb-1 text-[11px] font-medium text-text-main text-center line-clamp-2 w-full leading-tight">
                                        {name}
                                    </span>
                                </div>
                            );
                        })}
                    </div>
                )}
            </div>
        </div>
    );
};

export default CastleUnitCard;
