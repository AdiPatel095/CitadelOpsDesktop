import React, { useEffect, useMemo, useState } from 'react';
import {
  Activity,
  Bot,
  Coins,
  Hammer,
  HeartPulse,
	Send,
  Settings,
  Shield,
	Swords,
  Users,
  Wrench,
} from 'lucide-react';
import { useAuth } from '../context/AuthContext';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle, Switch } from '../components/ui';

interface AutomationViewProps {
	onOpenAutoBirdSettings: () => void;
	onOpenAutoStationSettings: () => void;
  onOpenAutoTCISettings: () => void;
  onOpenAutoSceatResSettings: () => void;
  onOpenRecruitTroopsSettings: () => void;
  onOpenAutoToolSettings: () => void;
  onOpenAutoHospitalSettings: () => void;
	onOpenAutoBeriWorldSettings: () => void;
}

interface AutomationFeature {
  id: string;
  name: string;
  description: string;
  enabled: boolean;
  detail: string;
  icon: React.ComponentType<{ className?: string }>;
  onToggle: () => void;
  onOpenSettings: () => void;
}

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

export const AutomationView: React.FC<AutomationViewProps> = ({
	onOpenAutoBirdSettings,
	onOpenAutoStationSettings,
  onOpenAutoTCISettings,
  onOpenAutoSceatResSettings,
  onOpenRecruitTroopsSettings,
  onOpenAutoToolSettings,
  onOpenAutoHospitalSettings,
	onOpenAutoBeriWorldSettings,
}) => {
  const {
    gameLoggedIn,
    recruitTroopsEnabled,
    autoRecruitMode,
    autoToolEnabled,
    autoToolMode,
    autoSceatResEnabled,
    autoHospitalEnabled,
    autoTCIEnabled,
    autoTCINextWakeUp,
	autoBirdEnabled,
	autoBirdNextWakeUp,
	autoStationEnabled,
	autoStationDetail,
    toggleRecruitTroops,
    toggleAutoTool,
    toggleAutoSceatRes,
    toggleAutoHospital,
    toggleAutoTCI,
	toggleAutoBird,
	toggleAutoStation,
		automationStates,
		autoBeriWorldEnabled,
		autoBeriWorldNextWakeUp,
		toggleAutoBeriWorld,
  } = useAuth();
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    const interval = window.setInterval(() => setNow(Date.now()), 30000);
    return () => window.clearInterval(interval);
  }, []);

  const features = useMemo<AutomationFeature[]>(() => [
	{
		id: 'autoBeriWorld',
		name: 'Auto Beri World',
		description: 'Checks Berimond capacity, transfers the configured troop batch, and applies the fixed march speed-up.',
		enabled: autoBeriWorldEnabled,
		detail: autoBeriWorldEnabled
			? automationStates.autoBeriWorld?.detail ?? formatNextWake(autoBeriWorldNextWakeUp, now)
			: 'Berimond troop transfers are paused',
		icon: Swords,
		onToggle: toggleAutoBeriWorld,
		onOpenSettings: onOpenAutoBeriWorldSettings,
	},
    {
		id: 'autoBird',
		name: 'Auto Bird',
		description: 'Stations troops above each castle reserve at protected same-kingdom alliance holdings.',
		enabled: autoBirdEnabled,
		detail: autoBirdEnabled
			? automationStates.autoBird?.detail ?? formatNextWake(autoBirdNextWakeUp, now)
			: 'Alliance stationing is paused',
		icon: Send,
		onToggle: toggleAutoBird,
		onOpenSettings: onOpenAutoBirdSettings,
	},
	{
		id: 'autoStation',
		name: 'Auto Station',
		description: 'Evacuates troops before incoming attacks and recalls them after a fresh clear snapshot.',
		enabled: autoStationEnabled,
		detail: autoStationEnabled
			? autoStationDetail || automationStates.autoStation?.detail || 'Monitoring incoming movements'
			: 'Threat protection is paused',
		icon: Shield,
		onToggle: toggleAutoStation,
		onOpenSettings: onOpenAutoStationSettings,
	},
	{
      id: 'autoRecruit',
      name: 'Auto Recruit',
      description: 'Keeps troop recruitment queues stocked from the configured plans.',
      enabled: recruitTroopsEnabled,
		detail: recruitTroopsEnabled
			? automationStates.autoRecruit?.detail ?? `${modeLabel(autoRecruitMode)} · waiting for policy status`
			: `${modeLabel(autoRecruitMode)} · paused`,
      icon: Users,
      onToggle: toggleRecruitTroops,
      onOpenSettings: onOpenRecruitTroopsSettings,
    },
    {
      id: 'autoTool',
      name: 'Auto Tool',
      description: 'Maintains tool production queues across configured castles.',
      enabled: autoToolEnabled,
		detail: autoToolEnabled
			? automationStates.autoTool?.detail ?? `${modeLabel(autoToolMode)} · waiting for policy status`
			: `${modeLabel(autoToolMode)} · paused`,
      icon: Wrench,
      onToggle: toggleAutoTool,
      onOpenSettings: onOpenAutoToolSettings,
    },
    {
      id: 'autoHospital',
      name: 'Auto Hospital',
      description: 'Processes hospital queues using the configured healing priorities.',
      enabled: autoHospitalEnabled,
		detail: autoHospitalEnabled
			? automationStates.autoHospital?.detail ?? 'Waiting for hospital policy status'
			: 'Automatic healing is paused',
      icon: HeartPulse,
      onToggle: toggleAutoHospital,
      onOpenSettings: onOpenAutoHospitalSettings,
    },
    {
      id: 'autoTCI',
      name: 'Auto TCI',
      description: 'Equips and renews temporary construction items automatically.',
      enabled: autoTCIEnabled,
		detail: autoTCIEnabled
			? automationStates.autoTCI?.detail ?? formatNextWake(autoTCINextWakeUp, now)
			: 'Construction-item automation is paused',
      icon: Hammer,
      onToggle: toggleAutoTCI,
      onOpenSettings: onOpenAutoTCISettings,
    },
    {
      id: 'autoSceatRes',
      name: 'Auto Sceat Resources',
      description: 'Balances kingdom resources and maintains Refinery, Toolsmith, Dragon Hoard, and Dragon Forge queues.',
      enabled: autoSceatResEnabled,
		detail: autoSceatResEnabled
			? automationStates.autoSceatRes?.detail ?? 'Waiting for crafting policy status'
			: 'Crafting and logistics are paused',
      icon: Coins,
      onToggle: toggleAutoSceatRes,
      onOpenSettings: onOpenAutoSceatResSettings,
    },
  ], [
	autoBirdEnabled,
	autoBirdNextWakeUp,
	autoBeriWorldEnabled,
	autoBeriWorldNextWakeUp,
    autoHospitalEnabled,
    autoRecruitMode,
    autoSceatResEnabled,
	autoStationDetail,
	autoStationEnabled,
    autoTCIEnabled,
    autoTCINextWakeUp,
    autoToolEnabled,
    autoToolMode,
		automationStates,
    now,
    onOpenAutoHospitalSettings,
	onOpenAutoBirdSettings,
	onOpenAutoBeriWorldSettings,
	onOpenAutoStationSettings,
    onOpenAutoSceatResSettings,
    onOpenAutoTCISettings,
    onOpenAutoToolSettings,
    onOpenRecruitTroopsSettings,
    toggleAutoHospital,
	toggleAutoBird,
	toggleAutoBeriWorld,
	toggleAutoStation,
    toggleAutoSceatRes,
    toggleAutoTCI,
    toggleAutoTool,
    toggleRecruitTroops,
  ]);
  const activeCount = features.filter((feature) => feature.enabled).length;

  return (
    <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 pb-10">
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div className="flex items-center gap-3">
          <div className="flex h-12 w-12 items-center justify-center rounded-2xl border border-primary/30 bg-primary/10 text-primary">
            <Bot className="h-6 w-6" />
          </div>
          <div>
            <h1 className="text-2xl font-bold text-text-main">Automation</h1>
            <p className="mt-1 text-sm text-text-muted">Central control for every automated CitadelOps feature.</p>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-2">
					<Badge variant={activeCount > 0 ? 'success' : 'secondary'}>{activeCount} of {features.length} enabled</Badge>
          <Badge variant={gameLoggedIn ? 'outline' : 'warning'}>{gameLoggedIn ? 'Live game state' : 'Last known game state'}</Badge>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-5 lg:grid-cols-2">
        {features.map((feature) => {
          const FeatureIcon = feature.icon;
          return (
            <Card key={feature.id} variant="solid" className={`liquid-prominent-header-card flex h-full flex-col transition-colors ${feature.enabled ? 'border-success/30' : ''}`}>
              <CardHeader className="liquid-card-header-prominent flex min-h-20 flex-row items-center justify-between gap-4 px-5 py-4 sm:px-6">
                <div className="flex min-w-0 items-center gap-3">
                  <div className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl border ${feature.enabled ? 'border-success/30 bg-success/10 text-success' : 'border-border-base bg-bg-input/45 text-text-muted'}`}>
                    <FeatureIcon className="h-5 w-5" />
                  </div>
                  <div className="min-w-0">
                    <div className="flex flex-wrap items-center gap-2">
                      <CardTitle className="text-base">{feature.name}</CardTitle>
                      <Badge variant={feature.enabled ? 'success' : 'danger'}>{feature.enabled ? 'On' : 'Off'}</Badge>
                    </div>
                    <p className="mt-1 text-[11px] font-semibold uppercase tracking-wide text-text-muted">Automation control</p>
                  </div>
                </div>
                <Switch checked={feature.enabled} onChange={feature.onToggle} size="sm" />
              </CardHeader>
              <CardContent className="liquid-prominent-header-content flex flex-1 flex-col gap-5 p-5 sm:p-6">
                <p className="min-h-10 text-sm font-medium leading-relaxed text-text-muted">{feature.description}</p>
                <div className="flex min-h-11 items-center gap-3 rounded-global border border-border-base bg-bg-input/35 px-4 py-3 text-xs font-semibold text-text-muted">
                  <Activity className={`h-4 w-4 shrink-0 ${feature.enabled ? 'text-success' : 'text-text-muted'}`} />
                  <span>{feature.detail}</span>
                </div>
                <div className="mt-auto flex flex-wrap items-center justify-end gap-2 border-t border-border-base/70 pt-4">
                  <Button variant="outline" size="sm" onClick={feature.onOpenSettings} leftIcon={<Settings className="h-4 w-4" />}>
                    Settings
                  </Button>
                </div>
              </CardContent>
            </Card>
          );
        })}
      </div>
    </div>
  );
};

export default AutomationView;
