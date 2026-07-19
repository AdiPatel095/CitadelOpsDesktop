import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Activity, RefreshCw, SlidersHorizontal } from 'lucide-react';
import { useCitadelAPI } from '../../api/ApiContext';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle, PillSelector, Select } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';
import { coinsUnderUpgradeReserve } from '../../utils/UpgradeCoinReserve';
import EquipmentOptimizer from './EquipmentOptimizer';
import {
	buildEquipmentEffectProfile,
	formatEquipmentCommonCap,
	formatEquipmentEffectLabel,
	formatEquipmentEffectValue,
	type EquipmentEffectGroup,
	type EquipmentEffectProfile,
	type EquipmentEffectScope,
	type MappedEquipmentEffect,
} from './EquipmentEffects';
import {
	EquipmentSellModal,
	EquipmentSwapModal,
	UnequipModal,
	UpgradeModal,
	type SaleRequest,
} from './EquipmentModals';
import {
	equipmentSlots,
	type CombatMode,
	type EquipmentLeader,
	type EquipmentMode,
	type EquipmentSlotRow,
	equipmentTargets,
	targetCombatMode,
	type EquipmentTarget,
} from './EquipmentTypes';

export default function EquipmentView() {
	const { state, configuration, submitIntent } = useCitadelAPI();
	const { effects, troops } = useMetadata();
	const [mode, setMode] = useState<EquipmentMode>('Commander');
	const [targetID, setTargetID] = useState('castle-1');
	const targetOptions = useMemo(() => equipmentTargets(), []);
	const target = useMemo(() => targetOptions.find((option) => option.id === targetID) ?? targetOptions[0]!, [targetID, targetOptions]);
	const combatMode = targetCombatMode(target.castleTypeID);
	const [selectedID, setSelectedID] = useState<number | null>(null);
	const [busy, setBusy] = useState(false);
	const [showSell, setShowSell] = useState(false);
	const [sellType, setSellType] = useState<'Equipment' | 'Gems'>('Equipment');
	const [showSwap, setShowSwap] = useState(false);
	const [showOptimizer, setShowOptimizer] = useState(false);
	const [unequipKind, setUnequipKind] = useState<'equipment' | 'gems' | null>(null);
	const [upgradeKind, setUpgradeKind] = useState<'equipment' | 'gem' | null>(null);
	const refreshedSession = useRef('');

	const leaders = useMemo<EquipmentLeader[]>(() => {
		if (!state) return [];
		if (mode === 'Commander') {
			return Object.values(state.commanders)
				.map((leader) => ({
					kind: 'commander' as const,
					id: leader.id,
					name: leader.name?.trim() || `Commander ${leader.visiblePosition ?? leader.id + 1}`,
					position: leader.visiblePosition ?? leader.id + 1,
					available: leader.available,
					equipment: leader.equipment,
					gems: leader.gems,
				}))
				.sort((left, right) => left.position - right.position || left.id - right.id);
		}
		return Object.values(state.castellans)
			.sort((left, right) => {
				const leftCastle = state.castles[String(left.castleId ?? '')];
				const rightCastle = state.castles[String(right.castleId ?? '')];
				return (leftCastle?.kingdomId ?? 99) - (rightCastle?.kingdomId ?? 99) || left.id - right.id;
			})
			.map((leader, index) => ({
				kind: 'castellan' as const,
				id: leader.id,
				name: leader.name?.trim() || state.castles[String(leader.castleId ?? '')]?.name?.trim() || `Castellan ${index + 1}`,
				position: index + 1,
				available: true,
				equipment: leader.equipment,
				gems: leader.gems,
			}));
	}, [mode, state]);

	useEffect(() => {
		if (selectedID != null && leaders.some((leader) => leader.id === selectedID)) return;
		setSelectedID(leaders[0]?.id ?? null);
	}, [leaders, selectedID]);

	const selected = leaders.find((leader) => leader.id === selectedID) ?? null;
	const rows = useMemo<EquipmentSlotRow[]>(() => equipmentSlots.map(({ slot, label }) => {
		const equipmentID = selected?.equipment[String(slot)];
		const gemID = selected?.gems[String(slot)];
		return {
			slot,
			label,
			item: equipmentID != null ? state?.inventory.equipment[String(equipmentID)] : undefined,
			gem: gemID != null ? state?.inventory.gems[String(gemID)] : undefined,
		};
	}), [selected, state?.inventory.equipment, state?.inventory.gems]);

	const effectProfile = useMemo(() => buildEquipmentEffectProfile(
		rows.flatMap((row) => [
			...(row.item?.effects.length ? [{ label: row.label, effects: row.item.effects }] : []),
			...(row.gem?.effects.length ? [{ label: `${row.label} gem`, effects: row.gem.effects }] : []),
		]),
		effects,
		troops,
		target.castleTypeID,
	), [effects, rows, target.castleTypeID, troops]);

	const candidateEffectIDs = useMemo(() => {
		if (!state || !selected) return [];
		const expectedType = selected.kind === 'commander' ? 2 : 1;
		const ids = new Set<number>();
		for (const item of Object.values(state.inventory.equipment)) {
			if (item.typeId !== expectedType) continue;
			for (const effect of item.effects ?? []) ids.add(effect.definitionId);
		}
		for (const gem of Object.values(state.inventory.gems)) {
			for (const effect of gem.effects ?? []) ids.add(effect.definitionId);
		}
		return Array.from(ids).filter((id) => id > 0).sort((left, right) => left - right);
	}, [selected, state]);

	const run = useCallback(async (work: () => Promise<unknown>, success?: string) => {
		setBusy(true);
		try {
			await work();
			if (success) Notifications.success(success);
			return true;
		} catch {
			return false;
		} finally {
			setBusy(false);
		}
	}, []);

	useEffect(() => {
		const key = state?.session.loggedIn ? state.session.changedAt : '';
		if (!key || refreshedSession.current === key) return;
		refreshedSession.current = key;
		void submitIntent('equipment.refresh').catch(() => undefined);
	}, [state?.session.changedAt, state?.session.loggedIn, submitIntent]);

	const sell = (request: SaleRequest) => void run(async () => {
		await submitIntent('equipment.refresh');
		await submitIntent('equipment.sell', request as unknown as Record<string, unknown>);
	}, 'Equipment storage cleanup completed').then((success) => success && setShowSell(false));

	const swap = (otherLeaderID: number) => {
		if (!selected) return;
		void run(() => submitIntent('equipment.swap', {
			leaderKind: selected.kind,
			firstLeaderId: selected.id,
			secondLeaderId: otherLeaderID,
		}), 'Equipment loadouts swapped').then((success) => success && setShowSwap(false));
	};

	const unequip = (slots: number[]) => {
		if (!selected || !unequipKind) return;
		void run(async () => {
			if (unequipKind === 'equipment') {
				await submitIntent('equipment.unequip', {
					leaderKind: selected.kind,
					leaderId: selected.id,
					equipmentIds: slots.map((slot) => selected.equipment[String(slot)]).filter(Boolean),
				});
				return;
			}
			for (const slot of slots) {
				const equipmentID = selected.equipment[String(slot)];
				if (!equipmentID) continue;
				await submitIntent('equipment.gem.unequip', {
					leaderKind: selected.kind, leaderId: selected.id, equipmentId: equipmentID,
				});
			}
		}, `${unequipKind === 'equipment' ? 'Equipment' : 'Gems'} unequipped`).then((success) => success && setUnequipKind(null));
	};

	const upgrade = (itemID: number, targetLevel: number) => {
		if (!upgradeKind) return;
		void run(() => submitIntent('equipment.upgrade', {
			itemKind: upgradeKind,
			itemId: itemID,
			targetLevel,
		}), `${upgradeKind === 'equipment' ? 'Equipment' : 'Gem'} upgraded`).then((success) => success && setUpgradeKind(null));
	};

	const scheduler = configuration?.sections.scheduler as Record<string, unknown> | undefined;
	const coinThreshold = Number(scheduler?.upgradeCoinThreshold ?? 0);
	const coins = Number(state?.player.resources['1'] ?? 0);
	const coinBlocked = coinsUnderUpgradeReserve(coins, coinThreshold);
	const controlsDisabled = !state?.session.loggedIn || busy || selected == null;

	return (
		<div className="equipment-view-shell">
			<StaleSessionBanner />
			<Card className="liquid-prominent-header-card equipment-workspace-card h-full min-h-0 flex flex-col">
				<CardHeader className="liquid-card-header-prominent flex flex-wrap items-center gap-4">
					<PillSelector ariaLabel="Equipment owner type" value={mode} options={['Commander', 'Castellan']} onChange={(value) => setMode(value as EquipmentMode)} />
					<div className="equipment-actions ml-auto">
						<Button size="sm" variant="outline" disabled={controlsDisabled || leaders.length < 2} onClick={() => setShowSwap(true)}><RefreshCw className="mr-1.5 h-4 w-4" />Swap Gear</Button>
						<Button size="sm" disabled={!state?.session.loggedIn || busy} onClick={() => { setSellType('Gems'); setShowSell(true); }} className="border border-warning/30 bg-warning/10 text-warning hover:border-warning/50 hover:bg-warning/20">Sell Gems</Button>
						<Button size="sm" disabled={!state?.session.loggedIn || busy} onClick={() => { setSellType('Equipment'); setShowSell(true); }} className="border border-warning/30 bg-warning/10 text-warning hover:border-warning/50 hover:bg-warning/20">Sell Equipment</Button>
					</div>
				</CardHeader>

				<CardContent className="liquid-prominent-header-content liquid-prominent-header-content-flush equipment-selection-body p-0">
					<div className="equipment-loadout-list custom-scrollbar">
						<div className="equipment-loadout-list-items">
							{leaders.map((leader) => (
								<button
									type="button"
									key={leader.id}
									onClick={() => setSelectedID(leader.id)}
									className={`equipment-loadout-item w-full rounded-global border px-3 py-2.5 text-left transition-all duration-200 ${selectedID === leader.id ? 'border-primary/30 bg-primary/10 text-primary shadow-[0_0_10px_var(--primary-glow)]' : 'border-transparent text-text-muted hover:bg-bg-card-hover hover:text-text-main'}`}
								>
									<span className="flex items-center gap-2">
										<span className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-bold ${selectedID === leader.id ? 'bg-primary text-bg-app' : 'border border-border-base bg-bg-app text-text-muted'}`}>{leader.position}</span>
										<span className="min-w-0 flex-1 truncate text-sm font-medium">{leader.name}</span>
									</span>
								</button>
							))}
							{leaders.length === 0 && <p className="px-3 py-5 text-center text-sm text-text-muted">No {mode.toLowerCase()} loadouts observed.</p>}
						</div>
					</div>

					<div className="equipment-report-sidebar">
						<EffectiveBattleReport
							leader={selected}
							effectProfile={effectProfile}
							target={target}
							targetOptions={targetOptions}
							onTargetChange={setTargetID}
						/>
					</div>

					<div className="equipment-stats-pane custom-scrollbar">
						<EquipmentStatsPane
							leader={selected}
							rows={rows}
							effectProfile={effectProfile}
							combatMode={combatMode}
							disabled={controlsDisabled}
							onUnequip={setUnequipKind}
							onUpgrade={setUpgradeKind}
							onReconfigure={() => setShowOptimizer(true)}
						/>
					</div>
				</CardContent>
			</Card>

			<EquipmentSellModal isOpen={showSell} itemType={sellType} onClose={() => setShowSell(false)} onConfirm={sell} busy={busy} />
			<EquipmentSwapModal isOpen={showSwap} leader={selected} leaders={leaders} onClose={() => setShowSwap(false)} onConfirm={swap} busy={busy} />
			<UnequipModal isOpen={unequipKind != null} kind={unequipKind ?? 'equipment'} leader={selected} rows={rows} onClose={() => setUnequipKind(null)} onConfirm={unequip} busy={busy} />
			<UpgradeModal isOpen={upgradeKind != null} kind={upgradeKind ?? 'equipment'} leader={selected} rows={rows} coinBlocked={coinBlocked} onClose={() => setUpgradeKind(null)} onConfirm={upgrade} busy={busy} />
			{showOptimizer && (
				<EquipmentOptimizer
					isOpen
					onClose={() => setShowOptimizer(false)}
					leader={selected}
					combatMode={combatMode}
					target={target}
					candidateEffectIDs={candidateEffectIDs}
					disabled={controlsDisabled}
				/>
			)}
		</div>
	);
}

function EffectiveBattleReport({
	leader,
	effectProfile,
	target,
	targetOptions,
	onTargetChange,
}: {
	leader: EquipmentLeader | null;
	effectProfile: EquipmentEffectProfile;
	target: EquipmentTarget;
	targetOptions: EquipmentTarget[];
	onTargetChange: (targetID: string) => void;
}) {
	return (
		<section className="flex h-full min-h-0 flex-col">
			<div className="equipment-report-header">
				<div className="min-w-0">
					<h3 className="flex items-center gap-2 text-base font-semibold text-text-main">
						<Activity className="h-4 w-4 shrink-0 text-primary" />
						Effective Battle Report
					</h3>
					<p className="mt-0.5 truncate text-xs text-text-muted">{leader?.name ?? 'Select a loadout'}</p>
				</div>
			</div>

			<div className="flex min-h-0 flex-1 flex-col">
				<div className="border-b border-border-base p-3">
					<Select
						value={target.id}
						options={targetOptions.map((option) => ({
							value: option.id,
							label: option.label,
							searchText: `${option.label} ${option.castleTypeID}`,
						}))}
						onChange={onTargetChange}
						placeholder="Target castle type"
						searchable
						searchPlaceholder="Search target castle type"
						ariaLabel="Battle target"
						className="w-full"
					/>
				</div>

				<div className="min-h-0 flex-1 overflow-y-auto p-3 custom-scrollbar">
					{leader && effectProfile.showcase.length > 0 ? (
						<ul className="overflow-hidden rounded-global border border-border-base bg-bg-app/40">
							{effectProfile.showcase.map((effect) => (
								<li key={effect.key} className="flex items-center justify-between gap-3 border-b border-border-base/60 px-3 py-2.5 last:border-b-0">
									<span className="min-w-0 flex-1 truncate text-xs text-text-muted" title={effect.label}>{effect.label}</span>
									<span className="shrink-0 font-mono text-sm font-semibold text-primary">{formatEquipmentEffectValue(effect, effect.value)}</span>
								</li>
							))}
						</ul>
					) : (
						<p className="py-8 text-center text-sm text-text-muted">
							{leader ? 'No mapped effects apply to this castle type.' : 'Select a loadout to view its effective battle report.'}
						</p>
					)}
				</div>
			</div>
		</section>
	);
}

function EquipmentStatsPane({
	leader,
	rows,
	effectProfile,
	combatMode,
	disabled,
	onUnequip,
	onUpgrade,
	onReconfigure,
}: {
	leader: EquipmentLeader | null;
	rows: EquipmentSlotRow[];
	effectProfile: EquipmentEffectProfile;
	combatMode: CombatMode;
	disabled: boolean;
	onUnequip: (kind: 'equipment' | 'gems') => void;
	onUpgrade: (kind: 'equipment' | 'gem') => void;
	onReconfigure: () => void;
}) {
	if (!leader) return <p className="py-12 text-center text-sm text-text-muted">Select a loadout.</p>;
	const equipmentCount = rows.filter((row) => row.item).length;
	const gemCount = rows.filter((row) => row.gem).length;
	const upgradeableGemCount = rows.filter((row) => (row.gem?.id ?? 0) > 0).length;
	return (
		<div className="p-4">
			<div className="mb-4">
				<div className="mb-2 flex items-center justify-between">
					<CardTitle className="truncate text-lg">{leader.name}</CardTitle>
				</div>
				<div className="flex flex-wrap items-center gap-2">
					<Badge variant={combatMode === 'PvP' ? 'danger' : 'success'}>{combatMode} Stats</Badge>
					<div className="ml-auto flex flex-wrap justify-end gap-2">
						<Button size="sm" variant="outline" disabled={disabled || equipmentCount === 0} onClick={() => onUpgrade('equipment')}>Upgrade Equipment</Button>
						<Button size="sm" disabled={disabled || upgradeableGemCount === 0} onClick={() => onUpgrade('gem')} className="border border-emerald-500/30 bg-emerald-500/10 text-emerald-400 hover:border-emerald-500/50 hover:bg-emerald-500/20">Upgrade Gem</Button>
						<Button size="sm" variant="secondary" disabled={disabled} onClick={onReconfigure} leftIcon={<SlidersHorizontal className="h-4 w-4" />}>Reconfigure</Button>
						<Button size="sm" variant="outline" disabled={disabled || equipmentCount === 0} onClick={() => onUnequip('equipment')}>Unequip Equipment</Button>
						<Button size="sm" disabled={disabled || gemCount === 0} onClick={() => onUnequip('gems')} className="border border-purple-500/30 bg-purple-500/10 text-purple-400 hover:border-purple-500/50 hover:bg-purple-500/20">Unequip Gem</Button>
					</div>
				</div>
			</div>

			<div className="space-y-4">
				{effectProfile.sections.map((section) => (
					<div key={section.key} className="rounded-global border border-border-base bg-bg-app p-3">
						<div className="mb-2 flex items-start justify-between gap-3">
							<div>
								<h3 className="text-xs font-bold uppercase tracking-wider text-text-muted">{section.title}</h3>
								<p className="mt-1 text-[11px] text-text-muted/80">{section.description}</p>
							</div>
							<Badge variant="outline" className="shrink-0 text-[10px]">{section.effectCount}</Badge>
						</div>
						<div className="space-y-0.5">
							{section.groups.map((group) => <EquipmentEffectGroupRows key={group.key} group={group} />)}
						</div>
					</div>
				))}
				{effectProfile.sections.length === 0 && <p className="py-8 text-center text-sm text-text-muted">No mapped equipment effects are available for this loadout.</p>}
			</div>
		</div>
	);
}

function EquipmentEffectGroupRows({ group }: { group: EquipmentEffectGroup }) {
	if (group.rows.length === 1) {
		return <EquipmentEffectDetailRow effect={group.rows[0]} includeCap />;
	}
	return (
		<div>
			<div className="rounded px-2 py-2 transition-colors hover:bg-bg-card-hover">
				<div className="flex items-start justify-between gap-3">
					<div className="min-w-0">
						<div className="flex flex-wrap items-center gap-2">
							<span className="text-sm font-medium text-text-muted">{group.label}</span>
							<Badge variant="outline" className="px-1.5 py-0 text-[9px]">{group.rows.length} effects</Badge>
							{group.capped && <Badge variant="warning" className="px-1.5 py-0 text-[9px]">Capped</Badge>}
						</div>
					</div>
					<div className="shrink-0 text-right">
						<div className="font-mono text-sm font-semibold text-primary">{formatEquipmentEffectValue(group, group.value)}</div>
						{group.capped && <div className="font-mono text-[11px] text-text-muted">raw {formatEquipmentEffectValue(group, group.rawValue)}</div>}
					</div>
				</div>
			</div>
			<div className="ml-3 border-l border-border-base pl-2">
				{group.commonCaps.map((cap) => (
					<div key={cap.capId} className="px-2 py-1.5 text-[11px] font-semibold text-text-muted">
						{formatEquipmentCommonCap(group, cap.max)}
					</div>
				))}
				{group.rows.map((effect) => (
					<EquipmentEffectDetailRow
						key={effect.key}
						effect={effect}
						includeCap={!group.commonCaps.some((cap) => cap.capId === effect.capId)}
					/>
				))}
			</div>
		</div>
	);
}

function EquipmentEffectDetailRow({
	effect,
	includeCap,
}: {
	effect: MappedEquipmentEffect;
	includeCap: boolean;
}) {
	return (
		<div className="rounded px-2 py-2 transition-colors hover:bg-bg-card-hover">
			<div className="flex items-start justify-between gap-3">
				<div className="min-w-0">
					<div className="flex flex-wrap items-center gap-2">
						<span className="text-sm text-text-muted">{formatEquipmentEffectLabel(effect)}</span>
						<Badge variant={effectScopeBadge(effect.scope)} className="px-1.5 py-0 text-[9px]">{effect.scope}</Badge>
						{effect.capped && <Badge variant="warning" className="px-1.5 py-0 text-[9px]">Capped</Badge>}
					</div>
					<div className="mt-1 text-[11px] text-text-muted/80">
						{effect.sources.join(' · ')}{includeCap && effect.cap ? ` · max ${formatEquipmentEffectValue(effect, effect.cap)}` : ''}
					</div>
				</div>
				<div className="shrink-0 text-right">
					<div className="font-mono text-sm font-semibold text-primary">{formatEquipmentEffectValue(effect, effect.value)}</div>
					{effect.capped && <div className="font-mono text-[11px] text-text-muted">raw {formatEquipmentEffectValue(effect, effect.rawValue)}</div>}
				</div>
			</div>
		</div>
	);
}

function effectScopeBadge(scope: EquipmentEffectScope): 'secondary' | 'danger' | 'success' {
	if (scope === 'PvP') return 'danger';
	if (scope === 'PvE') return 'success';
	return 'secondary';
}
