import React, { useEffect, useMemo, useRef, useState } from 'react';
import {
  Anchor,
  ArrowDown,
  ArrowUp,
  Camera,
  Castle,
  Clock3,
  Crosshair,
  FastForward,
  GripVertical,
  Hammer,
  Package,
  Search,
  Shield,
  Sparkles,
  Swords,
  Trash2,
  Truck,
  Waves,
} from 'lucide-react';
import type {
  AutoStormTroopCapPreviewV2,
  BuildingBlueprintDiffResponse,
  BuildingTargetCaptureMode,
} from '../../api/Contracts';
import { useCitadelAPI } from '../../api/ApiContext';
import { CitadelAPI } from '../../api/CitadelClient';
import {
  ATTACK_PRESETS_SECTION,
  parseAttackPresetDocument,
  summarizeAttackPreset,
} from '../../attackPresets/AttackPresetTypes';
import { Notifications } from '../../components/Notifications';
import { showTroopPicker, type UnitWithQuantity } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import { Badge, Button, Card, ChoiceChipGroup, Input, Select, SettingsModal, SettingsToggleRow, Switch } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';
import {
  AUTO_STORM_LUNA_PACKAGE_IDS,
  AUTO_STORM_BLUEPRINTS_SECTION,
  AUTO_STORM_SECTION,
  AUTO_STORM_TARGET_PRIORITIES,
  AUTO_STORM_TROOP_HISTORY_HOURS,
  clampAutoStormInteger,
  defaultAutoStormClientState,
	activateAutoStormBlueprint,
  parseAutoStormClientState,
  parseAutoStormBlueprintDocument,
	saveAutoStormBlueprint,
  type AutoStormClientStateV1,
  type AutoStormIslandSize,
  type AutoStormResource,
  type AutoStormTargetPriority,
} from '../AutoStormClientState';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import { DailyAttackLimitField } from './DailyAttackLimitField';

interface AutoStormSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
}

interface DecorationPresetOption {
  value: string;
  castleId: number;
  presetId: string;
  label: string;
  itemCount: number;
}

interface LunaPackage {
  id: number;
  name: string;
  type: string;
  price: number;
  stock: number;
  unitId: number;
  unitAmount: number;
  buildingId: number;
  buildingAmount: number;
  rewardDetail: string;
}

interface StormCastleOption {
  id: number;
  name: string;
  minLevel: number;
  costWood: number;
  costStone: number;
  costFood: number;
  costCoins: number;
  costPremium: number;
}

const FORT_LEVELS = [40, 50, 60, 70, 80];
const RESOURCE_OPTIONS: Array<{ value: AutoStormResource; label: string }> = [
  { value: 'wood', label: 'Wood' },
  { value: 'stone', label: 'Stone' },
  { value: 'aquamarine', label: 'Aquamarine' },
];
const SIZE_OPTIONS: Array<{ value: AutoStormIslandSize; label: string }> = [
  { value: 'large', label: 'Large' },
  { value: 'small', label: 'Small' },
];
const TARGET_PRIORITY_OPTIONS: Record<AutoStormTargetPriority, { label: string; detail: string }> = {
  'fort:80': { label: 'Level 80 fort', detail: 'Uses the selected fort attack preset.' },
  'fort:70': { label: 'Level 70 fort', detail: 'Uses the selected fort attack preset.' },
  'fort:60': { label: 'Level 60 fort', detail: 'Uses the selected fort attack preset.' },
  'fort:50': { label: 'Level 50 fort', detail: 'Uses the selected fort attack preset.' },
  'fort:40': { label: 'Level 40 fort', detail: 'Uses the selected fort attack preset.' },
  'island:large': { label: 'Large resource island', detail: 'Any selected resource; uses the island attack preset.' },
  'island:small': { label: 'Small resource island', detail: 'Any selected resource; uses the island attack preset.' },
};
const RESOURCE_RESERVES = [
  { key: '3', label: 'Wood' },
  { key: '4', label: 'Stone' },
  { key: '9', label: 'Aquamarine' },
];
const TIME_SKIP_RESERVES = [
  { key: 'MS1', label: '1m' },
  { key: 'MS2', label: '5m' },
  { key: 'MS3', label: '10m' },
  { key: 'MS4', label: '30m' },
  { key: 'MS5', label: '1h' },
  { key: 'MS6', label: '5h' },
  { key: 'MS7', label: '24h' },
];
const LUNA_PACKAGE_ID_SET = new Set(AUTO_STORM_LUNA_PACKAGE_IDS);

export const AutoStormSettingsModal: React.FC<AutoStormSettingsModalProps> = ({ isOpen, onClose }) => {
  const { state, configuration, captureBuildingTarget, updateConfiguration } = useCitadelAPI();
  const { getTool, getTroop } = useMetadata();
  const [draft, setDraft] = useState<AutoStormClientStateV1>(defaultAutoStormClientState);
  const [captureCastleId, setCaptureCastleId] = useState(0);
  const [capturing, setCapturing] = useState<BuildingTargetCaptureMode | null>(null);
  const [blueprintPreview, setBlueprintPreview] = useState<BuildingBlueprintDiffResponse | null>(null);
  const [saving, setSaving] = useState(false);
  const [stormCastleOptions, setStormCastleOptions] = useState<StormCastleOption[]>([]);
  const [loadingStormCastleOptions, setLoadingStormCastleOptions] = useState(false);
  const [stormCastleOptionsError, setStormCastleOptionsError] = useState('');
  const [lunaPackages, setLunaPackages] = useState<LunaPackage[]>([]);
  const [loadingPackages, setLoadingPackages] = useState(false);
  const [lunaSearch, setLunaSearch] = useState('');
  const [troopCapPreview, setTroopCapPreview] = useState<AutoStormTroopCapPreviewV2 | null>(null);
  const [troopCapPreviewError, setTroopCapPreviewError] = useState('');
  const [loadingTroopCapPreview, setLoadingTroopCapPreview] = useState(false);
  const [troopCapRefreshTick, setTroopCapRefreshTick] = useState(0);
  const [draggedTargetPriority, setDraggedTargetPriority] = useState<AutoStormTargetPriority | null>(null);
  const [targetPriorityDropTarget, setTargetPriorityDropTarget] = useState<AutoStormTargetPriority | null>(null);
  const initializedOpen = useRef(false);

  const stormCastles = useMemo(() => Object.values(state?.castles ?? {})
    .filter((castle) => castle.kingdomId === 4)
    .sort((left, right) => left.id - right.id), [state?.castles]);
  const troopDonorCastles = useMemo(() => Object.values(state?.castles ?? {})
    .filter((castle) => castle.kingdomId !== 4)
    .sort((left, right) => left.kingdomId - right.kingdomId || left.id - right.id), [state?.castles]);
  const stormCastle = stormCastles.find((castle) => castle.id === captureCastleId) ?? stormCastles[0];
  const selectedUnlockOption = stormCastleOptions.find((option) => option.id === draft.unlock.prebuiltCastleId);
  const stormUnlockState = state?.kingdomTransport.unlocks['4'];
  const attackPresets = useMemo(
    () => parseAttackPresetDocument(configuration?.sections[ATTACK_PRESETS_SECTION]),
    [configuration?.sections],
  );
  const decorationOptions = useMemo(
    () => parseDecorationPresetOptions(configuration?.sections['decorations.presets'], state?.castles ?? {}),
    [configuration?.sections, state?.castles],
  );
  const selectedDecorationValue = decorationOptions.find((option) => (
    option.castleId === draft.decorationPresetCastleId && option.presetId === draft.decorationPresetId
  ))?.value ?? '';
  const selectedFortPreset = attackPresets.presets.find((preset) => preset.id === draft.forts.presetId);
  const selectedIslandPreset = attackPresets.presets.find((preset) => preset.id === draft.islands.presetId);
  const troopCapPreviewSettings = useMemo(() => ({
    version: 1,
    troopImport: {
      enabled: draft.troopImport.enabled,
      minimumTroops: draft.troopImport.minimumTroops,
      historyHours: AUTO_STORM_TROOP_HISTORY_HOURS,
    },
    forts: {
      enabled: draft.forts.enabled,
      presetId: draft.forts.presetId,
    },
    islands: {
      enabled: draft.islands.enabled,
      presetId: draft.islands.presetId,
      defenseUnits: draft.islands.defenseUnits,
    },
  }), [
    draft.forts.enabled,
    draft.forts.presetId,
    draft.islands.defenseUnits,
    draft.islands.enabled,
    draft.islands.presetId,
    draft.troopImport.enabled,
    draft.troopImport.minimumTroops,
  ]);
  const stormMapState = state?.storm.map;
  const stormMapCoverage = stormMapState && stormMapState.windowCount
    ? `${stormMapState.coveredBounds.x2 - stormMapState.coveredBounds.x1 + 1} × ${stormMapState.coveredBounds.y2 - stormMapState.coveredBounds.y1 + 1} · ${stormMapState.windowCount} windows`
    : 'Learns on first sweep';
  const stormOpportunities = useMemo(() => {
	const nextReadyAt = stormMapState?.nextTargetReadyAt
	  ? Date.parse(stormMapState.nextTargetReadyAt)
	  : Number.POSITIVE_INFINITY;
	return {
	  ready: Math.max(0, stormMapState?.readyTargetCount ?? 0),
	  nextReadyAt: Number.isFinite(nextReadyAt) ? nextReadyAt : Number.POSITIVE_INFINITY,
	};
	}, [stormMapState?.nextTargetReadyAt, stormMapState?.readyTargetCount]);
  const configuredLunaPurchases = useMemo(
    () => new Map(draft.aquamarine.purchases.map((purchase) => [purchase.packageId, purchase])),
    [draft.aquamarine.purchases],
  );
  const visibleLunaPackages = useMemo(() => {
    const query = lunaSearch.trim().toLowerCase();
    if (!query) return lunaPackages;
    return lunaPackages.filter((item) => {
      const displayName = lunaPackageDisplayName(item, getTroop, getTool);
      return `${displayName} ${item.name} ${lunaPackageTypeLabel(item.type)} ${item.id} ${item.rewardDetail}`
        .toLowerCase()
        .includes(query);
    });
  }, [getTool, getTroop, lunaPackages, lunaSearch]);
  const activeTargetPriorities = useMemo(
    () => draft.targetPriority.filter((priority) => autoStormTargetPriorityEnabled(draft, priority)),
    [draft],
  );
  const lunaCountersCurrent = Boolean(
    stormCastle
    && state?.inventory.constructionOffersCastleId === stormCastle.id
    && state.inventory.constructionOffersKingdomId === 4
    && state.inventory.constructionOffersObservedAt,
  );

  const savedConfiguration = configuration?.sections[AUTO_STORM_SECTION];
  const blueprintDocument = useMemo(
    () => parseAutoStormBlueprintDocument(configuration?.sections[AUTO_STORM_BLUEPRINTS_SECTION]),
    [configuration?.sections],
  );
  const activeBlueprint = blueprintDocument.blueprints[blueprintDocument.activeId];
  const savedBlueprints = Object.values(blueprintDocument.blueprints)
    .sort((left, right) => left.id.localeCompare(right.id));

  useEffect(() => {
    if (!isOpen) {
      initializedOpen.current = false;
      return;
    }
    if (initializedOpen.current || !configuration) return;
    const current = parseAutoStormClientState(savedConfiguration);
    const target = activeBlueprint?.target ?? current.target;
    setDraft({ ...current, ...(target ? { target } : {}) });
    setCaptureCastleId(target?.castleId ?? 0);
    setBlueprintPreview(null);
    setDraggedTargetPriority(null);
    setTargetPriorityDropTarget(null);
    initializedOpen.current = true;
  }, [activeBlueprint?.target, configuration, isOpen, savedConfiguration]);

  useEffect(() => {
    if (!isOpen || captureCastleId > 0 || stormCastles.length === 0) return;
    setCaptureCastleId(stormCastles[0].id);
  }, [captureCastleId, isOpen, stormCastles]);

  useEffect(() => {
    if (!isOpen) return;
    let cancelled = false;
    setStormCastleOptionsError('');
    setLoadingStormCastleOptions(true);
    void CitadelAPI.getCatalog<Record<string, unknown>>('prebuiltcastles')
      .then((response) => {
        if (!cancelled) {
          setStormCastleOptions(parseStormCastleOptions(response.items, state?.player.level));
        }
      })
      .catch((error) => {
        if (!cancelled) {
          setStormCastleOptions([]);
          setStormCastleOptionsError(error instanceof Error ? error.message : 'Could not load the official Storm castle catalog.');
        }
      })
      .finally(() => { if (!cancelled) setLoadingStormCastleOptions(false); });
    return () => { cancelled = true; };
  }, [isOpen, state?.player.level]);

  useEffect(() => {
    if (!isOpen) return undefined;
    const timer = window.setInterval(() => setTroopCapRefreshTick((current) => current + 1), 60_000);
    return () => window.clearInterval(timer);
  }, [isOpen]);

  useEffect(() => {
    if (!isOpen || !configuration || !draft.troopImport.enabled) {
      setTroopCapPreview(null);
      setTroopCapPreviewError('');
      setLoadingTroopCapPreview(false);
      return undefined;
    }
    let cancelled = false;
    setTroopCapPreview(null);
    setTroopCapPreviewError('');
    setLoadingTroopCapPreview(true);
    const timer = window.setTimeout(() => {
      void CitadelAPI.previewAutoStormTroopCap({ settings: troopCapPreviewSettings })
        .then((preview) => {
          if (!cancelled) setTroopCapPreview(preview);
        })
        .catch((error) => {
          if (!cancelled) {
            setTroopCapPreviewError(error instanceof Error ? error.message : 'Could not calculate the Storm troop cap.');
          }
        })
        .finally(() => {
          if (!cancelled) setLoadingTroopCapPreview(false);
        });
    }, 150);
    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
  }, [
    configuration?.revision,
    draft.troopImport.enabled,
    isOpen,
    state?.automations?.autoStorm?.updatedAt,
    troopCapPreviewSettings,
    troopCapRefreshTick,
  ]);

  useEffect(() => {
    if (!isOpen) return;
    let cancelled = false;
    setLoadingPackages(true);
    void CitadelAPI.getCatalog<Record<string, unknown>>('packages')
      .then((response) => {
        if (!cancelled) setLunaPackages(parseLunaPackages(response.items));
      })
      .catch((error) => {
        if (!cancelled) {
          setLunaPackages([]);
          Notifications.error(error instanceof Error ? error.message : 'Could not load Luna shop packages.');
        }
      })
      .finally(() => { if (!cancelled) setLoadingPackages(false); });
    return () => { cancelled = true; };
  }, [isOpen]);

  const capture = async (mode: BuildingTargetCaptureMode) => {
    if (!stormCastle || capturing) return;
    setCapturing(mode);
    try {
      const target = await captureBuildingTarget({
        castleId: stormCastle.id,
        mode,
        expectedRevision: state?.revision,
      });
      const preview = await CitadelAPI.previewBuildingBlueprint({
        target,
        policy: {
          allowPremium: draft.build.allowPremium,
          resourceReserves: draft.build.resourceReserves,
        },
      });
      if (!preview.compilable) {
        const issue = [...preview.normal.issues, ...preview.fixed.issues]
          .find((candidate) => candidate.severity === 'error');
        throw new Error(issue?.message ?? 'The captured Storm blueprint cannot be compiled safely.');
      }
	  const savedBlueprints = configuration?.sections[AUTO_STORM_BLUEPRINTS_SECTION];
	  await updateConfiguration(
		AUTO_STORM_BLUEPRINTS_SECTION,
		saveAutoStormBlueprint(savedBlueprints, preview.target),
		savedBlueprints === undefined ? undefined : { expectedValue: savedBlueprints },
	  );
      setBlueprintPreview(preview);
      setDraft((current) => ({
        ...current,
        target,
        ...(mode === 'exact' ? { decorationPresetCastleId: 0, decorationPresetId: '' } : {}),
      }));
      Notifications.success(`${captureModeLabel(mode)} preflight passed and was saved.`);
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not capture the Storm layout.');
    } finally {
      setCapturing(null);
    }
  };

  const activateBlueprint = async (id: string) => {
    if (capturing || saving) return;
    const blueprint = blueprintDocument.blueprints[id];
    if (!blueprint) return;
    try {
	  const savedBlueprints = configuration?.sections[AUTO_STORM_BLUEPRINTS_SECTION];
	  await updateConfiguration(
		AUTO_STORM_BLUEPRINTS_SECTION,
		activateAutoStormBlueprint(savedBlueprints, id),
		savedBlueprints === undefined ? undefined : { expectedValue: savedBlueprints },
	  );
      setDraft((current) => ({ ...current, target: blueprint.target }));
      setCaptureCastleId(blueprint.target.castleId);
      setBlueprintPreview(null);
      Notifications.success(`${blueprint.name} activated.`);
	} catch (error) {
	  Notifications.error(error instanceof Error ? error.message : 'Could not activate the Storm blueprint.');
    }
  };

  const deactivateBlueprint = async () => {
    if (capturing || saving) return;
    try {
	  const savedBlueprints = configuration?.sections[AUTO_STORM_BLUEPRINTS_SECTION];
	  await updateConfiguration(
		AUTO_STORM_BLUEPRINTS_SECTION,
		activateAutoStormBlueprint(savedBlueprints, ''),
		savedBlueprints === undefined ? undefined : { expectedValue: savedBlueprints },
	  );
      setDraft((current) => {
        const { target: _target, ...rest } = current;
        return rest;
      });
      setBlueprintPreview(null);
      Notifications.success('Storm blueprint reconciliation paused. Saved blueprints were retained.');
	} catch (error) {
	  Notifications.error(error instanceof Error ? error.message : 'Could not pause Storm blueprint reconciliation.');
    }
  };

  const chooseDefenseUnits = async () => {
    const quantities = Object.fromEntries(draft.islands.defenseUnits.map((unit) => [unit.unitId, unit.amount]));
    const result = await showTroopPicker({
      mode: 'multi',
      title: 'Troops left to defend captured islands',
      allowQuantity: true,
      preselected: draft.islands.defenseUnits.map((unit) => unit.unitId),
      preselectedQuantities: quantities,
      stockQuantities: stormCastle?.units.stationed,
    });
    if (!Array.isArray(result)) return;
    const defenseUnits = (result as UnitWithQuantity[])
      .filter((unit) => unit.unitId > 0 && unit.quantity > 0)
      .slice(0, 8)
      .map((unit) => ({ unitId: unit.unitId, amount: unit.quantity }));
    setDraft((current) => ({
      ...current,
      islands: { ...current.islands, defenseUnits },
    }));
  };

  const moveTargetPriority = (source: AutoStormTargetPriority, targetPriority: AutoStormTargetPriority) => {
    if (source === targetPriority) return;
    setDraft((current) => {
      const active = current.targetPriority.filter((priority) => autoStormTargetPriorityEnabled(current, priority));
      const sourceIndex = active.indexOf(source);
      const targetIndex = active.indexOf(targetPriority);
      if (sourceIndex < 0 || targetIndex < 0) return current;
      const reordered = [...active];
      const [moved] = reordered.splice(sourceIndex, 1);
      reordered.splice(targetIndex, 0, moved);
      const activeSet = new Set<AutoStormTargetPriority>(reordered);
      return {
        ...current,
        targetPriority: [
          ...reordered,
          ...current.targetPriority.filter((priority) => !activeSet.has(priority)),
        ],
      };
    });
  };

  const moveTargetPriorityBy = (priority: AutoStormTargetPriority, offset: -1 | 1) => {
    const index = activeTargetPriorities.indexOf(priority);
    const target = activeTargetPriorities[index + offset];
    if (target) moveTargetPriority(priority, target);
  };

  const finishTargetPriorityDrag = () => {
    setDraggedTargetPriority(null);
    setTargetPriorityDropTarget(null);
  };

  const fortValid = !draft.forts.enabled || (draft.forts.levels.length > 0 && Boolean(draft.forts.presetId));
  const islandsValid = !draft.islands.enabled || (
    draft.islands.resources.length > 0
    && draft.islands.sizes.length > 0
    && Boolean(draft.islands.presetId)
    && draft.islands.defenseUnits.every((unit) => unit.unitId > 0 && unit.amount > 0)
  );
  const shopIDs = draft.aquamarine.purchases.map((purchase) => purchase.packageId);
  const shopValid = draft.aquamarine.purchases.length === 0 || (
    shopIDs.every((id) => id > 0)
    && new Set(shopIDs).size === shopIDs.length
    && draft.aquamarine.purchases.every((purchase) => purchase.unlimited || purchase.targetPurchases > 0)
  );
  const targetValid = draft.target == null || draft.target.kingdomId === 4;
  const validDonorIDs = new Set(troopDonorCastles.map((castle) => castle.id));
  const troopImportValid = !draft.troopImport.enabled || (
    draft.troopImport.donorCastleIds.length > 0
    && draft.troopImport.donorCastleIds.every((castleId) => validDonorIDs.has(castleId))
  );
  const unlockValid = !draft.unlock.enabled || Boolean(selectedUnlockOption);
  const canSave = fortValid && islandsValid && shopValid && targetValid && troopImportValid && unlockValid;

  const save = async () => {
    if (!canSave || saving) return;
    setSaving(true);
    try {
      const parsed = parseAutoStormClientState(draft);
      let settings: Omit<AutoStormClientStateV1, 'target'> | AutoStormClientStateV1 = parsed;
      if (blueprintDocument.activeId) {
        const { target: _legacyTarget, ...withoutLegacyTarget } = parsed;
        settings = withoutLegacyTarget;
      }
	  await updateConfiguration(
		AUTO_STORM_SECTION,
		settings,
		savedConfiguration === undefined ? undefined : { expectedValue: savedConfiguration },
	  );
      Notifications.success('Auto Storm settings saved.');
      onClose();
    } catch (error) {
      Notifications.error(error instanceof Error ? error.message : 'Could not save Auto Storm settings.');
    } finally {
      setSaving(false);
    }
  };

  const target = draft.target;
  const targetCastle = target ? state?.castles[String(target.castleId)] : undefined;
  const aquamarineBalance = stormCastle?.resources['9']?.amount ?? 0;

  return (
    <SettingsModal
      isOpen={isOpen}
      onClose={() => { if (!saving && !capturing) onClose(); }}
      maxWidth="full"
      title="Auto Storm"
      icon={<Waves className="h-5 w-5" />}
      description="Reconcile a captured Storm castle, attack selected forts and resource islands, and spend Aquamarine through guarded goals."
      onSave={() => void save()}
      isSaving={saving}
      saveDisabled={!canSave}
      cancelDisabled={capturing != null}
    >
      <div className="space-y-4">
        <Card variant="solid" className="p-4">
          <SectionHeading
            icon={Castle}
            title="Storm castle access"
            description="Choose the exact official starter castle Auto Storm may open when a new Storm season is locked. Existing and manually opened castles are reconciled without buying again."
          />
          <div className="mt-4 flex items-start justify-between gap-4 rounded-global border border-border-base bg-bg-app/30 p-3">
            <div>
              <div className="text-sm font-bold text-text-main">Automatically open the Storm castle</div>
              <p className="mt-1 text-xs text-text-muted">Runs only when the game reports Storm locked and no owned Storm castle exists.</p>
            </div>
            <Switch
              checked={draft.unlock.enabled}
              disabled={loadingStormCastleOptions || stormCastleOptions.length === 0}
              onChange={(enabled) => setDraft((current) => ({
                ...current,
                unlock: {
                  enabled,
                  prebuiltCastleId: enabled
                    ? current.unlock.prebuiltCastleId || preferredStormCastleOption(stormCastleOptions)?.id || 0
                    : current.unlock.prebuiltCastleId,
                },
              }))}
              ariaLabel="Automatically open the Storm castle"
            />
          </div>

          <label className="mt-3 block">
            <FieldLabel>Castle to open</FieldLabel>
            <Select
              value={draft.unlock.prebuiltCastleId > 0 ? String(draft.unlock.prebuiltCastleId) : ''}
              onChange={(value) => setDraft((current) => ({
                ...current,
                unlock: { ...current.unlock, prebuiltCastleId: Number(value) || 0 },
              }))}
              options={stormCastleOptions.map((option) => ({
                value: String(option.id),
                label: stormCastleOptionLabel(option),
              }))}
              placeholder={loadingStormCastleOptions ? 'Loading official choices…' : 'Choose an official Storm castle'}
              disabled={!draft.unlock.enabled || loadingStormCastleOptions || stormCastleOptions.length === 0}
              menuGrowToViewport
            />
          </label>

          <p className="mt-3 rounded-global border border-border-base bg-bg-app/35 px-3 py-2 text-xs text-text-muted">
            {stormCastle
              ? `Current Storm castle: ${stormCastle.name?.trim() || `Castle ${stormCastle.id}`} at ${stormCastle.x}:${stormCastle.y}. Auto Storm will use it; this choice applies to the next locked season.`
              : stormUnlockState?.unlocked || stormUnlockState?.created
                ? 'The game reports Storm already unlocked. Auto Storm will refresh your castle directory and continue without sending another unlock purchase.'
                : 'The game currently reports Storm locked. Auto Storm will wait unless automatic opening is enabled with a valid choice.'}
          </p>
          {selectedUnlockOption?.costPremium ? (
            <p className="mt-2 text-xs text-warning">
              This choice authorizes {selectedUnlockOption.costPremium.toLocaleString()} premium currency for the seasonal Storm castle unlock.
            </p>
          ) : null}
          {stormCastleOptionsError ? <p className="mt-2 text-xs text-error">{stormCastleOptionsError}</p> : null}
          {!unlockValid ? <p className="mt-2 text-xs text-error">Choose a currently available official Storm castle.</p> : null}
        </Card>

        <Card variant="solid" className="p-4">
          <SectionHeading
            icon={Camera}
            title="Durable castle blueprints"
            description="Capture a reusable end state. Each mode is retained separately, preflighted against official data, and activated without changing the other saved modes."
          />
          <div className="mt-4 grid gap-3 lg:grid-cols-[minmax(0,1fr)_repeat(3,auto)] lg:items-end">
            <label className="block">
              <FieldLabel>Storm castle</FieldLabel>
              <Select
                value={stormCastle ? String(stormCastle.id) : ''}
                onChange={(value) => setCaptureCastleId(Number(value) || 0)}
                options={stormCastles.map((castle) => ({
                  value: String(castle.id),
                  label: `${castle.name?.trim() || `Castle ${castle.id}`} · ${castle.x}:${castle.y}`,
                }))}
                placeholder={stormCastles.length > 0 ? 'Choose Storm castle' : 'Unlock Storm first'}
                disabled={stormCastles.length === 0 || capturing != null}
                menuGrowToViewport
              />
            </label>
            <Button
              variant="outline"
              disabled={!stormCastle || capturing != null}
              isLoading={capturing === 'functional'}
              onClick={() => void capture('functional')}
              leftIcon={<Hammer className="h-4 w-4" />}
            >
              Functional
            </Button>
            <Button
              variant="outline"
              disabled={!stormCastle || capturing != null}
              isLoading={capturing === 'layout'}
              onClick={() => void capture('layout')}
              leftIcon={<Castle className="h-4 w-4" />}
            >
              Layout
            </Button>
            <Button
              variant="outline"
              disabled={!stormCastle || capturing != null}
              isLoading={capturing === 'exact'}
              onClick={() => void capture('exact')}
              leftIcon={<Camera className="h-4 w-4" />}
            >
              Exact clone
            </Button>
          </div>

          {savedBlueprints.length > 0 ? (
            <div className="mt-3 flex flex-wrap items-center gap-2">
              <span className="text-[11px] font-semibold text-text-muted">Saved:</span>
              {savedBlueprints.map((blueprint) => (
                <Button
                  key={blueprint.id}
                  size="sm"
                  variant={blueprint.id === blueprintDocument.activeId ? 'primary' : 'ghost'}
                  disabled={capturing != null || saving}
                  onClick={() => void activateBlueprint(blueprint.id)}
                >
                  {blueprint.name}
                </Button>
              ))}
            </div>
          ) : null}

          {target ? (
            <div className="mt-4 rounded-global border border-primary/20 bg-primary/5 p-3">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <div className="flex flex-wrap items-center gap-2">
                    <Badge variant="success">{captureModeLabel(target.mode)}</Badge>
                    <span className="text-sm font-bold text-text-main">
                      {targetCastle?.name?.trim() || `Castle ${target.castleId}`}
                    </span>
                  </div>
                  <p className="mt-1 text-xs text-text-muted">Captured {formatDate(target.capturedAt)} from revision {target.revision.toLocaleString()}.</p>
                </div>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => void deactivateBlueprint()}
                  leftIcon={<Trash2 className="h-3.5 w-3.5" />}
                >
                  Pause target
                </Button>
              </div>
              <div className="mt-3 flex flex-wrap gap-2">
                <Badge variant="outline">{target.summary.groundCount} ground tiles</Badge>
                <Badge variant="outline">{target.summary.buildingCount} buildings</Badge>
                <Badge variant="outline">{target.summary.fixedCount} fixed</Badge>
                <Badge variant="outline">{target.summary.decorationCount} decorations</Badge>
                {blueprintPreview ? (
                  <Badge variant="outline">
                    Preflight: {blueprintPreview.satisfiedCount}/{blueprintPreview.targetCount} satisfied · {blueprintPreview.actionCount} actions
                  </Badge>
                ) : null}
              </div>
            </div>
          ) : (
            <p className="mt-3 rounded-global border border-border-base bg-bg-app/35 px-3 py-2 text-xs text-text-muted">
              No castle target is required for combat-only automation. Capturing one enables automated expansions, storage dependencies, logistics, construction, and layout reconciliation.
            </p>
          )}

          {target && target.mode !== 'exact' && target.mode !== 'full' ? (
            <label className="mt-4 block border-t border-border-base pt-4">
              <FieldLabel icon={Sparkles}>Decoration preset applied after construction</FieldLabel>
              <Select
                value={selectedDecorationValue}
                onChange={(value) => {
                  const option = decorationOptions.find((candidate) => candidate.value === value);
                  setDraft((current) => ({
                    ...current,
                    decorationPresetCastleId: option?.castleId ?? 0,
                    decorationPresetId: option?.presetId ?? '',
                  }));
                }}
                options={decorationOptions.map((option) => ({ value: option.value, label: option.label }))}
                placeholder={decorationOptions.length > 0 ? 'Optional saved decoration preset' : 'Save a decoration preset first'}
                disabled={decorationOptions.length === 0}
                searchable
                menuGrowToViewport
              />
              <p className="mt-2 text-xs text-text-muted">The preset may come from any castle. Its exact decoration IDs and coordinates are applied only after the building target is complete.</p>
            </label>
          ) : null}
        </Card>

        <div className="grid gap-4 xl:grid-cols-2">
          <Card variant="solid" className="p-4">
            <SectionHeading
              icon={Hammer}
              title="Construction and logistics"
              description="Gift packets are cleared after expansions. Storage dependencies and upgrade chains are inferred automatically."
            />
            <div className="mt-4 space-y-3">
              <SettingsToggleRow
                icon={<Truck className="h-3.5 w-3.5" />}
                title="Transport missing resources"
                description="Ship available resources from another owned kingdom castle when Storm cannot afford the next dependency."
                checked={draft.build.allowResourceTransport}
                onChange={(allowResourceTransport) => updateBuild(setDraft, { allowResourceTransport })}
              />
              <SettingsToggleRow
                icon={<FastForward className="h-3.5 w-3.5" />}
                title="Use time skips"
                description="Advance construction, resource transport, or troop transport one skip command at a time, waiting for each confirmed response and preserving the reserves below."
                checked={draft.build.allowTimeSkips}
                onChange={(allowTimeSkips) => updateBuild(setDraft, { allowTimeSkips })}
              />
              <SettingsToggleRow
                icon={<Sparkles className="h-3.5 w-3.5" />}
                title="Allow premium costs"
                description="Permit premium construction paths. Harbor levels 2 and 3 require this permission."
                checked={draft.build.allowPremium}
                onChange={(allowPremium) => updateBuild(setDraft, { allowPremium })}
                tone="warning"
              />
              <SettingsToggleRow
                icon={<Trash2 className="h-3.5 w-3.5" />}
                title="Allow demolition"
                description="Permit exact reconciliation to demolish unmanaged buildings that cannot be stored or moved."
                checked={draft.build.allowDemolition}
                onChange={(allowDemolition) => updateBuild(setDraft, { allowDemolition })}
                warning
              />
            </div>

            <div className="mt-4 border-t border-border-base pt-4">
              <FieldLabel>Storm castle spending reserves</FieldLabel>
              <div className="grid grid-cols-3 gap-2">
                {RESOURCE_RESERVES.map((resource) => (
                  <label key={resource.key} className="block">
                    <span className="mb-1 block text-[10px] font-semibold text-text-muted">{resource.label}</span>
                    <Input
                      type="number"
                      min={0}
                      value={draft.build.resourceReserves[resource.key] ?? 0}
                      onChange={(event) => updateNumberMap(
                        setDraft,
                        'resourceReserves',
                        resource.key,
                        clampAutoStormInteger(event.target.value, 0, Number.MAX_SAFE_INTEGER, 0),
                      )}
                      className="font-mono"
                    />
                  </label>
                ))}
              </div>
            </div>

            {draft.build.allowResourceTransport ? (
              <div className="mt-4 border-t border-border-base pt-4">
                <FieldLabel>Protected donor reserves</FieldLabel>
                <div className="grid grid-cols-3 gap-2">
                  {RESOURCE_RESERVES.map((resource) => (
                    <label key={resource.key} className="block">
                      <span className="mb-1 block text-[10px] font-semibold text-text-muted">{resource.label}</span>
                      <Input
                        type="number"
                        min={0}
                        value={draft.build.sourceResourceReserves[resource.key] ?? 0}
                        onChange={(event) => updateNumberMap(
                          setDraft,
                          'sourceResourceReserves',
                          resource.key,
                          clampAutoStormInteger(event.target.value, 0, Number.MAX_SAFE_INTEGER, 0),
                        )}
                        className="font-mono"
                      />
                    </label>
                  ))}
                </div>
                <p className="mt-2 text-[11px] text-text-muted">
                  Multi-resource shipments may use every amount above these donor floors, limited only by the Storm castle’s free storage.
                </p>
              </div>
            ) : null}

            {draft.build.allowTimeSkips ? (
              <div className="mt-4 border-t border-border-base pt-4">
                <FieldLabel>Time skips kept in reserve</FieldLabel>
                <div className="grid grid-cols-4 gap-2 sm:grid-cols-7">
                  {TIME_SKIP_RESERVES.map((skip) => (
                    <label key={skip.key} className="block">
                      <span className="mb-1 block text-center text-[10px] font-semibold text-text-muted">{skip.label}</span>
                      <Input
                        type="number"
                        min={0}
                        value={draft.build.timeSkipReserve[skip.key] ?? 0}
                        onChange={(event) => updateNumberMap(
                          setDraft,
                          'timeSkipReserve',
                          skip.key,
                          clampAutoStormInteger(event.target.value, 0, Number.MAX_SAFE_INTEGER, 0),
                        )}
                        className="px-2 text-center font-mono"
                      />
                    </label>
                  ))}
                </div>
              </div>
            ) : null}

            <div className="mt-4 border-t border-border-base pt-4">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="flex items-center gap-2 text-sm font-bold text-text-main"><Anchor className="h-4 w-4 text-primary" /> Upgrade Harbor</div>
                  <p className="mt-1 text-xs text-text-muted">Override the captured Harbor path and maintain it at a chosen level.</p>
                </div>
                <Switch
                  checked={draft.harbor.enabled}
                  onChange={(enabled) => setDraft((current) => ({ ...current, harbor: { ...current.harbor, enabled } }))}
                  ariaLabel="Automate Storm Harbor upgrades"
                />
              </div>
              {draft.harbor.enabled ? (
                <div className="mt-3">
                  <Select
                    value={String(draft.harbor.targetLevel)}
                    onChange={(value) => setDraft((current) => ({
                      ...current,
                      harbor: { ...current.harbor, targetLevel: Number(value) || 1 },
                    }))}
                    options={[1, 2, 3].map((level) => ({ value: String(level), label: `Harbor level ${level}` }))}
                  />
                  {draft.harbor.targetLevel > 1 && !draft.build.allowPremium ? (
                    <p className="mt-2 text-xs text-warning">Harbor levels 2–3 are premium paths and will wait until premium costs are allowed.</p>
                  ) : null}
                </div>
              ) : null}
            </div>
          </Card>

          <Card variant="solid" className="p-4">
            <SectionHeading
              icon={Swords}
              title="Forts and resource islands"
              description="Each target type has its own attack preset. Live attack capacity and troop inventory are validated before launch."
            />

            <div className="mt-4 rounded-global border border-border-base bg-bg-app/30 p-3">
              <HorseTravelBoostSelect
                value={draft.horseTravelBoostId}
                onChange={(horseTravelBoostId) => setDraft((current) => ({ ...current, horseTravelBoostId }))}
              />
            </div>

            <div className="mt-4 rounded-global border border-border-base bg-bg-app/30 p-3">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="text-sm font-bold text-text-main">Storm forts</div>
                  <p className="mt-1 text-xs text-text-muted">Attack only the selected official fort levels.</p>
                </div>
                <Switch
                  checked={draft.forts.enabled}
                  onChange={(enabled) => setDraft((current) => ({ ...current, forts: { ...current.forts, enabled } }))}
                  ariaLabel="Attack Storm forts"
                />
              </div>
              {draft.forts.enabled ? (
                <div className="mt-3 space-y-3 border-t border-border-base pt-3">
                  <ChoiceChipGroup
                    ariaLabel="Storm fort levels"
                    options={FORT_LEVELS.map((level) => ({ value: level, label: `Level ${level}` }))}
                    selected={draft.forts.levels}
                    onToggle={(level) => setDraft((current) => ({
                      ...current,
                      forts: { ...current.forts, levels: toggleChoice(current.forts.levels, level) },
                    }))}
                  />
                  <label className="block">
                    <FieldLabel>Minimum attacks remaining</FieldLabel>
                    <Input
                      type="number"
                      min={0}
                      value={draft.forts.minimumWins}
                      onChange={(event) => setDraft((current) => ({
                        ...current,
                        forts: {
                          ...current.forts,
                          minimumWins: clampAutoStormInteger(event.target.value, 0, Number.MAX_SAFE_INTEGER, 0),
                        },
                      }))}
                      className="font-mono"
                    />
                    <p className="mt-1 text-[11px] text-text-muted">Only launch against forts with at least this many attacks remaining. Use 0 for no minimum.</p>
                  </label>
                  <PresetSelect
                    value={draft.forts.presetId}
                    onChange={(presetId) => setDraft((current) => ({ ...current, forts: { ...current.forts, presetId } }))}
                    presets={attackPresets.presets}
                    placeholder="Fort attack preset"
                  />
                  {selectedFortPreset ? <PresetSummary preset={selectedFortPreset} /> : null}
                  {draft.forts.levels.length === 0 ? <p className="text-xs text-error">Select at least one fort level.</p> : null}
                </div>
              ) : null}
            </div>

            <div className="mt-3 rounded-global border border-border-base bg-bg-app/30 p-3">
              <div>
                <div className="flex items-center gap-2 text-sm font-bold text-text-main">
                  <Crosshair className="h-4 w-4 text-primary" /> Attack target priority
                </div>
                <p className="mt-1 text-xs text-text-muted">
                  Drag enabled targets into attack order, highest first. Distance only breaks ties between targets in the same row.
                </p>
              </div>

              {activeTargetPriorities.length > 0 ? (
                <div className="mt-3 space-y-2 border-t border-border-base pt-3" role="list" aria-label="Auto Storm target priority order">
                  {activeTargetPriorities.map((priority, index) => {
                    const option = TARGET_PRIORITY_OPTIONS[priority];
                    return (
                      <div
                        key={priority}
                        role="listitem"
                        draggable
                        onDragStart={(event) => {
                          event.dataTransfer.effectAllowed = 'move';
                          event.dataTransfer.setData('text/plain', priority);
                          setDraggedTargetPriority(priority);
                        }}
                        onDragOver={(event) => {
                          event.preventDefault();
                          event.dataTransfer.dropEffect = 'move';
                          if (priority !== draggedTargetPriority) setTargetPriorityDropTarget(priority);
                        }}
                        onDragLeave={(event) => {
                          if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setTargetPriorityDropTarget(null);
                        }}
                        onDrop={(event) => {
                          event.preventDefault();
                          const source = (draggedTargetPriority ?? event.dataTransfer.getData('text/plain')) as AutoStormTargetPriority;
                          if (AUTO_STORM_TARGET_PRIORITIES.some((candidate) => candidate === source)) {
                            moveTargetPriority(source, priority);
                          }
                          finishTargetPriorityDrag();
                        }}
                        onDragEnd={finishTargetPriorityDrag}
                        className={`flex cursor-grab items-center gap-3 rounded-global border bg-bg-card/45 p-3 transition-colors active:cursor-grabbing ${
                          draggedTargetPriority === priority
                            ? 'border-primary/40 opacity-45'
                            : targetPriorityDropTarget === priority
                              ? 'border-primary bg-primary/10'
                              : 'border-border-base hover:border-primary/30'
                        }`}
                      >
                        <GripVertical className="h-5 w-5 shrink-0 text-text-muted" aria-hidden="true" />
                        <span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-bg-app text-xs font-bold tabular-nums text-primary ring-1 ring-border-base">
                          {index + 1}
                        </span>
                        <span className="min-w-0 flex-1">
                          <span className="block text-xs font-bold text-text-main">{option.label}</span>
                          <span className="mt-0.5 block text-[11px] leading-4 text-text-muted">{option.detail}</span>
                        </span>
                        <span className="flex shrink-0 items-center gap-1">
                          <button
                            type="button"
                            disabled={index === 0}
                            onClick={() => moveTargetPriorityBy(priority, -1)}
                            className="rounded-md p-1.5 text-text-muted transition-colors hover:bg-primary/10 hover:text-primary disabled:pointer-events-none disabled:opacity-25"
                            aria-label={`Move ${option.label} up`}
                          >
                            <ArrowUp className="h-3.5 w-3.5" />
                          </button>
                          <button
                            type="button"
                            disabled={index === activeTargetPriorities.length - 1}
                            onClick={() => moveTargetPriorityBy(priority, 1)}
                            className="rounded-md p-1.5 text-text-muted transition-colors hover:bg-primary/10 hover:text-primary disabled:pointer-events-none disabled:opacity-25"
                            aria-label={`Move ${option.label} down`}
                          >
                            <ArrowDown className="h-3.5 w-3.5" />
                          </button>
                        </span>
                      </div>
                    );
                  })}
                </div>
              ) : (
                <p className="mt-3 border-t border-border-base pt-3 text-xs text-text-muted">
                  Enable forts or resource islands and select at least one target to set the attack order.
                </p>
              )}
            </div>

            <div className="mt-3 rounded-global border border-border-base bg-bg-app/30 p-3">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="text-sm font-bold text-text-main">Resource islands</div>
                  <p className="mt-1 text-xs text-text-muted">Capture matching resources and sizes, then return the report-confirmed surviving attack army to the Storm castle.</p>
                </div>
                <Switch
                  checked={draft.islands.enabled}
                  onChange={(enabled) => setDraft((current) => ({ ...current, islands: { ...current.islands, enabled } }))}
                  ariaLabel="Attack Storm resource islands"
                />
              </div>
              {draft.islands.enabled ? (
                <div className="mt-3 space-y-3 border-t border-border-base pt-3">
                  <div>
                    <FieldLabel>Resources</FieldLabel>
                    <ChoiceChipGroup
                      ariaLabel="Storm island resources"
                      options={RESOURCE_OPTIONS}
                      selected={draft.islands.resources}
                      onToggle={(resource) => setDraft((current) => ({
                        ...current,
                        islands: { ...current.islands, resources: toggleChoice(current.islands.resources, resource) },
                      }))}
                    />
                  </div>
                  <div>
                    <FieldLabel>Island sizes</FieldLabel>
                    <ChoiceChipGroup
                      ariaLabel="Storm island sizes"
                      options={SIZE_OPTIONS}
                      selected={draft.islands.sizes}
                      onToggle={(size) => setDraft((current) => ({
                        ...current,
                        islands: { ...current.islands, sizes: toggleChoice(current.islands.sizes, size) },
                      }))}
                    />
                  </div>
                  <PresetSelect
                    value={draft.islands.presetId}
                    onChange={(presetId) => setDraft((current) => ({ ...current, islands: { ...current.islands, presetId } }))}
                    presets={attackPresets.presets}
                    placeholder="Island attack preset"
                  />
                  {selectedIslandPreset ? <PresetSummary preset={selectedIslandPreset} /> : null}

                  <div className="border-t border-border-base pt-3">
                    <div className="flex flex-wrap items-center justify-between gap-3">
                      <div>
                        <div className="flex items-center gap-2 text-xs font-bold text-text-main"><Shield className="h-3.5 w-3.5 text-primary" /> Island defense units</div>
                        <p className="mt-1 text-[11px] text-text-muted">Choose dedicated occupation defenders. If empty, one survivor stays only after the successful battle report confirms the island army.</p>
                      </div>
                      <Button size="sm" variant="outline" onClick={() => void chooseDefenseUnits()} leftIcon={<Shield className="h-3.5 w-3.5" />}>
                        Choose units
                      </Button>
                    </div>
                    {draft.islands.defenseUnits.length > 0 ? (
                      <div className="mt-3 flex flex-wrap gap-2">
                        {draft.islands.defenseUnits.map((unit) => (
                          <div key={unit.unitId} className="flex items-center gap-2 rounded-global border border-border-base bg-bg-card/45 p-2">
                            <UnitImage unitId={unit.unitId} size={32} />
                            <div>
                              <div className="max-w-32 truncate text-xs font-bold text-text-main">{getTroop(unit.unitId)?.name ?? `Unit ${unit.unitId}`}</div>
                              <div className="text-[10px] text-text-muted">{unit.amount.toLocaleString()}</div>
                            </div>
                          </div>
                        ))}
                      </div>
                    ) : <p className="mt-2 text-[11px] text-text-muted">Automatic minimum occupation: after victory is reported, every surviving attack troop except one returns to the Storm castle.</p>}
                  </div>

                  {draft.islands.resources.length === 0 || draft.islands.sizes.length === 0 ? (
                    <p className="text-xs text-error">Select at least one resource and one island size.</p>
                  ) : null}
                </div>
              ) : null}
            </div>

            <div className="mt-3 rounded-global border border-border-base bg-bg-app/30 p-3">
              <div className="flex items-start justify-between gap-4">
                <div>
                  <div className="flex items-center gap-2 text-sm font-bold text-text-main"><Truck className="h-4 w-4 text-primary" /> Import missing troops</div>
                  <p className="mt-1 text-xs text-text-muted">Import missing attack or configured-defense troops from selected donor castles into the Storm castle before launch. Donors never send directly to an island.</p>
                </div>
                <Switch
                  checked={draft.troopImport.enabled}
                  onChange={(enabled) => setDraft((current) => ({
                    ...current,
                    troopImport: { ...current.troopImport, enabled },
                  }))}
                  ariaLabel="Import missing Storm troops"
                />
              </div>
              {draft.troopImport.enabled ? (
                <div className="mt-3 border-t border-border-base pt-3">
                  <FieldLabel>Donor castles</FieldLabel>
                  {troopDonorCastles.length > 0 ? (
                    <ChoiceChipGroup
                      ariaLabel="Storm troop donor castles"
                      options={troopDonorCastles.map((castle) => ({
                        value: castle.id,
                        label: `${castle.name?.trim() || `Castle ${castle.id}`} · K${castle.kingdomId}`,
                      }))}
                      selected={draft.troopImport.donorCastleIds}
                      onToggle={(castleId) => setDraft((current) => ({
                        ...current,
                        troopImport: {
                          ...current.troopImport,
                          donorCastleIds: toggleChoice(current.troopImport.donorCastleIds, castleId),
                        },
                      }))}
                    />
                  ) : <p className="text-xs text-text-muted">No non-Storm donor castles are currently observed.</p>}
                  <div className="mt-3 grid gap-3 md:grid-cols-3">
                    <label>
                      <FieldLabel>Minimum troops kept after launch</FieldLabel>
                      <Input
                        type="number"
                        min={0}
                        value={draft.troopImport.minimumTroops}
                        onChange={(event) => setDraft((current) => ({
                          ...current,
                          troopImport: {
                            ...current.troopImport,
                            minimumTroops: clampAutoStormInteger(event.target.value, 0, Number.MAX_SAFE_INTEGER, 0),
                          },
                        }))}
                      />
                      <p className="mt-1 text-[11px] text-text-muted">Imports the current preset mix so this many attack troops remain stationed in Storm after the next launch.</p>
                    </label>
                    <label>
                      <FieldLabel>Troop-use history</FieldLabel>
                      <Input readOnly value={`Past ${AUTO_STORM_TROOP_HISTORY_HOURS} hours`} />
                      <p className="mt-1 text-[11px] text-text-muted">Fixed rolling window used to calculate the average number of troops sent per hour.</p>
                    </label>
                    <label>
                      <FieldLabel>Current maximum in Storm</FieldLabel>
                      <Input
                        readOnly
                        value={loadingTroopCapPreview
                          ? 'Calculating…'
                          : troopCapPreview?.available
                            ? Math.max(0, Math.trunc(troopCapPreview.maximumTroops)).toLocaleString()
                            : 'Unavailable'}
                        className="font-mono"
                      />
                      <p className="mt-1 text-[11px] text-text-muted">
                        {loadingTroopCapPreview
                          ? 'Calculating directly from the settings shown here.'
                          : troopCapPreviewError
                            ? troopCapPreviewError
                            : troopCapPreview?.available
                              ? `${troopCapPreview.troopsPerAttack.toLocaleString()} troops in the largest enabled attack · ${troopCapPreview.troopsSentInHistory.toLocaleString()} troops sent over ${troopCapPreview.historyHours} hours · ${troopCapPreview.averageTroopsPerHour.toFixed(1)} troops/hour · ${troopCapPreview.bufferedTroops.toLocaleString()} at 2× hourly demand.${troopCapPreview.measuredAttacksInHistory < troopCapPreview.attacksInHistory ? ` ${troopCapPreview.measuredAttacksInHistory.toLocaleString()} of ${troopCapPreview.attacksInHistory.toLocaleString()} launches include measured troop totals.` : ''}`
                              : troopCapPreview?.detail ?? 'Enable a target type and choose its attack preset to calculate the cap.'}
                      </p>
                    </label>
                  </div>
                  <p className="mt-3 text-[11px] text-text-muted">The hard cap is the larger of the minimum reserve plus one largest enabled attack or twice the average troops sent per hour during the rolling past 24 hours. It counts troops stationed in Storm, away on active movements, waiting in transport, and ready to return from islands.</p>
                  <p className="mt-2 text-[11px] text-text-muted">Donors are checked in the displayed order, and partial shortages can be filled across several transfers. Time skips use the construction-and-logistics reserve above. Attack tools must already be stationed in Storm.</p>
                  {!troopImportValid ? <p className="mt-2 text-xs text-error">Select at least one currently observed donor castle.</p> : null}
                </div>
              ) : null}
            </div>

            <div className="mt-4 border-t border-border-base pt-4">
              <div>
                <FieldLabel>Map coverage</FieldLabel>
                <Input readOnly value={stormMapCoverage} />
                <p className="mt-1 text-[11px] text-text-muted">
                  {stormMapState?.windowCount && stormMapState.lastCompletedAt ? `Last completed ${formatDate(stormMapState.lastCompletedAt)}. ` : ''}
                  Coverage is scoped to the current server and account, then expanded when a completed sweep reaches an observed map edge.
                </p>
                <p className="mt-1 text-[11px] text-text-muted">
                  {stormOpportunities.ready} learned targets are ready now.
                  {Number.isFinite(stormOpportunities.nextReadyAt) ? ` Next readyAt label: ${formatDate(new Date(stormOpportunities.nextReadyAt).toISOString())}.` : ''}
                  {' '}Auto Storm wakes on these labels between full sweeps.
                </p>
              </div>
            </div>
          </Card>
        </div>

        <Card variant="solid" className="p-4">
          <SectionHeading
            icon={Package}
            title="Aquamarine spending"
            description="Buy prioritized Luna packages while the protected Aquamarine reserve remains intact."
          />
          <div className="mt-4 grid gap-3 md:grid-cols-2">
            <label>
              <FieldLabel>Keep Aquamarine</FieldLabel>
              <Input
                type="number"
                min={0}
                value={draft.aquamarine.reserve}
                onChange={(event) => setDraft((current) => ({
                  ...current,
                  aquamarine: {
                    ...current.aquamarine,
                    reserve: clampAutoStormInteger(event.target.value, 0, Number.MAX_SAFE_INTEGER, 0),
                  },
                }))}
                className="font-mono"
              />
              <p className="mt-1 text-[11px] text-text-muted">Current: {Math.trunc(aquamarineBalance).toLocaleString()}</p>
            </label>
            <label>
              <FieldLabel>Find a Luna reward</FieldLabel>
              <Input
                value={lunaSearch}
                onChange={(event) => setLunaSearch(event.target.value)}
                placeholder="Reward, category, or package ID"
                leftIcon={<Search className="h-4 w-4" />}
              />
              <p className="mt-1 text-[11px] text-text-muted">
                {loadingPackages ? 'Loading official catalog…' : `${lunaPackages.length} Luna rewards available`}
              </p>
            </label>
          </div>

          <p className="mt-3 rounded-global border border-warning/20 bg-warning/5 px-3 py-2 text-xs text-text-muted">
            These 17 rewards were captured from the current Luna storefront; reward amounts, Aquamarine prices, and shop caps come from official game data. Unlimited goals obey the protected reserve and stop at Luna's stock cap when one exists.
          </p>

          {loadingPackages ? (
            <div className="mt-3 rounded-global border border-border-base bg-bg-app/30 px-4 py-8 text-center text-sm text-text-muted">
              Loading Luna's official rewards…
            </div>
          ) : lunaPackages.length > 0 ? (
            <div className="mt-3 overflow-hidden rounded-global border border-border-base bg-bg-app/30">
              <div className="max-h-[min(34rem,52dvh)] overflow-auto custom-scrollbar">
                <table className="w-full min-w-[72rem] table-fixed text-left text-xs">
                  <thead className="sticky top-0 z-10 border-b border-border-base bg-bg-card/95 text-[10px] uppercase tracking-wider text-text-muted">
                    <tr>
                      <th className="w-[5rem] px-3 py-2.5 text-center font-black">Use</th>
                      <th className="w-[28%] px-3 py-2.5 font-black">Reward</th>
                      <th className="w-[9rem] px-3 py-2.5 text-right font-black">Cost</th>
                      <th className="w-[8rem] px-3 py-2.5 text-center font-black">Purchased / cap</th>
                      <th className="w-[10rem] px-3 py-2.5 font-black">Target total</th>
                      <th className="w-[10rem] px-3 py-2.5 font-black">Unlimited</th>
                      <th className="w-[7rem] px-3 py-2.5 font-black">Priority</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-border-base/70">
                    {visibleLunaPackages.map((item) => {
                      const purchase = configuredLunaPurchases.get(item.id);
                      const enabled = purchase != null;
                      const displayName = lunaPackageDisplayName(item, getTroop, getTool);
                      const purchased = lunaCountersCurrent
                        ? (state?.inventory.constructionOffers[String(item.id)] ?? 0)
                        : undefined;
                      return (
                        <tr key={item.id} className={`transition-colors ${enabled ? 'bg-primary/5' : 'hover:bg-bg-card-hover/40'}`}>
                          <td className="px-3 py-3 text-center align-middle">
                            <Switch
                              size="sm"
                              checked={enabled}
                              onChange={(checked) => toggleShopPurchase(setDraft, item.id, checked)}
                              ariaLabel={`${enabled ? 'Disable' : 'Enable'} ${displayName}`}
                            />
                          </td>
                          <td className="px-3 py-3 align-middle">
                            <div className="flex min-w-0 items-center gap-2">
                              <Badge variant="outline">{lunaPackageTypeLabel(item.type)}</Badge>
                              <div className="min-w-0">
                                <div className="truncate font-bold text-text-main" title={displayName}>{displayName}</div>
                                <div className="mt-0.5 truncate text-[10px] text-text-muted">
                                  PID {item.id}{item.rewardDetail ? ` · ${item.rewardDetail}` : ''}
                                </div>
                              </div>
                            </div>
                          </td>
                          <td className="px-3 py-3 text-right align-middle font-mono font-bold text-text-main">
                            {item.price.toLocaleString()} Aqua
                          </td>
                          <td className="px-3 py-3 text-center align-middle">
                            <div className="font-mono font-bold text-text-main">
                              {purchased == null ? '—' : purchased.toLocaleString()}{item.stock > 0 ? ` / ${item.stock.toLocaleString()}` : ''}
                            </div>
                            <div className="mt-0.5 text-[10px] text-text-muted">
                              {item.stock > 0 ? 'shop cap' : 'no shop cap'}
                            </div>
                          </td>
                          <td className="px-3 py-3 align-middle">
                            <Input
                              type="number"
                              min={1}
                              max={item.stock > 0 ? item.stock : undefined}
                              value={purchase?.targetPurchases ?? 1}
                              disabled={!enabled || purchase?.unlimited === true}
                              onChange={(event) => updateShopPurchase(setDraft, item.id, {
                                targetPurchases: clampAutoStormInteger(
                                  event.target.value,
                                  1,
                                  item.stock > 0 ? item.stock : Number.MAX_SAFE_INTEGER,
                                  1,
                                ),
                              })}
                              className="font-mono"
                            />
                          </td>
                          <td className="px-3 py-3 align-middle">
                            <div className="flex items-center gap-2">
                              <Switch
                                size="sm"
                                checked={purchase?.unlimited === true}
                                disabled={!enabled}
                                onChange={(unlimited) => updateShopPurchase(setDraft, item.id, { unlimited })}
                                ariaLabel={`Unlimited purchases for ${displayName}`}
                              />
                              <span className="text-[10px] leading-tight text-text-muted">
                                {item.stock > 0 ? `Until cap ${item.stock}` : 'Keep buying'}
                              </span>
                            </div>
                          </td>
                          <td className="px-3 py-3 align-middle">
                            <Input
                              type="number"
                              min={0}
                              value={purchase?.priority ?? ''}
                              disabled={!enabled}
                              onChange={(event) => updateShopPurchase(setDraft, item.id, {
                                priority: clampAutoStormInteger(event.target.value, 0, 1_000_000, purchase?.priority ?? 1),
                              })}
                              className="font-mono"
                            />
                          </td>
                        </tr>
                      );
                    })}
                    {visibleLunaPackages.length === 0 ? (
                      <tr>
                        <td colSpan={7} className="px-4 py-10 text-center text-sm text-text-muted">
                          No Luna rewards match “{lunaSearch.trim()}”.
                        </td>
                      </tr>
                    ) : null}
                  </tbody>
                </table>
              </div>
            </div>
          ) : (
            <div className="mt-3 rounded-global border border-error/20 bg-error/5 px-4 py-6 text-center text-sm text-text-muted">
              No Luna rewards were found in the loaded official package catalog.
            </div>
          )}
          {!shopValid ? <p className="mt-2 text-xs text-error">Use one unique Luna product per goal and a positive target count for finite goals.</p> : null}
          {draft.aquamarine.purchases.length > 0 ? (
            <p className="mt-1 text-[11px] text-text-muted">
              Lower priority numbers run first. An uncapped unlimited goal can consume all Aquamarine above the reserve before lower-priority rewards.
            </p>
          ) : null}
        </Card>

        <DailyAttackLimitField
          value={draft.dailyAttackLimit}
          onChange={(dailyAttackLimit) => setDraft((current) => ({ ...current, dailyAttackLimit }))}
          serverState={state?.dailyAttacks}
        />

        <Card variant="solid" className="p-4">
          <SectionHeading icon={Clock3} title="Cadence" description="Map refreshes are authoritative scans; policy checks react sooner to resource, build, troop, and movement changes." />
          <div className="mt-4 grid gap-3 sm:grid-cols-2">
            <label>
              <FieldLabel>Policy check interval</FieldLabel>
              <Input
                type="number"
                min={30}
                max={3600}
                value={draft.checkIntervalSec}
                onChange={(event) => setDraft((current) => ({
                  ...current,
                  checkIntervalSec: clampAutoStormInteger(event.target.value, 30, 3600, current.checkIntervalSec),
                }))}
                rightIcon={<span className="text-xs text-text-muted">sec</span>}
                className="font-mono"
              />
            </label>
            <label>
              <FieldLabel>Map refresh interval</FieldLabel>
              <Input
                readOnly
                value={draft.mapRefreshIntervalSec / 3600}
                rightIcon={<span className="text-xs text-text-muted">hours</span>}
                className="font-mono"
              />
              <p className="mt-1 text-[11px] text-text-muted">Failed or interrupted full sweeps are also held to this interval.</p>
            </label>
          </div>
        </Card>
      </div>
    </SettingsModal>
  );
};

function SectionHeading({
  icon: Icon,
  title,
  description,
}: {
  icon: React.ComponentType<{ className?: string }>;
  title: string;
  description: string;
}) {
  return (
    <div>
      <div className="flex items-center gap-2 text-sm font-black text-text-main"><Icon className="h-4 w-4 text-primary" /> {title}</div>
      <p className="mt-1 text-xs leading-relaxed text-text-muted">{description}</p>
    </div>
  );
}

function FieldLabel({ children, icon: Icon }: { children: React.ReactNode; icon?: React.ComponentType<{ className?: string }> }) {
  return (
    <span className="mb-1.5 flex items-center gap-1.5 text-[10px] font-black uppercase tracking-wider text-text-muted">
      {Icon ? <Icon className="h-3.5 w-3.5" /> : null}{children}
    </span>
  );
}

function PresetSelect({
  value,
  onChange,
  presets,
  placeholder,
}: {
  value: string;
  onChange: (value: string) => void;
  presets: ReturnType<typeof parseAttackPresetDocument>['presets'];
  placeholder: string;
}) {
  return (
    <label className="block">
      <FieldLabel icon={Crosshair}>Attack preset</FieldLabel>
      <Select
        value={value}
        onChange={onChange}
        options={presets.map((preset) => ({ value: preset.id, label: preset.name }))}
        placeholder={presets.length > 0 ? placeholder : 'Create an Attack Preset first'}
        disabled={presets.length === 0}
        searchable
        menuGrowToViewport
      />
    </label>
  );
}

function PresetSummary({ preset }: { preset: ReturnType<typeof parseAttackPresetDocument>['presets'][number] }) {
  const summary = summarizeAttackPreset(preset);
  return (
    <div className="flex flex-wrap gap-2">
      <Badge variant="outline">{summary.waves} waves</Badge>
      <Badge variant="outline">{summary.troops.toLocaleString()} troops</Badge>
      <Badge variant="outline">{summary.tools.toLocaleString()} tools</Badge>
    </div>
  );
}

function autoStormTargetPriorityEnabled(
  state: AutoStormClientStateV1,
  priority: AutoStormTargetPriority,
): boolean {
  if (priority.startsWith('fort:')) {
    return state.forts.enabled && state.forts.levels.includes(Number(priority.slice('fort:'.length)));
  }
  return state.islands.enabled
    && state.islands.sizes.includes(priority.slice('island:'.length) as AutoStormIslandSize);
}

function captureModeLabel(mode: string): string {
  if (mode === 'functional') return 'Functional';
  if (mode === 'layout' || mode === 'buildings') return 'Layout';
  return 'Exact clone';
}

function updateBuild(
  setDraft: React.Dispatch<React.SetStateAction<AutoStormClientStateV1>>,
  update: Partial<AutoStormClientStateV1['build']>,
) {
  setDraft((current) => ({ ...current, build: { ...current.build, ...update } }));
}

function updateNumberMap(
  setDraft: React.Dispatch<React.SetStateAction<AutoStormClientStateV1>>,
  field: 'resourceReserves' | 'sourceResourceReserves' | 'timeSkipReserve',
  key: string,
  value: number,
) {
  setDraft((current) => {
    const next = { ...current.build[field] };
    if (value > 0) next[key] = value;
    else delete next[key];
    return { ...current, build: { ...current.build, [field]: next } };
  });
}

function toggleChoice<T>(values: T[], value: T): T[] {
  return values.includes(value) ? values.filter((candidate) => candidate !== value) : [...values, value];
}

function toggleShopPurchase(
  setDraft: React.Dispatch<React.SetStateAction<AutoStormClientStateV1>>,
  packageId: number,
  enabled: boolean,
) {
  setDraft((current) => {
    const purchases = current.aquamarine.purchases.filter((purchase) => purchase.packageId !== packageId);
    if (!enabled || packageId <= 0) {
      return { ...current, aquamarine: { ...current.aquamarine, purchases } };
    }
    const priority = current.aquamarine.purchases.reduce((maximum, purchase) => Math.max(maximum, purchase.priority), 0) + 1;
    return {
      ...current,
      aquamarine: {
        ...current.aquamarine,
        purchases: [...purchases, { packageId, targetPurchases: 1, unlimited: false, priority }],
      },
    };
  });
}

function updateShopPurchase(
  setDraft: React.Dispatch<React.SetStateAction<AutoStormClientStateV1>>,
  packageId: number,
  update: Partial<AutoStormClientStateV1['aquamarine']['purchases'][number]>,
) {
  setDraft((current) => ({
    ...current,
    aquamarine: {
      ...current.aquamarine,
      purchases: current.aquamarine.purchases.map((purchase) => (
        purchase.packageId === packageId ? { ...purchase, ...update } : purchase
      )),
    },
  }));
}

function parseDecorationPresetOptions(
  value: unknown,
  castles: Record<string, { id: number; name?: string }>,
): DecorationPresetOption[] {
  if (!isRecord(value) || !isRecord(value.castles)) return [];
  const result: DecorationPresetOption[] = [];
  for (const [castleKey, rawPresets] of Object.entries(value.castles)) {
    const castleId = Number(castleKey);
    if (!Number.isFinite(castleId) || castleId <= 0 || !Array.isArray(rawPresets)) continue;
    for (const rawPreset of rawPresets) {
      if (!isRecord(rawPreset) || typeof rawPreset.id !== 'string' || typeof rawPreset.name !== 'string' || !Array.isArray(rawPreset.items)) continue;
      result.push({
        value: `${castleKey}:${rawPreset.id}`,
        castleId,
        presetId: rawPreset.id,
        label: `${rawPreset.name} · ${castles[castleKey]?.name?.trim() || `Castle ${castleKey}`} · ${rawPreset.items.length} decorations`,
        itemCount: rawPreset.items.length,
      });
    }
  }
  return result.sort((left, right) => left.label.localeCompare(right.label));
}

function parseStormCastleOptions(rows: Record<string, unknown>[], playerLevel?: number): StormCastleOption[] {
  const availableLevel = playerLevel && playerLevel > 0 ? playerLevel : Number.MAX_SAFE_INTEGER;
  const options: StormCastleOption[] = [];
  for (const row of rows) {
    const spaces = String(row.spaceIDs ?? '')
      .split(',')
      .map((value) => Number(value.trim()))
      .filter(Number.isFinite);
    const id = positiveInteger(row.preBuiltCastleID);
    const minLevel = positiveInteger(row.minLevel);
    if (!spaces.includes(4) || id <= 0 || minLevel > availableLevel) continue;
    options.push({
      id,
      name: stringValue(row.comment2),
      minLevel,
      costWood: positiveInteger(row.costWood),
      costStone: positiveInteger(row.costStone),
      costFood: positiveInteger(row.costFood),
      costCoins: positiveInteger(row.costC1),
      costPremium: positiveInteger(row.costC2),
    });
  }
  return options.sort((left, right) => left.id - right.id);
}

function preferredStormCastleOption(options: StormCastleOption[]): StormCastleOption | undefined {
  return options.find((option) => option.costPremium === 0) ?? options[0];
}

function stormCastleOptionLabel(option: StormCastleOption): string {
  const costs = [
    [option.costWood, 'wood'],
    [option.costStone, 'stone'],
    [option.costFood, 'food'],
    [option.costCoins, 'coins'],
    [option.costPremium, 'premium'],
  ] as const;
  const price = costs
    .filter(([amount]) => amount > 0)
    .map(([amount, currency]) => `${amount.toLocaleString()} ${currency}`)
    .join(' · ');
  return `${stormCastleOptionName(option)}${price ? ` · ${price}` : ''} · ID ${option.id}`;
}

function stormCastleOptionName(option: StormCastleOption): string {
  switch (option.name.toLowerCase()) {
    case 'cheapcamp': return 'Starter castle';
    case 'resourcecamp': return 'Resource castle';
    case 'c2camp': return 'Premium castle';
    default: return humanizeLunaName(option.name) || `Storm castle ${option.id}`;
  }
}

function parseLunaPackages(rows: Record<string, unknown>[]): LunaPackage[] {
  const byID = new Map<number, LunaPackage>();
  for (const row of rows) {
    const id = positiveInteger(row.packageID);
    if (!LUNA_PACKAGE_ID_SET.has(id) || stringValue(row.comment2).toLowerCase() !== "luna's trade boat") continue;
    const price = positiveInteger(row.packagePriceAquamarine);
    if (id <= 0 || price <= 0) continue;
    const type = id === 3124 || id === 3125 ? 'relicGem' : stringValue(row.packageType);
    const unitId = positiveInteger(row.unitID);
    const unitAmount = positiveInteger(row.unitAmount);
    const buildingId = positiveInteger(row.buildingID);
    const buildingAmount = positiveInteger(row.buildingAmount);
    byID.set(id, {
      id,
      price,
      name: lunaPackageName(id, row),
      type,
      stock: positiveInteger(row.stock),
      unitId,
      unitAmount,
      buildingId,
      buildingAmount,
      rewardDetail: lunaRewardDetail(id, row, type, unitAmount, buildingAmount),
    });
  }
  return AUTO_STORM_LUNA_PACKAGE_IDS.flatMap((id) => {
    const item = byID.get(id);
    return item ? [item] : [];
  });
}

function lunaPackageName(id: number, row: Record<string, unknown>): string {
  switch (id) {
    case 3116: return 'Crab rock (3,500 PO)';
    case 3117: return 'Whale lagoon (3,750 PO)';
    case 3118: return 'Tide garden (3,500 PO)';
    case 3119: return 'Silver pieces';
    case 3120: return 'Gold pieces';
    case 3122: return 'Skip 5 hours';
    case 3123: return 'Skip 24 hours';
    case 3124: return 'Castellan generation-2 gem';
    case 3125: return 'Commander generation-2 gem';
    case 2795:
    case 2796:
    case 2797:
    case 2798: {
      const amount = positiveInteger(row.amountC1);
      return amount > 0 ? `${amount.toLocaleString()} coins` : 'Coins';
    }
    default: return humanizeLunaName(stringValue(row.comment1)) || `Luna package ${id}`;
  }
}

type LunaNameLookup = (id: number) => { name: string } | undefined;

function lunaPackageDisplayName(
  item: LunaPackage,
  getTroop: LunaNameLookup,
  getTool: LunaNameLookup,
): string {
  if (item.type === 'deco') return item.name;
  const metadata = item.type === 'tool' && item.unitId > 0
    ? getTool(item.unitId)
    : item.type === 'soldier' && item.unitId > 0
      ? getTroop(item.unitId)
      : undefined;
  return metadata?.name?.trim() || item.name;
}

function lunaPackageTypeLabel(value: string): string {
  switch (value.toLowerCase()) {
    case 'currency': return 'Currency';
    case 'deco': return 'Decoration';
    case 'gem': return 'Gem';
    case 'item': return 'Equipment';
    case 'minuteskip': return 'Time skips';
    case 'relicgem': return 'Relic gem';
    case 'relicitem': return 'Relic equipment';
    case 'soldier': return 'Troops';
    case 'tool': return 'Attack tools';
    case 'xp': return 'Experience';
    default: return humanizeLunaName(value) || 'Package';
  }
}

function lunaRewardDetail(
  id: number,
  row: Record<string, unknown>,
  type: string,
  unitAmount: number,
  buildingAmount: number,
): string {
  if (id === 3124) return '1 random generation-2 Castellan gem per purchase';
  if (id === 3125) return '1 random generation-2 Commander gem per purchase';
  const rewards: string[] = [];
  if (unitAmount > 0) rewards.push(`${unitAmount.toLocaleString()} ${type === 'tool' ? 'tools' : 'troops'} per purchase`);
  if (buildingAmount > 0) rewards.push(`${buildingAmount.toLocaleString()} ${buildingAmount === 1 ? 'decoration' : 'decorations'} per purchase`);
  const knownRewards: Array<[string, string]> = [
    ['amountC1', 'coins'],
    ['amountXP', 'XP'],
    ['addSceatToken', 'Sceat tokens'],
    ['addSilverToken', 'silver pieces'],
    ['addGoldToken', 'gold pieces'],
    ['add5HourSkip', '5-hour skips'],
    ['add24HourSkip', '24-hour skips'],
  ];
  for (const [field, label] of knownRewards) {
    const amount = positiveInteger(row[field]);
    if (amount > 0) rewards.push(`${amount.toLocaleString()} ${label} per purchase`);
  }
  return rewards.join(' · ');
}

function humanizeLunaName(value: string): string {
  return value
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/[_-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function positiveInteger(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.trunc(parsed) : 0;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function formatDate(value: string): string {
  const timestamp = Date.parse(value);
  return Number.isFinite(timestamp) ? new Date(timestamp).toLocaleString() : value;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}
