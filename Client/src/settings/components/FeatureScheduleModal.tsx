import React, { useEffect, useMemo, useRef, useState } from 'react';
import { CalendarDays } from 'lucide-react';
import { Badge, SettingsModal } from '../../components/ui';
import {
  createEmptyWeeklySchedule,
  normalizeFeatureSchedules,
  type FeatureSchedules,
  type WeeklySchedule,
} from '../SchedulerTypes';
import { defaultAutoToolSettings, normalizeAutoToolSettings } from '../AutoToolClientState';
import { defaultRecruitTroopsSettings, normalizeRecruitTroopsSettings } from '../RecruitTroopsClientState';
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
import { highestUnitIDsByFamily, unitIDsAvailableByFamilyAcrossCastles } from '../UnitUpgradeFamily';

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
      const availableIDs = queueableIDsForCastle(queueableCatalog, castleID, field);
      return featureID.startsWith('autoRecruit:')
        ? highestUnitIDsByFamily(availableIDs, troops)
        : availableIDs;
    }

    const knownCastleIDs = Object.keys(queueableCatalog).filter(
      (id) => queueableBuildingRowsLoaded(queueableCatalog, id) && queueableIDsForCastle(queueableCatalog, id, field).length > 0,
    );
    let enabledCastleIDs: string[] = [];
    if (featureID === 'autoRecruit') {
      const settings = normalizeRecruitTroopsSettings(
        configuration?.sections['automation.recruitTroops'] ?? defaultRecruitTroopsSettings(),
      );
      enabledCastleIDs = knownCastleIDs.filter((id) => settings.castles[id]?.enabled);
    } else if (featureID === 'autoTool') {
      const settings = normalizeAutoToolSettings(
        configuration?.sections['automation.autoTool'] ?? defaultAutoToolSettings(),
      );
      enabledCastleIDs = knownCastleIDs.filter((id) => settings.castles[id]?.enabled);
    }
    if (enabledCastleIDs.length > 0) {
      if (featureID === 'autoRecruit') {
        return unitIDsAvailableByFamilyAcrossCastles(
          enabledCastleIDs.map((id) => queueableIDsForCastle(queueableCatalog, id, field)),
          troops,
        );
      }
      return queueableIDsForCastles(queueableCatalog, enabledCastleIDs, field, 'intersection');
    }
    if (knownCastleIDs.length > 0) {
      const availableIDs = queueableIDsForCastles(queueableCatalog, knownCastleIDs, field, 'union');
      return featureID === 'autoRecruit'
        ? highestUnitIDsByFamily(availableIDs, troops)
        : availableIDs;
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
            unitRange: {
              minOptionId: 'unitIDMin',
              maxOptionId: 'unitIDMax',
            },
          },
          {
            id: 'unitIDMin',
            label: 'Unit family minimum ID',
            type: 'number',
            integer: true,
            min: 1,
            hidden: true,
          },
          {
            id: 'unitIDMax',
            label: 'Unit family maximum ID',
            type: 'number',
            integer: true,
            min: 1,
            hidden: true,
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
    <SettingsModal
      isOpen={isOpen && !!featureID}
      onClose={onClose}
      maxWidth="full"
      title={`${featureLabel} Schedule`}
      icon={<CalendarDays className="h-5 w-5" />}
      titleTrailing={isDirty ? <Badge variant="warning">Unsaved</Badge> : undefined}
      onSave={handleSave}
      saveLabel="Save Schedule"
      saveDisabled={!isDirty}
      isSaving={saving}
      cancelLabel="Close"
    >
      <div className="scheduler-modal-shell">
        {saveError && <p className="mb-3 text-xs text-error">{saveError}</p>}
        <WeeklyScheduler
          value={selectedSchedule}
          onChange={handleScheduleChange}
          slotOptionsConfig={slotOptionsConfig}
        />
      </div>
    </SettingsModal>
  );
};
