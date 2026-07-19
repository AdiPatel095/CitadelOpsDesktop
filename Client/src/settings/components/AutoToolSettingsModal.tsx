import React from 'react';
import {
  QueueProductionSettingsModal,
  type QueueProductionSettingsModalProps,
} from './QueueProductionSettingsModal';

type AutoToolSettingsModalProps = Omit<QueueProductionSettingsModalProps, 'kind'>;

export const AutoToolSettingsModal: React.FC<AutoToolSettingsModalProps> = (props) => (
  <QueueProductionSettingsModal {...props} kind="tool" />
);
