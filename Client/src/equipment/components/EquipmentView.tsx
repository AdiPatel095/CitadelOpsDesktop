import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { ArrowUpCircle, Crown, Gem, RefreshCw, Shield, Swords, Trash2 } from 'lucide-react';
import { useCitadelAPI } from '../../api/ApiContext';
import type { EquipmentEffectV2, EquipmentInstanceV2, GemInstanceV2 } from '../../api/Contracts';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Card, CardContent, CardHeader, CardTitle, Input, PillSelector, Switch } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';
import { coinsUnderUpgradeReserve } from '../../utils/UpgradeCoinReserve';
import EquipmentOptimizer from './EquipmentOptimizer';
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
} from './EquipmentTypes';

const autoSellEnabledKey = 'equipmentAutoSellNonRelicEquipment';
const autoSellIntervalKey = 'equipmentAutoSellNonRelicEquipmentIntervalMinutes';

export default function EquipmentView() {
	const { state, configuration, submitIntent } = useCitadelAPI();
	const { getEffect, getEquipment, getGem } = useMetadata();
	const [mode, setMode] = useState<EquipmentMode>('Commander');
	const [combatMode, setCombatMode] = useState<CombatMode>('PvP');
	const [selectedID, setSelectedID] = useState<number | null>(null);
	const [busy, setBusy] = useState(false);
	const [showSell, setShowSell] = useState(false);
	const [sellType, setSellType] = useState<'Equipment' | 'Gems'>('Equipment');
	const [showSwap, setShowSwap] = useState(false);
	const [unequipKind, setUnequipKind] = useState<'equipment' | 'gems' | null>(null);
	const [upgradeKind, setUpgradeKind] = useState<'equipment' | 'gem' | null>(null);
	const [autoSell, setAutoSell] = useState(() => readBoolean(autoSellEnabledKey));
	const [autoSellMinutes, setAutoSellMinutes] = useState(() => readInterval());
	const autoSellRunning = useRef(false);
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

	const effectTotals = useMemo(() => {
		const totals = new Map<number, number>();
		for (const row of rows) {
			for (const source of [row.item?.effects, row.gem?.effects]) {
				for (const effect of source ?? []) {
					totals.set(effect.definitionId, (totals.get(effect.definitionId) ?? 0) + effectMagnitude(effect.values));
				}
			}
		}
		return Array.from(totals, ([id, value]) => ({ id, value }))
			.sort((left, right) => effectLabel(left.id, getEffect).localeCompare(effectLabel(right.id, getEffect)) || left.id - right.id);
	}, [getEffect, rows]);

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

	const refresh = useCallback(() => run(() => submitIntent('equipment.refresh'), 'Equipment refreshed'), [run, submitIntent]);

	useEffect(() => {
		const key = state?.session.loggedIn ? state.session.changedAt : '';
		if (!key || refreshedSession.current === key) return;
		refreshedSession.current = key;
		void submitIntent('equipment.refresh').catch(() => undefined);
	}, [state?.session.changedAt, state?.session.loggedIn, submitIntent]);

	useEffect(() => {
		try {
			localStorage.setItem(autoSellEnabledKey, String(autoSell));
			localStorage.setItem(autoSellIntervalKey, String(autoSellMinutes));
		} catch {
			// Browser privacy settings may disable local storage; the live setting still works.
		}
		if (!autoSell || !state?.session.loggedIn) return;
		const sell = async () => {
			if (autoSellRunning.current) return;
			autoSellRunning.current = true;
			try {
				await submitIntent('equipment.refresh', {}, { actor: 'ui:auto-sell' });
				await submitIntent('equipment.sell', {
					category: 'non_relic_equipment', sellLookItems: false, sellPost2026: false,
				}, { actor: 'ui:auto-sell' });
			} catch {
				// Intent failures already publish a user-facing notification.
			} finally {
				autoSellRunning.current = false;
			}
		};
		void sell();
		const timer = window.setInterval(() => void sell(), autoSellMinutes * 60_000);
		return () => window.clearInterval(timer);
	}, [autoSell, autoSellMinutes, state?.session.loggedIn, submitIntent]);

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
			<div className="equipment-layout">
				<div className="equipment-main-panel">
					<Card className="liquid-prominent-header-card h-full min-h-0 flex flex-col">
						<CardHeader className="liquid-card-header-prominent flex-row flex-wrap items-center gap-3">
							<PillSelector value={mode} options={['Commander', 'Castellan']} onChange={(value) => setMode(value as EquipmentMode)} />
							<PillSelector value={combatMode} options={['PvP', 'PvE']} onChange={(value) => setCombatMode(value as CombatMode)} />
							<div className="equipment-actions ml-auto">
								<div className="flex items-center gap-2 rounded-global border border-primary/25 bg-primary/5 px-3 py-1.5 text-sm text-text-main" title="Automatically sell eligible old non-relic equipment.">
									<span className="font-medium">AutoSell</span>
									<Switch checked={autoSell} onChange={setAutoSell} size="sm" disabled={!state?.session.loggedIn} />
									<span className="text-xs text-text-muted">Every</span>
									<Input type="number" min={1} max={1440} value={autoSellMinutes} onChange={(event) => setAutoSellMinutes(clampInterval(Number(event.target.value)))} className="h-8 w-16 px-2 py-1 text-center" />
									<span className="text-xs text-text-muted">min</span>
								</div>
								<Button size="sm" variant="secondary" disabled={!state?.session.loggedIn || busy} onClick={() => void refresh()} leftIcon={<RefreshCw className="h-4 w-4" />}>Refresh</Button>
								<Button size="sm" variant="outline" disabled={controlsDisabled || leaders.length < 2} onClick={() => setShowSwap(true)} leftIcon={<RefreshCw className="h-4 w-4" />}>Swap Gear</Button>
								<Button size="sm" variant="outline" disabled={!state?.session.loggedIn || busy} onClick={() => { setSellType('Gems'); setShowSell(true); }} leftIcon={<Gem className="h-4 w-4" />}>Sell Gems</Button>
								<Button size="sm" variant="outline" disabled={!state?.session.loggedIn || busy} onClick={() => { setSellType('Equipment'); setShowSell(true); }} leftIcon={<Trash2 className="h-4 w-4" />}>Sell Equipment</Button>
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
											className={`equipment-loadout-item flex w-full items-center gap-2 rounded-global border px-3 py-2.5 text-left transition-all ${selectedID === leader.id ? 'border-primary/30 bg-primary/10 text-primary' : 'border-transparent text-text-muted hover:bg-bg-card-hover hover:text-text-main'}`}
										>
											<span className={`flex h-6 w-6 shrink-0 items-center justify-center rounded-full text-xs font-bold ${selectedID === leader.id ? 'bg-primary text-bg-app' : 'border border-border-base bg-bg-app'}`}>{leader.position}</span>
											<span className="min-w-0 flex-1 truncate text-sm font-medium">{leader.name}</span>
											{leader.kind === 'commander' && !leader.available ? <Badge variant="warning">Busy</Badge> : null}
										</button>
									))}
									{leaders.length === 0 && <p className="px-3 py-5 text-center text-sm text-text-muted">No {mode.toLowerCase()} loadouts observed.</p>}
								</div>
							</div>

							<div className="equipment-stats-pane custom-scrollbar">
								<EquipmentStatsPane
									leader={selected}
									rows={rows}
									effectTotals={effectTotals}
									getEffectName={(id) => effectLabel(id, getEffect)}
									getEquipmentName={(item) => getEquipment(item.definitionId)?.name ?? `${slotName(item.slot)} ${item.id}`}
									getGemName={(gem) => getGem(gem.definitionId)?.name ?? `${gem.id > 0 ? 'Relic Gem' : 'Gem'} ${gem.definitionId}`}
									disabled={controlsDisabled}
									onUnequip={setUnequipKind}
									onUpgrade={setUpgradeKind}
								/>
							</div>
						</CardContent>
					</Card>
				</div>

				<div className="equipment-priority-panel">
					<EquipmentOptimizer leader={selected} combatMode={combatMode} candidateEffectIDs={candidateEffectIDs} disabled={controlsDisabled} />
				</div>
			</div>

			<EquipmentSellModal isOpen={showSell} itemType={sellType} onClose={() => setShowSell(false)} onConfirm={sell} busy={busy} />
			<EquipmentSwapModal isOpen={showSwap} leader={selected} leaders={leaders} onClose={() => setShowSwap(false)} onConfirm={swap} busy={busy} />
			<UnequipModal isOpen={unequipKind != null} kind={unequipKind ?? 'equipment'} leader={selected} rows={rows} onClose={() => setUnequipKind(null)} onConfirm={unequip} busy={busy} />
			<UpgradeModal isOpen={upgradeKind != null} kind={upgradeKind ?? 'equipment'} leader={selected} rows={rows} coinBlocked={coinBlocked} onClose={() => setUpgradeKind(null)} onConfirm={upgrade} busy={busy} />
		</div>
	);
}

function EquipmentStatsPane({
	leader,
	rows,
	effectTotals,
	getEffectName,
	getEquipmentName,
	getGemName,
	disabled,
	onUnequip,
	onUpgrade,
}: {
	leader: EquipmentLeader | null;
	rows: EquipmentSlotRow[];
	effectTotals: Array<{ id: number; value: number }>;
	getEffectName: (id: number) => string;
	getEquipmentName: (item: EquipmentInstanceV2) => string;
	getGemName: (gem: GemInstanceV2) => string;
	disabled: boolean;
	onUnequip: (kind: 'equipment' | 'gems') => void;
	onUpgrade: (kind: 'equipment' | 'gem') => void;
}) {
	if (!leader) return <p className="py-12 text-center text-sm text-text-muted">Select a loadout.</p>;
	const equipmentCount = rows.filter((row) => row.item).length;
	const gemCount = rows.filter((row) => row.gem).length;
	const upgradeableGemCount = rows.filter((row) => (row.gem?.id ?? 0) > 0).length;
	return (
		<div className="space-y-5 p-4">
			<div className="flex flex-wrap items-center gap-3">
				<div className={`flex h-9 w-9 items-center justify-center rounded-lg ${leader.kind === 'commander' ? 'bg-primary/10 text-primary' : 'bg-warning/10 text-warning'}`}>
					{leader.kind === 'commander' ? <Swords className="h-5 w-5" /> : <Crown className="h-5 w-5" />}
				</div>
				<div className="min-w-0 flex-1"><CardTitle className="truncate text-base">{leader.name}</CardTitle><p className="text-[11px] text-text-muted">ID {leader.id} · {equipmentCount}/5 pieces · {gemCount}/4 gems</p></div>
				<Badge variant={leader.available ? 'success' : 'warning'}>{leader.available ? 'Available' : 'Busy'}</Badge>
			</div>

			<div className="flex flex-wrap gap-2">
				<Button size="sm" variant="outline" disabled={disabled || equipmentCount === 0} onClick={() => onUnequip('equipment')} leftIcon={<Shield className="h-4 w-4" />}>Unequip Equipment</Button>
				<Button size="sm" variant="outline" disabled={disabled || gemCount === 0} onClick={() => onUnequip('gems')} leftIcon={<Gem className="h-4 w-4" />}>Unequip Gems</Button>
				<Button size="sm" variant="secondary" disabled={disabled || equipmentCount === 0} onClick={() => onUpgrade('equipment')} leftIcon={<ArrowUpCircle className="h-4 w-4" />}>Upgrade Equipment</Button>
				<Button size="sm" variant="secondary" disabled={disabled || upgradeableGemCount === 0} onClick={() => onUpgrade('gem')} leftIcon={<ArrowUpCircle className="h-4 w-4" />}>Upgrade Gem</Button>
			</div>

			{effectTotals.length > 0 && (
				<div className="rounded-global border border-border-base bg-bg-app/35 p-3">
					<h3 className="mb-2 text-[10px] font-bold uppercase tracking-wider text-text-muted">Combined live effects</h3>
					<div className="grid gap-x-5 gap-y-1.5 sm:grid-cols-2">
						{effectTotals.map((effect) => (
							<div key={effect.id} className="flex items-center justify-between gap-3 text-xs"><span className="truncate text-text-muted">{getEffectName(effect.id)}</span><span className="shrink-0 font-mono font-semibold text-text-main">{formatValue(effect.value)}</span></div>
						))}
					</div>
				</div>
			)}

			<div className="grid gap-3 md:grid-cols-2">
				{rows.map((row) => (
					<div key={row.slot} className="rounded-global border border-border-base bg-bg-card/55 p-4">
						<div className="flex items-start justify-between gap-3">
							<div className="min-w-0"><p className="text-[10px] font-bold uppercase tracking-wider text-text-muted">{row.label}</p><h3 className="mt-1 truncate text-sm font-bold text-text-main">{row.item ? getEquipmentName(row.item) : 'Empty slot'}</h3></div>
							{row.item?.setId ? <Badge variant="outline">Set {row.item.setId}</Badge> : null}
						</div>
						{row.item && <p className="mt-1 text-[10px] text-text-muted">Instance {row.item.id} · Level {row.item.level ?? 0} · Rarity {row.item.rarityId ?? 0}</p>}
						{row.item && <EffectList effects={row.item.effects ?? []} getEffectName={getEffectName} />}
						{row.gem && (
							<div className="mt-3 rounded-lg border border-purple-500/25 bg-purple-500/5 p-3">
								<div className="flex items-center gap-2 text-xs font-semibold text-purple-300"><Gem className="h-3.5 w-3.5" /><span className="truncate">{getGemName(row.gem)}</span><span className="ml-auto text-[10px] text-text-muted">Lv {row.gem.level ?? 0}</span></div>
								<EffectList effects={row.gem.effects ?? []} getEffectName={getEffectName} compact />
							</div>
						)}
					</div>
				))}
			</div>
		</div>
	);
}

function EffectList({ effects, getEffectName, compact = false }: { effects: EquipmentEffectV2[]; getEffectName: (id: number) => string; compact?: boolean }) {
	if (effects.length === 0) return null;
	return (
		<div className={`${compact ? 'mt-2' : 'mt-3'} space-y-1.5`}>
			{effects.map((effect, index) => (
				<div key={`${effect.wireId}-${index}`} className="flex items-start justify-between gap-3 text-xs">
					<span className="min-w-0 truncate text-text-muted">{getEffectName(effect.definitionId)}</span>
					<span className="shrink-0 font-mono font-semibold text-text-main">
						{effect.values.map(formatValue).join(' / ')}{effect.rollPercent != null ? ` · ${stars(effect.rollPercent)}★` : ''}
					</span>
				</div>
			))}
		</div>
	);
}

function effectMagnitude(values: number[]): number {
	if (values.length === 0) return 0;
	if (values.length === 1) return values[0];
	if (values.length % 2 === 0 && values.every((value, index) => index % 2 === 1 || Number.isInteger(value) && value >= 1)) {
		return Math.max(...values.filter((_, index) => index % 2 === 1));
	}
	return values[values.length - 1];
}

function stars(percent: number): number {
	if (percent >= 100) return 7;
	if (percent >= 90) return 6;
	if (percent >= 80) return 5;
	if (percent >= 70) return 4;
	if (percent >= 60) return 3;
	if (percent >= 40) return 2;
	return 1;
}

function effectLabel(id: number, getEffect: (id: number) => { name?: string } | undefined): string {
	return getEffect(id)?.name?.trim() || `Effect ${id}`;
}

function slotName(slot: number): string {
	return equipmentSlots.find((entry) => entry.slot === slot)?.label ?? `Slot ${slot}`;
}

function formatValue(value: number): string {
	return Number.isInteger(value) ? value.toLocaleString() : value.toLocaleString(undefined, { maximumFractionDigits: 1 });
}

function clampInterval(value: number): number {
	if (!Number.isFinite(value)) return 1;
	return Math.max(1, Math.min(1440, Math.round(value)));
}

function readBoolean(key: string): boolean {
	try { return localStorage.getItem(key) === 'true'; } catch { return false; }
}

function readInterval(): number {
	try { return clampInterval(Number(localStorage.getItem(autoSellIntervalKey) ?? 1)); } catch { return 1; }
}
