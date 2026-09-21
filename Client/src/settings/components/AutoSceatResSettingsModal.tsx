import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import {
  ArrowDown,
  ArrowUp,
  Coins,
  Factory,
  Gem,
  Plus,
  ShieldCheck,
  Trash2,
  Truck,
  Warehouse,
} from 'lucide-react';
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ChoiceChipGroup,
  Input,
  ScheduleSummaryRow,
  Select,
  SettingsToggleRow,
  SettingsModal,
  Switch,
} from '../../components/ui';
import {
  emptyAutoSceatResCatalog,
  defaultAutoSceatResSettings,
  normalizeAutoSceatResSettings,
  normalizeAutoSceatResCatalog,
  persistAutoSceatResSettings,
  type AutoSceatBuildingPlan,
  type AutoSceatBuildingState,
  type AutoSceatRecipeCatalogEntry,
  type AutoSceatResCatalog,
  type AutoSceatResClientSettings,
  type AutoSceatStorageNode,
} from '../AutoSceatResClientState';
import { normalizeFeatureSchedules, scheduleSummary } from '../SchedulerTypes';
import { useCitadelAPI } from '../../api/ApiContext';
import { CitadelAPI } from '../../api/CitadelClient';
import { configurationSection } from '../Configuration';
import { AutoSceatRecipePickerModal } from './AutoSceatRecipePickerModal';
import { useMetadata } from '../../context/MetadataContext';

interface AutoSceatResSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
}

const EMPTY_BUILDING_PLAN: AutoSceatBuildingPlan = {
  enabled: false,
  steps: [],
  cursor: 0,
  autoRentActiveSlot: false,
  autoRentQueueSlots: 0,
};

function recipeForID(catalog: AutoSceatResCatalog, recipeID: number): AutoSceatRecipeCatalogEntry | undefined {
  return catalog.recipes.find((recipe) => recipe.recipeID === recipeID);
}

function formatCompact(value: number): string {
  return new Intl.NumberFormat(undefined, { notation: 'compact', maximumFractionDigits: 1 }).format(value);
}

function rentalTotal(plan: AutoSceatBuildingPlan): number {
  const queueCosts = [0, 500_000, 3_500_000, 10_000_000];
  return (plan.autoRentActiveSlot ? 5_000_000 : 0) + queueCosts[plan.autoRentQueueSlots];
}

function buildingIcon(queueTypeID: number): React.ReactNode {
  if (queueTypeID >= 3) return <ShieldCheck className="h-4 w-4 text-warning" />;
  return <Factory className="h-4 w-4 text-primary" />;
}

export const AutoSceatResSettingsModal: React.FC<AutoSceatResSettingsModalProps> = ({
  isOpen,
  onClose,
  onOpenFeatureSchedule,
}) => {
  const { t: localizeStatic } = useStaticLocale();
  const { configuration, state, submitIntent } = useCitadelAPI();
  const { currencies } = useMetadata();
  const [settings, setSettings] = useState<AutoSceatResClientSettings>(() => defaultAutoSceatResSettings());
  const [catalog, setCatalog] = useState<AutoSceatResCatalog>(() => emptyAutoSceatResCatalog());
  const [catalogError, setCatalogError] = useState<string | null>(null);
  const featureSchedules = normalizeFeatureSchedules(
    configurationSection(configuration, 'scheduler').featureSchedules,
  );
  const [pickerTarget, setPickerTarget] = useState<{ castleID: number; building: AutoSceatBuildingState } | null>(null);
  const [isSaving, setIsSaving] = useState(false);
  const [saveError, setSaveError] = useState<string | null>(null);
  const craftingRevision = state?.observations.crin?.lastRevision ?? 0;

  useEffect(() => {
    if (!isOpen) {
      setSaveError(null);
      return;
    }
    setSettings(normalizeAutoSceatResSettings(
      configuration?.sections['automation.autoSceatResources'] ?? defaultAutoSceatResSettings(),
    ));
    let active = true;
    const load = async () => {
      try {
        if (state?.session.socketReady) {
          await submitIntent('crafting.refresh');
        }
        const projection = await CitadelAPI.getProjection<AutoSceatResCatalog>('crafting');
        if (!active) return;
        setCatalog(normalizeAutoSceatResCatalog(projection));
        setCatalogError(null);
      } catch (error) {
        if (active) setCatalogError(error instanceof Error ? error.message : 'Could not load crafting data.');
      }
    };
    void load();
    return () => { active = false; };
  }, [configuration?.sections, isOpen, state?.session.socketReady, submitIntent]);

  useEffect(() => {
    if (!isOpen || craftingRevision === 0) return;
    let active = true;
    void CitadelAPI.getProjection<AutoSceatResCatalog>('crafting')
      .then((projection) => {
        if (!active) return;
        setCatalog(normalizeAutoSceatResCatalog(projection));
        setCatalogError(null);
      })
      .catch((error) => {
        if (active) setCatalogError(error instanceof Error ? error.message : 'Could not load crafting data.');
      });
    return () => { active = false; };
  }, [craftingRevision, isOpen]);

  const craftingNodes = useMemo(() => catalog.nodes.filter((node) => node.canCraft && node.buildings.length > 0), [catalog.nodes]);
  const storageNodes = useMemo(() => catalog.nodes.filter((node) => !node.canCraft), [catalog.nodes]);
  const schedule = featureSchedules.autoSceatRes;
  const timeSkips = useMemo(() => Object.values(currencies)
    .map((currency) => ({
      id: typeof currency.JSONKey === 'string' ? currency.JSONKey.toUpperCase() : '',
      label: currency.name,
    }))
    .filter((currency) => /^MS\d+$/.test(currency.id))
    .sort((left, right) => Number(left.id.slice(2)) - Number(right.id.slice(2))), [currencies]);

  const buildingPlan = (castleID: number, queueTypeID: number): AutoSceatBuildingPlan => (
    settings.castles[String(castleID)]?.buildings[String(queueTypeID)] ?? EMPTY_BUILDING_PLAN
  );

  const updateBuildingPlan = (
    castleID: number,
    queueTypeID: number,
    update: (current: AutoSceatBuildingPlan) => AutoSceatBuildingPlan,
  ) => {
    setSettings((current) => {
      const castleKey = String(castleID);
      const buildingKey = String(queueTypeID);
      const castle = current.castles[castleKey] ?? { buildings: {} };
      const nextPlan = update(castle.buildings[buildingKey] ?? EMPTY_BUILDING_PLAN);
      return normalizeAutoSceatResSettings({
        ...current,
        castles: {
          ...current.castles,
          [castleKey]: {
            buildings: {
              ...castle.buildings,
              [buildingKey]: nextPlan,
            },
          },
        },
      });
    });
  };

  const addRecipe = (castleID: number, building: AutoSceatBuildingState, recipe: AutoSceatRecipeCatalogEntry) => {
    updateBuildingPlan(castleID, building.queueTypeID, (current) => ({
      ...current,
      enabled: true,
      steps: [...current.steps, { recipeID: recipe.recipeID, repeat: 1 }],
    }));
  };

  const handleSave = async () => {
    setIsSaving(true);
    setSaveError(null);
    try {
      await persistAutoSceatResSettings(settings);
      onClose();
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Could not save Auto Sceat Resources settings.');
    } finally {
      setIsSaving(false);
    }
  };

  const handleClose = () => {
    if (isSaving) return;
    setSettings(normalizeAutoSceatResSettings(
      configuration?.sections['automation.autoSceatResources'] ?? defaultAutoSceatResSettings(),
    ));
    onClose();
  };

  const renderToggle = (
    label: string,
    detail: string,
    checked: boolean,
    onChange: (checked: boolean) => void,
    disabled = false,
  ) => (
    <SettingsToggleRow
      title={label}
      description={detail}
      checked={checked}
      onChange={onChange}
      disabled={disabled}
    />
  );

  const renderStorageNode = (node: AutoSceatStorageNode) => {
    const kingdomResources = ['coal', 'oil', 'glass', 'iron'];
    return (
      <div key={node.castleID} className="rounded-global border border-border-base bg-bg-card/55 px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="min-w-0">
            <div className="truncate text-sm font-black text-text-main">{node.name}</div>
            <div className="text-[11px] font-semibold text-text-muted">{node.role} · Kingdom {node.kingdomID}</div>
          </div>
          <Badge variant={node.stormBuffer ? 'warning' : 'secondary'}>{node.stormBuffer ? 'Buffer' : 'Storage'}</Badge>
        </div>
        <div className="mt-3 grid grid-cols-2 gap-1.5">
          {kingdomResources.map((resource) => {
            const amount = node.resources[resource] ?? 0;
            const max = node.storage[resource] ?? 0;
            if (amount <= 0 && max <= 0) return null;
            return (
              <div key={resource} className="flex items-center justify-between rounded-lg bg-bg-input/45 px-2 py-1.5 text-[10px] font-bold text-text-muted">
                <span className="capitalize">{resource}</span>
                <span className="tabular-nums text-text-main">{formatCompact(amount)} / {formatCompact(max)}</span>
              </div>
            );
          })}
        </div>
      </div>
    );
  };

  return (
    <>
      <SettingsModal
        isOpen={isOpen}
        onClose={handleClose}
        maxWidth="full"
        title={localizeStatic("ui.settings.components.autoSceatResSettingsModal.title.auto.sceat.resources.a647166a")}
        icon={<Factory className="h-5 w-5" />}
        description={localizeStatic("ui.settings.components.autoSceatResSettingsModal.description.research.aware.crafting.queues.and.kingdom.resource.394ed6a5")}
        onSave={handleSave}
        isSaving={isSaving}
      >
        <div className="mx-auto flex w-full max-w-[1780px] flex-col gap-5 pb-2">
          {catalogError && (
            <div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error">
              {catalogError}
            </div>
          )}
          {saveError && (
            <div className="rounded-global border border-error/30 bg-error/10 px-4 py-3 text-sm font-semibold text-error" role="alert">
              {saveError}
            </div>
          )}
          <Card variant="solid" className="liquid-prominent-header-card">
            <CardHeader className="liquid-card-header-prominent">
              <div>
                <CardTitle className="flex items-center gap-2 text-base"><Truck className="h-4 w-4 text-primary" />Automation & Logistics</CardTitle>
                <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.schedule.queue.checks.control.resource.movement.and.77b1a0cd" /></p>
              </div>
            </CardHeader>
            <CardContent className="liquid-prominent-header-content grid gap-4 p-5 xl:grid-cols-[1fr_1.2fr_1fr]">
              <div className="grid content-start gap-3">
                <div className="grid grid-cols-2 gap-3">
                  <label className="grid gap-1.5 text-xs font-bold text-text-muted">
                    Check interval
                    <Input
                      type="number"
                      min={1}
                      value={Math.max(1, Math.round(settings.checkIntervalSec / 60))}
                      onChange={(event) => setSettings((current) => normalizeAutoSceatResSettings({ ...current, checkIntervalSec: Number(event.target.value) * 60 }))}
                      rightIcon={<span className="text-[10px] font-black uppercase"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.min.1f6fa6f6" /></span>}
                    />
                  </label>
                  <label className="grid gap-1.5 text-xs font-bold text-text-muted">Minimum shipment
                    <Input type="number" min={0} value={settings.minimumShipmentSize} onChange={(event) => setSettings((current) => normalizeAutoSceatResSettings({ ...current, minimumShipmentSize: Number(event.target.value) }))} />
                  </label>
                </div>
                <ScheduleSummaryRow
                  summary={schedule ? scheduleSummary(schedule) : 'Runs any time'}
                  onEdit={() => onOpenFeatureSchedule('autoSceatRes', 'Auto Sceat Resources')}
                />
                <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
                  <label className="grid gap-1.5 text-xs font-bold text-text-muted">Overflow starts
                    <Input type="number" min={50} max={100} value={settings.overflowThresholdPercent} onChange={(event) => setSettings((current) => normalizeAutoSceatResSettings({ ...current, overflowThresholdPercent: Number(event.target.value) }))} rightIcon={<span className="text-xs font-black">%</span>} />
                  </label>
                  <label className="grid gap-1.5 text-xs font-bold text-text-muted">Minimum coin reserve
                    <Input type="number" min={0} value={settings.minimumCoinReserve} onChange={(event) => setSettings((current) => normalizeAutoSceatResSettings({ ...current, minimumCoinReserve: Number(event.target.value) }))} leftIcon={<Coins className="h-4 w-4" />} />
                  </label>
                  <label className="grid gap-1.5 text-xs font-bold text-text-muted">Minimum ruby reserve
                    <Input type="number" min={0} value={settings.minimumRubyReserve} onChange={(event) => setSettings((current) => normalizeAutoSceatResSettings({ ...current, minimumRubyReserve: Number(event.target.value) }))} leftIcon={<Gem className="h-4 w-4" />} />
                  </label>
                </div>
              </div>

              <div className="grid content-start gap-2.5">
                {renderToggle('Resource logistics', 'Drains sovereign-resource surplus into configured future queue refills, even while queues are full.', settings.autoKingdomTransport, (checked) => setSettings((current) => ({ ...current, autoKingdomTransport: checked })))}
                {renderToggle('Use transport time skips', 'Applies only selected skips, one command per confirmed response, to kingdom resource transports (TT 2).', settings.useKingdomTimeSkips, (checked) => setSettings((current) => ({ ...current, useKingdomTimeSkips: checked })), !settings.autoKingdomTransport)}
                {renderToggle('Use Storm as overflow buffer', 'Storm can hold kingdom resources even though it cannot craft them.', settings.useStormBuffer, (checked) => setSettings((current) => ({ ...current, useStormBuffer: checked })))}
                {renderToggle('Allow ruby recipes', 'Explicit permission for recipes whose official cost includes C2/rubies.', settings.allowRubyRecipes, (checked) => setSettings((current) => ({ ...current, allowRubyRecipes: checked })))}
                {renderToggle('Ruby-skip blocked overflow', 'Completes at most one Green main resource craft per cycle when threshold overflow cannot be moved, using the official remaining-time ruby price.', settings.useRubyOverflowSkip, (checked) => setSettings((current) => ({ ...current, useRubyOverflowSkip: checked })), !settings.autoKingdomTransport)}
                {settings.useRubyOverflowSkip && (
                  <div className="rounded-global border border-warning/30 bg-warning/8 px-3 py-2 text-[10px] font-semibold leading-relaxed text-warning">
                    <LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.ruby.spending.is.limited.to.production.slots.e2ddc881" /></div>
                )}
              </div>

              <div className="grid content-start gap-3">
                <div>
                  <div className="text-xs font-black uppercase tracking-wide text-text-muted"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.allowed.transport.skips.73eba8a9" /></div>
                  <ChoiceChipGroup
                    className="mt-2"
                    ariaLabel={localizeStatic("ui.settings.components.autoSceatResSettingsModal.ariaLabel.allowed.transport.skips.73eba8a9")}
                    options={timeSkips.map((skip) => ({ value: skip.id, label: skip.label }))}
                    selected={settings.allowedTimeSkips}
                    disabled={!settings.useKingdomTimeSkips || !settings.autoKingdomTransport}
                    onToggle={(skipID) => setSettings((current) => normalizeAutoSceatResSettings({
                      ...current,
                      allowedTimeSkips: current.allowedTimeSkips.includes(skipID)
                        ? current.allowedTimeSkips.filter((id) => id !== skipID)
                        : [...current.allowedTimeSkips, skipID],
                    }))}
                  />
                  {settings.useKingdomTimeSkips && settings.autoKingdomTransport && (
                    <div className="mt-3 grid grid-cols-2 gap-2 sm:grid-cols-3">
                      {timeSkips.filter((skip) => settings.allowedTimeSkips.includes(skip.id)).map((skip) => (
                        <label key={skip.id} className="grid gap-1 text-[10px] font-bold text-text-muted">
                          Keep {skip.label}
                          <Input
                            type="number"
                            min={0}
                            value={settings.timeSkipReserve[skip.id] ?? 0}
                            onChange={(event) => setSettings((current) => normalizeAutoSceatResSettings({
                              ...current,
                              timeSkipReserve: {
                                ...current.timeSkipReserve,
                                [skip.id]: Number(event.target.value),
                              },
                            }))}
                            className="!py-1.5 text-xs"
                          />
                        </label>
                      ))}
                    </div>
                  )}
                </div>
                <div className="rounded-global border border-border-base bg-bg-input/35 px-4 py-3 text-[11px] font-medium leading-relaxed text-text-muted">
                  <LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.the.smallest.selected.skip.that.completes.a.e43403b9" /></div>
              </div>
            </CardContent>
          </Card>

          <div className="grid grid-cols-1 gap-5 2xl:grid-cols-[minmax(0,1fr)_20rem]">
            <div className="grid gap-5">
              {craftingNodes.map((node) => (
                <Card key={node.castleID} variant="solid">
                  <CardHeader className="flex flex-row items-center justify-between gap-3">
                    <div className="min-w-0">
                      <CardTitle className="truncate text-base">{node.name}</CardTitle>
                      <p className="mt-1 text-xs font-semibold text-text-muted">{node.role} · Kingdom {node.kingdomID} · {node.buildings.length} crafting building{node.buildings.length === 1 ? '' : 's'}</p>
                    </div>
                    <Badge variant="success"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.crafting.000b8216" /></Badge>
                  </CardHeader>
                  <CardContent className="grid gap-4 p-5 xl:grid-cols-2">
                    {node.buildings.map((building) => {
                      const plan = buildingPlan(node.castleID, building.queueTypeID);
                      const weeklyRental = rentalTotal(plan);
                      return (
                        <div key={building.queueTypeID} className={`rounded-global border p-4 transition ${plan.enabled ? 'border-primary/30 bg-primary/[0.035]' : 'border-border-base bg-bg-card/45'}`}>
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <div className="flex items-center gap-2 text-sm font-black text-text-main">{buildingIcon(building.queueTypeID)}{building.name}</div>
                              <div className="mt-1 text-[11px] font-semibold text-text-muted">
                                Live slots: {building.activeRecipes.length}/{building.activeCapacity} active · {building.queuedRecipes.length}/{building.queueCapacity} queued
                              </div>
                            </div>
                            <div className="flex items-center gap-2">
                              <Badge variant={plan.enabled ? 'success' : 'secondary'}>{plan.enabled ? 'On' : 'Off'}</Badge>
                              <Switch checked={plan.enabled} onChange={(checked) => updateBuildingPlan(node.castleID, building.queueTypeID, (current) => ({ ...current, enabled: checked }))} size="sm" ariaLabel={`Enable ${building.name} crafting in ${node.name}`} />
                            </div>
                          </div>

                          <div className="mt-4 grid gap-2 rounded-global border border-border-base bg-bg-input/25 p-3 sm:grid-cols-2">
                            <div className="flex items-center justify-between gap-3">
                              <div>
                                <div className="text-xs font-bold text-text-main"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.rent.second.active.deea61a0" /></div>
                                <div className="text-[10px] font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.5m.coins.7.days.a508e0e6" /></div>
                              </div>
                              <Switch checked={plan.autoRentActiveSlot} onChange={(checked) => updateBuildingPlan(node.castleID, building.queueTypeID, (current) => ({ ...current, autoRentActiveSlot: checked }))} size="sm" ariaLabel={`Rent a second active slot for ${building.name} in ${node.name}`} />
                            </div>
                            <div className="flex items-center justify-between gap-3">
                              <div>
                                <div className="text-xs font-bold text-text-main"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.rent.extra.queue.slots.0e3a3da0" /></div>
                                <div className="text-[10px] font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.0.5m.10m.coins.7.days.2db82a0e" /></div>
                              </div>
                              <Switch
                                checked={plan.autoRentQueueSlots > 0}
                                onChange={(checked) => updateBuildingPlan(node.castleID, building.queueTypeID, (current) => ({
                                  ...current,
                                  autoRentQueueSlots: checked ? Math.max(1, current.autoRentQueueSlots) : 0,
                                }))}
                                size="sm"
                                ariaLabel={`Rent extra queue slots for ${building.name} in ${node.name}`}
                              />
                            </div>
                            {plan.autoRentQueueSlots > 0 && (
                              <div className="sm:col-span-2">
                                <div className="mb-1 text-xs font-bold text-text-main"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.queue.slots.to.rent.4b5d84c2" /></div>
                                <Select
                                  value={String(plan.autoRentQueueSlots)}
                                  onChange={(value) => updateBuildingPlan(node.castleID, building.queueTypeID, (current) => ({ ...current, autoRentQueueSlots: Number(value) }))}
                                  options={[
                                    { value: '1', label: '+1 · 0.5m' },
                                    { value: '2', label: '+2 · 3.5m total' },
                                    { value: '3', label: '+3 · 10m total' },
                                  ]}
                                />
                              </div>
                            )}
                            {weeklyRental > 0 && <div className="sm:col-span-2 text-[10px] font-bold text-warning">Maximum selected renewal: {formatCompact(weeklyRental)} coins per 7 days for this building.</div>}
                          </div>

                          <div className="mt-4 flex items-center justify-between gap-3">
                            <div>
                              <div className="text-xs font-black uppercase tracking-wide text-text-muted"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.repeating.recipe.cycle.ca9d6751" /></div>
                              <div className="mt-0.5 text-[10px] font-medium text-text-muted"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.one.item.fills.every.slot.add.more.9a74f34c" /></div>
                            </div>
                            <Button variant="outline" size="sm" onClick={() => setPickerTarget({ castleID: node.castleID, building })} leftIcon={<Plus className="h-4 w-4" />}><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.recipe.aec69352" /></Button>
                          </div>

                          <div className="mt-3 grid gap-2">
                            {plan.steps.map((step, index) => {
                              const recipe = recipeForID(catalog, step.recipeID);
                              return (
                                <div key={`${step.recipeID}-${index}`} className="flex min-w-0 items-center gap-3 rounded-global border border-border-base bg-bg-card/65 px-3 py-2.5">
                                  <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-lg border border-border-base bg-bg-input/60">
                                    {recipe?.output.iconUrl ? <img src={recipe.output.iconUrl} alt="" className="h-8 w-8 object-contain" /> : <span className="text-xs font-black text-primary">#{step.recipeID}</span>}
                                  </span>
                                  <div className="min-w-0 flex-1">
                                    <div className="truncate text-xs font-black text-text-main">{recipe?.output.name ?? `Recipe #${step.recipeID}`}</div>
                                    <div className="mt-0.5 flex flex-wrap gap-1 text-[10px] font-semibold text-text-muted">
                                      {recipe && <><span>Level {recipe.level}</span><span>·</span><span>{recipe.type}</span><span>·</span></>}
                                      <span>Recipe #{step.recipeID}</span>
                                    </div>
                                  </div>
                                  <label className="flex shrink-0 items-center gap-1 text-[10px] font-bold text-text-muted">
                                    ×
                                    <input
                                      type="number"
                                      min={1}
                                      max={100}
                                      value={step.repeat}
                                      onChange={(event) => updateBuildingPlan(node.castleID, building.queueTypeID, (current) => ({
                                        ...current,
                                        steps: current.steps.map((item, itemIndex) => itemIndex === index ? { ...item, repeat: Number(event.target.value) } : item),
                                      }))}
                                      className="w-14 rounded-lg border border-border-base bg-bg-input/70 px-2 py-1 text-center text-xs font-black text-text-main outline-none focus:border-primary"
                                    />
                                  </label>
                                  <div className="flex shrink-0 items-center">
                                    <Button variant="ghost" size="icon" disabled={index === 0} onClick={() => updateBuildingPlan(node.castleID, building.queueTypeID, (current) => {
                                      const steps = [...current.steps];
                                      [steps[index - 1], steps[index]] = [steps[index], steps[index - 1]];
                                      return { ...current, steps };
                                    })} title={localizeStatic("ui.settings.components.autoSceatResSettingsModal.title.move.up.c66feb5e")}><ArrowUp className="h-3.5 w-3.5" /></Button>
                                    <Button variant="ghost" size="icon" disabled={index === plan.steps.length - 1} onClick={() => updateBuildingPlan(node.castleID, building.queueTypeID, (current) => {
                                      const steps = [...current.steps];
                                      [steps[index], steps[index + 1]] = [steps[index + 1], steps[index]];
                                      return { ...current, steps };
                                    })} title={localizeStatic("ui.settings.components.autoSceatResSettingsModal.title.move.down.40bb50da")}><ArrowDown className="h-3.5 w-3.5" /></Button>
                                    <Button variant="ghost" size="icon" className="text-error" onClick={() => updateBuildingPlan(node.castleID, building.queueTypeID, (current) => ({ ...current, steps: current.steps.filter((_, itemIndex) => itemIndex !== index), cursor: 0 }))} title={localizeStatic("ui.settings.components.autoSceatResSettingsModal.title.remove.c3812fc4")}><Trash2 className="h-3.5 w-3.5" /></Button>
                                  </div>
                                </div>
                              );
                            })}
                            {plan.steps.length === 0 && (
                              <button type="button" onClick={() => setPickerTarget({ castleID: node.castleID, building })} className="rounded-global border border-dashed border-border-base bg-bg-card/35 px-4 py-5 text-center text-xs font-semibold text-text-muted transition hover:border-primary/40 hover:text-primary">
                                <LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.add.the.first.recipe.for.this.building.9195186d" /></button>
                            )}
                          </div>
                        </div>
                      );
                    })}
                  </CardContent>
                </Card>
              ))}
              {craftingNodes.length === 0 && (
                <Card variant="solid"><CardContent className="p-10 text-center text-sm font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.no.crafting.buildings.are.loaded.connect.the.d367e3ff" /></CardContent></Card>
              )}
            </div>

            <Card variant="solid" className="h-fit 2xl:sticky 2xl:top-0">
              <CardHeader>
                <div>
                  <CardTitle className="flex items-center gap-2 text-base"><Warehouse className="h-4 w-4 text-primary" />Additional Storage Nodes</CardTitle>
                  <p className="mt-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.the.four.crafting.castles.are.donors.and.35534627" /></p>
                </div>
              </CardHeader>
              <CardContent className="grid gap-3 p-4">
                {storageNodes.map(renderStorageNode)}
                {storageNodes.length === 0 && <div className="rounded-global border border-dashed border-border-base px-4 py-6 text-center text-xs font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.no.additional.storage.nodes.discovered.71f389fd" /></div>}
                <div className="rounded-global border border-primary/20 bg-primary/[0.04] px-4 py-3 text-[11px] font-medium leading-relaxed text-text-muted">
                  <LocalizedText messageKey="ui.settings.components.autoSceatResSettingsModal.logistics.capacity.is.calculated.automatically.from.current.52f0c37c" /></div>
              </CardContent>
            </Card>
          </div>
        </div>
      </SettingsModal>

      <AutoSceatRecipePickerModal
        isOpen={pickerTarget != null}
        building={pickerTarget?.building ?? null}
        catalog={catalog}
        allowRubyRecipes={settings.allowRubyRecipes}
        onClose={() => setPickerTarget(null)}
        onSelect={(recipe) => {
          if (pickerTarget) addRecipe(pickerTarget.castleID, pickerTarget.building, recipe);
        }}
      />
    </>
  );
};
