import React, { useState, useEffect, useRef } from 'react';
import { CalendarDays, Castle, Clock3, Copy, Trash2, Plus, Settings } from 'lucide-react';
import { showTroopPicker } from '../../components/TroopPickerModal';
import { showToolPicker } from '../../components/ToolPickerModal';
import UnitImage from '../../components/UnitImage';
import ToolImage from '../../components/ToolImage';
import { useMetadata } from '../../context/MetadataContext';
import { Modal, Button, Input, Card, CardHeader, CardTitle, CardContent, Badge, Switch, PillSelector, SectionCard, SettingsModal } from '../../components/ui';
import {
  DEFAULT_RECRUIT_CHECK_INTERVAL_MIN,
  MIN_RECRUIT_CHECK_INTERVAL_MIN,
  defaultRecruitTroopsSettings,
  normalizeRecruitTroopsSettings,
  persistRecruitTroopsSettings,
  recruitCheckIntervalMinutesToSec,
  recruitCheckIntervalSecToMinutes,
  recruitCastleScheduleID,
} from '../RecruitTroopsClientState';
import {
  DEFAULT_AUTO_TOOL_CHECK_INTERVAL_MIN,
  MIN_AUTO_TOOL_CHECK_INTERVAL_MIN,
  autoToolCheckIntervalMinutesToSec,
  autoToolCheckIntervalSecToMinutes,
  autoToolCastleScheduleID,
  defaultAutoToolSettings,
  normalizeAutoToolSettings,
  persistAutoToolSettings,
} from '../AutoToolClientState';
import {
  applyQueueProductionCastleIdentityMetadata,
  queueProductionCastleConfigurationKey,
  queueProductionKnownStormCastleIDs,
  type QueueProductionClientSettingsV1,
  type QueueProductionItem,
} from '../QueueProductionClientState';
import {
  formatMinuteOfDay,
  normalizeFeatureSchedules,
  WEEK_DAYS,
  type WeeklySchedule,
} from '../SchedulerTypes';
import { useCitadelAPI } from '../../api/ApiContext';
import { configurationSection } from '../Configuration';
import { castleOptionsFromState, type CastleOptionV2 } from '../../api/Selectors';
import {
  buildQueueableProductionCatalog,
  queueableBuildingRowsLoaded,
  queueableCastleEligible,
  queueableIDsForCastle,
  queueableIDsForCastles,
  type QueueableProductionField,
} from '../QueueableProductionCatalog';
import {
  highestAvailableUnitIDInFamily,
  highestUnitIDsByFamily,
  unitIDsAvailableByFamilyAcrossCastles,
  unitUpgradeFamily,
} from '../UnitUpgradeFamily';

export interface QueueProductionSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
  kind: 'recruit' | 'tool';
}

interface QueueProductionDefinition {
  kind: 'recruit' | 'tool';
  configurationSection: 'automation.recruitTroops' | 'automation.autoTool';
  featureID: 'autoRecruit' | 'autoTool';
  featureLabel: 'Auto Recruit' | 'Auto Tool';
  settingsTitle: string;
  modeTitle: string;
  modeDescription: string;
  queueField: QueueableProductionField;
  scheduleOptionKey: 'unitID' | 'toolID';
  itemLabel: 'unit' | 'tool';
  itemLabelPlural: 'units' | 'tools';
  itemFallbackLabel: 'Unit' | 'Tool';
  emptyItemsLabel: string;
  globalPickerTitle: string;
  castleSpecificLabel: string;
  noCastlesHelp: string;
  defaultCheckIntervalMin: number;
  minCheckIntervalMin: number;
  defaultSettings: () => QueueProductionClientSettingsV1;
  normalizeSettings: (raw: unknown) => QueueProductionClientSettingsV1;
  persistSettings: (settings: QueueProductionClientSettingsV1) => Promise<unknown>;
  checkIntervalMinutesToSec: (value: number) => number;
  checkIntervalSecToMinutes: (value: number) => number;
  castleScheduleID: (castleID: number | string) => string;
}

const DEFINITIONS: Record<QueueProductionSettingsModalProps['kind'], QueueProductionDefinition> = {
  recruit: {
    kind: 'recruit',
    configurationSection: 'automation.recruitTroops',
    featureID: 'autoRecruit',
    featureLabel: 'Auto Recruit',
    settingsTitle: 'Recruit Troops Settings',
    modeTitle: 'Recruit Mode',
    modeDescription: 'Choose one shared recruit list or separate lists and schedules per castle.',
    queueField: 'recruitUnitIds',
    scheduleOptionKey: 'unitID',
    itemLabel: 'unit',
    itemLabelPlural: 'units',
    itemFallbackLabel: 'Unit',
    emptyItemsLabel: 'No recruit units',
    globalPickerTitle: 'Select shared recruit unit',
    castleSpecificLabel: 'castle-specific recruit units',
    noCastlesHelp: 'Connect and refresh castle data before configuring Auto Recruit, or verify the castle has a barracks.',
    defaultCheckIntervalMin: DEFAULT_RECRUIT_CHECK_INTERVAL_MIN,
    minCheckIntervalMin: MIN_RECRUIT_CHECK_INTERVAL_MIN,
    defaultSettings: defaultRecruitTroopsSettings,
    normalizeSettings: normalizeRecruitTroopsSettings,
    persistSettings: persistRecruitTroopsSettings,
    checkIntervalMinutesToSec: recruitCheckIntervalMinutesToSec,
    checkIntervalSecToMinutes: recruitCheckIntervalSecToMinutes,
    castleScheduleID: recruitCastleScheduleID,
  },
  tool: {
    kind: 'tool',
    configurationSection: 'automation.autoTool',
    featureID: 'autoTool',
    featureLabel: 'Auto Tool',
    settingsTitle: 'Auto Tool Settings',
    modeTitle: 'Tool Mode',
    modeDescription: 'Choose one shared tool list or separate lists and schedules per castle.',
    queueField: 'toolIds',
    scheduleOptionKey: 'toolID',
    itemLabel: 'tool',
    itemLabelPlural: 'tools',
    itemFallbackLabel: 'Tool',
    emptyItemsLabel: 'No tools',
    globalPickerTitle: 'Select shared tool',
    castleSpecificLabel: 'castle-specific tools',
    noCastlesHelp: 'Connect and refresh castle data before configuring Auto Tool, or verify the castle has a siege or defense workshop.',
    defaultCheckIntervalMin: DEFAULT_AUTO_TOOL_CHECK_INTERVAL_MIN,
    minCheckIntervalMin: MIN_AUTO_TOOL_CHECK_INTERVAL_MIN,
    defaultSettings: defaultAutoToolSettings,
    normalizeSettings: normalizeAutoToolSettings,
    persistSettings: persistAutoToolSettings,
    checkIntervalMinutesToSec: autoToolCheckIntervalMinutesToSec,
    checkIntervalSecToMinutes: autoToolCheckIntervalSecToMinutes,
    castleScheduleID: autoToolCastleScheduleID,
  },
};

type ItemScope = { type: 'global' } | { type: 'castle'; castleId: string; liveCastleId: string };

export const QueueProductionSettingsModal: React.FC<QueueProductionSettingsModalProps> = ({
  isOpen,
  onClose,
  onOpenFeatureSchedule,
  kind,
}) => {
  const definition = DEFINITIONS[kind];
  const { configuration, state } = useCitadelAPI();
  const { getTroop, getTool, buildings, troops, tools, isLoading: metadataLoading } = useMetadata();
  const castles = castleOptionsFromState(state);
  const [settings, setSettings] = useState<QueueProductionClientSettingsV1>(() => definition.defaultSettings());
  const autoStormSettings = configurationSection(configuration, 'automation.autoStorm');
  const knownStormCastleIDs = queueProductionKnownStormCastleIDs(
    state?.storm.lastScannedAt,
    state?.storm.map.sourceCastleId,
    autoStormSettings.target,
  );
  const liveCastleIdentities = castles.map(({ id, kingdomId }) => ({ id, kingdomId }));
  const configurationKeyForCastle = (castle: CastleOptionV2) => queueProductionCastleConfigurationKey(
    settings,
    castle,
    liveCastleIdentities,
    knownStormCastleIDs,
  );
  const featureSchedules = normalizeFeatureSchedules(
    configurationSection(configuration, 'scheduler').featureSchedules,
  );
  const queueableCatalog = buildQueueableProductionCatalog(state, buildings, troops, tools);
  const queueableCatalogLoaded = state != null && !metadataLoading;
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);

  const [editingItem, setEditingItem] = useState<{ scope: ItemScope, item: QueueProductionItem } | null>(null);
  const loadedSettingsSignature = useRef<string | null>(null);
  useEffect(() => {
    if (!isOpen) {
      loadedSettingsSignature.current = null;
      setSaveError(null);
      return;
    }

    const rawSettings = configuration?.sections[definition.configurationSection] ?? definition.defaultSettings();
    const settingsSignature = `${definition.configurationSection}:${JSON.stringify(rawSettings)}`;
    // Saving a nested calendar replaces the full configuration snapshot. Preserve this
    // modal's unsaved mode and item edits unless its own persisted section changed.
    if (loadedSettingsSignature.current === settingsSignature) return;

    loadedSettingsSignature.current = settingsSignature;
    setSettings(definition.normalizeSettings(rawSettings));
  }, [configuration?.sections, definition, isOpen]);

  const itemName = (itemID: number) => (
    (kind === 'recruit' ? getTroop(itemID)?.name : getTool(itemID)?.name)
    || `${definition.itemFallbackLabel} #${itemID}`
  );

  const itemImage = (itemID: number, size: number, className: string) => (
    kind === 'recruit'
      ? <UnitImage unitId={itemID} size={size} showLevel className={className} />
      : <ToolImage toolId={itemID} size={size} showLevel className={className} />
  );

  const itemWithCurrentUnitRange = (itemID: number, current?: QueueProductionItem): QueueProductionItem => {
    const item: QueueProductionItem = { id: itemID, amount: current?.amount ?? 0 };
    if (kind !== 'recruit') return item;
    const family = troops[itemID] ? unitUpgradeFamily(itemID, troops) : null;
    if (family) {
      item.minId = family.minId;
      item.maxId = family.maxId;
    } else if (current?.minId || current?.maxId) {
      item.minId = current.minId;
      item.maxId = current.maxId;
    }
    return item;
  };

  const unitRangeLabel = (item: QueueProductionItem): string => {
    if (kind !== 'recruit') return '';
    const family = troops[item.id] ? unitUpgradeFamily(item.id, troops) : null;
    const minID = family?.minId ?? item.minId ?? item.id;
    const maxID = family?.maxId ?? item.maxId ?? item.id;
    return minID === maxID ? `Auto ID ${minID}` : `Auto IDs ${minID}-${maxID}`;
  };

  const currentItemForUnitFamily = (items: QueueProductionItem[], unitID: number) => {
    const exact = items.find((item) => item.id === unitID);
    if (exact || kind !== 'recruit') return exact;
    return items.find((item) => unitUpgradeFamily(item.id, troops)?.ids.includes(unitID));
  };

  const currentItemsForScope = (scope: ItemScope) => {
    if (scope.type === 'global') return settings.globalItems;
    return settings.castles[scope.castleId]?.items ?? [];
  };

  const allowedItemIDsForScope = (scope: ItemScope): number[] | undefined => {
    if (!queueableCatalogLoaded) return undefined;
    if (scope.type === 'castle') {
      if (!queueableBuildingRowsLoaded(queueableCatalog, scope.liveCastleId)) return undefined;
      const availableIDs = queueableIDsForCastle(queueableCatalog, scope.liveCastleId, definition.queueField);
      return kind === 'recruit' ? highestUnitIDsByFamily(availableIDs, troops) : availableIDs;
    }

    const knownCastleIDs = eligibleCastles
      .map((castle) => castle.id)
      .filter((castleID) => queueableBuildingRowsLoaded(queueableCatalog, castleID));
    const enabledCastleIDs = knownCastleIDs.filter((castleID) => {
      const castle = eligibleCastles.find((candidate) => candidate.id === castleID);
      return castle != null && settings.castles[configurationKeyForCastle(castle)]?.enabled;
    });
    if (enabledCastleIDs.length > 0) {
      if (kind === 'recruit') {
        return unitIDsAvailableByFamilyAcrossCastles(
          enabledCastleIDs.map((castleID) => queueableIDsForCastle(queueableCatalog, castleID, definition.queueField)),
          troops,
        );
      }
      return queueableIDsForCastles(queueableCatalog, enabledCastleIDs, definition.queueField, 'intersection');
    }
    if (knownCastleIDs.length > 0) {
      const availableIDs = queueableIDsForCastles(queueableCatalog, knownCastleIDs, definition.queueField, 'union');
      return kind === 'recruit' ? highestUnitIDsByFamily(availableIDs, troops) : availableIDs;
    }
    return undefined;
  };

  const updateItemsForScope = (scope: ItemScope, items: QueueProductionItem[]) => {
    setSettings(prev => {
      if (scope.type === 'global') {
        return { ...prev, globalItems: items };
      }
      const castleSettings = prev.castles[scope.castleId] ?? { enabled: true, items: [], cursor: 0 };
      const sameRotation = castleSettings.items.length === items.length
        && castleSettings.items.every((item, index) => (
          item.id === items[index]?.id
          && (item.amount ?? 0) === (items[index]?.amount ?? 0)
          && (item.minId ?? 0) === (items[index]?.minId ?? 0)
          && (item.maxId ?? 0) === (items[index]?.maxId ?? 0)
        ));
      return {
        ...prev,
        castles: {
          ...prev.castles,
          [scope.castleId]: {
            ...castleSettings,
            items,
            cursor: sameRotation ? castleSettings.cursor : 0,
          },
        },
      };
    });
  };

  const handleAddItem = async (scope: ItemScope, title: string) => {
    const currentItems = currentItemsForScope(scope);
    const allowedItemIDs = allowedItemIDsForScope(scope);
    const isPerCastleRecruitRotation = kind === 'recruit' && settings.mode === 'perCastle' && scope.type === 'castle';
    if (isPerCastleRecruitRotation) {
      const pickerPreselected = currentItems
        .map((item) => highestAvailableUnitIDInFamily(item.id, allowedItemIDs, troops))
        .filter((itemID): itemID is number => itemID != null);
      const result = await showTroopPicker({
        mode: 'multi',
        title,
        preselected: Array.from(new Set(pickerPreselected)),
        allowedUnitIds: allowedItemIDs,
      });
      if (!Array.isArray(result)) return;
      const allowed = allowedItemIDs == null ? null : new Set(allowedItemIDs);
      const selectedIDs = highestUnitIDsByFamily(
        result.filter((item): item is number => typeof item === 'number' && (!allowed || allowed.has(item))),
        troops,
      );
      updateItemsForScope(
        scope,
        selectedIDs.map((itemID) => itemWithCurrentUnitRange(
          itemID,
          currentItemForUnitFamily(currentItems, itemID),
        )),
      );
      return;
    }

    const currentItemID = currentItems[0]?.id;
    const preselectedItemID = kind === 'recruit' && currentItemID
      ? highestAvailableUnitIDInFamily(currentItemID, allowedItemIDs, troops)
      : currentItemID;
    const commonOptions = { mode: 'single' as const, title, preselected: preselectedItemID ? [preselectedItemID] : [] };
    const result = kind === 'recruit'
      ? await showTroopPicker({ ...commonOptions, allowedUnitIds: allowedItemIDs })
      : await showToolPicker({ ...commonOptions, allowedToolIds: allowedItemIDs });

    if (typeof result === 'number') {
      updateItemsForScope(scope, [itemWithCurrentUnitRange(result, currentItems[0])]);
    }
  };

  const openEditModal = (scope: ItemScope, item: QueueProductionItem) => {
    setEditingItem({ scope, item });
  };

  const closeEditModal = () => {
    setEditingItem(null);
  };

  const saveEditModal = () => {
    closeEditModal();
  };

  const deleteFromEditModal = () => {
    if (!editingItem) return;

    updateItemsForScope(
      editingItem.scope,
      currentItemsForScope(editingItem.scope).filter(item => item.id !== editingItem.item.id),
    );
    closeEditModal();
  };

  const updateCastleEnabled = (castleId: string, enabled: boolean) => {
    setSettings(prev => {
      const castleSettings = prev.castles[castleId] ?? { enabled: false, items: [], cursor: 0 };
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
      checkIntervalSec: definition.checkIntervalMinutesToSec(
        raw === '' ? definition.defaultCheckIntervalMin : Number(raw),
      ),
    }));
  };

  const handleSave = async () => {
    setIsSaving(true);
    setSaveError(null);
    const rangedSettings = kind === 'recruit'
      ? {
          ...settings,
          globalItems: settings.globalItems.map((item) => itemWithCurrentUnitRange(item.id, item)),
          castles: Object.fromEntries(Object.entries(settings.castles).map(([castleID, castle]) => [
            castleID,
            { ...castle, items: castle.items.map((item) => itemWithCurrentUnitRange(item.id, item)) },
          ])),
        }
      : settings;
    const nextSettings = applyQueueProductionCastleIdentityMetadata(
      rangedSettings,
      liveCastleIdentities,
      knownStormCastleIDs,
    );
    setSettings(nextSettings);
    try {
      await definition.persistSettings(nextSettings);
      onClose();
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : `Could not save ${definition.featureLabel} settings.`);
    } finally {
      setIsSaving(false);
    }
  };

  const handleClose = () => {
    if (!isSaving) onClose();
  };

  const eligibleCastles = queueableCatalogLoaded
    ? castles.filter((castle) => queueableCastleEligible(queueableCatalog, castle.id, definition.queueField))
    : castles;
  const enabledCastleCount = eligibleCastles.filter(
    (castle) => settings.castles[configurationKeyForCastle(castle)]?.enabled,
  ).length;
  const isGlobalMode = settings.mode === 'global';
  const globalSchedule = featureSchedules[definition.featureID];
  const globalScheduleEnabled = !!globalSchedule?.enabled;
  const globalUsesScheduledItems = !!(globalScheduleEnabled && globalSchedule?.slotOptionsEnabled);
  const shouldShowSharedItemPanel = isGlobalMode && !globalUsesScheduledItems;
  const globalHasItemSourcePanel = shouldShowSharedItemPanel || globalUsesScheduledItems;

  const renderItems = (
    items: QueueProductionItem[],
    scope: ItemScope,
    addLabel: string,
    addTitle: string,
  ) => {
    if (items.length === 0) {
      return (
        <div className="flex min-h-[6.75rem] flex-col items-center justify-center rounded-global border border-dashed border-border-base bg-bg-card/45 p-5 text-center">
          <div className="mb-3 text-xs font-bold uppercase tracking-wider text-text-muted/70">
            {definition.emptyItemsLabel}
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

    const showsRecruitRotation = kind === 'recruit' && settings.mode === 'perCastle'
      && scope.type === 'castle' && items.length > 1;
    const nextRotationIndex = showsRecruitRotation
      ? (settings.castles[scope.castleId]?.cursor ?? 0) % items.length
      : -1;

    return (
      <div className="space-y-3">
        <div className="flex flex-wrap gap-4 content-start">
        {items.map((item, index) => (
          <button
            key={`${scope.type}-${scope.type === 'castle' ? scope.castleId : 'global'}-${item.id}`}
            type="button"
            className={`group/item relative flex w-[5.75rem] flex-col items-center gap-2 rounded-global border bg-bg-card/70 p-3 text-center shadow-sm transition-transform hover:-translate-y-1 hover:border-primary/45 ${
              index === nextRotationIndex ? 'border-primary/70 ring-1 ring-primary/25' : 'border-border-base'
            }`}
            title={`${showsRecruitRotation ? `Rotation ${index + 1}: ` : ''}${itemName(item.id)}${kind === 'recruit' ? ` · ${unitRangeLabel(item)} · highest available` : ''}`}
            onClick={() => openEditModal(scope, item)}
          >
            {showsRecruitRotation && (
              <span className="absolute left-1.5 top-1.5 z-10 rounded-full border border-primary/35 bg-bg-card/95 px-1.5 py-0.5 text-[10px] font-black text-primary shadow-sm">
                {index === nextRotationIndex ? `Next · ${index + 1}` : index + 1}
              </span>
            )}
            {itemImage(item.id, 66, 'rounded-xl')}
            <span className="line-clamp-2 min-h-[2rem] text-xs font-bold leading-tight text-text-main">
              {itemName(item.id)}
            </span>
            {kind === 'recruit' && (
              <span className="text-[10px] font-bold uppercase tracking-wide text-primary">
                {unitRangeLabel(item)}
              </span>
            )}
            <span className="pointer-events-none absolute inset-0 flex items-center justify-center rounded-global bg-black/30 opacity-0 transition-opacity group-hover/item:opacity-100">
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
        {showsRecruitRotation && (
          <p className="text-[11px] font-semibold leading-relaxed text-text-muted">
            Queues one stack at a time in numbered order, then repeats. The next unit advances only after a successful recruit.
          </p>
        )}
      </div>
    );
  };

  const itemIDFromScheduleSlot = (slot: WeeklySchedule['slots'][number]) => {
    const raw = slot.options?.[definition.scheduleOptionKey];
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
      fallbackItemID?: number | null;
      emptyItemLabel?: string;
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
              {options.title ?? `Scheduled ${definition.itemFallbackLabel}s`}
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
            {options.emptySlotsLabel ?? `No scheduled ${definition.itemLabel} slots`}
          </div>
        ) : (
          <div className="grid gap-2">
            {visibleSlots.map((slot) => {
              const itemID = itemIDFromScheduleSlot(slot) ?? options.fallbackItemID ?? null;
              const label = itemID
                ? itemName(itemID)
                : options.emptyItemLabel ?? `No ${definition.itemLabel} selected`;
              const scheduledRange = itemID
                ? unitRangeLabel({ id: itemID, amount: 0 })
                : '';
              const day = WEEK_DAYS[slot.day]?.short ?? 'Day';
              return (
                <div
                  key={slot.id}
                  className="flex min-w-0 items-center gap-3 rounded-global border border-border-base bg-bg-card/65 px-3 py-2"
                  title={`${day} ${formatMinuteOfDay(slot.startMinute)}-${formatMinuteOfDay(slot.endMinute)} · ${label}`}
                >
                  {itemID ? (
                    itemImage(itemID, 34, 'shrink-0 rounded-lg')
                  ) : (
                    <span className="flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-lg border border-border-base bg-bg-card text-xs font-black text-text-muted">
                      -
                    </span>
                  )}
                  <div className="min-w-0 flex-1">
                    <div className="truncate text-xs font-bold text-text-main">{label}</div>
                    <div className="mt-0.5 text-[11px] font-semibold text-text-muted">
                      {day} {formatMinuteOfDay(slot.startMinute)}-{formatMinuteOfDay(slot.endMinute)}
                      {scheduledRange ? ` · ${scheduledRange}` : ''}
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

  const renderSharedItemPanel = (className = '') => {
    const sharedItemID = settings.globalItems[0]?.id ?? null;

    return (
      <SectionCard
        title={`Shared ${definition.itemFallbackLabel}`}
        description={globalScheduleEnabled
          ? `Calendar windows control when this shared ${definition.itemLabel} is queued.`
          : `Shared ${definition.itemLabel} can be overridden by the ${definition.featureLabel} calendar.`}
        titleClassName="text-base"
        className={`flex flex-col ${className}`}
        contentClassName="flex flex-1 flex-col gap-4 p-5"
        actions={(
          <div className="flex shrink-0 items-center gap-2">
            {globalScheduleEnabled && (
              <Badge variant="primary">
                {globalSchedule?.slots.length ?? 0} slot{globalSchedule?.slots.length === 1 ? '' : 's'}
              </Badge>
            )}
            <Button
              variant="outline"
              size="sm"
              onClick={() => onOpenFeatureSchedule(definition.featureID, definition.featureLabel)}
              leftIcon={<CalendarDays className="h-4 w-4" />}
            >
              Calendar
            </Button>
          </div>
        )}
      >
          {renderItems(
            settings.globalItems,
            { type: 'global' },
            `Select ${definition.itemLabel}`,
            definition.globalPickerTitle,
          )}
          {globalScheduleEnabled && globalSchedule && (
            renderScheduleSlots(globalSchedule, {
              title: 'Scheduled Windows',
              description: `All enabled castles queue the shared ${definition.itemLabel} during these periods.`,
              editTitle: `${definition.featureLabel} calendar`,
              onEdit: () => onOpenFeatureSchedule(definition.featureID, definition.featureLabel),
              hideEditButton: true,
              fallbackItemID: sharedItemID,
              emptyItemLabel: `Shared ${definition.itemLabel} not selected`,
              emptySlotsLabel: 'No scheduled windows',
            })
          )}
      </SectionCard>
    );
  };

  const renderCastleScheduleSlots = (
    schedule: WeeklySchedule,
    castle: CastleOptionV2,
    castleConfigurationKey: string,
  ) => renderScheduleSlots(schedule, {
    description: `${definition.itemFallbackLabel} choices come from this castle's calendar slots.`,
    editTitle: `${castle.name} ${definition.featureLabel} schedule`,
    onEdit: () => onOpenFeatureSchedule(
      definition.castleScheduleID(castleConfigurationKey),
      `${definition.featureLabel} - ${castle.name}`,
    ),
  });

  const renderGlobalSchedulePanel = (schedule: WeeklySchedule, className = '') => (
    <SectionCard
      title="Shared Schedule"
      description={`Scheduled ${definition.itemLabelPlural} replace the shared ${definition.itemLabel} picker.`}
      icon={<CalendarDays className="h-4 w-4" />}
      titleClassName="text-base"
      className={`flex flex-col ${className}`}
      contentClassName="flex-1 p-5"
      actions={(
        <div className="flex shrink-0 items-center gap-2">
          <Badge variant="primary">
            {schedule.slots.length} slot{schedule.slots.length === 1 ? '' : 's'}
          </Badge>
          <Button
            variant="outline"
            size="sm"
            onClick={() => onOpenFeatureSchedule(definition.featureID, definition.featureLabel)}
            leftIcon={<CalendarDays className="h-4 w-4" />}
          >
            Calendar
          </Button>
        </div>
      )}
    >
        {renderScheduleSlots(schedule, {
          title: 'Time Slots',
          description: `All enabled castles use these calendar ${definition.itemLabel} slots.`,
          editTitle: `${definition.featureLabel} calendar`,
          onEdit: () => onOpenFeatureSchedule(definition.featureID, definition.featureLabel),
          hideEditButton: true,
        })}
    </SectionCard>
  );

  const renderGlobalItemSourcePanel = (className = '') => {
    if (globalUsesScheduledItems && globalSchedule) {
      return renderGlobalSchedulePanel(globalSchedule, className);
    }
    if (shouldShowSharedItemPanel) {
      return renderSharedItemPanel(className);
    }
    return null;
  };

  const renderNoCastlesPanel = (className = '') => (
    <SectionCard
      title="Castle Coverage"
      description="Enabled castles will appear here after game data refresh."
      icon={<Castle className="h-4 w-4" />}
      actions={<Badge variant="secondary" className="shrink-0">No data</Badge>}
      titleClassName="text-base"
      className={`flex flex-col ${className}`}
      contentClassName="flex flex-1 p-4"
    >
        <div className="flex min-h-[6.75rem] w-full items-center gap-4 rounded-global border border-dashed border-border-base bg-bg-card/35 p-4">
          <div className="hidden h-10 w-10 shrink-0 place-items-center rounded-global border border-primary/20 bg-primary/10 text-primary sm:grid">
            <Castle className="h-5 w-5" />
          </div>
          <div className="min-w-0">
            <div className="text-xs font-bold uppercase tracking-wider text-text-muted">No castles available</div>
            <p className="mt-2 max-w-sm text-sm leading-relaxed text-text-muted">
              {definition.noCastlesHelp}
            </p>
          </div>
        </div>
    </SectionCard>
  );

  const renderGlobalCastleTogglePanel = (className = '') => (
    <SectionCard
      title="Castle Toggles"
      description={`Enable ${definition.featureLabel} coverage for each castle.`}
      icon={<Castle className="h-4 w-4" />}
      titleClassName="text-base"
      className={`flex flex-col ${className}`}
      contentClassName="flex flex-1 flex-col gap-2 p-4"
      actions={(
        <Badge variant="primary" className="shrink-0">
          {enabledCastleCount}/{eligibleCastles.length}
        </Badge>
      )}
    >
        {eligibleCastles.map((castle) => {
          const castleId = configurationKeyForCastle(castle);
          const castleSettings = settings.castles[castleId] ?? { enabled: false, items: [], cursor: 0 };

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
    </SectionCard>
  );

  return (
    <>
      <SettingsModal
        isOpen={isOpen}
        onClose={handleClose}
        maxWidth={isGlobalMode ? '6xl' : 'full'}
        title={definition.settingsTitle}
        icon={<Settings className="h-5 w-5" />}
        description="Queue slots, schedules, and castle coverage"
        onSave={handleSave}
        isSaving={isSaving}
      >
        <div className={`recruit-modal-shell mx-auto flex w-full flex-col gap-5 overflow-visible pb-2 ${isGlobalMode ? 'max-w-6xl' : 'max-w-[min(1840px,98vw)]'}`}>
          {saveError && (
            <div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error" role="alert">
              {saveError}
            </div>
          )}
          <div className="grid grid-cols-1 gap-4 xl:grid-cols-[minmax(17rem,0.8fr)_minmax(22rem,1.15fr)_minmax(11rem,0.5fr)]">
            <SectionCard
              title="Queue Check"
              description="Minutes between castle cycles."
              icon={<Clock3 className="h-4 w-4" />}
              titleClassName="text-base"
            >
                <Input
                  type="text"
                  value={definition.checkIntervalSecToMinutes(settings.checkIntervalSec).toLocaleString()}
                  onChange={(e) => updateCheckIntervalMinutes(e.target.value)}
                  className="font-mono text-lg font-black tabular-nums"
                  rightIcon={<span className="text-xs font-bold uppercase text-text-muted">min</span>}
                />
                <p className="mt-2 text-[11px] font-medium text-text-muted">
                  Minimum {definition.minCheckIntervalMin.toLocaleString()} minute. Default is {definition.defaultCheckIntervalMin.toLocaleString()} minutes.
                </p>
            </SectionCard>

            <SectionCard
              title={definition.modeTitle}
              description={definition.modeDescription}
              titleClassName="text-base"
            >
                <PillSelector
                  ariaLabel={`${definition.featureLabel} configuration scope`}
                  value={settings.mode}
                  onChange={(value) => updateMode(value === 'perCastle' ? 'perCastle' : 'global')}
                  options={[
                    {
                      value: 'global',
                      label: 'Same across all Castles',
                      icon: <Copy className="h-4 w-4" />,
                      title: `Use one ${kind === 'recruit' ? 'recruit unit' : 'tool'} for all enabled castles`,
                    },
                    {
                      value: 'perCastle',
                      label: 'Specific per Castle',
                      icon: <Settings className="h-4 w-4" />,
                      title: `Use separate ${kind === 'recruit' ? 'recruit units' : 'tools'} and schedules per castle`,
                    },
                  ]}
                  size="body"
                  fullWidth
                />
            </SectionCard>

            <SectionCard
              title="Enabled"
              description={`Castles selected for ${definition.featureLabel}.`}
              titleClassName="text-base"
              contentClassName="flex flex-wrap items-center justify-between gap-4 p-5"
            >
                <div className="text-4xl font-black tabular-nums text-primary">{enabledCastleCount}</div>
                <Badge variant={isGlobalMode ? 'primary' : 'secondary'}>
                  {isGlobalMode ? 'shared' : 'per castle'}
                </Badge>
            </SectionCard>
          </div>

          {kind === 'recruit' && (
            <SectionCard
              title="Glory-title fallback"
              description="Controls level-11 Protector of the North and Valkyrie Sniper slots when your current glory title no longer unlocks them."
              titleClassName="text-base"
              contentClassName="flex flex-wrap items-center justify-between gap-4 p-5"
            >
              <div className="min-w-0 flex-1">
                <div className="text-sm font-bold text-text-main">Recruit level 10 if glory title is lost</div>
                <p className="mt-1 text-xs font-semibold leading-relaxed text-text-muted">
                  Off by default. When off, affected recruit slots stay softly paused until the required title returns.
                </p>
              </div>
              <div className="flex shrink-0 items-center gap-3">
                <Badge variant={settings.recruitLevel10OnTitleLoss ? 'success' : 'outline'}>
                  {settings.recruitLevel10OnTitleLoss ? 'Level 10 fallback on' : 'Soft pause'}
                </Badge>
                <Switch
                  checked={settings.recruitLevel10OnTitleLoss === true}
                  onChange={(checked) => setSettings((previous) => ({
                    ...previous,
                    recruitLevel10OnTitleLoss: checked,
                  }))}
                />
              </div>
            </SectionCard>
          )}

          {isGlobalMode ? (
            eligibleCastles.length === 0 ? (
              globalHasItemSourcePanel ? (
                <div className="grid gap-4 lg:grid-cols-[minmax(0,1.15fr)_minmax(18rem,0.85fr)]">
                  {renderGlobalItemSourcePanel('h-full')}
                  {renderNoCastlesPanel('h-full')}
                </div>
              ) : (
                renderNoCastlesPanel()
              )
            ) : (
              <div className={`grid gap-4 ${globalHasItemSourcePanel ? 'lg:grid-cols-[minmax(0,1fr)_minmax(20rem,0.75fr)]' : ''}`}>
                {renderGlobalItemSourcePanel('h-full')}
                {renderGlobalCastleTogglePanel('h-full')}
              </div>
            )
          ) : eligibleCastles.length === 0 ? (
            renderNoCastlesPanel()
          ) : (
            <>
              <div className="grid w-full auto-rows-max grid-cols-1 gap-5 lg:grid-cols-2 2xl:grid-cols-3">
                {eligibleCastles.map((castle) => {
                  const castleId = configurationKeyForCastle(castle);
                  const castleScheduleID = definition.castleScheduleID(castleId);
                  const castleSchedule = featureSchedules[castleScheduleID];
                  const scheduledItemSchedule = !isGlobalMode && castleSchedule?.enabled && castleSchedule.slotOptionsEnabled ? castleSchedule : null;
                  const castleUsesScheduledItems = !!scheduledItemSchedule;
                  const castleSettings = settings.castles[castleId] ?? { enabled: false, items: [], cursor: 0 };
                  const displayedItems = isGlobalMode ? [] : castleSettings.items;
                  const hasItems = displayedItems.length > 0;

                  return (
                    <Card
                      key={castle.id}
                      variant="solid"
                      className={`liquid-prominent-header-card flex min-h-0 flex-col ${
                        castleSettings.enabled ? 'border-primary/35 shadow-[0_0_24px_-12px_var(--primary-glow)]' : ''
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
                              onClick={() => onOpenFeatureSchedule(castleScheduleID, `${definition.featureLabel} - ${castle.name}`)}
                              title={`${castle.name} ${definition.featureLabel} schedule`}
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
                              ? `Uses shared ${definition.itemLabel}`
                              : castleUsesScheduledItems
                                ? `Uses scheduled ${definition.itemLabel} slots`
                                : `Uses ${definition.castleSpecificLabel}`}
                          </span>
                          {scheduledItemSchedule ? (
                            <Badge variant="primary">
                              {scheduledItemSchedule.slots.length} slot{scheduledItemSchedule.slots.length !== 1 ? 's' : ''}
                            </Badge>
                          ) : hasItems && (
                            <Badge variant="primary">
                              {displayedItems.length} {definition.itemLabel}{displayedItems.length !== 1 ? 's' : ''}
                            </Badge>
                          )}
                        </div>
                        {isGlobalMode ? (
                          <div className="rounded-global border border-border-base bg-bg-card/55 px-4 py-3 text-xs font-semibold text-text-main">
                            Shared {definition.itemLabel}
                          </div>
                        ) : scheduledItemSchedule ? (
                          renderCastleScheduleSlots(scheduledItemSchedule, castle, castleId)
                        ) : (
                          renderItems(
                            castleSettings.items,
                            { type: 'castle', castleId, liveCastleId: String(castle.id) },
                            `Select ${kind === 'recruit' ? 'units' : definition.itemLabel}`,
                            `Select ${kind === 'recruit' ? 'recruit rotation' : 'tool'} - ${castle.name}`,
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
      </SettingsModal>

      <Modal
        isOpen={!!editingItem}
        onClose={closeEditModal}
        maxWidth="sm"
        title={editingItem ? itemName(editingItem.item.id) : definition.itemFallbackLabel}
        footer={
          <>
            <Button variant="danger" onClick={deleteFromEditModal} leftIcon={<Trash2 className="w-4 h-4" />}>Remove</Button>
            <Button variant="primary" onClick={saveEditModal} className="flex-[2]">Done</Button>
          </>
        }
      >
        <div className="flex flex-col items-center gap-6 py-4">
          {editingItem && (
            itemImage(editingItem.item.id, 80, 'rounded-2xl shadow-lg')
          )}

          {editingItem && kind === 'recruit' && (
            <div className="rounded-global border border-primary/20 bg-primary/5 px-4 py-3 text-center">
              <div className="text-xs font-black uppercase tracking-wide text-primary">
                {unitRangeLabel(editingItem.item)}
              </div>
              <p className="mt-1 text-xs font-semibold text-text-muted">
                Auto Recruit queues the highest currently available upgrade in this unit family.
              </p>
            </div>
          )}

        </div>
      </Modal>
    </>
  );
};
