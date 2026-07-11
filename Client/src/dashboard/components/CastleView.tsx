import React from 'react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import DecorationPresetsPanel from '../../components/DecorationPresetsPanel';
import { useAuth } from '../../context/AuthContext';
import { useCastleFocus } from '../../context/CastleFocusContext';
import CastleResourceCard from './CastleResourceCard.tsx';
import CastleUnitCard from './CastleUnitCard.tsx';
import CastleQueuesCard from './CastleQueuesCard.tsx';
import { Card, CardHeader, CardTitle, CardContent } from '../../components/ui';

/**
 * Single-castle hub driven by GameState.CastleFocus (mirrored as `castleFocus`): resources, units, queues, decorations.
 */
const CastleView: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { castle } = useCastleFocus();
  const focusedAid = castle?.id ?? 0;
  const castleName = castle?.name?.trim() || (focusedAid > 0 ? `Castle ${focusedAid}` : '');

  if (focusedAid <= 0) {
    return (
      <div className="flex flex-col gap-6">
        <StaleSessionBanner />
        <div className="rounded-global border border-dashed border-border-light bg-bg-card/50 px-6 py-12 text-center backdrop-blur-2xl">
          <p className="text-sm font-medium text-text-main">No castle in focus</p>
          <p className="mt-2 text-xs text-text-muted mx-auto max-w-md">
            Choose a castle from the Focus strip under the header, or focus one in-game (JAA). The Castle view shows
            resources, units, queues, and decoration presets for that castle only.
          </p>
        </div>
      </div>
    );
  }

  if (!castle) {
    return (
      <div className="flex flex-col gap-6">
        <StaleSessionBanner />
        <div className="rounded-global border border-border-light bg-bg-card/60 px-6 py-10 text-center backdrop-blur-2xl">
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
    <div className="flex flex-col gap-6">
      <StaleSessionBanner />

      <div className="castle-dashboard-grid md:items-stretch">
        <Card className="liquid-prominent-header-card flex flex-col min-h-0">
          <CardHeader className="liquid-card-header-prominent">
            <CardTitle className="text-primary">Decorations</CardTitle>
          </CardHeader>
          <CardContent className="liquid-prominent-header-content decoration-card-content flex-1 overflow-auto custom-scrollbar">
            <DecorationPresetsPanel />
          </CardContent>
        </Card>
        
        <CastleResourceCard
          title="Resources"
          resources={castle.resources}
        />
        
        <CastleUnitCard
          title="Units"
          troopsMixed={castle.units.total}
          troopsI={castle.units.stationed}
          troopsTU={castle.units.traveling}
        />
        
        <CastleQueuesCard />
      </div>
    </div>
  );
};

export default CastleView;
