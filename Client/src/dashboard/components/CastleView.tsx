import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import React, { useState } from 'react';
import CastleOverviewCard from './CastleOverviewCard';
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
  const [decorationsExpanded, setDecorationsExpanded] = useState(() => {
    try { return localStorage.getItem('citadelops.castle.decorations.expanded.v1') === 'true'; }
    catch { return false; }
  });
  const changeDecorationsExpanded = (expanded: boolean) => {
    setDecorationsExpanded(expanded);
    try { localStorage.setItem('citadelops.castle.decorations.expanded.v1', String(expanded)); }
    catch { /* Device storage is optional; keep the current session usable. */ }
  };
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

      <CastleOverviewCard castle={castle} />
      <div className="castle-dashboard-layout">
        <div className="castle-dashboard-grid">
          <div data-castle-queues><CastleQueuesCard /></div>
          <div data-castle-troops>
            <CastleUnitCard
              title={localizeStatic("ui.dashboard.components.castleView.title.troop.overview.8a4ce681")}
              troopsMixed={castle.units.total}
              troopsI={castle.units.stationed}
              troopsTU={castle.units.traveling}
            />
          </div>
        </div>
      </div>
      <SectionCard
        title={localizeStatic("ui.dashboard.components.castleView.title.decorations.5a02b053")}
        collapsible expanded={decorationsExpanded} onExpandedChange={changeDecorationsExpanded}
        data-castle-decorations
      >
        <DecorationPresetsPanel />
      </SectionCard>
    </div>
  );
};

export default CastleView;
