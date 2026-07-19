import React, { useEffect, useMemo, useState } from 'react';
import {
  Bot,
  Coins,
  Crosshair,
  Hammer,
  HeartPulse,
  Settings,
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
  PageHeader,
  ScheduleSummaryRow,
  StatusIndicator,
  Switch,
  type StatusTone,
} from '../components/ui';
import {
  AUTO_EQUIPMENT_CLEANUP_FEATURE_ID,
  type AutoEquipmentCleanupController,
} from '../settings/AutoEquipmentCleanup';
import { scheduleSummary } from '../settings/SchedulerTypes';

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
  onOpenAutoKhanSettings: () => void;
  onOpenAutoStormSettings: () => void;
  autoEquipmentCleanup: AutoEquipmentCleanupController;
  onOpenFeatureSchedule: (id: string, label: string) => void;
}

interface AutomationFeature {
  id: string;
  group: AutomationGroupID;
  name: string;
  description: string;
  enabled: boolean;
  detail: string;
  status: string;
  icon: React.ComponentType<{ className?: string }>;
  onToggle: () => void;
  onOpenSettings: () => void;
  disabled?: boolean;
}

type AutomationGroupID = 'production' | 'upkeep' | 'support' | 'offense';

const automationGroups: Array<{
  id: AutomationGroupID;
  name: string;
  icon: React.ComponentType<{ className?: string }>;
}> = [
  { id: 'production', name: 'Production & Building', icon: Wrench },
  { id: 'upkeep', name: 'Resources & Upkeep', icon: Coins },
  { id: 'support', name: 'Recovery & Support', icon: HeartPulse },
  { id: 'offense', name: 'Offense', icon: Crosshair },
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

function modeLabel(mode: 'global' | 'perCastle'): string {
  return mode === 'perCastle' ? 'Per-castle plan' : 'Global plan';
}

function automationStatusTone(status: string): StatusTone {
  switch (status.toLowerCase()) {
    case 'completed':
    case 'success':
      return 'success';
    case 'failed':
    case 'error':
      return 'danger';
    case 'blocked':
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

function automationStatusLabel(status: string): string {
  if (!status) return 'Unknown';
  return status.charAt(0).toUpperCase() + status.slice(1).replaceAll('_', ' ');
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
  onOpenAutoKhanSettings,
  onOpenAutoStormSettings,
  autoEquipmentCleanup,
  onOpenFeatureSchedule,
}) => {
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
    autoKhanEnabled,
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
		toggleAutoKhan,
		toggleAutoStorm,
		automationStates,
  } = useAuth();
  const [now, setNow] = useState(() => Date.now());
  const [isEquipmentCleanupSettingsOpen, setIsEquipmentCleanupSettingsOpen] = useState(false);

  useEffect(() => {
    const interval = window.setInterval(() => setNow(Date.now()), 30000);
    return () => window.clearInterval(interval);
  }, []);

  const equipmentCleanupScheduleLabel = autoEquipmentCleanup.schedule?.enabled
    ? scheduleSummary(autoEquipmentCleanup.schedule)
    : 'Runs any time';

  const features = useMemo<AutomationFeature[]>(() => [
    {
      id: 'autoRecruit',
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
      group: 'production',
      name: 'Auto Sceat Resources',
      description: 'Balances kingdom resources and maintains Refinery, Toolsmith, Dragon Hoard, and Dragon Forge queues.',
      enabled: autoSceatResEnabled,
      detail: autoSceatResEnabled
			? automationStates.autoSceatRes?.detail ?? 'Waiting for crafting policy status'
			: 'Crafting and logistics are paused',
      status: automationStates.autoSceatRes?.status ?? (autoSceatResEnabled ? 'waiting' : 'disabled'),
      icon: Coins,
      onToggle: toggleAutoSceatRes,
      onOpenSettings: onOpenAutoSceatResSettings,
    },
    {
      id: 'autoFoodBalance',
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
		id: 'autoTowers',
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
		id: 'autoKhan',
		group: 'offense',
		name: 'Auto Khan',
		description: 'Chains Khan camp hits and retaliations while keeping the Great Empire main castle on its defense preset.',
		enabled: autoKhanEnabled,
		detail: autoKhanEnabled
			? automationStates.autoKhan?.detail ?? 'Waiting for the Nomad event and Khan camp'
			: 'Khan camp attacks and taunts are paused',
		status: automationStates.autoKhan?.status ?? (autoKhanEnabled ? 'waiting' : 'disabled'),
		icon: Crosshair,
		onToggle: toggleAutoKhan,
		onOpenSettings: onOpenAutoKhanSettings,
	},
    {
      id: 'autoStorm',
      group: 'offense',
      name: 'Auto Storm',
      description: 'Builds a captured Storm castle target, attacks selected forts and islands, and spends Aquamarine by priority.',
      enabled: autoStormEnabled,
      detail: autoStormEnabled
        ? automationStates.autoStorm?.detail ?? 'Waiting for an unlocked Storm castle or configured goal'
        : 'Storm construction and attacks are paused',
      status: automationStates.autoStorm?.status ?? (autoStormEnabled ? 'waiting' : 'disabled'),
      icon: Crosshair,
      onToggle: toggleAutoStorm,
      onOpenSettings: onOpenAutoStormSettings,
    },
  ], [
    autoHospitalEnabled,
    autoEquipmentCleanup,
    autoRecruitMode,
    autoSceatResEnabled,
    autoFoodBalanceEnabled,
    autoTCIEnabled,
    autoTCINextWakeUp,
    autoTowerEnabled,
    autoInvasionEnabled,
		autoNomadEnabled,
    autoKhanEnabled,
    autoStormEnabled,
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
    onOpenAutoKhanSettings,
    onOpenAutoStormSettings,
    onOpenRecruitTroopsSettings,
    toggleAutoHospital,
    toggleAutoSceatRes,
    toggleAutoFoodBalance,
    toggleAutoTCI,
    toggleAutoTower,
    toggleAutoInvasion,
		toggleAutoNomad,
    toggleAutoKhan,
    toggleAutoStorm,
    toggleAutoTool,
    toggleRecruitTroops,
  ]);
  const activeCount = features.filter((feature) => feature.enabled).length;
  const groupedFeatures = automationGroups
    .map((group) => ({ ...group, features: features.filter((feature) => feature.group === group.id) }))
    .filter((group) => group.features.length > 0);

  return (
    <div className="mx-auto flex w-full max-w-[1800px] flex-col gap-4 pb-10">
      <PageHeader
        title="Automation"
        description="Configure automation and see what each feature is doing now."
        icon={<Bot className="h-5 w-5" />}
        actions={(
          <div className="flex flex-wrap justify-end gap-2">
            <Badge variant={activeCount > 0 ? 'success' : 'secondary'}>{activeCount} of {features.length} enabled</Badge>
            <Badge variant={gameLoggedIn ? 'outline' : 'warning'}>{gameLoggedIn ? 'Live game state' : 'Last known game state'}</Badge>
          </div>
        )}
      />

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
              </div>
              <div className="automation-function-grid">
                {group.features.map((feature) => {
                  const FeatureIcon = feature.icon;
                  return (
                    <div
                      key={feature.id}
                      className={`automation-function-row ${feature.enabled ? 'automation-function-row-active' : ''}`}
                    >
                      <Switch
                        checked={feature.enabled}
                        onChange={feature.onToggle}
                        size="sm"
                        ariaLabel={`Toggle ${feature.name}`}
                        disabled={feature.disabled}
                      />
                      <div className="automation-function-copy">
                        <div className="flex min-w-0 items-center gap-2">
                          <FeatureIcon className="h-3.5 w-3.5 shrink-0 text-text-muted" />
                          <h3>{feature.name}</h3>
                        </div>
                        <p>{feature.description}</p>
                        <StatusIndicator
                          tone={automationStatusTone(feature.status)}
                          label={automationStatusLabel(feature.status)}
                          detail={feature.detail}
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
