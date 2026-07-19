import React from 'react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import DecorationPresetsPanel from '../../components/DecorationPresetsPanel';
import { useCastleFocus } from '../../context/CastleFocusContext';
import CastleUnitCard from './CastleUnitCard.tsx';
import CastleQueuesCard from './CastleQueuesCard.tsx';
import { EmptyState, SectionCard } from '../../components/ui';

/**
 * Single-castle hub driven by GameState.CastleFocus (mirrored as `castleFocus`): units, queues, and decorations.
 */
const CastleView: React.FC = () => {
  const { castle } = useCastleFocus();
  const focusedAid = castle?.id ?? 0;
  const castleName = castle?.name?.trim() || (focusedAid > 0 ? `Castle ${focusedAid}` : '');

  if (focusedAid <= 0) {
    return (
      <div className="flex flex-col gap-6">
        <StaleSessionBanner />
        <EmptyState
          title="No castle in focus"
          description="Choose a castle from the Focus strip under the header, or focus one in-game (JAA). The Castle view shows units, queues, and decoration presets for that castle only."
          className="border-border-light bg-bg-card/50 backdrop-blur-2xl"
        />
      </div>
    );
  }

  if (!castle) {
    return (
      <div className="flex flex-col gap-6">
        <StaleSessionBanner />
        <EmptyState
          title={castleName}
          description="No castle data yet for this focus. Stay on the castle in-game; updates arrive over the websocket automatically."
          className="border-border-light bg-bg-card/60 backdrop-blur-2xl [border-style:solid]"
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <StaleSessionBanner />

      <div className="castle-dashboard-grid">
        <div className="castle-dashboard-left">
          <SectionCard
            variant="glass"
            title="Decorations"
            titleClassName="text-primary"
            className="flex min-h-0 flex-col"
            contentClassName="decoration-card-content custom-scrollbar flex-1 overflow-auto"
          >
              <DecorationPresetsPanel />
          </SectionCard>
          <CastleQueuesCard />
        </div>

        <div className="castle-dashboard-units">
          <CastleUnitCard
            title="Troop Overview"
            troopsMixed={castle.units.total}
            troopsI={castle.units.stationed}
            troopsTU={castle.units.traveling}
          />
        </div>
      </div>
    </div>
  );
};

export default CastleView;
