import React from 'react';
import { MapPin, RefreshCw } from 'lucide-react';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { useAuth } from '../../context/AuthContext';
import { Button, SectionCard } from '../../components/ui';
import { useRiftMap } from '../context/RiftMapContext';
import { formatRiftDelta } from '../types/RiftMapCoords';

const RiftCoordDisplay: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { castle } = useCastleFocus();
  const { riftMapCoords, refreshRiftMapCoords } = useRiftMap();

  const centerX = riftMapCoords?.centerX ?? castle?.x ?? 0;
  const centerY = riftMapCoords?.centerY ?? castle?.y ?? 0;
  const kingdomID = riftMapCoords?.kingdomID ?? castle?.kingdomId ?? 0;
  const castleName = castle?.name?.trim() || 'Castle';
  const rift = riftMapCoords?.rift ?? null;
  const found = riftMapCoords?.found ?? false;
  const riftKid = riftMapCoords?.riftKingdomID ?? 0;
  const hasCastleCoords = centerX !== 0 || centerY !== 0;

  return (
    <SectionCard
      variant="solid"
      title="Rift location"
      titleClassName="text-lg text-primary"
      description={(
        <>
          Single world Rift on K{riftKid || 0}
          {hasCastleCoords ? (
            <>
              {' '}
              · {castleName} at ({centerX}, {centerY}) · K{kingdomID}
            </>
          ) : null}
        </>
      )}
      descriptionClassName=""
      headerClassName="flex-row gap-4"
      actions={(
        <Button
          variant="outline"
          size="sm"
          className="shrink-0"
          disabled={!gameLoggedIn}
          onClick={() => refreshRiftMapCoords(true)}
          title={gameLoggedIn ? 'Refresh Rift coords from game (GAA)' : 'Connect to refresh live map data'}
          leftIcon={<RefreshCw className="w-3.5 h-3.5" />}
        >
          Refresh
        </Button>
      )}
    >
      {!found || !rift ? (
          <p className="text-sm text-text-muted">
            {gameLoggedIn
              ? 'Rift not in map cache yet. Open the world map near the Rift or press Refresh to request GAA.'
              : 'No Rift tile in the last snapshot. Connect and refresh after the map has loaded once.'}
          </p>
        ) : (
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
            <div className="flex items-start gap-3">
              <div className="rounded-lg bg-primary/10 p-2.5 text-primary">
                <MapPin className="h-5 w-5" />
              </div>
              <div>
                <p className="text-[10px] uppercase tracking-wider text-text-muted font-semibold">Coordinates</p>
                <p className="text-2xl font-bold font-mono text-text-main mt-0.5">
                  {rift.x}, {rift.y}
                </p>
                <p className="text-sm text-text-muted mt-1">
                  Rift
                  {rift.name?.trim() ? ` · ${rift.name.trim()}` : ''}
                </p>
              </div>
            </div>

            {hasCastleCoords ? (
              <div className="w-full rounded-lg border border-border-base bg-bg-card/40 px-4 py-3 md:w-auto md:min-w-[12rem]">
                <p className="text-[10px] uppercase tracking-wider text-text-muted font-semibold">
                  From {castleName}
                </p>
                <p className="text-lg font-semibold text-text-main mt-1">
                  {riftMapCoords?.distance ?? 0} tiles
                </p>
                <p className="text-xs font-mono text-text-muted mt-1">
                  Δ {formatRiftDelta(riftMapCoords?.deltaX ?? 0, riftMapCoords?.deltaY ?? 0)}
                </p>
              </div>
            ) : (
              <p className="text-sm text-text-muted">
                Focus a castle with map coords to see distance from your castle.
              </p>
            )}
          </div>
        )}
    </SectionCard>
  );
};

export default RiftCoordDisplay;
