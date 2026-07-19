import { useEffect, useMemo, useRef, useState } from 'react';
import { Activity, ArrowDown, ArrowUp, Info, Plus, RefreshCw, Search, X } from 'lucide-react';
import type { EquipmentOptimizeResponse, EquipmentPriorityV2, GameStateV2 } from '../../api/Contracts';
import { useCitadelAPI } from '../../api/ApiContext';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Input, MetricTile, Modal } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';
import type { CombatMode, EquipmentLeader, EquipmentTarget } from './EquipmentTypes';
import {
	cacheEquipmentPriorityProfile,
	equipmentPrioritySection,
	inferredEquipmentPriorityProfile,
	groupEquipmentPriorityEffects,
	normalizeEquipmentPriorityProfile,
	readCachedEquipmentPriorityProfile,
	readEquipmentPriorityProfile,
	storedEquipmentPriorityProfile,
	targetCandidateEffectIDs,
	type EquipmentPriorityGroup,
	type EquipmentPriorityProfile,
} from './EquipmentOptimizerState';

type Tier = 1 | 2;

export default function EquipmentOptimizer({
	isOpen,
	onClose,
	leader,
	combatMode,
	target,
	candidateEffectIDs,
	disabled,
}: {
	isOpen: boolean;
	onClose: () => void;
	leader: EquipmentLeader | null;
	combatMode: CombatMode;
	target: EquipmentTarget;
	candidateEffectIDs: number[];
	disabled: boolean;
}) {
	const { state, configuration, submitIntent, optimizeEquipment } = useCitadelAPI();
	const { effects, getEffect, getEquipment, getGem } = useMetadata();
	const [priorityProfile, setPriorityProfile] = useState<EquipmentPriorityProfile>({ tier1: [], tier2: [] });
	const [showPicker, setShowPicker] = useState(false);
	const [showInfo, setShowInfo] = useState(false);
	const [search, setSearch] = useState('');
	const [optimizing, setOptimizing] = useState(false);
	const [applying, setApplying] = useState(false);
	const [preview, setPreview] = useState<EquipmentOptimizeResponse | null>(null);
	const priorityRef = useRef<EquipmentPriorityProfile>({ tier1: [], tier2: [] });
	const optimisticProfiles = useRef<Record<string, EquipmentPriorityProfile>>({});
	const optimizeRequest = useRef(0);
	const candidateEffectKey = candidateEffectIDs.join(',');
	const candidateEffects = useMemo(() => candidateEffectKey === '' ? [] : candidateEffectKey.split(',').map(Number), [candidateEffectKey]);
	const prioritySection = useMemo(() => equipmentPrioritySection(state?.player.id, leader, target.id), [leader, state?.player.id, target.id]);
	const storedProfileJSON = useMemo(() => {
		try {
			return JSON.stringify(prioritySection ? configuration?.sections[prioritySection] ?? null : null);
		} catch {
			return 'null';
		}
	}, [configuration?.sections, prioritySection]);
	const targetEffects = useMemo(() => targetCandidateEffectIDs(candidateEffects, effects, target.castleTypeID), [candidateEffects, effects, target.castleTypeID]);
	const storedProfile = useMemo(() => {
		try {
			return readEquipmentPriorityProfile(JSON.parse(storedProfileJSON), targetEffects);
		} catch {
			return null;
		}
	}, [storedProfileJSON, targetEffects]);
	const cachedProfile = useMemo(() => readCachedEquipmentPriorityProfile(prioritySection, targetEffects), [prioritySection, targetEffects]);
	const inferredProfile = useMemo(() => inferredEquipmentPriorityProfile(candidateEffects, effects, leader?.kind, target.castleTypeID), [candidateEffects, effects, leader?.kind, target.castleTypeID]);

	useEffect(() => {
		optimizeRequest.current++;
		const initial = prioritySection && targetEffects.length > 0
			? optimisticProfiles.current[prioritySection] ?? cachedProfile ?? storedProfile ?? inferredProfile
			: { tier1: [], tier2: [] };
		const next = normalizeEquipmentPriorityProfile(initial, targetEffects);
		priorityRef.current = next;
		setPriorityProfile(next);
		setPreview(null);
	}, [cachedProfile, inferredProfile, prioritySection, storedProfile, targetEffects]);

	const tier1 = priorityProfile.tier1;
	const tier2 = priorityProfile.tier2;

	const used = useMemo(() => new Set([...tier1, ...tier2]), [tier1, tier2]);
	const priorityGroups = useMemo(() => groupEquipmentPriorityEffects(targetEffects, effects), [effects, targetEffects]);
	const availableGroups = useMemo(() => priorityGroups
		.map((group) => ({ ...group, effectIDs: group.effectIDs.filter((id) => !used.has(id)) }))
		.filter((group) => {
			if (group.effectIDs.length === 0) return false;
			const query = search.trim().toLowerCase();
			if (!query) return true;
			return group.label.toLowerCase().includes(query)
				|| group.effectIDs.some((id) => {
					const effect = getEffect(id);
					return String(effect?.name ?? '').toLowerCase().includes(query)
						|| String(effect?.internalName ?? '').toLowerCase().includes(query)
						|| String(id).includes(query);
				});
		})
		.sort((left, right) => left.label.localeCompare(right.label) || left.key.localeCompare(right.key)),
	[getEffect, priorityGroups, search, used]);

	const updatePriorities = (change: (current: EquipmentPriorityProfile) => EquipmentPriorityProfile) => {
		const next = normalizeEquipmentPriorityProfile(change(priorityRef.current), targetEffects);
		priorityRef.current = next;
		setPriorityProfile(next);
		optimizeRequest.current++;
		setPreview(null);
		if (prioritySection) {
			optimisticProfiles.current[prioritySection] = next;
			cacheEquipmentPriorityProfile(prioritySection, next);
			void submitIntent('config.update', {
				section: prioritySection,
				value: storedEquipmentPriorityProfile(next),
			}, { actor: 'ui:equipment-priority' }).catch(() => undefined);
		}
	};
	const addGroup = (group: EquipmentPriorityGroup, tier: Tier) => {
		updatePriorities((current) => tier === 1
			? { ...current, tier1: [...current.tier1, ...group.effectIDs] }
			: { ...current, tier2: [...current.tier2, ...group.effectIDs] });
		setSearch('');
	};
	const remove = (id: number) => {
		updatePriorities((current) => ({
			tier1: current.tier1.filter((value) => value !== id),
			tier2: current.tier2.filter((value) => value !== id),
		}));
	};
	const moveTier = (id: number, from: Tier) => {
		updatePriorities((current) => from === 1
			? { tier1: current.tier1.filter((value) => value !== id), tier2: [...current.tier2.filter((value) => value !== id), id] }
			: { tier1: [...current.tier1.filter((value) => value !== id), id], tier2: current.tier2.filter((value) => value !== id) });
	};
	const reorder = (tier: Tier, index: number, direction: -1 | 1) => {
		updatePriorities((current) => {
			const values = tier === 1 ? current.tier1 : current.tier2;
			const target = index + direction;
			if (target < 0 || target >= values.length) return current;
			const next = [...values];
			[next[index], next[target]] = [next[target], next[index]];
			return tier === 1 ? { ...current, tier1: next } : { ...current, tier2: next };
		});
	};

	const priorities = useMemo<EquipmentPriorityV2[]>(() => [
		...tier1.map((effectId, position) => ({ effectId, tier: 1 as const, position })),
		...tier2.map((effectId, position) => ({ effectId, tier: 2 as const, position })),
	], [tier1, tier2]);

	const optimize = async () => {
		if (!leader || tier1.length + tier2.length === 0) return;
		const requestID = ++optimizeRequest.current;
		setOptimizing(true);
		try {
			await submitIntent('equipment.refresh');
			try {
				const result = await optimizeEquipment({
					leaderKind: leader.kind,
					leaderId: leader.id,
					combatMode: combatMode === 'PvP' ? 'pvp' : 'pve',
					priorities,
				});
				if (requestID === optimizeRequest.current) setPreview(result);
			} catch (error) {
				Notifications.error(error instanceof Error ? error.message : 'Could not optimize this loadout');
			}
		} finally {
			setOptimizing(false);
		}
	};

	const apply = async () => {
		if (!preview) return;
		setApplying(true);
		try {
			await submitIntent('equipment.reconfigure', {
				leaderKind: preview.leaderKind,
				leaderId: preview.leaderId,
				equipment: preview.proposed.equipment,
				gems: preview.proposed.gems,
			}, { expectedRevision: preview.stateRevision });
			Notifications.success(`Reconfigured ${leader?.name ?? preview.leaderKind}`);
			setPreview(null);
		} finally {
			setApplying(false);
		}
	};

	return (
		<>
			<Modal
				isOpen={isOpen}
				onClose={onClose}
				title={(
					<span className="flex min-w-0 items-center gap-2">
						<Activity className="h-5 w-5 shrink-0 text-primary" />
						<span className="truncate">Stat Priority &amp; Reconfigure</span>
					</span>
				)}
				maxWidth="4xl"
				footer={(
					<>
						<Button variant="ghost" onClick={onClose}>Cancel</Button>
						<Button
							onClick={optimize}
							disabled={disabled || !leader || tier1.length + tier2.length === 0}
							isLoading={optimizing}
							leftIcon={<RefreshCw className="h-4 w-4" />}
						>
							Preview Reconfiguration
						</Button>
					</>
				)}
			>
				<div className="space-y-4">
					<div className="flex flex-wrap items-start justify-between gap-3 rounded-global border border-border-base bg-bg-app/45 p-3">
						<div className="min-w-0 flex-1">
							<div className="flex flex-wrap items-center gap-2">
								<span className="truncate text-sm font-semibold text-text-main">{leader?.name ?? 'No loadout selected'}</span>
								<Badge variant={combatMode === 'PvP' ? 'danger' : 'success'}>{target.label}</Badge>
							</div>
							<p className="mt-1 text-xs leading-relaxed text-text-muted">{target.description} Priorities are saved per leader and target.</p>
						</div>
						<div className="flex shrink-0 items-center gap-2">
							<Button size="icon" variant="ghost" onClick={() => setShowInfo(true)} aria-label="Explain stat priority"><Info className="h-4 w-4" /></Button>
							<Button
								size="sm"
								variant="outline"
								onClick={() => { setSearch(''); setShowPicker(true); }}
								disabled={availableGroups.length === 0}
								leftIcon={<Plus className="h-4 w-4" />}
							>
								Add Stat Group
							</Button>
						</div>
					</div>

					<div className="grid gap-4 md:grid-cols-2">
						<PriorityTier title="Maximize" tier={1} ids={tier1} getName={(id) => effectName(id, getEffect)} onRemove={remove} onMoveTier={moveTier} onReorder={reorder} />
						<PriorityTier title="Coverage" tier={2} ids={tier2} getName={(id) => effectName(id, getEffect)} onRemove={remove} onMoveTier={moveTier} onReorder={reorder} />
					</div>
				</div>
			</Modal>

			<Modal isOpen={showPicker} onClose={() => setShowPicker(false)} title="Add Stat Group" maxWidth="2xl">
				<div className="space-y-3">
					<Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search stat groups or effects" leftIcon={<Search className="h-4 w-4" />} />
					<div className="max-h-[60vh] space-y-2 overflow-y-auto custom-scrollbar">
						{availableGroups.map((group) => (
							<div key={group.key} className="flex items-center gap-2 rounded-global border border-border-base bg-bg-app/50 p-3">
								<span className="min-w-0 flex-1">
									<span className="block truncate text-sm font-medium text-text-main">{group.label}</span>
									<span className="block truncate text-[10px] text-text-muted" title={group.effectIDs.map((id) => effectName(id, getEffect)).join(' · ')}>{group.effectIDs.length} eligible stat{group.effectIDs.length === 1 ? '' : 's'} · {group.effectIDs.map((id) => effectName(id, getEffect)).join(' · ')}</span>
								</span>
								<Button size="sm" variant="danger" onClick={() => addGroup(group, 1)}>Maximize all</Button>
								<Button size="sm" variant="outline" onClick={() => addGroup(group, 2)}>Coverage all</Button>
							</div>
						))}
						{availableGroups.length === 0 && <p className="py-6 text-center text-sm text-text-muted">No matching unused stat groups.</p>}
					</div>
				</div>
			</Modal>

			<Modal isOpen={showInfo} onClose={() => setShowInfo(false)} title="How Stat Priority Works" maxWidth="2xl">
				<div className="space-y-3 text-sm text-text-muted">
					<p><span className="font-semibold text-error">Maximize</span> effects receive the strongest position-decayed score.</p>
					<p><span className="font-semibold text-primary">Coverage</span> effects receive a presence bonus, then a lower weighted score.</p>
					<p>The server searches storage plus the selected leader’s current pieces, respects official effect caps and set bonuses, and never borrows gear from another leader.</p>
				</div>
			</Modal>

			<OptimizerPreview
				preview={preview}
				state={state}
				priorityIDs={priorities.map((priority) => priority.effectId)}
				getEffectName={(id) => effectName(id, getEffect)}
				getEquipmentName={(id) => getEquipment(state?.inventory.equipment[String(id)]?.definitionId ?? 0)?.name ?? `Equipment ${id}`}
				getGemName={(id) => getGem(state?.inventory.gems[String(id)]?.definitionId ?? 0)?.name ?? `Gem ${id}`}
				onClose={() => setPreview(null)}
				onApply={apply}
				applying={applying}
			/>
		</>
	);
}

function PriorityTier({
	title,
	tier,
	ids,
	getName,
	onRemove,
	onMoveTier,
	onReorder,
}: {
	title: string;
	tier: Tier;
	ids: number[];
	getName: (id: number) => string;
	onRemove: (id: number) => void;
	onMoveTier: (id: number, tier: Tier) => void;
	onReorder: (tier: Tier, index: number, direction: -1 | 1) => void;
}) {
	return (
		<div className={`overflow-hidden rounded-global border ${tier === 1 ? 'border-error/30 bg-error/5' : 'border-primary/30 bg-primary/5'}`}>
			<div className="flex items-center justify-between border-b border-border-base/50 px-3 py-2">
				<span className={`text-xs font-bold uppercase tracking-wider ${tier === 1 ? 'text-error' : 'text-primary'}`}>{tier}. {title}</span>
				<Badge variant={tier === 1 ? 'danger' : 'primary'}>{ids.length}</Badge>
			</div>
			<div className="space-y-1.5 p-2">
				{ids.map((id, index) => (
					<div key={id} className="flex items-center gap-1 rounded-global border border-border-base/50 bg-bg-card px-2 py-2">
						<span className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full bg-bg-app text-[10px] font-bold text-text-muted">{index + 1}</span>
						<span className="min-w-0 flex-1 truncate text-xs text-text-main" title={getName(id)}>{getName(id)}</span>
						<button type="button" disabled={index === 0} onClick={() => onReorder(tier, index, -1)} className="p-1 text-text-muted hover:text-primary disabled:opacity-25"><ArrowUp className="h-3 w-3" /></button>
						<button type="button" disabled={index === ids.length - 1} onClick={() => onReorder(tier, index, 1)} className="p-1 text-text-muted hover:text-primary disabled:opacity-25"><ArrowDown className="h-3 w-3" /></button>
						<button type="button" onClick={() => onMoveTier(id, tier)} className="px-1 text-[9px] font-bold uppercase text-text-muted hover:text-primary">T{tier === 1 ? 2 : 1}</button>
						<button type="button" onClick={() => onRemove(id)} className="p-1 text-text-muted hover:text-error"><X className="h-3 w-3" /></button>
					</div>
				))}
				{ids.length === 0 && <p className="py-3 text-center text-xs text-text-muted">Add effects here</p>}
			</div>
		</div>
	);
}

function OptimizerPreview({
	preview,
	state,
	priorityIDs,
	getEffectName,
	getEquipmentName,
	getGemName,
	onClose,
	onApply,
	applying,
}: {
	preview: EquipmentOptimizeResponse | null;
	state: GameStateV2 | null;
	priorityIDs: number[];
	getEffectName: (id: number) => string;
	getEquipmentName: (id: number) => string;
	getGemName: (id: number) => string;
	onClose: () => void;
	onApply: () => void;
	applying: boolean;
}) {
	const effectRows = useMemo(() => {
		if (!preview) return [];
		const current = new Map(preview.current.effects.map((effect) => [effect.definitionId, effect]));
		const proposed = new Map(preview.proposed.effects.map((effect) => [effect.definitionId, effect]));
		const ids = Array.from(new Set([...current.keys(), ...proposed.keys()]));
		return ids.map((id) => ({ id, current: current.get(id)?.value ?? 0, proposed: proposed.get(id)?.value ?? 0, cap: proposed.get(id)?.cap ?? current.get(id)?.cap }))
			.filter((row) => row.current !== row.proposed)
			.sort((left, right) => Math.abs(right.proposed - right.current) - Math.abs(left.proposed - left.current));
	}, [preview]);
	const unavailablePriorities = useMemo(() => {
		if (!preview) return [];
		const proposed = new Map(preview.proposed.effects.map((effect) => [effect.definitionId, effect.value]));
		return priorityIDs.filter((id) => proposed.get(id) === undefined || proposed.get(id) === 0);
	}, [preview, priorityIDs]);
	void state;
	return (
		<Modal
			isOpen={preview != null}
			onClose={onClose}
			title="Reconfiguration Preview"
			maxWidth="5xl"
			footer={(
				<>
					<Button variant="ghost" onClick={onClose}>Cancel</Button>
					<Button onClick={onApply} isLoading={applying}>Apply Loadout</Button>
				</>
			)}
		>
			{preview && (
				<div className="space-y-5">
					{unavailablePriorities.length > 0 && (
						<p className="rounded-global border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">
							No eligible value was found for {unavailablePriorities.map(getEffectName).join(' · ')}. This is still the best available loadout.
						</p>
					)}
					<div className="grid gap-3 sm:grid-cols-3">
						<MetricTile className="p-3 [&_.ui-metric-value]:text-lg" label="Current score" value={formatNumber(preview.current.score)} />
						<MetricTile className="border-primary/30 bg-primary/5 p-3 [&_.ui-metric-value]:text-lg" label="Proposed score" value={formatNumber(preview.proposed.score)} tone="brand" />
						<MetricTile className="border-primary/30 bg-primary/5 p-3 [&_.ui-metric-value]:text-lg" label="Improvement" value={`+${formatNumber(preview.proposed.score - preview.current.score)}`} tone="brand" />
					</div>
					<div className="grid gap-4 lg:grid-cols-2">
						<LoadoutColumn title="Current" equipment={preview.current.equipment} gems={preview.current.gems} getEquipmentName={getEquipmentName} getGemName={getGemName} />
						<LoadoutColumn title="Proposed" equipment={preview.proposed.equipment} gems={preview.proposed.gems} getEquipmentName={getEquipmentName} getGemName={getGemName} />
					</div>
					<div className="overflow-hidden rounded-global border border-border-base">
						<div className="grid grid-cols-[minmax(0,1fr)_6rem_6rem] bg-bg-card-hover px-3 py-2 text-[10px] font-bold uppercase tracking-wider text-text-muted"><span>Effect</span><span className="text-right">Current</span><span className="text-right">Proposed</span></div>
						<div className="max-h-64 overflow-y-auto custom-scrollbar">
							{effectRows.map((row) => (
								<div key={row.id} className="grid grid-cols-[minmax(0,1fr)_6rem_6rem] border-t border-border-base/50 px-3 py-2 text-xs">
									<span className="truncate text-text-main">{getEffectName(row.id)}{row.cap ? ` (cap ${formatNumber(row.cap)})` : ''}</span>
									<span className="text-right font-mono text-text-muted">{formatNumber(row.current)}</span>
									<span className={`text-right font-mono font-semibold ${row.proposed >= row.current ? 'text-success' : 'text-warning'}`}>{formatNumber(row.proposed)}</span>
								</div>
							))}
						</div>
					</div>
					<p className="text-xs text-text-muted">Candidates: {Object.entries(preview.candidates.equipmentBySlot).map(([slot, count]) => `slot ${slot}: ${count}`).join(' · ')} · gems: {preview.candidates.gems}</p>
				</div>
			)}
		</Modal>
	);
}

function LoadoutColumn({
	title,
	equipment,
	gems,
	getEquipmentName,
	getGemName,
}: {
	title: string;
	equipment: Record<string, number>;
	gems: Record<string, number>;
	getEquipmentName: (id: number) => string;
	getGemName: (id: number) => string;
}) {
	return (
		<div className="rounded-global border border-border-base bg-bg-app/40 p-3">
			<h4 className="mb-2 text-xs font-bold uppercase tracking-wider text-text-muted">{title}</h4>
			<div className="space-y-2">
				{[1, 2, 3, 4, 6].map((slot) => {
					const equipmentID = equipment[String(slot)];
					const gemID = gems[String(slot)];
					return (
						<div key={slot} className="rounded-lg border border-border-base/60 bg-bg-card/50 px-3 py-2">
							<p className="truncate text-xs font-medium text-text-main">{slotLabel(slot)} · {equipmentID ? getEquipmentName(equipmentID) : 'Empty'}</p>
							{gemID ? <p className="mt-0.5 truncate text-[10px] text-purple-300">{getGemName(gemID)}</p> : null}
						</div>
					);
				})}
			</div>
		</div>
	);
}

function effectName(id: number, getEffect: (id: number) => { name?: string } | undefined): string {
	return getEffect(id)?.name?.trim() || `Effect ${id}`;
}

function slotLabel(slot: number): string {
	return ({ 1: 'Armor', 2: 'Weapon', 3: 'Helmet', 4: 'Artifact', 6: 'Hero' } as Record<number, string>)[slot] ?? `Slot ${slot}`;
}

function formatNumber(value: number): string {
	return Number.isInteger(value) ? value.toLocaleString() : value.toLocaleString(undefined, { maximumFractionDigits: 1 });
}
