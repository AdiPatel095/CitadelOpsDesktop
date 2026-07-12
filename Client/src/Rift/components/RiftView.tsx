import React from 'react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import RiftAttackTemplate from './RiftAttackTemplate';
import RiftMaidenCommsPanel from './RiftMaidenCommsPanel';

const RiftView: React.FC = () => {
  return (
    <div className="flex flex-col gap-6">
      <StaleSessionBanner />

      <RiftMaidenCommsPanel />

      <RiftAttackTemplate />
    </div>
  );
};

export default RiftView;
