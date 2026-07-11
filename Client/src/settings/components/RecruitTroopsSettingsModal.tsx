import React, { useState, useEffect } from 'react';
import { CalendarDays, Castle, Clock3, Copy, Trash2, Save, Plus, Settings } from 'lucide-react';
import { showTroopPicker } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import { useMetadata } from '../../context/MetadataContext';
import { Modal, Button, Input, Card, CardHeader, CardTitle, CardContent, Badge, Switch, PillSelector } from '../../components/ui';
import {
  DEFAULT_RECRUIT_CHECK_INTERVAL_MIN,
  MIN_RECRUIT_CHECK_INTERVAL_MIN,
  loadRecruitTroopsSettingsFromStorage,
  normalizeRecruitTroopsSettings,
  notifyRecruitTroopsSettingsChanged,
  persistRecruitTroopsSettings,
  recruitCheckIntervalMinutesToSec,
  recruitCheckIntervalSecToMinutes,
  recruitCastleScheduleID,
  type RecruitTroopsClientSettingsV1,
  type RecruitTroopsItem,
} from '../RecruitTroopsClientState';
import {
  formatMinuteOfDay,
  normalizeFeatureSchedules,
  WEEK_DAYS,
  type WeeklySchedule,
} from '../SchedulerTypes';
import { useCitadelAPI } from '../../api/ApiContext';
import { configurationSection } from '../Configuration';
import { castleOptionsFromState, type CastleOptionV2 } from '../../api/StateAdapters';
import {
  buildQueueableProductionCatalog,
  queueableBuildingRowsLoaded,
  queueableCastleEligible,
  queueableIDsForCastle,
  queueableIDsForCastles,
} from '../QueueableProductionCatalog';

interface RecruitTroopsSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
}

type RecruitItemScope = { type: 'global' } | { type: 'castle'; castleId: string };

export const RecruitTroopsSettingsModal: React.FC<RecruitTroopsSettingsModalProps> = ({ isOpen, onClose, onOpenFeatureSchedule }) => {
  const { configuration, state } = useCitadelAPI();
  const { getTroop, buildings, troops, tools, isLoading: metadataLoading } = useMetadata();
  const castles = castleOptionsFromState(state);
  const [settings, setSettings] = useState<RecruitTroopsClientSettingsV1>(() => loadRecruitTroopsSettingsFromStorage());
  const featureSchedules = normalizeFeatureSchedules(
    configurationSection(configuration, 'scheduler').featureSchedules,
  );
  const queueableCatalog = buildQueueableProductionCatalog(state, buildings, troops, tools);
  const queueableCatalogLoaded = state != null && !metadataLoading;
  const [isSaving, setIsSaving] = useState(false);

  const [editingUnit, setEditingUnit] = useState<{ scope: RecruitItemScope, item: RecruitTroopsItem } | null>(null);
  useEffect(() => {
    if (isOpen) {
      setSettings(normalizeRecruitTroopsSettings(
        configuration?.sections['automation.recruitTroops'] ?? loadRecruitTroopsSettingsFromStorage(),
      ));
    }
  }, [configuration?.sections, isOpen]);

  useEffect(() => {
    if (!isOpen) return;
    notifyRecruitTroopsSettingsChanged(settings);
  }, [isOpen, settings.mode]);

  const currentItemsForScope = (scope: RecruitItemScope) => {
    if (scope.type === 'global') return settings.globalItems;
    return settings.castles[scope.castleId]?.items ?? [];
  };

  const allowedUnitIdsForScope = (scope: RecruitItemScope): number[] | undefined => {
    if (!queueableCatalogLoaded) return undefined;
    if (scope.type === 'castle') {
      if (!queueableBuildingRowsLoaded(queueableCatalog, scope.castleId)) return undefined;
      return queueableIDsForCastle(queueableCatalog, scope.castleId, 'recruitUnitIds');
    }

    const knownCastleIDs = eligibleCastles
      .map((castle) => castle.id)
      .filter((castleID) => queueableBuildingRowsLoaded(queueableCatalog, castleID));
    const enabledCastleIDs = knownCastleIDs.filter((castleID) => settings.castles[String(castleID)]?.enabled);
    if (enabledCastleIDs.length > 0) {
      return queueableIDsForCastles(queueableCatalog, enabledCastleIDs, 'recruitUnitIds', 'intersection');
    }
    if (knownCastleIDs.length > 0) {
      return queueableIDsForCastles(queueableCatalog, knownCastleIDs, 'recruitUnitIds', 'union');
    }
    return undefined;
  };

  const updateItemsForScope = (scope: RecruitItemScope, items: RecruitTroopsItem[]) => {
    setSettings(prev => {
      if (scope.type === 'global') {
        return { ...prev, globalItems: items };
      }
      const castleSettings = prev.castles[scope.castleId] ?? { enabled: true, items: [] };
      return {
        ...prev,
        castles: {
          ...prev.castles,
          [scope.castleId]: {
            ...castleSettings,
            items,
          },
        },
      };
    });
  };

  const handleAddItem = async (scope: RecruitItemScope, title: string) => {
    const currentItems = currentItemsForScope(scope);

    const result = await showTroopPicker({
      mode: 'single',
      title,
      preselected: currentItems[0]?.id ? [currentItems[0].id] : [],
      allowedUnitIds: allowedUnitIdsForScope(scope),
    });

    if (typeof result === 'number') {
      updateItemsForScope(scope, [{ id: result, amount: 0 }]);
    }
  };

  const openEditModal = (scope: RecruitItemScope, item: RecruitTroopsItem) => {
    setEditingUnit({ scope, item });
  };

  const closeEditModal = () => {
    setEditingUnit(null);
  };

  const saveEditModal = () => {
    closeEditModal();
  };

  const deleteFromEditModal = () => {
    if (!editingUnit) return;

    updateItemsForScope(
      editingUnit.scope,
      currentItemsForScope(editingUnit.scope).filter(item => item.id !== editingUnit.item.id),
    );
    closeEditModal();
  };

  const updateCastleEnabled = (castleId: string, enabled: boolean) => {
    setSettings(prev => {
      const castleSettings = prev.castles[castleId] ?? { enabled: false, items: [] };
      return {
        ...prev,
        castles: {
          ...prev.castles,
          [castleId]: {
            ...castleSettings,
            enabled,
          },
        },
      };
    });
  };

  const updateMode = (mode: 'global' | 'perCastle') => {
    setSettings(prev => {
      if (prev.mode === mode) return prev;
      if (mode === 'global' && prev.globalItems.length === 0) {
        const firstCastleWithItems = Object.values(prev.castles).find(castle => castle.items.length > 0);
        return {
          ...prev,
          mode,
          globalItems: firstCastleWithItems?.items ?? prev.globalItems,
        };
      }
      if (mode === 'perCastle' && prev.globalItems.length > 0) {
        const castlesWithInheritedTargets = { ...prev.castles };
        Object.entries(castlesWithInheritedTargets).forEach(([castleId, castleSettings]) => {
          if (castleSettings.items.length > 0) return;
          castlesWithInheritedTargets[castleId] = {
            ...castleSettings,
            items: prev.globalItems,
          };
        });
        return {
          ...prev,
          mode,
          castles: castlesWithInheritedTargets,
        };
      }
      return { ...prev, mode };
    });
  };

  const updateCheckIntervalMinutes = (value: string) => {
    const raw = value.replace(/,/g, '');
    if (!/^\d*$/.test(raw)) return;
    setSettings(prev => ({
      ...prev,
      checkIntervalSec: recruitCheckIntervalMinutesToSec(raw === '' ? DEFAULT_RECRUIT_CHECK_INTERVAL_MIN : Number(raw)),
    }));
  };

  const handleSave = () => {
    setIsSaving(true);
    const sent = persistRecruitTroopsSettings(settings);
    setIsSaving(false);
    if (sent) onClose();
  };

  const handleClose = () => {
    notifyRecruitTroopsSettingsChanged(loadRecruitTroopsSettingsFromStorage());
    onClose();
  };

  const eligibleCastles = queueableCatalogLoaded
    ? castles.filter((castle) => queueableCastleEligible(queueableCatalog, castle.id, 'recruitUnitIds'))
    : castles;
  const enabledCastleCount = eligibleCastles.filter(castle => settings.castles[castle.id.toString()]?.enabled).length;
  const isGlobalMode = settings.mode === 'global';
  const autoRecruitSchedule = featureSchedules.autoRecruit;
  const autoRecruitScheduleEnabled = !!autoRecruitSchedule?.enabled;
  const autoRecruitUsesScheduledUnits = !!(autoRecruitScheduleEnabled && autoRecruitSchedule?.slotOptionsEnabled);
  const shouldShowSharedUnitPanel = isGlobalMode && !autoRecruitUsesScheduledUnits;
  const globalHasUnitSourcePanel = shouldShowSharedUnitPanel || autoRecruitUsesScheduledUnits;

  const renderRecruitUnits = (
    items: RecruitTroopsItem[],
    scope: RecruitItemScope,
    addLabel: string,
    addTitle: string,
  ) => {
    if (items.length === 0) {
      return (
        <div className="flex min-h-[6.75rem] flex-col items-center justify-center rounded-global border border-dashed border-border-base bg-bg-card/45 p-5 text-center">
          <div className="mb-3 text-xs font-bold uppercase tracking-wider text-text-muted/70">
            No recruit units
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => handleAddItem(scope, addTitle)}
            leftIcon={<Plus className="h-4 w-4" />}
          >
            {addLabel}
          </Button>
        </div>
      );
    }

    return (
      <div className="flex flex-wrap gap-4 content-start">
        {items.map((item) => (
          <button
            key={`${scope.type}-${scope.type === 'castle' ? scope.castleId : 'global'}-${item.id}`}
            type="button"
            className="group/unit relative flex w-[5.75rem] flex-col items-center gap-2 rounded-global border border-border-base bg-bg-card/70 p-3 text-center shadow-sm transition-transform hover:-translate-y-1 hover:border-primary/45"
            title={getTroop(item.id)?.name || 'Unknown Unit'}
            onClick={() => openEditModal(scope, item)}
          >
            <UnitImage unitId={item.id} size={66} showLevel={true} className="rounded-xl" />
            <span className="line-clamp-2 min-h-[2rem] text-xs font-bold leading-tight text-text-main">
              {getTroop(item.id)?.name || `Unit #${item.id}`}
            </span>
            <span className="pointer-events-none absolute inset-0 flex items-center justify-center rounded-global bg-black/30 opacity-0 transition-opacity group-hover/unit:opacity-100">
              <Settings className="h-5 w-5 text-white drop-shadow-md" />
            </span>
          </button>
        ))}
        <button
          type="button"
          onClick={() => handleAddItem(scope, addTitle)}
          className="flex min-h-[9.35rem] w-[5.75rem] flex-col items-center justify-center gap-2 rounded-global border-2 border-dashed border-border-base bg-bg-card/45 text-xs font-bold uppercase tracking-wide text-text-muted transition-colors hover:border-primary hover:bg-primary/5 hover:text-primary"
          title={addLabel}
        >
          <Plus className="h-5 w-5" />
          Select
        </button>
      </div>
    );
  };

  const unitIDFromScheduleSlot = (slot: WeeklySchedule['slots'][number]) => {
    const raw = slot.options?.unitID;
    if (typeof raw === 'number' && Number.isFinite(raw) && raw > 0) return raw;
    if (typeof raw === 'string') {
      const parsed = Number(raw);
      if (Number.isFinite(parsed) && parsed > 0) return parsed;
    }
    return null;
  };

  const renderScheduleSlots = (
    schedule: WeeklySchedule,
    options: {
      title?: string;
      description: string;
      editTitle?: string;
      onEdit: () => void;
      hideEditButton?: boolean;
      fallbackUnitID?: number | null;
      emptyUnitLabel?: string;
      emptySlotsLabel?: string;
    },
  ) => {
    const visibleSlots = schedule.slots.slice(0, 5);
    const hiddenCount = Math.max(0, schedule.slots.length - visibleSlots.length);

    return (
      <div className="rounded-global border border-primary/20 bg-primary/5 p-3">
        <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
          <div className="min-w-0">
            <div className="text-xs font-bold uppercase tracking-wide text-primary">
              {options.title ?? 'Scheduled Units'}
            </div>
            <p className="mt-1 text-[11px] font-semibold text-text-muted">
              {options.description}
            </p>
          </div>
          {!options.hideEditButton && (
            <Button
              variant="outline"
              size="sm"
              onClick={options.onEdit}
              title={options.editTitle}
              leftIcon={<CalendarDays className="h-4 w-4" />}
            >
              Edit
            </Button>
          )}
        </div>

        {visibleSlots.length === 0 ? (
          <div className="rounded-global border border-dashed border-border-base bg-bg-card/45 px-4 py-3 text-xs font-semibold text-text-muted">
            {options.emptySlotsLabel ?? 'No scheduled unit slots'}
          </div>
        ) : (
          <div className="grid gap-2">
            {visibleSlots.map((slot) => {
              const unitID = unitIDFromScheduleSlot(slot) ?? options.fallbackUnitID ?? null;
              const unitName = unitID
                ? getTroop(unitID)?.name || `Unit #${unitID}`
                : options.emptyUnitLabel ?? 'No unit selected';
              const day = WEEK_DAYS[slot.day]?.short ?? 'Day';
              return (
                <div
                  key={slot.id}
                  className="flex min-w-0 items-center gap-3 rounded-global border border-border-base bg-bg-card/65 px-3 py-2"
                  title={`${day} ${formatMinuteOfDay(slot.startMinute)}-${formatMinuteOfDay(slot.endMinute)} · ${unitName}`}
                >
                  {unitID ? (
                    <UnitImage unitId={unitID} size={34} showLevel={true} className="shrink-0 rounded-lg" />
                  ) : (
                    <span className="flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-lg border border-border-base bg-bg-card text-xs font-black text-text-muted">
                      -
                    </span>
                  )}
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-xs font-bold text-text-main">{unitName}</div>
                    <div className="mt-0.5 text-[11px] font-semibold text-text-muted">
                      {day} {formatMinuteOfDay(slot.startMinute)}-{formatMinuteOfDay(slot.endMinute)}
                    </div>
                  </div>
                </div>
              );
            })}
            {hiddenCount > 0 && (
              <div className="rounded-global border border-border-base bg-bg-card/45 px-3 py-2 text-center text-[11px] font-bold uppercase tracking-wide text-text-muted">
                +{hiddenCount} more slot{hiddenCount === 1 ? '' : 's'}
              </div>
            )}
          </div>
        )}
      </div>
    );
  };

  const renderSharedUnitPanel = (className = '') => {
    const sharedUnitID = settings.globalItems[0]?.id ?? null;

    return (
      <Card variant="solid" className={`liquid-prominent-header-card flex flex-col ${className}`}>
        <CardHeader className="liquid-card-header-prominent flex flex-row items-center justify-between gap-3">
          <div className="min-w-0">
            <CardTitle className="text-base">Shared Unit</CardTitle>
            <p className="mt-1 text-xs text-text-muted">
              {autoRecruitScheduleEnabled
                ? 'Calendar windows control when this shared unit is queued.'
                : 'Shared unit can be overridden by the Auto Recruit calendar.'}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {autoRecruitScheduleEnabled && (
              <Badge variant="primary">
                {autoRecruitSchedule?.slots.length ?? 0} slot{autoRecruitSchedule?.slots.length === 1 ? '' : 's'}
              </Badge>
            )}
            <Button
              variant="outline"
              size="sm"
              onClick={() => onOpenFeatureSchedule('autoRecruit', 'Auto Recruit')}
              leftIcon={<CalendarDays className="h-4 w-4" />}
            >
              Calendar
            </Button>
          </div>
        </CardHeader>
        <CardContent className="liquid-prominent-header-content flex flex-1 flex-col gap-4 p-5">
          {renderRecruitUnits(
            settings.globalItems,
            { type: 'global' },
            'Select unit',
            'Select shared recruit unit',
          )}
          {autoRecruitScheduleEnabled && autoRecruitSchedule && (
            renderScheduleSlots(autoRecruitSchedule, {
              title: 'Scheduled Windows',
              description: 'All enabled castles queue the shared unit during these periods.',
              editTitle: 'Auto Recruit calendar',
              onEdit: () => onOpenFeatureSchedule('autoRecruit', 'Auto Recruit'),
              hideEditButton: true,
              fallbackUnitID: sharedUnitID,
              emptyUnitLabel: 'Shared unit not selected',
              emptySlotsLabel: 'No scheduled windows',
            })
          )}
        </CardContent>
      </Card>
    );
  };

  const renderCastleScheduleSlots = (schedule: WeeklySchedule, castle: CastleOptionV2) => renderScheduleSlots(schedule, {
    description: 'Unit choices come from this castle\'s calendar slots.',
    editTitle: `${castle.name} Auto Recruit schedule`,
    onEdit: () => onOpenFeatureSchedule(recruitCastleScheduleID(castle.id), `Auto Recruit - ${castle.name}`),
  });

  const renderGlobalSchedulePanel = (schedule: WeeklySchedule, className = '') => (
    <Card variant="solid" className={`liquid-prominent-header-card flex flex-col ${className}`}>
      <CardHeader className="liquid-card-header-prominent flex flex-row items-center justify-between gap-3">
        <div className="min-w-0">
          <CardTitle className="flex items-center gap-2 text-base">
            <CalendarDays className="h-4 w-4 text-primary" />
            Shared Schedule
          </CardTitle>
          <p className="mt-1 text-xs text-text-muted">Scheduled units replace the shared unit picker.</p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          <Badge variant="primary">
            {schedule.slots.length} slot{schedule.slots.length === 1 ? '' : 's'}
          </Badge>
          <Button
            variant="outline"
            size="sm"
            onClick={() => onOpenFeatureSchedule('autoRecruit', 'Auto Recruit')}
            leftIcon={<CalendarDays className="h-4 w-4" />}
          >
            Calendar
          </Button>
        </div>
      </CardHeader>
      <CardContent className="liquid-prominent-header-content flex-1 p-5">
        {renderScheduleSlots(schedule, {
          title: 'Time Slots',
          description: 'All enabled castles use these calendar unit slots.',
          editTitle: 'Auto Recruit calendar',
          onEdit: () => onOpenFeatureSchedule('autoRecruit', 'Auto Recruit'),
          hideEditButton: true,
        })}
      </CardContent>
    </Card>
  );

  const renderGlobalUnitSourcePanel = (className = '') => {
    if (autoRecruitUsesScheduledUnits && autoRecruitSchedule) {
      return renderGlobalSchedulePanel(autoRecruitSchedule, className);
    }
    if (shouldShowSharedUnitPanel) {
      return renderSharedUnitPanel(className);
    }
    return null;
  };

  const renderNoCastlesPanel = (className = '') => (
    <Card variant="solid" className={`liquid-prominent-header-card flex flex-col ${className}`}>
      <CardHeader className="liquid-card-header-prominent flex flex-row items-center justify-between gap-3">
        <div className="min-w-0">
          <CardTitle className="flex items-center gap-2 text-base">
            <Castle className="h-4 w-4 text-primary" />
            Castle Coverage
          </CardTitle>
          <p className="mt-1 text-xs text-text-muted">Enabled castles will appear here after game data refresh.</p>
        </div>
        <Badge variant="secondary" className="shrink-0">No data</Badge>
      </CardHeader>
      <CardContent className="liquid-prominent-header-content flex flex-1 p-4">
        <div className="flex min-h-[6.75rem] w-full items-center gap-4 rounded-global border border-dashed border-border-base bg-bg-card/35 p-4">
          <div className="hidden h-10 w-10 shrink-0 place-items-center rounded-global border border-primary/20 bg-primary/10 text-primary sm:grid">
            <Castle className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <div className="text-xs font-bold uppercase tracking-wider text-text-muted">No castles available</div>
            <p className="mt-2 max-w-sm text-sm leading-relaxed text-text-muted">
              Connect and refresh castle data before configuring Auto Recruit, or verify the castle has a barracks.
            </p>
          </div>
        </div>
      </CardContent>
    </Card>
  );

  const renderGlobalCastleTogglePanel = (className = '') => (
    <Card variant="solid" className={`liquid-prominent-header-card flex flex-col ${className}`}>
      <CardHeader className="liquid-card-header-prominent flex flex-row items-center justify-between gap-3">
        <div className="min-w-0">
          <CardTitle className="flex items-center gap-2 text-base">
            <Castle className="h-4 w-4 text-primary" />
            Castle Toggles
          </CardTitle>
          <p className="mt-1 text-xs text-text-muted">Enable Auto Recruit coverage for each castle.</p>
        </div>
        <Badge variant="primary" className="shrink-0">
          {enabledCastleCount}/{eligibleCastles.length}
        </Badge>
      </CardHeader>
      <CardContent className="liquid-prominent-header-content flex flex-1 flex-col gap-2 p-4">
        {eligibleCastles.map((castle) => {
          const castleId = castle.id.toString();
          const castleSettings = settings.castles[castleId] ?? { enabled: false, items: [] };

          return (
            <div
              key={castle.id}
              className={`flex min-w-0 items-center justify-between gap-4 rounded-global border px-4 py-3 transition-colors ${
                castleSettings.enabled
                  ? 'border-primary/25 bg-primary/5'
                  : 'border-border-base bg-bg-card/45'
              }`}
            >
              <div className="min-w-0">
                <div className="truncate text-sm font-bold text-text-main">{castle.name}</div>
                <div className="mt-1 truncate text-[11px] font-semibold uppercase tracking-wide text-text-muted">
                  {castle.type} · #{castle.id}
                </div>
              </div>
              <div className="flex shrink-0 items-center gap-3">
                <Badge variant={castleSettings.enabled ? 'success' : 'outline'}>
                  {castleSettings.enabled ? 'On' : 'Off'}
                </Badge>
                <Switch
                  checked={castleSettings.enabled}
                  onChange={(checked) => updateCastleEnabled(castleId, checked)}
                />
              </div>
            </div>
          );
        })}
      </CardContent>
    </Card>
  );

  return (
    <>
      <Modal
        isOpen={isOpen}
        onClose={handleClose}
        maxWidth={isGlobalMode ? '6xl' : 'full'}
        title={
          <div className="scheduler-modal-title">
            <span className="scheduler-modal-title-mark" aria-hidden="true">
              <Settings className="h-5 w-5" />
            </span>
            <span className="flex min-w-0 flex-col">
              <span className="scheduler-modal-title-text">Recruit Troops Settings</span>
              <span className="mt-1 text-xs font-semibold text-text-muted">
                Queue slots, schedules, and castle coverage
              </span>
            </span>
          </div>
        }
        footer={
          <>
            <Button variant="ghost" onClick={handleClose} className="px-6">Cancel</Button>
            <Button variant="primary" onClick={handleSave} disabled={isSaving} className="px-8" leftIcon={<Save className="w-4 h-4" />}>
              {isSaving ? 'Saving...' : 'Save Settings'}
            </Button>
          </>
        }
      >
        <div className={`recruit-modal-shell mx-auto flex w-full flex-col gap-5 overflow-visible pb-2 ${isGlobalMode ? 'max-w-6xl' : 'max-w-[min(1840px,98vw)]'}`}>
          <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(17rem,0.8fr)_minmax(22rem,1.15fr)_minmax(11rem,0.5fr)]">
            <Card variant="solid" className="liquid-prominent-header-card">
              <CardHeader className="liquid-card-header-prominent">
                <div>
                  <CardTitle className="flex items-center gap-2 text-base">
                    <Clock3 className="h-4 w-4 text-primary" />
                    Queue Check
                  </CardTitle>
                  <p className="mt-1 text-xs text-text-muted">Minutes between castle cycles.</p>
                </div>
              </CardHeader>
              <CardContent className="liquid-prominent-header-content p-5">
                <Input
                  type="text"
                  value={recruitCheckIntervalSecToMinutes(settings.checkIntervalSec).toLocaleString()}
                  onChange={(e) => updateCheckIntervalMinutes(e.target.value)}
                  className="font-mono text-lg font-black tabular-nums"
                  rightIcon={<span className="text-xs font-bold uppercase text-text-muted">min</span>}
                />
                <p className="mt-2 text-[11px] font-medium text-text-muted">
                  Minimum {MIN_RECRUIT_CHECK_INTERVAL_MIN.toLocaleString()} minute. Default is {DEFAULT_RECRUIT_CHECK_INTERVAL_MIN.toLocaleString()} minutes.
                </p>
              </CardContent>
            </Card>

            <Card variant="solid" className="liquid-prominent-header-card">
              <CardHeader className="liquid-card-header-prominent">
                <div>
                  <CardTitle className="text-base">Recruit Mode</CardTitle>
                  <p className="mt-1 text-xs text-text-muted">
                    Choose one shared recruit list or separate lists and schedules per castle.
                  </p>
                </div>
              </CardHeader>
              <CardContent className="liquid-prominent-header-content p-5">
                <PillSelector
                  value={settings.mode}
                  onChange={(value) => updateMode(value === 'perCastle' ? 'perCastle' : 'global')}
                  options={[
                    {
                      value: 'global',
                      label: 'Same across all Castles',
                      icon: <Copy className="h-4 w-4" />,
                      title: 'Use one recruit unit for all enabled castles',
                    },
                    {
                      value: 'perCastle',
                      label: 'Specific per Castle',
                      icon: <Settings className="h-4 w-4" />,
                      title: 'Use separate recruit units and schedules per castle',
                    },
                  ]}
                  fullWidth
                />
              </CardContent>
            </Card>

            <Card variant="solid" className="liquid-prominent-header-card">
              <CardHeader className="liquid-card-header-prominent">
                <div>
                  <CardTitle className="text-base">Enabled</CardTitle>
                  <p className="mt-1 text-xs text-text-muted">Castles selected for Auto Recruit.</p>
                </div>
              </CardHeader>
              <CardContent className="liquid-prominent-header-content flex flex-wrap items-center justify-between gap-4 p-5">
                <div className="text-4xl font-black tabular-nums text-primary">{enabledCastleCount}</div>
                <Badge variant={isGlobalMode ? 'primary' : 'secondary'}>
                  {isGlobalMode ? 'shared' : 'per castle'}
                </Badge>
              </CardContent>
            </Card>
          </div>

          {isGlobalMode ? (
            eligibleCastles.length === 0 ? (
              globalHasUnitSourcePanel ? (
                <div className="grid gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(18rem,0.85fr)]">
                  {renderGlobalUnitSourcePanel('h-full')}
                  {renderNoCastlesPanel('h-full')}
                </div>
              ) : (
                renderNoCastlesPanel()
              )
            ) : (
              <div className={`grid gap-4 ${globalHasUnitSourcePanel ? 'lg:grid-cols-[minmax(0,1fr)_minmax(20rem,0.75fr)]' : ''}`}>
                {renderGlobalUnitSourcePanel('h-full')}
                {renderGlobalCastleTogglePanel('h-full')}
              </div>
            )
          ) : eligibleCastles.length === 0 ? (
            renderNoCastlesPanel()
          ) : (
            <>
              <div className="grid w-full auto-rows-max grid-cols-1 gap-5 lg:grid-cols-2 2xl:grid-cols-3">
                {eligibleCastles.map((castle) => {
                  const castleId = castle.id.toString();
                  const castleScheduleID = recruitCastleScheduleID(castle.id);
                  const castleSchedule = featureSchedules[castleScheduleID];
                  const scheduledUnitSchedule = !isGlobalMode && castleSchedule?.enabled && castleSchedule.slotOptionsEnabled ? castleSchedule : null;
                  const castleUsesScheduledUnits = !!scheduledUnitSchedule;
                  const castleSettings = settings.castles[castleId] ?? { enabled: false, items: [] };
                  const displayedItems = isGlobalMode ? [] : castleSettings.items;
                  const hasItems = displayedItems.length > 0;

                  return (
                    <Card
                      key={castle.id}
                      variant="solid"
                      className={`liquid-prominent-header-card flex min-h-0 flex-col ${
                        castleSettings.enabled ? 'border-primary/35 shadow-[0_0_24px_-12px_rgba(52,211,153,0.55)]' : ''
                      }`}
                    >
                      <CardHeader className="liquid-card-header-prominent flex flex-row items-center justify-between gap-4">
                        <div className="min-w-0">
                          <div className="flex flex-wrap items-center gap-2">
                            <CardTitle className="min-w-0 truncate text-lg text-text-main">{castle.name}</CardTitle>
                            <Badge variant={castleSettings.enabled ? 'success' : 'outline'}>
                              {castleSettings.enabled ? 'Enabled' : 'Disabled'}
                            </Badge>
                          </div>
                          <p className="mt-1 truncate text-xs font-semibold uppercase tracking-wide text-text-muted">
                            {castle.type} · #{castle.id}
                          </p>
                        </div>
                        <div className="flex shrink-0 items-center gap-2">
                          {!isGlobalMode && (
                            <Button
                              variant="ghost"
                              size="icon"
                              onClick={() => onOpenFeatureSchedule(castleScheduleID, `Auto Recruit - ${castle.name}`)}
                              title={`${castle.name} Auto Recruit schedule`}
                            >
                              <CalendarDays className="h-4 w-4" />
                            </Button>
                          )}
                          <Switch
                            checked={castleSettings.enabled}
                            onChange={(checked) => updateCastleEnabled(castleId, checked)}
                          />
                        </div>
                      </CardHeader>

                      <CardContent className="liquid-prominent-header-content flex flex-1 flex-col gap-4 p-5">
                        <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-text-muted">
                          <span>
                            {isGlobalMode
                              ? 'Uses shared unit'
                              : castleUsesScheduledUnits
                                ? 'Uses scheduled unit slots'
                                : 'Uses castle-specific recruit units'}
                          </span>
                          {scheduledUnitSchedule ? (
                            <Badge variant="primary">
                              {scheduledUnitSchedule.slots.length} slot{scheduledUnitSchedule.slots.length !== 1 ? 's' : ''}
                            </Badge>
                          ) : hasItems && (
                            <Badge variant="primary">
                              {displayedItems.length} unit{displayedItems.length !== 1 ? 's' : ''}
                            </Badge>
                          )}
                        </div>
                        {isGlobalMode ? (
                          <div className="rounded-global border border-border-base bg-bg-card/55 px-4 py-3 text-xs font-semibold text-text-main">
                            Shared unit
                          </div>
                        ) : scheduledUnitSchedule ? (
                          renderCastleScheduleSlots(scheduledUnitSchedule, castle)
                        ) : (
                          renderRecruitUnits(
                            castleSettings.items,
                            { type: 'castle', castleId },
                            'Select unit',
                            `Select recruit unit - ${castle.name}`,
                          )
                        )}
                      </CardContent>
                    </Card>
                  );
                })}
              </div>
            </>
          )}
        </div>
      </Modal>

      <Modal
        isOpen={!!editingUnit}
        onClose={closeEditModal}
        maxWidth="sm"
        title={editingUnit ? getTroop(editingUnit.item.id)?.name || 'Unit' : 'Unit'}
        footer={
          <>
            <Button variant="danger" onClick={deleteFromEditModal} leftIcon={<Trash2 className="w-4 h-4" />}>Remove</Button>
            <Button variant="primary" onClick={saveEditModal} className="flex-[2]">Done</Button>
          </>
        }
      >
        <div className="flex flex-col items-center gap-6 py-4">
          {editingUnit && (
            <UnitImage unitId={editingUnit.item.id} size={80} showLevel={true} className="rounded-2xl shadow-lg" />
          )}

        </div>
      </Modal>
    </>
  );
};
