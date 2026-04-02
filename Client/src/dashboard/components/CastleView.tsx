import React, { useEffect, useMemo } from 'react';
import DecorationPresetsPanel from '../../components/DecorationPresetsPanel';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { useCastleResources } from '../context/CastleResourceContext.tsx';
import CastleResourceCard from './CastleResourceCard.tsx';
import CastleUnitCard from './CastleUnitCard.tsx';
import CastleQueuesCard from './CastleQueuesCard.tsx';

/**
 * Single-castle hub driven by GameState.CastleFocus (mirrored as `castleFocus`): resources, units, queues, decorations.
 */
const CastleView: React.FC = () => {
  const { castleFocus } = useCastleFocus();
  const { castleResources, isCastleResourcesLoading, requestCastleResource } = useCastleResources();

  const focusedAid = castleFocus?.aid && castleFocus.aid > 0 ? castleFocus.aid : 0;

  useEffect(() => {
    if (focusedAid > 0) {
      requestCastleResource(focusedAid);
    }
  }, [focusedAid, requestCastleResource]);

  const castle = useMemo(
    () => (focusedAid > 0 ? castleResources.get(focusedAid) : undefined),
    [castleResources, focusedAid]
  );

  const loadingFocused = focusedAid > 0 && Boolean(isCastleResourcesLoading[focusedAid]);
  const castleName =
    castle?.castleName?.trim() ||
    castleFocus?.castleName?.trim() ||
    (focusedAid > 0 ? `Castle ${focusedAid}` : '');

  if (focusedAid <= 0) {
    return (
      <div className="dashboard">
        <div className="rounded-global border border-dashed border-border-light bg-bg-card/50 px-6 py-12 text-center">
          <p className="text-sm font-medium text-text-main">No castle in focus</p>
          <p className="mt-2 text-xs text-text-muted mx-auto max-w-md">
            Choose a castle from the Focus strip under the header, or focus one in-game (JAA). The Castle view shows
            resources, units, queues, and decoration presets for that castle only.
          </p>
        </div>
      </div>
    );
  }

  if (!castle && loadingFocused) {
    return (
      <div className="dashboard">
        <div className="rounded-global border border-border-base bg-bg-card/80 px-6 py-10 text-center">
          <p className="text-sm text-text-muted">Loading {castleName}…</p>
        </div>
      </div>
    );
  }

  if (!castle) {
    return (
      <div className="dashboard">
        <div className="rounded-global border border-border-base bg-bg-card/80 px-6 py-10 text-center">
          <p className="text-sm font-medium text-text-main">{castleName}</p>
          <p className="mt-2 text-xs text-text-muted mx-auto max-w-md">
            No castle data yet for this focus. Stay on the castle in-game; updates arrive over the websocket
            automatically.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="dashboard">
      <header className="flex flex-col gap-1 border-b border-border-base pb-4">
        <h1 className="text-2xl font-bold tracking-tight text-text-main">{castleName}</h1>
        <p className="text-xs text-text-muted">
          Focused castle (aid {focusedAid}
          {castleFocus?.kingdomID != null && castleFocus.kingdomID !== 0 ? ` · kingdom ${castleFocus.kingdomID}` : ''}
          ). Switch focus from the strip under the header to change castle.
        </p>
      </header>

      <div className="grid grid-cols-1 gap-6 md:grid-cols-2 md:items-stretch">
        <CastleQueuesCard />
        <CastleResourceCard
          title="Resources"
          resources={castle.amount}
          storage={castle.storage}
          production={castle.production}
        />
        <CastleUnitCard
          title="Units"
          troopsMixed={castle.troops?.troopsMixed ?? {}}
          troopsI={castle.troops?.troopsI ?? {}}
          troopsTU={castle.troops?.troopsTU ?? {}}
        />
        <div className="castle-card flex min-h-0 flex-col md:min-h-[22rem]">
          <h3 className="castle-name">Decorations</h3>
          <p className="-mt-2 mb-4 text-xs text-text-muted">
            Save and apply decoration presets from your current in-game focus (JAA). Apply runs the smart replacer (SOB
            / EBU) until the layout matches.
          </p>
          <div className="min-h-0 flex-1 overflow-auto">
            <DecorationPresetsPanel />
          </div>
        </div>
      </div>
    </div>
  );
};

export default CastleView;
