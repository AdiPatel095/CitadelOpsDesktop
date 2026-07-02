import React, { useEffect, useMemo, useRef, useState } from 'react';
import { CalendarDays, Save } from 'lucide-react';
import { FrontendWebsocket } from '../../Websocket';
import { Badge, Button, Modal } from '../../components/ui';
import {
  createEmptyWeeklySchedule,
  normalizeFeatureSchedules,
  type FeatureSchedules,
  type WeeklySchedule,
} from '../SchedulerTypes';
import { loadAutoToolSettingsFromStorage } from '../AutoToolClientState';
import { loadRecruitTroopsSettingsFromStorage } from '../RecruitTroopsClientState';
import {
  normalizeQueueableProductionCatalog,
  queueableBuildingRowsLoaded,
  queueableIDsForCastle,
  queueableIDsForCastles,
  type QueueableProductionCatalog,
  type QueueableProductionField,
} from '../QueueableProductionCatalog';
import { WeeklyScheduler, type ScheduleSlotOptionsConfig } from './WeeklyScheduler';

interface FeatureScheduleModalProps {
  isOpen: boolean;
  featureID: string | null;
  featureLabel: string;
  onClose: () => void;
}

export const FeatureScheduleModal: React.FC<FeatureScheduleModalProps> = ({
  isOpen,
  featureID,
  featureLabel,
  onClose,
}) => {
  const [featureSchedules, setFeatureSchedules] = useState<FeatureSchedules>({});
  const [queueableCatalog, setQueueableCatalog] = useState<QueueableProductionCatalog>({});
  const [queueableCatalogLoaded, setQueueableCatalogLoaded] = useState(false);
  const [isDirty, setIsDirty] = useState(false);
  const dirtyRef = useRef(false);

  const setDirty = (dirty: boolean) => {
    dirtyRef.current = dirty;
    setIsDirty(dirty);
  };

  useEffect(() => {
    if (!isOpen) return;

    const handleMessage = (msg: any) => {
      if (msg.type === 'queueableProductionCatalog') {
        setQueueableCatalog(normalizeQueueableProductionCatalog(msg.payload));
        setQueueableCatalogLoaded(true);
        return;
      }
      if (msg.type !== 'schedulerSettings' || !msg.payload || dirtyRef.current) return;
      setFeatureSchedules(normalizeFeatureSchedules(msg.payload.featureSchedules));
    };

    FrontendWebsocket.addMessageListener(handleMessage);
    FrontendWebsocket.sendGetSchedulerSettings();
    FrontendWebsocket.sendMessage({ type: 'getQueueableProductionCatalog' });

    return () => {
      FrontendWebsocket.removeMessageListener(handleMessage);
    };
  }, [isOpen]);

  useEffect(() => {
    if (isOpen) return;
    setDirty(false);
  }, [isOpen]);

  const selectedSchedule = useMemo(() => {
    if (!featureID) return createEmptyWeeklySchedule();
    return featureSchedules[featureID] ?? createEmptyWeeklySchedule();
  }, [featureID, featureSchedules]);

  const queueableIDsForFeature = (field: QueueableProductionField): number[] | undefined => {
    if (!featureID || !queueableCatalogLoaded) return undefined;
    const [, castleID] = featureID.split(':', 2);
    if (castleID) {
      if (!queueableBuildingRowsLoaded(queueableCatalog, castleID)) return undefined;
      return queueableIDsForCastle(queueableCatalog, castleID, field);
    }

    const knownCastleIDs = Object.keys(queueableCatalog).filter(
      (id) => queueableBuildingRowsLoaded(queueableCatalog, id) && queueableIDsForCastle(queueableCatalog, id, field).length > 0,
    );
    let enabledCastleIDs: string[] = [];
    if (featureID === 'autoRecruit') {
      const settings = loadRecruitTroopsSettingsFromStorage();
      enabledCastleIDs = knownCastleIDs.filter((id) => settings.castles[id]?.enabled);
    } else if (featureID === 'autoTool') {
      const settings = loadAutoToolSettingsFromStorage();
      enabledCastleIDs = knownCastleIDs.filter((id) => settings.castles[id]?.enabled);
    }
    if (enabledCastleIDs.length > 0) {
      return queueableIDsForCastles(queueableCatalog, enabledCastleIDs, field, 'intersection');
    }
    if (knownCastleIDs.length > 0) {
      return queueableIDsForCastles(queueableCatalog, knownCastleIDs, field, 'union');
    }
    return undefined;
  };

  const slotOptionsConfig = useMemo<ScheduleSlotOptionsConfig | undefined>(() => {
    if (!featureID) return undefined;
    if (featureID === 'autoRecruit' || featureID.startsWith('autoRecruit:')) {
      return {
        enabledLabel: 'Specify Unit Per Period',
        formTitle: 'Period Recruit Unit',
        fields: [
          {
            id: 'unitID',
            label: 'Unit',
            type: 'number',
            picker: 'troop',
            placeholder: '216',
            required: true,
            integer: true,
            min: 1,
            allowedUnitIds: queueableIDsForFeature('recruitUnitIds'),
          },
        ],
      };
    }
    if (featureID === 'autoTool' || featureID.startsWith('autoTool:')) {
      return {
        enabledLabel: 'Specify Tool Per Period',
        formTitle: 'Period Tool',
        fields: [
          {
            id: 'toolID',
            label: 'Tool',
            type: 'number',
            picker: 'tool',
            placeholder: '611',
            required: true,
            integer: true,
            min: 1,
            allowedToolIds: queueableIDsForFeature('toolIds'),
          },
        ],
      };
    }
    return undefined;
  }, [featureID, queueableCatalog, queueableCatalogLoaded]);

  const handleScheduleChange = (schedule: WeeklySchedule) => {
    if (!featureID) return;
    setFeatureSchedules((prev) => normalizeFeatureSchedules({
      ...prev,
      [featureID]: schedule,
    }));
    setDirty(true);
  };

  const handleSave = () => {
    if (!featureID) return;
    const normalized = normalizeFeatureSchedules(featureSchedules);
    setFeatureSchedules(normalized);
    const sent = FrontendWebsocket.sendSaveSchedulerSettings({ featureSchedules: normalized });
    if (!sent) return;
    setDirty(false);
    onClose();
  };

  return (
    <Modal
      isOpen={isOpen && !!featureID}
      onClose={onClose}
      maxWidth="full"
      title={
        <div className="scheduler-modal-title">
          <span className="scheduler-modal-title-mark" aria-hidden="true">
            <CalendarDays className="h-5 w-5" />
          </span>
          <span className="scheduler-modal-title-text">{featureLabel} Schedule</span>
          {isDirty && <Badge variant="warning">Unsaved</Badge>}
        </div>
      }
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Close
          </Button>
          <Button
            variant="primary"
            onClick={handleSave}
            disabled={!isDirty}
            leftIcon={<Save className="h-4 w-4" />}
          >
            Save Schedule
          </Button>
        </>
      }
    >
      <div className="scheduler-modal-shell">
        <WeeklyScheduler
          value={selectedSchedule}
          onChange={handleScheduleChange}
          slotOptionsConfig={slotOptionsConfig}
        />
      </div>
    </Modal>
  );
};
