import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
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
  const { t: localizeStatic } = useStaticLocale();
  const { castle } = useCastleFocus();
  const focusedAid = castle?.id ?? 0;
  const castleName = castle?.name?.trim() || (focusedAid > 0 ? `Castle ${focusedAid}` : '');

  if (focusedAid <= 0) {
    return (
      <div className="flex flex-col gap-6">
        <StaleSessionBanner />
        <EmptyState
          title={localizeStatic("ui.dashboard.components.castleView.title.no.castle.in.focus.5168d69e")}
          description={localizeStatic("ui.dashboard.components.castleView.description.choose.a.castle.from.the.focus.strip.1d7c4f72")}
          className="border-border-light bg-bg-card/50"
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
          description={localizeStatic("ui.dashboard.components.castleView.description.no.castle.data.yet.for.this.focus.55cdfe24")}
          className="border-border-light bg-bg-card/60 [border-style:solid]"
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
            variant="solid"
            title={localizeStatic("ui.dashboard.components.castleView.title.decorations.5a02b053")}
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
            title={localizeStatic("ui.dashboard.components.castleView.title.troop.overview.8a4ce681")}
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
