import React from 'react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import DecorationPresetsPanel from '../../components/DecorationPresetsPanel';
import { Icons } from '../../components/Icons';
import { useCastleFocus } from '../../context/CastleFocusContext';
import CastleUnitCard from './CastleUnitCard.tsx';
import CastleQueuesCard from './CastleQueuesCard.tsx';
import { Badge, EmptyState, PageHeader, SectionCard } from '../../components/ui';

/**
 * Single-castle hub driven by GameState.CastleFocus (mirrored as `castleFocus`): units, queues, and decorations.
 */
const CastleView: React.FC = () => {
  const { castle } = useCastleFocus();
  const focusedAid = castle?.id ?? 0;
  const castleName = castle?.name?.trim() || (focusedAid > 0 ? `Castle ${focusedAid}` : '');
  const troopTotal = castle
    ? Object.values(castle.units.total).reduce((total, count) => total + count, 0)
    : 0;
  const dashboardHeader = (
    <PageHeader
      eyebrow="Empire overview"
      title={castleName || 'Castle command'}
      description="Manage the focused stronghold, understand production at a glance, and keep troop readiness visible."
      icon={<Icons.Castle />}
      meta={(
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant={focusedAid > 0 ? 'primary' : 'secondary'}>
            {focusedAid > 0 ? 'Focused castle' : 'Awaiting focus'}
          </Badge>
          {focusedAid > 0 && <Badge variant="outline">AID {focusedAid}</Badge>}
          {troopTotal > 0 && <Badge variant="secondary">{troopTotal.toLocaleString()} troops</Badge>}
        </div>
      )}
    />
  );

  if (focusedAid <= 0) {
    return (
      <div className="flex flex-col gap-6">
        {dashboardHeader}
        <StaleSessionBanner />
        <EmptyState
          title="No castle in focus"
          description="Choose a castle from the Focus strip under the header, or focus one in-game (JAA). The Castle view shows units, queues, and decoration presets for that castle only."
          className="border-border-light bg-bg-card/50"
        />
      </div>
    );
  }

  if (!castle) {
    return (
      <div className="flex flex-col gap-6">
        {dashboardHeader}
        <StaleSessionBanner />
        <EmptyState
          title={castleName}
          description="No castle data yet for this focus. Stay on the castle in-game; updates arrive over the websocket automatically."
          className="border-border-light bg-bg-card/60 [border-style:solid]"
        />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-6">
      {dashboardHeader}
      <StaleSessionBanner />

      <div className="castle-dashboard-grid">
        <div className="castle-dashboard-left">
          <SectionCard
            variant="solid"
            title="Decorations"
            titleClassName="text-primary"
            className="flex min-h-0 flex-col"
            contentClassName="custom-scrollbar flex-1 overflow-auto"
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
