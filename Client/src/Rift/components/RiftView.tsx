import React from 'react';
import { MapPinned } from 'lucide-react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { Badge } from '../../components/ui';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { useRiftMap } from '../context/RiftMapContext';
import RiftAttackTemplate from './RiftAttackTemplate';
import RiftMaidenCommsPanel from './RiftMaidenCommsPanel';

const RiftView: React.FC = () => {
  const { castle } = useCastleFocus();
  const { riftMapCoords, riftCRALaunch } = useRiftMap();
  const launchCount = riftCRALaunch?.launches.length ?? 0;

  return (
    <section className="flex flex-col gap-6" aria-labelledby="rift-view-title">
      <header className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div className="flex items-start gap-3">
          <div className="rounded-xl border border-primary/25 bg-primary/10 p-2.5 text-primary shadow-glow">
            <MapPinned className="h-5 w-5" />
          </div>
          <div>
            <h1 id="rift-view-title" className="text-2xl font-bold text-text-main">Rift operations</h1>
            <p className="mt-1 max-w-2xl text-sm text-text-muted">
              Send shield-maiden probe waves and replay captured attack formations from your main or focused castle.
            </p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2 sm:justify-end">
          <Badge
            variant={riftMapCoords?.found ? 'success' : 'warning'}
            className="normal-case tracking-normal"
          >
            {riftMapCoords?.found ? 'Rift target acquired' : 'Open the world map near the Rift to acquire target'}
          </Badge>
          <Badge variant="secondary" className="normal-case tracking-normal">
            {launchCount} captured template{launchCount === 1 ? '' : 's'}
          </Badge>
          {castle ? (
            <Badge variant="outline" className="normal-case tracking-normal">
              Focus · {castle.name?.trim() || `Castle ${castle.id}`}
            </Badge>
          ) : null}
        </div>
      </header>

      <StaleSessionBanner />

      <RiftMaidenCommsPanel />

      <RiftAttackTemplate />
    </section>
  );
};

export default RiftView;
