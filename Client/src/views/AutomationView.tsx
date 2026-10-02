import { Banner } from '../components/ui/Banner';
import { Card, SectionHeader } from '../components/ui';
import { StatusBadge } from '../components/ui/StatusBadge';
import { useAutomationPlayerStatus } from '../settings/readiness/useAutomationPlayerStatus';
import { automationPlayerStatus } from '../settings/readiness/playerStatus';
import type { SettingsFeatureId as StatusFeatureId } from '../settings/disclosure/placement';
import { rateView, dailyView, type CountView } from '../components/automation/attackCounts';
import { useHostedRuntimePresence } from '../config/Deployment';
import {nextWakeParameters,timedRemainingParameters} from '../i18n/automationDuration';
import {automationDetailMessage, automationLaneMessage} from '../i18n/automationMessages';
import {useLocalizedMessage} from '../i18n/useLocalizedMessage';
import {messageLanguageAttributes} from '../i18n/messageLanguage';
import {describeMessage, type MessageKey, type MessageParameters} from '../i18n/messages';
import type {LocalizedMessage} from '../i18n/formatMessage';
import { LocalizedRichText } from "../i18n/LocalizedRichText";
import { useLocale as useStaticLocale } from "../i18n/LocaleContext";
import { LocalizedText } from "../i18n/LocalizedText";
import React, { useEffect, useMemo, useState } from 'react';
import {
  Bird,
  Bot,
  Castle,
  Coins,
  Crosshair,
  Hammer,
  HeartPulse,
  Settings,
  Shield,
  ShoppingCart,
  Trash2,
  Users,
  Wheat,
  Wrench,
  Zap,
} from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import {
  Badge,
  Button,
  Input,
  Modal,
  ModalTitle,
  ScheduleSummaryRow,
  Switch,
} from '../components/ui';
import type { AttackLaunchRatesV2, AutomationStateV2 } from '../api/Contracts';
import {
  AUTO_EQUIPMENT_CLEANUP_FEATURE_ID,
  AUTO_EQUIPMENT_CLEANUP_ENABLED_KEY,
  type AutoEquipmentCleanupController,
} from '../settings/AutoEquipmentCleanup';
import { scheduleSummary } from '../settings/SchedulerTypes';
import { CitadelAPI } from '../api/CitadelClient';
import { useCitadelAPI } from '../api/ApiContext';
import { parseAutoBeriWorldSettings } from '../settings/AutoBeriWorldClientState';
import { configurationSection } from '../settings/Configuration';
import { AutomationSafetyPanel } from '../components/AutomationSafetyPanel';
import { AutomationFeatureFeedback } from '../components/AutomationFeatureFeedback';
import { GoalPicker } from '../components/GoalPicker';
import { SetupChecklist } from '../components/SetupChecklist';
import { StopFooter } from '../components/StopControl';
import { checkIntervalLine } from '../settings/disclosure/summaries';
import type { SettingsFeatureId } from '../settings/disclosure/placement';
import { focusReadinessTargetWhenReady } from '../settings/readiness/focusReadinessTarget';
import type { ReadinessCheck } from '../settings/readiness/Readiness';
import { requestSettingsFix } from '../settings/readiness/settingsFixRequest';
import { scopeKey } from '../settings/onboarding/accountScope';
import { goalById } from '../settings/onboarding/goals';
import { useGoal } from '../settings/onboarding/goalStore';
import { AUTOMATION_GOALS } from '../settings/onboarding/goals';
import { useSettingsDisclosure } from '../settings/disclosure/useSettingsDisclosure';
import { AutomationRunStrip } from '../settings/components/AutomationRunStrip';
import { SettingsSection } from '../settings/components/SettingsSection';

interface AutomationViewProps {
  onOpenAutoTCISettings: () => void;
  onOpenAutoSceatResSettings: () => void;
  onOpenAutoFoodBalanceSettings: () => void;
  onOpenRecruitTroopsSettings: () => void;
  onOpenAutoToolSettings: () => void;
  onOpenAutoHospitalSettings: () => void;
  onOpenAutoTowerSettings: () => void;
  onOpenAutoFortressSettings: () => void;
  onOpenAutoInvasionSettings: () => void;
  onOpenAutoNomadSettings: () => void;
  onOpenAutoAdvisorSettings: () => void;
  onOpenAutoBoosterSettings: () => void;
  onOpenAutoBuyerSettings: () => void;
  onOpenAutoKhanSettings: () => void;
  onOpenAutoBeriWorldSettings: () => void;
  onOpenAutoStormSettings: () => void;
  onOpenAutoStationSettings: () => void;
  onOpenAutoBirdSettings: () => void;
  autoEquipmentCleanup: AutoEquipmentCleanupController;
  onOpenFeatureSchedule: (id: string, label: string) => void;
  onOpenAutomationDuration: (featureKey: string, featureLabel: string) => void;
}

interface AutomationFeature {
  id: string;
  enabledKey: string;
  group: AutomationGroupID;
  name: string;
  description: string;
  enabled: boolean;
  detail?: string;
  detailDescriptor?: LocalizedMessage;
  status: string;
  statusLanes?: AutomationStatusLane[];
  icon: React.ComponentType<{ className?: string }>;
  onToggle: () => void;
  onOpenSettings: () => void;
  disabled?: boolean;
}

export interface AutomationStatusLane {
  id: string;
  label: string;
  status: string;
  detail?: string;
  detailDescriptor?: LocalizedMessage;
  toggle?: {
    checked: boolean;
    onChange: (checked: boolean) => void;
    ariaLabel: string;
    disabled?: boolean;
  };
}

type AutomationGroupID = 'production' | 'upkeep' | 'support' | 'offense';

const automationGroups: Array<{
  id: AutomationGroupID;
  name: string;
  icon: React.ComponentType<{ className?: string }>;
}> = [
  { id: 'offense', name: 'Offense', icon: Crosshair },
  { id: 'production', name: 'Production & Building', icon: Wrench },
  { id: 'upkeep', name: 'Resources & Upkeep', icon: Coins },
  { id: 'support', name: 'Recovery & Support', icon: HeartPulse },
];

type DisplayTranslator = (key:MessageKey,params?:MessageParameters)=>string;
function formatNextWake(timestamp:number,now:number,locale:string,t:DisplayTranslator):string {
  return t('automation.nextCheck',nextWakeParameters(timestamp,now,locale));
}
function formatTimedRemaining(expiresAt:number,now:number,locale:string,t:DisplayTranslator):string {
  return t('automation.timeLeft',timedRemainingParameters(expiresAt,now,locale));
}

function combinedAutomationStatus(
  statuses: Array<string | undefined>,
  enabled: boolean,
): string {
  if (!enabled) return 'disabled';
  const availableStatuses = statuses.filter((status): status is string => Boolean(status));
  for (const status of ['error', 'blocked', 'failed']) {
    if (availableStatuses.includes(status)) return status;
  }
  if (availableStatuses.includes('gated')) return 'gated';
  for (const status of ['running', 'ready']) {
    if (availableStatuses.includes(status)) return status;
  }
  if (availableStatuses.length > 0 && availableStatuses.every((status) => status === 'complete')) return 'complete';
  return availableStatuses[0] ?? 'unknown';
}

function automationStatusLane(
  id: string,
  label: string,
  runtime: AutomationStateV2 | undefined,
  enabled: boolean,
  fallbackDetail: string,
  fallbackLane: string,
): AutomationStatusLane {
  const hasRuntimeDetail = typeof runtime?.detail === 'string';
  // Fallback wording only while the game has reported nothing for this lane; a reported status without a
  // detail is shown as is, never paired with an invented reason.
  const useFallback = enabled && !runtime;
  const detail = enabled ? hasRuntimeDetail ? runtime.detail : useFallback ? fallbackDetail : undefined : undefined;
  const detailDescriptor = !enabled || !detail ? undefined
    : hasRuntimeDetail ? automationDetailMessage(detail, runtime.detailDescriptor)
    : describeMessage('automation.waitingLane', {lane: fallbackLane.replaceAll('-', '_')});
  return {
    id,
    label,
    status: enabled ? runtime?.status ?? 'unknown' : 'disabled',
    detail,
    detailDescriptor,
  };
}

function stormMissingDecorationWarningLanes(
  runtime: AutomationStateV2 | undefined,
  enabled: boolean,
): AutomationStatusLane[] {
  const missingDecorations = runtime?.metrics?.stormMissingDecorations;
  if (!enabled || typeof missingDecorations !== 'number' || !Number.isFinite(missingDecorations) || missingDecorations <= 0) return [];

  return [{
    id: 'builder-missing-decorations',
    label: 'Builder warning',
    status: 'warning',
    detail: `${missingDecorations.toLocaleString()} target decoration${missingDecorations === 1 ? '' : 's'} unavailable in storage; skipped while the rest of the target continues.`,
    detailDescriptor: describeMessage('automation.missingDecorations',{count:missingDecorations}),
  }];
}

export function AutomationStatusLines({
  featureId,
  buildLaneActive,
  featureName,
  status,
  detail,
  detailDescriptor,
  lanes,
}: {
  featureId: StatusFeatureId;
  buildLaneActive?: boolean;
  featureName: string;
  status: string;
  detail?: string;
  detailDescriptor?: LocalizedMessage;
  lanes?: AutomationStatusLane[];
}) {
  const player = useAutomationPlayerStatus(featureId, { buildLaneActive });
  const {t,locale,direction} = useStaticLocale();
  const hasLanes = Boolean(lanes?.length);
  const lines: AutomationStatusLane[] = hasLanes
    ? [{ id: 'overall', label: 'Overall', status }, ...(lanes ?? [])]
    : [{ id: 'overall', label: '', status, detail, detailDescriptor }];

  return (
    <div
      className={`automation-function-status-list ${hasLanes ? 'automation-function-status-list-multi' : ''}`}
      aria-label={t('automation.accessibleStatus',{feature:featureName})}
      lang={locale}
      dir={direction}
    >
      {lines.map((line, index) => {
        const value = index === 0 ? player.overall : line.id === 'builder-missing-decorations'
          ? player.lanes.find(lane => lane.id === line.id)?.value ?? player.overall
          : player.lanes[index - 1]?.value ?? player.overall;
        return <AutomationStatusLine key={line.id} line={line} value={value} />;
      })}
    </div>
  );
}

function AutomationStatusLine({line, value}:{line:AutomationStatusLane; value:ReturnType<typeof automationPlayerStatus>}) {
  const label=useLocalizedMessage(automationLaneMessage(line.id),line.label);
  return <div className="automation-function-status-line">
    {line.label ? <span className="automation-function-status-lane" {...messageLanguageAttributes(label)}>{label.text}</span> : null}
    <StatusBadge {...value} />
    {line.toggle ? <Switch checked={line.toggle.checked} onChange={line.toggle.onChange} ariaLabel={line.toggle.ariaLabel} disabled={line.toggle.disabled} className="automation-function-status-toggle" /> : null}
  </div>;
}

function countTime(since: string | undefined, locale: string): string {
  const timestamp = since ? Date.parse(since) : NaN;
  if (!Number.isFinite(timestamp)) return '';
  const today = new Date(timestamp).toDateString() === new Date().toDateString();
  return new Intl.DateTimeFormat(locale, { ...(today ? {} : { dateStyle: 'short' as const }), timeStyle: 'short' }).format(timestamp);
}

function attackRateLabel(view: CountView, t: DisplayTranslator): string {
  return view.kind === 'unknown' ? '—' : t('automation.rate', { state: 'known', count: view.count });
}

function attackRateTitle(feature: string, view: CountView, locale: string, t: DisplayTranslator): string {
  if (view.kind === 'unknown') return t('copy.countUnknownTitle');
  return view.window === 'since'
    ? t('copy.sinceTitle', { feature, count: view.count, time: countTime(view.since, locale) })
    : t('automation.rateTitle', { state: 'known', feature, count: view.count });
}

function dailyAttackCountLabel(view: CountView, locale: string, t: DisplayTranslator): string {
  if (view.kind === 'unknown') return '—';
  return view.window === 'since'
    ? t('copy.since', { count: view.count, time: countTime(view.since, locale) })
    : t('copy.today', { count: view.count });
}

function dailyAttackCountTitle(feature: string, view: CountView, locale: string, t: DisplayTranslator): string {
  if (view.kind === 'unknown') return t('copy.countUnknownTitle');
  return t(view.window === 'since' ? 'copy.sinceTitle' : 'copy.todayTitle',
    { feature, count: view.count, time: countTime(view.since, locale) });
}

export const AutomationView: React.FC<AutomationViewProps> = ({
  onOpenAutoTCISettings,
  onOpenAutoSceatResSettings,
  onOpenAutoFoodBalanceSettings,
  onOpenRecruitTroopsSettings,
  onOpenAutoToolSettings,
  onOpenAutoHospitalSettings,
  onOpenAutoTowerSettings,
  onOpenAutoFortressSettings,
  onOpenAutoInvasionSettings,
  onOpenAutoNomadSettings,
  onOpenAutoAdvisorSettings,
  onOpenAutoBoosterSettings,
  onOpenAutoBuyerSettings,
  onOpenAutoKhanSettings,
  onOpenAutoBeriWorldSettings,
  onOpenAutoStormSettings,
  onOpenAutoStationSettings,
  onOpenAutoBirdSettings,
  autoEquipmentCleanup,
  onOpenFeatureSchedule,
  onOpenAutomationDuration,
}) => {
  const { t: localizeStatic,locale } = useStaticLocale();
  const { configuration, state: gameState } = useCitadelAPI();
  const {
    gameLoggedIn,
    recruitTroopsEnabled,
    autoToolEnabled,
    autoSceatResEnabled,
    autoFoodBalanceEnabled,
    autoHospitalEnabled,
    autoTCIEnabled,
    autoTCINextWakeUp,
    autoTowerEnabled,
    autoFortressEnabled,
    autoInvasionEnabled,
    autoNomadEnabled,
    autoAdvisorEnabled,
	autoBoosterEnabled,
	autoBuyerEnabled,
    autoKhanEnabled,
    autoBeriWorldEnabled,
    autoStormEnabled,
    autoStationEnabled,
    autoBirdEnabled,
    toggleRecruitTroops,
    toggleAutoTool,
    toggleAutoSceatRes,
    toggleAutoFoodBalance,
    toggleAutoHospital,
    toggleAutoTCI,
		toggleAutoTower,
		toggleAutoFortress,
		toggleAutoInvasion,
		toggleAutoNomad,
		toggleAutoAdvisor,
		toggleAutoBooster,
		toggleAutoBuyer,
		toggleAutoKhan,
		toggleAutoBeriWorld,
		toggleAutoStorm,
		toggleAutoStation,
		toggleAutoBird,
		automationStates,
		automationTimedUntilByKey,
  } = useAuth();
  const [now, setNow] = useState(() => Date.now());
  const [isEquipmentCleanupSettingsOpen, setIsEquipmentCleanupSettingsOpen] = useState(false);
  const cleanupDisclosure = useSettingsDisclosure('autoEquipmentCleanup');
  const [attackRates, setAttackRates] = useState<AttackLaunchRatesV2 | null | undefined>(undefined);
  const presence = useHostedRuntimePresence();
  const offline = presence.mode === 'checkpoint';
  const attackLaunchesByFeature = offline ? null : attackRates?.launchesByFeature;

  useEffect(() => {
    const interval = window.setInterval(() => setNow(Date.now()), 30000);
    return () => window.clearInterval(interval);
  }, []);

  useEffect(() => {
    let cancelled = false;
    const refreshAttackRates = async () => {
      try {
        const rates = await CitadelAPI.getAttackLaunchRates();
        if (!cancelled) {
          setAttackRates(rates);
        }
      } catch {
        if (!cancelled) {
          setAttackRates(null);
        }
      }
    };
    void refreshAttackRates();
    const interval = window.setInterval(() => void refreshAttackRates(), 30000);
    return () => {
      cancelled = true;
      window.clearInterval(interval);
    };
  }, []);

  const equipmentCleanupScheduleLabel = autoEquipmentCleanup.schedule?.enabled
    ? scheduleSummary(autoEquipmentCleanup.schedule)
    : 'Runs any time';
  const autoStormRuntime = automationStates.autoStorm;
  const autoStormShopRuntime = automationStates.autoStormShop;
  const autoStormBuildRuntime = automationStates.autoStormBuild;
  const autoBeriTransferRuntime = automationStates.autoBeriWorld;
  const autoBeriAttackRuntime = automationStates.autoBeriWorldAttack;
  const autoBeriToolRuntime = automationStates.autoBeriWorldTools;
  const autoBeriBuildRuntime = automationStates.autoBeriWorldBuild;
  const autoBeriWorldConfiguration = configurationSection(configuration, 'automation.autoBeriWorld');
  const autoBeriBuildEnabled = parseAutoBeriWorldSettings(autoBeriWorldConfiguration).build.enabled;
  const autoKhanAttackRuntime = automationStates.autoKhan;
  const autoKhanCooldownRuntime = automationStates['autoKhan:cooldown'];
  const autoKhanRageRuntime = automationStates['autoKhan:rage'];
  const autoKhanDefenseRuntime = automationStates['autoKhan:defense'];
  const autoSceatRuntime = automationStates.autoSceatRes;
  const autoSceatLogisticsRuntime = automationStates.autoSceatResLogistics;
  const autoBeriWorldStatus = combinedAutomationStatus(
    [
      autoBeriTransferRuntime?.status,
      autoBeriAttackRuntime?.status,
      autoBeriToolRuntime?.status,
      autoBeriBuildEnabled ? autoBeriBuildRuntime?.status : undefined,
    ],
    autoBeriWorldEnabled,
  );
  const autoStormStatus = combinedAutomationStatus(
    [autoStormRuntime?.status, autoStormShopRuntime?.status, autoStormBuildRuntime?.status],
    autoStormEnabled,
  );
  const autoKhanStatus = combinedAutomationStatus(
    [
      autoKhanAttackRuntime?.status,
      autoKhanCooldownRuntime?.status,
      autoKhanRageRuntime?.status,
      autoKhanDefenseRuntime?.status,
    ],
    autoKhanEnabled,
  );
  const autoSceatStatus = combinedAutomationStatus(
    [autoSceatRuntime?.status, autoSceatLogisticsRuntime?.status],
    autoSceatResEnabled,
  );

  const features = useMemo<AutomationFeature[]>(() => [
    {
      id: 'autoRecruit',
      enabledKey: 'recruit_troops',
      group: 'production',
      name: 'Auto Recruit',
      description: 'Keeps troop recruitment queues stocked from the configured plans.',
      enabled: recruitTroopsEnabled,
      detail: recruitTroopsEnabled ? automationStates.autoRecruit?.detail : undefined,
      status: automationStates.autoRecruit?.status ?? (recruitTroopsEnabled ? 'unknown' : 'disabled'),
      icon: Users,
      onToggle: toggleRecruitTroops,
      onOpenSettings: onOpenRecruitTroopsSettings,
    },
    {
      id: 'autoTool',
      enabledKey: 'auto_tool',
      group: 'production',
      name: 'Auto Tool',
      description: 'Maintains tool production queues across configured castles.',
      enabled: autoToolEnabled,
      detail: autoToolEnabled ? automationStates.autoTool?.detail : undefined,
      status: automationStates.autoTool?.status ?? (autoToolEnabled ? 'unknown' : 'disabled'),
      icon: Wrench,
      onToggle: toggleAutoTool,
      onOpenSettings: onOpenAutoToolSettings,
    },
    {
      id: 'autoHospital',
      enabledKey: 'auto_hospital',
      group: 'support',
      name: 'Auto Hospital',
      description: 'Processes hospital queues using the configured healing priorities.',
      enabled: autoHospitalEnabled,
      detail: autoHospitalEnabled ? automationStates.autoHospital?.detail : undefined,
      status: automationStates.autoHospital?.status ?? (autoHospitalEnabled ? 'unknown' : 'disabled'),
      icon: HeartPulse,
      onToggle: toggleAutoHospital,
      onOpenSettings: onOpenAutoHospitalSettings,
    },
    {
      id: 'autoStation',
      enabledKey: 'auto_station',
      group: 'support',
      name: 'Auto Station',
      description: 'Moves troops out of a castle before an incoming attack lands, leaves behind only the troops you chose to defend, and recalls the rest when it is clear.',
      enabled: autoStationEnabled,
      detail: autoStationEnabled ? automationStates.autoStation?.detail : undefined,
      status: automationStates.autoStation?.status ?? (autoStationEnabled ? 'unknown' : 'disabled'),
      icon: Shield,
      onToggle: toggleAutoStation,
      onOpenSettings: onOpenAutoStationSettings,
    },
    {
      id: 'autoBird',
      enabledKey: 'auto_bird',
      group: 'support',
      name: 'Auto Bird',
      description: "Sends each castle's troops to the nearest alliance member's castle in repeating Bird cycles, using only members with more protection days than you set, and keeping only the troops you set aside at home.",
      enabled: autoBirdEnabled,
      detail: autoBirdEnabled ? automationStates.autoBird?.detail : undefined,
      status: automationStates.autoBird?.status ?? (autoBirdEnabled ? 'unknown' : 'disabled'),
      icon: Bird,
      onToggle: toggleAutoBird,
      onOpenSettings: onOpenAutoBirdSettings,
    },
    {
      id: 'autoTCI',
      enabledKey: 'auto_tci',
      group: 'upkeep',
      name: 'Auto TCI',
      description: 'Equips and renews temporary construction items automatically.',
      enabled: autoTCIEnabled,
      detail: autoTCIEnabled
			? automationStates.autoTCI?.detail ?? formatNextWake(autoTCINextWakeUp, now,locale,localizeStatic)
			: 'Construction-item automation is paused',
      detailDescriptor: autoTCIEnabled && automationStates.autoTCI?.detail===undefined ? describeMessage('automation.nextCheck',nextWakeParameters(autoTCINextWakeUp,now,locale)) : undefined,
      status: automationStates.autoTCI?.status ?? (autoTCIEnabled ? 'unknown' : 'disabled'),
      icon: Hammer,
      onToggle: toggleAutoTCI,
      onOpenSettings: onOpenAutoTCISettings,
    },
    {
      id: 'autoSceatRes',
      enabledKey: 'auto_sceat_resources',
      group: 'production',
      name: 'Auto Sceat Resources',
      description: 'Balances kingdom resources and maintains Refinery, Toolsmith, Dragon Hoard, and Dragon Forge queues.',
      enabled: autoSceatResEnabled,
      detail: autoSceatResEnabled ? autoSceatRuntime?.detail : undefined,
      status: autoSceatStatus,
      statusLanes: [
        automationStatusLane('crafting', 'Crafting', autoSceatRuntime, autoSceatResEnabled, 'Waiting for crafting policy status', 'crafting'),
        automationStatusLane('logistics', 'Logistics', autoSceatLogisticsRuntime, autoSceatResEnabled, 'Waiting for logistics policy status', 'logistics'),
      ],
      icon: Coins,
      onToggle: toggleAutoSceatRes,
      onOpenSettings: onOpenAutoSceatResSettings,
    },
    {
      id: 'autoFoodBalance',
      enabledKey: 'auto_food_balance',
      group: 'upkeep',
      name: 'Auto Food Balance',
      description: 'Protects Food, Honey, Mead, and Beef reserves across owned castles.',
      enabled: autoFoodBalanceEnabled,
      detail: autoFoodBalanceEnabled ? automationStates.autoFoodBalance?.detail : undefined,
      status: automationStates.autoFoodBalance?.status ?? (autoFoodBalanceEnabled ? 'unknown' : 'disabled'),
      icon: Wheat,
      onToggle: toggleAutoFoodBalance,
      onOpenSettings: onOpenAutoFoodBalanceSettings,
    },
    {
      id: 'autoBooster',
      enabledKey: 'auto_booster',
      group: 'upkeep',
      name: 'Auto Booster',
      description: 'Buys only the 2,500-ruby daily global fortress-speed boost after a fresh exact-price and reserve check.',
      enabled: autoBoosterEnabled,
      detail: autoBoosterEnabled ? automationStates.autoBooster?.detail : undefined,
      status: automationStates.autoBooster?.status ?? (autoBoosterEnabled ? 'unknown' : 'disabled'),
      icon: Zap,
      onToggle: toggleAutoBooster,
      onOpenSettings: onOpenAutoBoosterSettings,
    },
    {
      id: 'autoBuyer',
      enabledKey: 'auto_buyer',
      group: 'upkeep',
      name: 'Auto Buyer',
      description: 'Buys selected reset stock and maintains specialist and feast duration floors within explicit reserves.',
      enabled: autoBuyerEnabled,
      detail: autoBuyerEnabled ? automationStates.autoBuyer?.detail : undefined,
      status: automationStates.autoBuyer?.status ?? (autoBuyerEnabled ? 'unknown' : 'disabled'),
      icon: ShoppingCart,
      onToggle: toggleAutoBuyer,
      onOpenSettings: onOpenAutoBuyerSettings,
    },
	{
		id: 'autoTowers',
		enabledKey: 'auto_towers',
		group: 'offense',
		name: 'Auto Towers',
		description: 'Attacks ready robber-baron towers with regular waves or Baron Advisor chains bounded by a daily Time Skip budget.',
		enabled: autoTowerEnabled,
		detail: autoTowerEnabled ? automationStates.autoTowers?.detail : undefined,
		status: automationStates.autoTowers?.status ?? (autoTowerEnabled ? 'unknown' : 'disabled'),
		icon: Crosshair,
		onToggle: toggleAutoTower,
		onOpenSettings: onOpenAutoTowerSettings,
	},
	{
		id: 'autoFortress',
		enabledKey: 'auto_fortress',
		group: 'offense',
		name: 'Auto Fortress',
		description: 'Wins outer-kingdom fortresses with a speed-first Direwolf wave, guarded supply, and exact cooldown tracking.',
		enabled: autoFortressEnabled,
		detail: autoFortressEnabled ? automationStates.autoFortress?.detail : undefined,
		status: automationStates.autoFortress?.status ?? (autoFortressEnabled ? 'unknown' : 'disabled'),
		icon: Castle,
		onToggle: toggleAutoFortress,
		onOpenSettings: onOpenAutoFortressSettings,
	},
	{
      id: AUTO_EQUIPMENT_CLEANUP_FEATURE_ID,
      enabledKey: AUTO_EQUIPMENT_CLEANUP_ENABLED_KEY,
      group: 'upkeep',
      name: 'Auto Equipment Cleanup',
      description: 'Sells eligible old non-relic equipment and gems from storage automatically.',
      enabled: autoEquipmentCleanup.enabled,
      detail: `${equipmentCleanupScheduleLabel} · polls every ${autoEquipmentCleanup.intervalMinutes} min`,
      status: automationStates[AUTO_EQUIPMENT_CLEANUP_FEATURE_ID]?.status ?? (autoEquipmentCleanup.enabled ? 'waiting' : 'disabled'),
      icon: Trash2,
      onToggle: () => autoEquipmentCleanup.setEnabled(!autoEquipmentCleanup.enabled),
      onOpenSettings: () => setIsEquipmentCleanupSettingsOpen(true),
      // Turning it on needs the game connection; turning it off (Stop) never does.
      disabled: !gameLoggedIn && !autoEquipmentCleanup.enabled,
    },
    {
      id: 'autoInvasion',
      enabledKey: 'auto_invasion',
      group: 'offense',
      name: 'Auto Invasion',
      description: 'Uses a CitadelOps attack preset against Foreign Lords and Bloodcrow castles until the score target is reached.',
      enabled: autoInvasionEnabled,
      detail: autoInvasionEnabled ? automationStates.autoInvasion?.detail : undefined,
      status: automationStates.autoInvasion?.status ?? (autoInvasionEnabled ? 'unknown' : 'disabled'),
      icon: Crosshair,
      onToggle: toggleAutoInvasion,
      onOpenSettings: onOpenAutoInvasionSettings,
    },
	{
		id: 'autoNomad',
		enabledKey: 'auto_nomad',
		group: 'offense',
		name: 'Auto Nomad / Samurai',
		description: 'Maxes four regular camps, locks the weakest, and chains available commanders into that one camp.',
		enabled: autoNomadEnabled,
		detail: autoNomadEnabled ? automationStates.autoNomad?.detail : undefined,
		status: automationStates.autoNomad?.status ?? (autoNomadEnabled ? 'unknown' : 'disabled'),
		icon: Crosshair,
		onToggle: toggleAutoNomad,
		onOpenSettings: onOpenAutoNomadSettings,
	},
	{
		id: 'autoAdvisor',
		enabledKey: 'auto_advisor',
		group: 'offense',
		name: 'Auto Advisor',
		description: 'Starts one server-managed Nomad or Samurai advisor chain, sized to event time and current resources.',
		enabled: autoAdvisorEnabled,
		detail: autoAdvisorEnabled ? automationStates.autoAdvisor?.detail : undefined,
		status: automationStates.autoAdvisor?.status ?? (autoAdvisorEnabled ? 'unknown' : 'disabled'),
		icon: Bot,
		onToggle: toggleAutoAdvisor,
		onOpenSettings: onOpenAutoAdvisorSettings,
	},
	{
		id: 'autoKhan',
		enabledKey: 'auto_khan',
		group: 'offense',
		name: 'Auto Khan',
		description: 'Chains Khan camp hits and retaliations while keeping the Great Empire main castle on its defense preset.',
		enabled: autoKhanEnabled,
		detail: autoKhanEnabled ? autoKhanAttackRuntime?.detail : undefined,
		status: autoKhanStatus,
		statusLanes: [
			automationStatusLane('attacks', 'Attacks', autoKhanAttackRuntime, autoKhanEnabled, 'Waiting for the Khan attack policy', 'khan-attacks'),
			automationStatusLane('cooldowns', 'Cooldowns', autoKhanCooldownRuntime, autoKhanEnabled, 'Waiting for the Khan cooldown policy', 'khan-cooldowns'),
			automationStatusLane('rage', 'Rage', autoKhanRageRuntime, autoKhanEnabled, 'Waiting for the Khan rage policy', 'khan-rage'),
			automationStatusLane('defense', 'Defense', autoKhanDefenseRuntime, autoKhanEnabled, 'Waiting for the Khan defense policy', 'khan-defense'),
		],
		icon: Crosshair,
		onToggle: toggleAutoKhan,
		onOpenSettings: onOpenAutoKhanSettings,
	},
	{
		id: 'autoBeriWorld',
		enabledKey: 'auto_beri_world',
		group: 'offense',
		name: 'Auto Beri World',
		description: 'Transfers troops, attacks each next tower, brings the loot home, and spends confirmed Berimond resources on a captured camp build and upgrade target.',
		enabled: autoBeriWorldEnabled,
		detail: autoBeriWorldEnabled ? autoBeriTransferRuntime?.detail : undefined,
		status: autoBeriWorldStatus,
		statusLanes: [
			automationStatusLane('transfers', 'Transfers', autoBeriTransferRuntime, autoBeriWorldEnabled, 'Waiting for the Berimond transfer policy', 'beri-transfers'),
			automationStatusLane('attacks', 'Attacks', autoBeriAttackRuntime, autoBeriWorldEnabled, 'Waiting for the Berimond attack policy', 'beri-attacks'),
			automationStatusLane('tools', 'Tools', autoBeriToolRuntime, autoBeriWorldEnabled, 'Waiting for the Berimond tool policy', 'beri-tools'),
			automationStatusLane(
				'builder',
				'Builder',
				autoBeriBuildRuntime,
				autoBeriWorldEnabled && autoBeriBuildEnabled,
				'Waiting for the Berimond builder policy',
                'beri-builder',
			),
		],
		icon: Crosshair,
		onToggle: toggleAutoBeriWorld,
		onOpenSettings: onOpenAutoBeriWorldSettings,
	},
    {
      id: 'autoStorm',
      enabledKey: 'auto_storm',
      group: 'offense',
      name: 'Auto Storm',
      description: 'Builds a captured Storm castle target, attacks selected forts and islands, and spends Aquamarine by priority.',
      enabled: autoStormEnabled,
      detail: autoStormEnabled ? autoStormRuntime?.detail : undefined,
      status: autoStormStatus,
      statusLanes: [
        automationStatusLane('combat', 'Combat', autoStormRuntime, autoStormEnabled, 'Waiting for the Storm combat policy', 'storm-combat'),
        automationStatusLane('aquamarine-shop', 'Aquamarine shop', autoStormShopRuntime, autoStormEnabled, 'Waiting for the Aquamarine shop policy', 'storm-shop'),
        automationStatusLane('builder', 'Builder', autoStormBuildRuntime, autoStormEnabled, 'Waiting for the Storm builder policy', 'storm-builder'),
        ...stormMissingDecorationWarningLanes(autoStormBuildRuntime, autoStormEnabled),
      ],
      icon: Crosshair,
      onToggle: toggleAutoStorm,
      onOpenSettings: onOpenAutoStormSettings,
    },
  ], [
    locale,
    localizeStatic,
    autoHospitalEnabled,
    autoEquipmentCleanup,
    recruitTroopsEnabled,
    autoSceatResEnabled,
    autoSceatRuntime,
    autoSceatLogisticsRuntime,
    autoFoodBalanceEnabled,
    autoTCIEnabled,
    autoTCINextWakeUp,
    autoTowerEnabled,
    autoFortressEnabled,
    autoInvasionEnabled,
		autoNomadEnabled,
		autoAdvisorEnabled,
		autoBoosterEnabled,
		autoBuyerEnabled,
    autoKhanEnabled,
    autoKhanAttackRuntime,
    autoKhanCooldownRuntime,
    autoKhanRageRuntime,
    autoKhanDefenseRuntime,
    autoBeriWorldEnabled,
    autoBeriBuildEnabled,
    autoBeriTransferRuntime,
    autoBeriAttackRuntime,
    autoBeriToolRuntime,
    autoBeriBuildRuntime,
    autoBeriWorldStatus,
    autoStationEnabled,
    autoBirdEnabled,
    autoStormEnabled,
    autoStormRuntime,
    autoStormShopRuntime,
    autoStormBuildRuntime,
    autoStormStatus,
    autoKhanStatus,
    autoSceatStatus,
    autoToolEnabled,
		automationStates,
    equipmentCleanupScheduleLabel,
    gameLoggedIn,
    now,
    onOpenAutoHospitalSettings,
    onOpenAutoSceatResSettings,
    onOpenAutoFoodBalanceSettings,
    onOpenAutoTCISettings,
    onOpenAutoToolSettings,
    onOpenAutoTowerSettings,
    onOpenAutoFortressSettings,
    onOpenAutoInvasionSettings,
		onOpenAutoNomadSettings,
		onOpenAutoAdvisorSettings,
		onOpenAutoBoosterSettings,
		onOpenAutoBuyerSettings,
    onOpenAutoKhanSettings,
    onOpenAutoBeriWorldSettings,
    onOpenAutoStormSettings,
    onOpenAutoStationSettings,
    onOpenAutoBirdSettings,
    onOpenRecruitTroopsSettings,
    toggleAutoHospital,
    toggleAutoSceatRes,
    toggleAutoFoodBalance,
    toggleAutoTCI,
    toggleAutoTower,
    toggleAutoFortress,
    toggleAutoInvasion,
		toggleAutoNomad,
		toggleAutoAdvisor,
		toggleAutoBooster,
		toggleAutoBuyer,
    toggleAutoKhan,
    toggleAutoBeriWorld,
    toggleAutoStorm,
    toggleAutoStation,
    toggleAutoBird,
    toggleAutoTool,
    toggleRecruitTroops,
  ]);
  const groupedFeatures = automationGroups
    .map((group) => ({ ...group, features: features.filter((feature) => feature.group === group.id) }))
    .filter((group) => group.features.length > 0);


  // Goal-led entry (CIT-19): optional, never a gate. The panel exists only after the player chose a goal.
  const goalApi = useGoal(scopeKey(gameState));
  const [goalPickerOpen, setGoalPickerOpen] = useState(false);
  const activeGoal = goalById(goalApi.goal?.goalId);
  const activeGoalFeature = activeGoal ? features.find((feature) => feature.id === activeGoal.featureId) : undefined;
  const anyAutomationOn = features.some((feature) => feature.enabled);
  const openGoalEditor = (check?: ReadinessCheck) => {
    if (!activeGoalFeature) return;
    if (check) requestSettingsFix(activeGoalFeature.id as SettingsFeatureId, check, activeGoalFeature.onOpenSettings);
    else activeGoalFeature.onOpenSettings();
  };
  const goToGoalSwitch = () => {
    if (!activeGoal) return;
    const id = `automation-switch-${activeGoal.featureId}`;
    document.getElementById(id)?.scrollIntoView({ block: 'center', behavior: 'smooth' });
    focusReadinessTargetWhenReady(id);
  };
  const goalButton = (
    <Button variant="secondary" size="sm" id="goal-entry" onClick={() => setGoalPickerOpen(true)} data-goal-entry>
      <LocalizedText messageKey="goalEntry.button" />
    </Button>
  );
  const goalEntry = (
    <>
      {activeGoal && activeGoalFeature ? (
        <SetupChecklist
          goal={activeGoal}
          collapsed={goalApi.goal?.collapsed === true}
          onSetCollapsed={goalApi.setCollapsed}
          onOpenEditor={openGoalEditor}
          onGoToSwitch={goToGoalSwitch}
          // The opener that started the goal is gone once the checklist replaces it, so focus is placed on purpose (CIT-19 QA).
          onDone={() => { goalApi.clear(); focusReadinessTargetWhenReady('goal-entry'); }}
          onChooseAnother={() => { goalApi.clear(); setGoalPickerOpen(true); }}
        />
      ) : (
        <div className="flex flex-wrap items-center justify-between gap-2" data-goal-entry-row>
          {!anyAutomationOn ? (
            <div className="min-w-0 text-caption" data-goal-empty>
              <div className="font-bold text-text-main"><LocalizedText messageKey="goalEntry.emptyTitle" /></div>
              <div className="text-text-muted"><LocalizedText messageKey="goalEntry.emptyBody" /></div>
            </div>
          ) : <span />}
          {goalButton}
        </div>
      )}
      {goalPickerOpen ? (
        <GoalPicker
          goals={AUTOMATION_GOALS}
          onChoose={(goalId) => { goalApi.choose(goalId); setGoalPickerOpen(false); focusReadinessTargetWhenReady('setup-checklist'); }}
          onClose={() => setGoalPickerOpen(false)}
        />
      ) : null}
    </>
  );

  return (
    <div className="mx-auto flex w-full max-w-[1800px] flex-col gap-4 pb-10">
      <AutomationSafetyPanel states={automationStates} now={now} />
      {goalEntry}
      <div className="automation-function-groups">
        {groupedFeatures.map((group) => {
          return (
            <section key={group.id} className="automation-function-group">
              <SectionHeader title={group.name} />
              <Banner tone="info">
                  <span><LocalizedRichText messageKey="ui.rich.views.automationView.right.click.a.toggle.for.temporary.activation.c34579ee" params={{}} tags={{strong0: children => <strong className="text-text-main">{children}</strong>}} /></span>
              </Banner>
              <div className="automation-function-grid">
                {group.features.map((feature) => {
                  const FeatureIcon = feature.icon;
                  const timedUntil = automationTimedUntilByKey[feature.enabledKey];
                  const attackLaunchCount = rateView(attackRates, feature.id, offline);
                  const dailyAttackLaunchCount = dailyView(attackRates, feature.id, offline);
                  return (
                    <Card
                      key={feature.id}
                      className={`automation-function-row ${feature.enabled ? 'automation-function-row-active' : ''}`}
                    >
                      <span
                        id={`automation-switch-${feature.id}`}
                        className="shrink-0"
                        onContextMenu={(event) => {
                          event.preventDefault();
                          onOpenAutomationDuration(feature.enabledKey, feature.name);
                        }}
                        title={`Right-click to run ${feature.name} for a duration`}
                      >
                        <Switch
                          checked={feature.enabled}
                          onChange={feature.onToggle}
                          ariaLabel={`Toggle ${feature.name}`}
                          disabled={feature.disabled}
                        />
                      </span>
                      <div className="automation-function-copy">
                        <div className="flex min-w-0 items-center gap-2">
                          <FeatureIcon className="h-3.5 w-3.5 shrink-0 text-text-muted" />
                          <h3>{feature.name}</h3>
                          {feature.group === 'offense' ? (
                            <>
                              <Badge
                                variant="outline"
                                className="shrink-0 whitespace-nowrap"
                                title={attackRateTitle(feature.name, attackLaunchCount,locale,localizeStatic)}
                                aria-label={attackRateTitle(feature.name, attackLaunchCount,locale,localizeStatic)}
                              >
                                {attackRateLabel(attackLaunchCount,localizeStatic)}
                              </Badge>
                              <Badge
                                variant="outline"
                                className="shrink-0 whitespace-nowrap"
                                title={dailyAttackCountTitle(feature.name, dailyAttackLaunchCount,locale,localizeStatic)}
                                aria-label={dailyAttackCountTitle(feature.name, dailyAttackLaunchCount,locale,localizeStatic)}
                              >
                                {dailyAttackCountLabel(dailyAttackLaunchCount,locale,localizeStatic)}
                              </Badge>
                            </>
                          ) : null}
                          {timedUntil ? <Badge variant="outline">{formatTimedRemaining(timedUntil, now,locale,localizeStatic)}</Badge> : null}
                        </div>
                        <p>{feature.description}</p>
                        <AutomationStatusLines
                          featureId={feature.id as StatusFeatureId}
                          buildLaneActive={feature.id === 'autoBeriWorld' ? autoBeriBuildEnabled : undefined}
                          featureName={feature.name}
                          status={feature.status}
                          detail={feature.detail}
                          detailDescriptor={feature.detailDescriptor ?? automationDetailMessage(feature.detail,automationStates[feature.id]?.detailDescriptor)}
                          lanes={feature.statusLanes}
                        />
                        <AutomationFeatureFeedback
                          featureId={feature.id as SettingsFeatureId}
                          enabled={feature.enabled}
                          onOpenSettings={feature.onOpenSettings}
                          launchesByFeature={attackLaunchesByFeature}
                          buildLaneActive={feature.id === 'autoBeriWorld' ? autoBeriBuildEnabled : undefined}
                        />
                      </div>
                      <Button iconOnly
                        variant="ghost"
                        size="md"
                        className="automation-function-settings"
                        onClick={feature.onOpenSettings}
                        aria-label={`Open ${feature.name} settings`}
                        title={`Open ${feature.name} settings`}
                      >
                        <Settings className="h-3.5 w-3.5" />
                      </Button>
                    </Card>
                  );
                })}
              </div>
            </section>
          );
        })}
      </div>

      <Modal
        isOpen={isEquipmentCleanupSettingsOpen}
        onClose={() => setIsEquipmentCleanupSettingsOpen(false)}
        maxWidth="md"
        title={
          <ModalTitle icon={<Trash2 className="h-5 w-5" />}><LocalizedText messageKey="ui.views.automationView.auto.equipment.cleanup.4116a164" /></ModalTitle>
        }
        footer={<><StopFooter featureId="autoEquipmentCleanup" /><Button variant="ghost" onClick={() => setIsEquipmentCleanupSettingsOpen(false)}><LocalizedText messageKey="common.close" /></Button></>}
      >
        <AutomationRunStrip
          featureId="autoEquipmentCleanup"
          saveMode="immediate"
          onOpenDuration={() => {
            setIsEquipmentCleanupSettingsOpen(false);
            onOpenAutomationDuration(AUTO_EQUIPMENT_CLEANUP_ENABLED_KEY, 'Auto Equipment Cleanup');
          }}
        />
        <div className="flex flex-col gap-4">
          <SettingsSection disclosure={cleanupDisclosure} section="schedule" className="flex flex-col gap-4">
          <ScheduleSummaryRow
            summary={equipmentCleanupScheduleLabel}
            actionLabel="Edit schedule"
            className="bg-bg-card/45 p-4"
            onEdit={() => {
                setIsEquipmentCleanupSettingsOpen(false);
                onOpenFeatureSchedule(AUTO_EQUIPMENT_CLEANUP_FEATURE_ID, 'Auto Equipment Cleanup');
            }}
          />

          <p className="text-caption text-text-muted">
            <LocalizedText messageKey="ui.views.automationView.the.schedule.decides.when.cleanup.may.run.ec3b83b8" /></p>
          </SettingsSection>
          <SettingsSection
            disclosure={cleanupDisclosure}
            section="timing"
            summary={[checkIntervalLine(autoEquipmentCleanup.intervalMinutes * 60)]}
            customCount={autoEquipmentCleanup.intervalMinutes !== 1 ? 1 : 0}
          >
          <div className="flex flex-wrap items-center justify-between gap-4 rounded-global border border-primary/20 bg-primary/5 p-4">
            <div className="min-w-0">
              <div className="text-body font-semibold text-text-main"><LocalizedText messageKey="ui.views.automationView.poll.interval.47ea8f5d" /></div>
              <p className="mt-1 text-caption text-text-muted"><LocalizedText messageKey="ui.views.automationView.checks.equipment.storage.at.this.interval.while.1ed68fd4" /></p>
            </div>
            <div className="flex items-center gap-2">
              <div className="w-20">
                <Input
                  type="number"
                  min={1}
                  max={1440}
                  value={autoEquipmentCleanup.intervalMinutes}
                  onChange={(event) => autoEquipmentCleanup.setIntervalMinutes(Number(event.target.value))}
                  className="h-9 px-2 py-1 text-center"
                  aria-label={localizeStatic("ui.views.automationView.aria-label.equipment.cleanup.poll.interval.in.minutes.a1c6845c")}
                />
              </div>
              <span className="text-caption font-semibold text-text-muted"><LocalizedText messageKey="ui.views.automationView.min.1f6fa6f6" /></span>
            </div>
          </div>

          </SettingsSection>
        </div>
      </Modal>
    </div>
  );
};

export default AutomationView;
