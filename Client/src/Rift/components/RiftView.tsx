import React from 'react';
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
    <section className="flex flex-col gap-6" aria-label="Rift operations">
      <StaleSessionBanner />

      <RiftMaidenCommsPanel
        headerActions={(
          <div className="flex w-full flex-wrap items-center gap-2 sm:w-auto sm:justify-end">
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
        )}
      />

      <RiftAttackTemplate />
    </section>
  );
};

export default RiftView;
