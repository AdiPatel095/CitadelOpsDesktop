import React, { useEffect, useMemo, useRef, useState } from 'react';
import { CalendarDays, Save } from 'lucide-react';
import { Badge, Button, Modal } from '../../components/ui';
import {
  createEmptyWeeklySchedule,
  normalizeFeatureSchedules,
  type FeatureSchedules,
  type WeeklySchedule,
} from '../SchedulerTypes';
import { loadAutoToolSettingsFromStorage, normalizeAutoToolSettings } from '../AutoToolClientState';
import { loadRecruitTroopsSettingsFromStorage, normalizeRecruitTroopsSettings } from '../RecruitTroopsClientState';
import {
  buildQueueableProductionCatalog,
  queueableBuildingRowsLoaded,
  queueableIDsForCastle,
  queueableIDsForCastles,
  type QueueableProductionField,
} from '../QueueableProductionCatalog';
import { WeeklyScheduler, type ScheduleSlotOptionsConfig } from './WeeklyScheduler';
import { useCitadelAPI } from '../../api/ApiContext';
import { configurationSection } from '../Configuration';
import { useMetadata } from '../../context/MetadataContext';

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
  const { configuration, state, updateConfiguration } = useCitadelAPI();
  const { buildings, troops, tools, isLoading: metadataLoading } = useMetadata();
  const [featureSchedules, setFeatureSchedules] = useState<FeatureSchedules>({});
  const queueableCatalog = buildQueueableProductionCatalog(state, buildings, troops, tools);
  const queueableCatalogLoaded = state != null && !metadataLoading;
  const [isDirty, setIsDirty] = useState(false);
  const [saveError, setSaveError] = useState('');
  const [saving, setSaving] = useState(false);
  const dirtyRef = useRef(false);

  const setDirty = (dirty: boolean) => {
    dirtyRef.current = dirty;
    setIsDirty(dirty);
  };

  useEffect(() => {
    if (!isOpen || dirtyRef.current) return;
    const scheduler = configurationSection(configuration, 'scheduler');
    setFeatureSchedules(normalizeFeatureSchedules(scheduler.featureSchedules));
  }, [configuration?.sections.scheduler, isOpen]);

  useEffect(() => {
    if (isOpen) return;
    setDirty(false);
    setSaveError('');
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
      const settings = normalizeRecruitTroopsSettings(
        configuration?.sections['automation.recruitTroops'] ?? loadRecruitTroopsSettingsFromStorage(),
      );
      enabledCastleIDs = knownCastleIDs.filter((id) => settings.castles[id]?.enabled);
    } else if (featureID === 'autoTool') {
      const settings = normalizeAutoToolSettings(
        configuration?.sections['automation.autoTool'] ?? loadAutoToolSettingsFromStorage(),
      );
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
    const scheduler = configurationSection(configuration, 'scheduler');
    setSaving(true);
    setSaveError('');
    void updateConfiguration('scheduler', { ...scheduler, featureSchedules: normalized })
      .then(() => {
        setDirty(false);
        onClose();
      })
      .catch((error) => setSaveError(error instanceof Error ? error.message : 'Could not save schedule'))
      .finally(() => setSaving(false));
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
            disabled={!isDirty || saving}
            leftIcon={<Save className="h-4 w-4" />}
          >
            {saving ? 'Saving…' : 'Save Schedule'}
          </Button>
        </>
      }
    >
      <div className="scheduler-modal-shell">
        {saveError && <p className="mb-3 text-xs text-error">{saveError}</p>}
        <WeeklyScheduler
          value={selectedSchedule}
          onChange={handleScheduleChange}
          slotOptionsConfig={slotOptionsConfig}
        />
      </div>
    </Modal>
  );
};
