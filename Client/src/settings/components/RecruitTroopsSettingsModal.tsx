import React from 'react';
import {
  QueueProductionSettingsModal,
  type QueueProductionSettingsModalProps,
} from './QueueProductionSettingsModal';

type RecruitTroopsSettingsModalProps = Omit<QueueProductionSettingsModalProps, 'kind'>;

export const RecruitTroopsSettingsModal: React.FC<RecruitTroopsSettingsModalProps> = (props) => (
  <QueueProductionSettingsModal {...props} kind="recruit" />
);
