import React from 'react';
import UnitImage from '../../components/UnitImage';
import { Card, CardHeader, CardTitle, CardContent } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';

interface CastleUnitCardProps {
  title: string;
  troopsMixed: { [unitID: string]: number };
  troopsI: { [unitID: string]: number };
  troopsTU: { [unitID: string]: number };
}

function formatBadgeCount(value: number): string {
  return value.toLocaleString();
}

const CastleUnitCard: React.FC<CastleUnitCardProps> = ({ title, troopsMixed, troopsI, troopsTU }) => {
  const { getTroop } = useMetadata();
  const sortedUnitIds = Object.keys(troopsMixed)
    .map(Number)
    .filter(id => !isNaN(id) && troopsMixed[id] > 0)
    .sort((a, b) => (troopsMixed[b] || 0) - (troopsMixed[a] || 0));

  return (
    <Card className="liquid-prominent-header-card flex flex-col min-h-0">
      <CardHeader className="liquid-card-header-prominent">
        <CardTitle className="text-primary">{title}</CardTitle>
      </CardHeader>

      <CardContent className="liquid-prominent-header-content flex-1 overflow-y-auto custom-scrollbar">
        {sortedUnitIds.length === 0 ? (
          <div className="text-center py-8 text-text-muted">
            <p className="text-sm">No units found</p>
          </div>
        ) : (
          <div className="grid grid-cols-3 gap-x-3 gap-y-5 pb-3 pt-2 sm:grid-cols-4 md:grid-cols-5">
            {sortedUnitIds.map(unitId => {
              const name = getTroop(unitId)?.name || `Unit ${unitId}`;
              const inCastle = troopsI[unitId] || 0;
              const travelling = troopsTU[unitId] || 0;

              return (
                <div key={unitId} className="relative pt-3 pb-2">
                  <div className="relative flex min-h-[164px] flex-col items-center justify-center rounded-global border border-border-light bg-bg-card/45 px-2 pb-4 pt-4 shadow-sm backdrop-blur-xl transition-all duration-200 hover:border-primary/50 hover:bg-bg-card-hover/70">
                    <span className="absolute left-1/2 top-0 z-10 max-w-[calc(100%-20px)] -translate-x-1/2 -translate-y-1/2 truncate rounded-full bg-bg-card/90 px-3 py-1 text-center text-[10px] font-bold text-text-main shadow-sm ring-1 ring-border-light backdrop-blur-xl">
                      {name}
                    </span>

                    <div className="relative flex w-full flex-1 items-center justify-center">
                      <UnitImage unitId={unitId} size={104} showLevel={true} className="drop-shadow-md" />
                    </div>
                  </div>
                  <span className="absolute bottom-0 left-0 z-10 min-w-[3rem] -translate-x-[8%] translate-y-[0%] rounded-full bg-bg-card/95 px-3 py-1 text-center text-[11px] font-bold tabular-nums text-text-main shadow-md ring-1 ring-border-light backdrop-blur-xl">
                    {formatBadgeCount(inCastle)}
                  </span>
                  <span className="absolute bottom-0 right-0 z-10 min-w-[3rem] translate-x-[8%] translate-y-[0%] rounded-full bg-primary px-3 py-1 text-center text-[11px] font-bold tabular-nums text-text-inverted shadow-md ring-1 ring-primary/30">
                    {formatBadgeCount(travelling)}
                  </span>
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
};

export default CastleUnitCard;
