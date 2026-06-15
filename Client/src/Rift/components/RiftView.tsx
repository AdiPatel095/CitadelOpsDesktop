import React from 'react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { Card, CardContent } from '../../components/ui';
import RiftCoordDisplay from './RiftCoordDisplay';
import RiftAttackTemplate from './RiftAttackTemplate';
import RiftMaidenCommsPanel from './RiftMaidenCommsPanel';

const RiftView: React.FC = () => {
  return (
    <div className="flex flex-col gap-6">
      <StaleSessionBanner />
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold text-text-main mb-1">Rift</h1>
          <p className="text-sm text-text-muted">Rift expeditions, rewards, and automation.</p>
        </div>
      </div>

      <RiftCoordDisplay />

      <RiftMaidenCommsPanel />

      <RiftAttackTemplate />

      <Card>
        <CardContent className="flex items-center justify-center py-20">
          <p className="text-text-muted font-medium">Rift features coming soon...</p>
        </CardContent>
      </Card>
    </div>
  );
};

export default RiftView;
