import React, { useEffect, useMemo, useState } from 'react';
import {
  Bot,
  Coins,
  Crosshair,
  Hammer,
  HeartPulse,
  MousePointerClick,
  Settings,
  ShoppingCart,
  Trash2,
  Users,
  Wheat,
  Wrench,
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
  type StatusTone,
} from '../components/ui';
import type { AttackLaunchDailySessionV2, AutomationStateV2 } from '../api/Contracts';
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

interface AutomationViewProps {
  onOpenAutoTCISettings: () => void;
  onOpenAutoSceatResSettings: () => void;
  onOpenAutoFoodBalanceSettings: () => void;
  onOpenRecruitTroopsSettings: () => void;
  onOpenAutoToolSettings: () => void;
  onOpenAutoHospitalSettings: () => void;
  onOpenAutoTowerSettings: () => void;
  onOpenAutoInvasionSettings: () => void;
  onOpenAutoNomadSettings: () => void;
  onOpenAutoAdvisorSettings: () => void;
  onOpenAutoBuyerSettings: () => void;
  onOpenAutoKhanSettings: () => void;
  onOpenAutoBeriWorldSettings: () => void;
  onOpenAutoStormSettings: () => void;
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
  status: string;
  statusLanes?: AutomationStatusLane[];
  icon: React.ComponentType<{ className?: string }>;
  onToggle: () => void;
  onOpenSettings: () => void;
  disabled?: boolean;
}

interface AutomationStatusLane {
  id: string;
  label: string;
  status: string;
  detail?: string;
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

function formatNextWake(timestamp: number, now: number): string {
  if (timestamp <= 0) return 'Waiting for the next scheduled check';
  const msLeft = timestamp - now;
  if (msLeft <= 0) return 'Next check is due now';
  const totalMinutes = Math.ceil(msLeft / 60000);
  const days = Math.floor(totalMinutes / 1440);
  const hours = Math.floor((totalMinutes % 1440) / 60);
  const minutes = totalMinutes % 60;
  if (days > 0) return `Next check in ${days}d${hours > 0 ? ` ${hours}h` : ''}`;
  if (hours > 0) return `Next check in ${hours}h${minutes > 0 ? ` ${minutes}m` : ''}`;
  return `Next check in ${Math.max(1, minutes)}m`;
}

function formatTimedRemaining(expiresAt: number, now: number): string {
  const totalMinutes = Math.max(1, Math.ceil((expiresAt - now) / 60_000));
  const days = Math.floor(totalMinutes / 1440);
  const hours = Math.floor((totalMinutes % 1440) / 60);
  const minutes = totalMinutes % 60;
  if (days > 0) return `${days}d${hours > 0 ? ` ${hours}h` : ''} left`;
  if (hours > 0) return `${hours}h${minutes > 0 ? ` ${minutes}m` : ''} left`;
  return `${minutes}m left`;
}

function modeLabel(mode: 'global' | 'perCastle'): string {
  return mode === 'perCastle' ? 'Per-castle plan' : 'Global plan';
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
  return availableStatuses[0] ?? 'waiting';
}

function automationStatusLane(
  id: string,
  label: string,
  runtime: AutomationStateV2 | undefined,
  enabled: boolean,
  fallbackDetail: string,
): AutomationStatusLane {
  return {
    id,
    label,
    status: enabled ? runtime?.status ?? 'waiting' : 'disabled',
    detail: enabled ? runtime?.detail ?? fallbackDetail : undefined,
  };
}

function automationStatusTone(status: string): StatusTone {
  switch (status.toLowerCase()) {
    case 'complete':
    case 'completed':
    case 'success':
      return 'success';
    case 'failed':
    case 'error':
      return 'danger';
    case 'blocked':
    case 'gated':
    case 'retrying':
      return 'warning';
    case 'running':
      return 'info';
    case 'enabled':
    case 'scheduled':
      return 'brand';
    default:
      return 'neutral';
  }
}

function AutomationStatusLines({
  featureName,
  status,
  detail,
  lanes,
}: {
  featureName: string;
  status: string;
  detail?: string;
  lanes?: AutomationStatusLane[];
}) {
  const hasLanes = Boolean(lanes?.length);
  const lines: AutomationStatusLane[] = hasLanes
    ? [{ id: 'overall', label: 'Overall', status }, ...(lanes ?? [])]
    : [{ id: 'overall', label: '', status, detail }];

  return (
    <div
      className={`automation-function-status-list ${hasLanes ? 'automation-function-status-list-multi' : ''}`}
      aria-label={`${featureName} status`}
    >
      {lines.map((line) => (
        <div
          key={line.id}
          className={`ui-status ui-status-${automationStatusTone(line.status)} automation-function-status-line ${line.label ? 'automation-function-status-line-lane' : ''} ${line.toggle ? 'automation-function-status-line-toggle' : ''}`}
        >
          <span className="ui-status-symbol" aria-hidden="true" />
          {line.label ? <span className="automation-function-status-lane">{line.label}</span> : null}
          <span className="ui-status-label">{automationStatusLabel(line.status)}</span>
          {line.detail || line.toggle ? <span className="ui-status-detail">{line.detail}</span> : null}
          {line.toggle ? (
            <Switch
              checked={line.toggle.checked}
              onChange={line.toggle.onChange}
              size="sm"
              tone="feature"
              ariaLabel={line.toggle.ariaLabel}
              disabled={line.toggle.disabled}
              className="automation-function-status-toggle"
            />
          ) : null}
        </div>
      ))}
    </div>
  );
}

function automationStatusLabel(status: string): string {
  if (!status) return 'Unknown';
  return status.charAt(0).toUpperCase() + status.slice(1).replaceAll('_', ' ');
}

function attackRateLabel(count: number | null | undefined): string {
  if (count === undefined) return 'Loading rate';
  if (count === null) return 'Rate unavailable';
  return `${count.toLocaleString()} ${count === 1 ? 'attack' : 'attacks'} / hr`;
}

function attackRateCount(
  launchesByFeature: Record<string, number> | null | undefined,
  featureID: string,
): number | null | undefined {
  if (launchesByFeature === undefined) return undefined;
  if (launchesByFeature === null) return null;
  return launchesByFeature[featureID] ?? 0;
}

function attackRateTitle(featureName: string, count: number | null | undefined): string {
  if (count === undefined) return `Loading the current ${featureName} attack rate.`;
  if (count === null) return `The current ${featureName} attack rate is unavailable.`;
  return `${count.toLocaleString()} ${count === 1 ? 'attack was' : 'attacks were'} launched by ${featureName} in the past 60 minutes.`;
}

function dailyAttackSessionCount(
  session: AttackLaunchDailySessionV2 | null | undefined,
  featureID: string,
): number | null | undefined {
  if (session === undefined) return undefined;
  if (session === null) return null;
  return session.launchesByFeature[featureID] ?? 0;
}

function dailyAttackCountLabel(count: number | null | undefined): string {
  if (count === undefined) return 'Loading daily';
  if (count === null) return 'Daily unavailable';
  return `${count.toLocaleString()} since reset`;
}

function dailyAttackCountTitle(
  featureName: string,
  count: number | null | undefined,
  sessionStartedAt?: string,
): string {
  if (count === undefined) return `Loading the current ${featureName} daily attack session count.`;
  if (count === null) return `The ${featureName} daily attack session count is unavailable until a server reset boundary is observed.`;
  const startedAt = sessionStartedAt ? new Date(sessionStartedAt) : null;
  const boundary = startedAt && !Number.isNaN(startedAt.getTime())
    ? ` since the server daily attack-count session began at ${startedAt.toLocaleString()}`
    : ' in the current server daily attack-count session';
  return `${count.toLocaleString()} confirmed ${count === 1 ? 'attack was' : 'attacks were'} launched by ${featureName}${boundary}.`;
}

export const AutomationView: React.FC<AutomationViewProps> = ({
  onOpenAutoTCISettings,
  onOpenAutoSceatResSettings,
  onOpenAutoFoodBalanceSettings,
  onOpenRecruitTroopsSettings,
  onOpenAutoToolSettings,
  onOpenAutoHospitalSettings,
  onOpenAutoTowerSettings,
  onOpenAutoInvasionSettings,
  onOpenAutoNomadSettings,
  onOpenAutoAdvisorSettings,
  onOpenAutoBuyerSettings,
  onOpenAutoKhanSettings,
  onOpenAutoBeriWorldSettings,
  onOpenAutoStormSettings,
  autoEquipmentCleanup,
  onOpenFeatureSchedule,
  onOpenAutomationDuration,
}) => {
  const { configuration } = useCitadelAPI();
  const {
    gameLoggedIn,
    recruitTroopsEnabled,
    autoRecruitMode,
    autoToolEnabled,
    autoToolMode,
    autoSceatResEnabled,
    autoFoodBalanceEnabled,
    autoHospitalEnabled,
    autoTCIEnabled,
    autoTCINextWakeUp,
    autoTowerEnabled,
    autoInvasionEnabled,
		autoNomadEnabled,
    autoAdvisorEnabled,
		autoBuyerEnabled,
    autoKhanEnabled,
    autoBeriWorldEnabled,
    autoStormEnabled,
    toggleRecruitTroops,
    toggleAutoTool,
    toggleAutoSceatRes,
    toggleAutoFoodBalance,
    toggleAutoHospital,
    toggleAutoTCI,
		toggleAutoTower,
		toggleAutoInvasion,
		toggleAutoNomad,
		toggleAutoAdvisor,
		toggleAutoBuyer,
		toggleAutoKhan,
		toggleAutoBeriWorld,
		toggleAutoStorm,
		automationStates,
		automationTimedUntilByKey,
  } = useAuth();
  const [now, setNow] = useState(() => Date.now());
  const [isEquipmentCleanupSettingsOpen, setIsEquipmentCleanupSettingsOpen] = useState(false);
  const [attackLaunchesByFeature, setAttackLaunchesByFeature] = useState<Record<string, number> | null | undefined>(undefined);
  const [dailyAttackSession, setDailyAttackSession] = useState<AttackLaunchDailySessionV2 | null | undefined>(undefined);

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
          setAttackLaunchesByFeature(rates.launchesByFeature);
          setDailyAttackSession(rates.dailySession ?? null);
        }
      } catch {
        if (!cancelled) {
          setAttackLaunchesByFeature(null);
          setDailyAttackSession(null);
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
      detail: recruitTroopsEnabled
			? automationStates.autoRecruit?.detail ?? `${modeLabel(autoRecruitMode)} · waiting for policy status`
			: `${modeLabel(autoRecruitMode)} · paused`,
      status: automationStates.autoRecruit?.status ?? (recruitTroopsEnabled ? 'waiting' : 'disabled'),
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
      detail: autoToolEnabled
			? automationStates.autoTool?.detail ?? `${modeLabel(autoToolMode)} · waiting for policy status`
			: `${modeLabel(autoToolMode)} · paused`,
      status: automationStates.autoTool?.status ?? (autoToolEnabled ? 'waiting' : 'disabled'),
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
      detail: autoHospitalEnabled
			? automationStates.autoHospital?.detail ?? 'Waiting for hospital policy status'
			: 'Automatic healing is paused',
      status: automationStates.autoHospital?.status ?? (autoHospitalEnabled ? 'waiting' : 'disabled'),
      icon: HeartPulse,
      onToggle: toggleAutoHospital,
      onOpenSettings: onOpenAutoHospitalSettings,
    },
    {
      id: 'autoTCI',
      enabledKey: 'auto_tci',
      group: 'upkeep',
      name: 'Auto TCI',
      description: 'Equips and renews temporary construction items automatically.',
      enabled: autoTCIEnabled,
      detail: autoTCIEnabled
			? automationStates.autoTCI?.detail ?? formatNextWake(autoTCINextWakeUp, now)
			: 'Construction-item automation is paused',
      status: automationStates.autoTCI?.status ?? (autoTCIEnabled ? 'waiting' : 'disabled'),
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
      detail: autoSceatResEnabled
			? autoSceatRuntime?.detail ?? 'Waiting for crafting policy status'
			: 'Crafting and logistics are paused',
      status: autoSceatStatus,
      statusLanes: [
        automationStatusLane('crafting', 'Crafting', autoSceatRuntime, autoSceatResEnabled, 'Waiting for crafting policy status'),
        automationStatusLane('logistics', 'Logistics', autoSceatLogisticsRuntime, autoSceatResEnabled, 'Waiting for logistics policy status'),
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
      detail: autoFoodBalanceEnabled
			? automationStates.autoFoodBalance?.detail ?? 'Waiting for food-balance policy status'
			: 'Food balancing is paused',
      status: automationStates.autoFoodBalance?.status ?? (autoFoodBalanceEnabled ? 'waiting' : 'disabled'),
      icon: Wheat,
      onToggle: toggleAutoFoodBalance,
      onOpenSettings: onOpenAutoFoodBalanceSettings,
    },
    {
      id: 'autoBuyer',
      enabledKey: 'auto_buyer',
      group: 'upkeep',
      name: 'Auto Buyer',
      description: 'Buys selected reset stock and maintains specialist and feast duration floors within explicit reserves.',
      enabled: autoBuyerEnabled,
      detail: autoBuyerEnabled
        ? automationStates.autoBuyer?.detail ?? 'Waiting for configured stock or upkeep goals'
        : 'Automatic purchases are paused',
      status: automationStates.autoBuyer?.status ?? (autoBuyerEnabled ? 'waiting' : 'disabled'),
      icon: ShoppingCart,
      onToggle: toggleAutoBuyer,
      onOpenSettings: onOpenAutoBuyerSettings,
    },
    {
		id: 'autoTowers',
		enabledKey: 'auto_towers',
		group: 'offense',
		name: 'Auto Towers',
		description: 'Attacks ready robber-baron towers with configured two-flank troop waves.',
		enabled: autoTowerEnabled,
		detail: autoTowerEnabled
			? automationStates.autoTowers?.detail ?? 'Waiting for tower map coverage'
			: 'Tower attacks are paused',
		status: automationStates.autoTowers?.status ?? (autoTowerEnabled ? 'waiting' : 'disabled'),
		icon: Crosshair,
		onToggle: toggleAutoTower,
		onOpenSettings: onOpenAutoTowerSettings,
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
      disabled: !gameLoggedIn,
    },
    {
      id: 'autoInvasion',
      enabledKey: 'auto_invasion',
      group: 'offense',
      name: 'Auto Invasion',
      description: 'Uses a CitadelOps attack preset against Foreign Lords and Bloodcrow castles until the score target is reached.',
      enabled: autoInvasionEnabled,
      detail: autoInvasionEnabled
        ? automationStates.autoInvasion?.detail ?? 'Waiting for an active invasion event'
        : 'Invasion attacks are paused',
      status: automationStates.autoInvasion?.status ?? (autoInvasionEnabled ? 'waiting' : 'disabled'),
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
		detail: autoNomadEnabled
			? automationStates.autoNomad?.detail ?? 'Waiting for an active Nomad or Samurai event'
			: 'Nomad and Samurai camp attacks are paused',
		status: automationStates.autoNomad?.status ?? (autoNomadEnabled ? 'waiting' : 'disabled'),
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
		detail: autoAdvisorEnabled
			? automationStates.autoAdvisor?.detail ?? 'Waiting for an active advisor-enabled event'
			: 'Advisor attacks are paused',
		status: automationStates.autoAdvisor?.status ?? (autoAdvisorEnabled ? 'waiting' : 'disabled'),
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
		detail: autoKhanEnabled
			? autoKhanAttackRuntime?.detail ?? 'Waiting for the Nomad event and Khan camp'
			: 'Khan camp attacks and taunts are paused',
		status: autoKhanStatus,
		statusLanes: [
			automationStatusLane('attacks', 'Attacks', autoKhanAttackRuntime, autoKhanEnabled, 'Waiting for the Khan attack policy'),
			automationStatusLane('cooldowns', 'Cooldowns', autoKhanCooldownRuntime, autoKhanEnabled, 'Waiting for the Khan cooldown policy'),
			automationStatusLane('rage', 'Rage', autoKhanRageRuntime, autoKhanEnabled, 'Waiting for the Khan rage policy'),
			automationStatusLane('defense', 'Defense', autoKhanDefenseRuntime, autoKhanEnabled, 'Waiting for the Khan defense policy'),
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
		detail: autoBeriWorldEnabled
			? autoBeriTransferRuntime?.detail ?? 'Waiting for Berimond availability and configuration'
			: 'Berimond transfers, tool purchases, tower attacks, and construction are paused',
		status: autoBeriWorldStatus,
		statusLanes: [
			automationStatusLane('transfers', 'Transfers', autoBeriTransferRuntime, autoBeriWorldEnabled, 'Waiting for the Berimond transfer policy'),
			automationStatusLane('attacks', 'Attacks', autoBeriAttackRuntime, autoBeriWorldEnabled, 'Waiting for the Berimond attack policy'),
			automationStatusLane('tools', 'Tools', autoBeriToolRuntime, autoBeriWorldEnabled, 'Waiting for the Berimond tool policy'),
			automationStatusLane(
				'builder',
				'Builder',
				autoBeriBuildRuntime,
				autoBeriWorldEnabled && autoBeriBuildEnabled,
				'Waiting for the Berimond builder policy',
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
      detail: autoStormEnabled
        ? autoStormRuntime?.detail ?? 'Waiting for an unlocked Storm castle or configured goal'
        : 'Storm construction and attacks are paused',
      status: autoStormStatus,
      statusLanes: [
        automationStatusLane('combat', 'Combat', autoStormRuntime, autoStormEnabled, 'Waiting for the Storm combat policy'),
        automationStatusLane('aquamarine-shop', 'Aquamarine shop', autoStormShopRuntime, autoStormEnabled, 'Waiting for the Aquamarine shop policy'),
        automationStatusLane('builder', 'Builder', autoStormBuildRuntime, autoStormEnabled, 'Waiting for the Storm builder policy'),
      ],
      icon: Crosshair,
      onToggle: toggleAutoStorm,
      onOpenSettings: onOpenAutoStormSettings,
    },
  ], [
    autoHospitalEnabled,
    autoEquipmentCleanup,
    autoRecruitMode,
    recruitTroopsEnabled,
    autoSceatResEnabled,
    autoSceatRuntime,
    autoSceatLogisticsRuntime,
    autoFoodBalanceEnabled,
    autoTCIEnabled,
    autoTCINextWakeUp,
    autoTowerEnabled,
    autoInvasionEnabled,
		autoNomadEnabled,
		autoAdvisorEnabled,
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
    autoStormEnabled,
    autoStormRuntime,
    autoStormShopRuntime,
    autoStormBuildRuntime,
    autoStormStatus,
    autoKhanStatus,
    autoSceatStatus,
    autoToolEnabled,
    autoToolMode,
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
    onOpenAutoInvasionSettings,
		onOpenAutoNomadSettings,
		onOpenAutoAdvisorSettings,
		onOpenAutoBuyerSettings,
    onOpenAutoKhanSettings,
    onOpenAutoBeriWorldSettings,
    onOpenAutoStormSettings,
    onOpenRecruitTroopsSettings,
    toggleAutoHospital,
    toggleAutoSceatRes,
    toggleAutoFoodBalance,
    toggleAutoTCI,
    toggleAutoTower,
    toggleAutoInvasion,
		toggleAutoNomad,
		toggleAutoAdvisor,
		toggleAutoBuyer,
    toggleAutoKhan,
    toggleAutoBeriWorld,
    toggleAutoStorm,
    toggleAutoTool,
    toggleRecruitTroops,
  ]);
  const groupedFeatures = automationGroups
    .map((group) => ({ ...group, features: features.filter((feature) => feature.group === group.id) }))
    .filter((group) => group.features.length > 0);

  return (
    <div className="mx-auto flex w-full max-w-[1800px] flex-col gap-4 pb-10">
      <div className="automation-function-groups">
        {groupedFeatures.map((group) => {
          const GroupIcon = group.icon;
          return (
            <section key={group.id} className="automation-function-group">
              <div className="automation-function-group-heading">
                <span className="automation-function-group-icon" aria-hidden="true">
                  <GroupIcon className="h-4 w-4" />
                </span>
                <h2>{group.name}</h2>
                <span className="automation-function-group-rule" aria-hidden="true" />
                <div className="automation-right-click-banner automation-right-click-inline" role="note">
                  <MousePointerClick aria-hidden="true" />
                  <span><strong className="text-text-main">Right-click</strong> a toggle for temporary activation</span>
                </div>
              </div>
              <div className="automation-function-grid">
                {group.features.map((feature) => {
                  const FeatureIcon = feature.icon;
                  const timedUntil = automationTimedUntilByKey[feature.enabledKey];
                  const attackLaunchCount = feature.group === 'offense'
                    ? attackRateCount(attackLaunchesByFeature, feature.id)
                    : undefined;
                  const dailyAttackLaunchCount = feature.group === 'offense'
                    ? dailyAttackSessionCount(dailyAttackSession, feature.id)
                    : undefined;
                  return (
                    <div
                      key={feature.id}
                      className={`automation-function-row ${feature.enabled ? 'automation-function-row-active' : ''}`}
                    >
                      <span
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
                          size="sm"
                          tone="feature"
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
                                title={attackRateTitle(feature.name, attackLaunchCount)}
                              >
                                {attackRateLabel(attackLaunchCount)}
                              </Badge>
                              <Badge
                                variant="outline"
                                className="shrink-0 whitespace-nowrap"
                                title={dailyAttackCountTitle(feature.name, dailyAttackLaunchCount, dailyAttackSession?.startedAt)}
                              >
                                {dailyAttackCountLabel(dailyAttackLaunchCount)}
                              </Badge>
                            </>
                          ) : null}
                          {timedUntil ? <Badge variant="outline">{formatTimedRemaining(timedUntil, now)}</Badge> : null}
                        </div>
                        <p>{feature.description}</p>
                        <AutomationStatusLines
                          featureName={feature.name}
                          status={feature.status}
                          detail={feature.detail}
                          lanes={feature.statusLanes}
                        />
                      </div>
                      <Button
                        variant="ghost"
                        size="icon"
                        className="automation-function-settings"
                        onClick={feature.onOpenSettings}
                        aria-label={`Open ${feature.name} settings`}
                        title={`Open ${feature.name} settings`}
                      >
                        <Settings className="h-3.5 w-3.5" />
                      </Button>
                    </div>
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
          <ModalTitle icon={<Trash2 className="h-5 w-5" />}>Auto Equipment Cleanup</ModalTitle>
        }
        footer={<Button variant="ghost" onClick={() => setIsEquipmentCleanupSettingsOpen(false)}>Close</Button>}
      >
        <div className="flex flex-col gap-4">
          <div className="flex flex-wrap items-center justify-between gap-4 rounded-global border border-primary/20 bg-primary/5 p-4">
            <div className="min-w-0">
              <div className="text-sm font-bold text-text-main">Poll interval</div>
              <p className="mt-1 text-xs text-text-muted">Checks equipment storage at this interval while cleanup is allowed to run.</p>
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
                  aria-label="Equipment cleanup poll interval in minutes"
                />
              </div>
              <span className="text-xs font-semibold text-text-muted">min</span>
            </div>
          </div>

          <ScheduleSummaryRow
            summary={equipmentCleanupScheduleLabel}
            actionLabel="Edit schedule"
            className="bg-bg-card/45 p-4"
            onEdit={() => {
                setIsEquipmentCleanupSettingsOpen(false);
                onOpenFeatureSchedule(AUTO_EQUIPMENT_CLEANUP_FEATURE_ID, 'Auto Equipment Cleanup');
            }}
          />

          <p className="text-xs leading-relaxed text-text-muted">
            The schedule decides when cleanup may run. The poll interval decides how often it checks while the schedule is active.
          </p>
        </div>
      </Modal>
    </div>
  );
};

export default AutomationView;
