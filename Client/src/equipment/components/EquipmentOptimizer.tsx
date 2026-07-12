import { useEffect, useMemo, useState } from 'react';
import { Activity, ArrowDown, ArrowUp, Info, Plus, RefreshCw, Search, X } from 'lucide-react';
import type { EquipmentOptimizeResponse, EquipmentPriorityV2, GameStateV2 } from '../../api/Contracts';
import { useCitadelAPI } from '../../api/ApiContext';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Card, CardContent, CardHeader, Input, Modal } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';
import type { CombatMode, EquipmentLeader } from './EquipmentTypes';

type Tier = 1 | 2;

export default function EquipmentOptimizer({
	leader,
	combatMode,
	candidateEffectIDs,
	disabled,
}: {
	leader: EquipmentLeader | null;
	combatMode: CombatMode;
	candidateEffectIDs: number[];
	disabled: boolean;
}) {
	const { state, submitIntent, optimizeEquipment } = useCitadelAPI();
	const { effects, getEffect, getEquipment, getGem } = useMetadata();
	const [tier1, setTier1] = useState<number[]>([]);
	const [tier2, setTier2] = useState<number[]>([]);
	const [showPicker, setShowPicker] = useState(false);
	const [showInfo, setShowInfo] = useState(false);
	const [search, setSearch] = useState('');
	const [optimizing, setOptimizing] = useState(false);
	const [applying, setApplying] = useState(false);
	const [preview, setPreview] = useState<EquipmentOptimizeResponse | null>(null);

	useEffect(() => {
		setTier1([]);
		const matcher = leader?.kind === 'castellan'
			? /(defensive|defense).*(melee|range)|(melee|range).*defen/i
			: /(offensive|attack).*(melee|range)|(melee|range).*offen/i;
		const defaults = candidateEffectIDs
			.filter((id) => matcher.test(String(effects[id]?.internalName ?? effects[id]?.name ?? '')))
			.slice(0, 2);
		setTier2(defaults);
		setPreview(null);
	}, [candidateEffectIDs, effects, leader?.kind]);

	const used = useMemo(() => new Set([...tier1, ...tier2]), [tier1, tier2]);
	const unused = useMemo(() => candidateEffectIDs.filter((id) => !used.has(id)), [candidateEffectIDs, used]);
	const available = useMemo(() => unused
		.filter((id) => {
			const query = search.trim().toLowerCase();
			if (!query) return true;
			const effect = getEffect(id);
			return String(effect?.name ?? '').toLowerCase().includes(query)
				|| String(effect?.internalName ?? '').toLowerCase().includes(query)
				|| String(id).includes(query);
		})
		.sort((left, right) => effectName(left, getEffect).localeCompare(effectName(right, getEffect)) || left - right),
	[getEffect, search, unused]);

	const add = (id: number, tier: Tier) => {
		if (tier === 1) setTier1((current) => [...current, id]);
		else setTier2((current) => [...current, id]);
		setSearch('');
	};
	const remove = (id: number) => {
		setTier1((current) => current.filter((value) => value !== id));
		setTier2((current) => current.filter((value) => value !== id));
	};
	const moveTier = (id: number, from: Tier) => {
		remove(id);
		if (from === 1) setTier2((current) => [...current, id]);
		else setTier1((current) => [...current, id]);
	};
	const reorder = (tier: Tier, index: number, direction: -1 | 1) => {
		const setter = tier === 1 ? setTier1 : setTier2;
		setter((current) => {
			const target = index + direction;
			if (target < 0 || target >= current.length) return current;
			const next = [...current];
			[next[index], next[target]] = [next[target], next[index]];
			return next;
		});
	};

	const priorities = (): EquipmentPriorityV2[] => [
		...tier1.map((effectId, position) => ({ effectId, tier: 1 as const, position })),
		...tier2.map((effectId, position) => ({ effectId, tier: 2 as const, position })),
	];

	const optimize = async () => {
		if (!leader || tier1.length + tier2.length === 0) return;
		setOptimizing(true);
		try {
			await submitIntent('equipment.refresh');
			try {
				const result = await optimizeEquipment({
					leaderKind: leader.kind,
					leaderId: leader.id,
					combatMode: combatMode === 'PvP' ? 'pvp' : 'pve',
					priorities: priorities(),
				});
				setPreview(result);
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
			});
			Notifications.success(`Reconfigured ${leader?.name ?? preview.leaderKind}`);
			setPreview(null);
		} finally {
			setApplying(false);
		}
	};

	return (
		<Card className="liquid-prominent-header-card h-full min-h-0 flex flex-col relative">
			<CardHeader className="liquid-card-header-prominent flex-row items-center justify-between gap-2">
				<div>
					<h3 className="flex items-center gap-2 text-base font-semibold text-text-main">
						<Activity className="h-4 w-4 text-primary" /> Stat Priority
						<button type="button" onClick={() => setShowInfo(true)} className="text-text-muted hover:text-primary" aria-label="Explain optimizer"><Info className="h-3.5 w-3.5" /></button>
					</h3>
					<p className="mt-0.5 text-xs text-text-muted">Official effects, deterministic beam search</p>
				</div>
				<Button size="icon" variant="outline" onClick={() => { setSearch(''); setShowPicker(true); }} disabled={unused.length === 0} aria-label="Add effect"><Plus className="h-4 w-4" /></Button>
			</CardHeader>

			<CardContent className="flex-1 space-y-3 overflow-y-auto p-3 custom-scrollbar">
				<PriorityTier title="Maximize" tier={1} ids={tier1} getName={(id) => effectName(id, getEffect)} onRemove={remove} onMoveTier={moveTier} onReorder={reorder} />
				<PriorityTier title="Coverage" tier={2} ids={tier2} getName={(id) => effectName(id, getEffect)} onRemove={remove} onMoveTier={moveTier} onReorder={reorder} />
			</CardContent>

			<div className="border-t border-border-base bg-bg-card-hover/40 p-3">
				<Button
					className="w-full"
					onClick={optimize}
					disabled={disabled || !leader || tier1.length + tier2.length === 0}
					isLoading={optimizing}
					leftIcon={<RefreshCw className="h-4 w-4" />}
				>
					Preview Reconfiguration
				</Button>
			</div>

			<Modal isOpen={showPicker} onClose={() => setShowPicker(false)} title="Add Official Effect" maxWidth="2xl">
				<div className="space-y-3">
					<Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search effects" leftIcon={<Search className="h-4 w-4" />} />
					<div className="max-h-[60vh] space-y-2 overflow-y-auto custom-scrollbar">
						{available.map((id) => (
							<div key={id} className="flex items-center gap-2 rounded-global border border-border-base bg-bg-app/50 p-3">
								<span className="min-w-0 flex-1">
									<span className="block truncate text-sm font-medium text-text-main">{effectName(id, getEffect)}</span>
									<span className="block font-mono text-[10px] text-text-muted">Effect {id}</span>
								</span>
								<Button size="sm" variant="danger" onClick={() => add(id, 1)}>Maximize</Button>
								<Button size="sm" variant="outline" onClick={() => add(id, 2)}>Coverage</Button>
							</div>
						))}
						{available.length === 0 && <p className="py-6 text-center text-sm text-text-muted">No matching unused effects.</p>}
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
				getEffectName={(id) => effectName(id, getEffect)}
				getEquipmentName={(id) => getEquipment(state?.inventory.equipment[String(id)]?.definitionId ?? 0)?.name ?? `Equipment ${id}`}
				getGemName={(id) => getGem(state?.inventory.gems[String(id)]?.definitionId ?? 0)?.name ?? `Gem ${id}`}
				onClose={() => setPreview(null)}
				onApply={apply}
				applying={applying}
			/>
		</Card>
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
	getEffectName,
	getEquipmentName,
	getGemName,
	onClose,
	onApply,
	applying,
}: {
	preview: EquipmentOptimizeResponse | null;
	state: GameStateV2 | null;
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
					<div className="grid gap-3 sm:grid-cols-3">
						<Metric label="Current score" value={formatNumber(preview.current.score)} />
						<Metric label="Proposed score" value={formatNumber(preview.proposed.score)} emphasis />
						<Metric label="Improvement" value={`+${formatNumber(preview.proposed.score - preview.current.score)}`} emphasis />
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

function Metric({ label, value, emphasis = false }: { label: string; value: string; emphasis?: boolean }) {
	return <div className={`rounded-global border p-3 ${emphasis ? 'border-primary/30 bg-primary/5' : 'border-border-base bg-bg-app/40'}`}><p className="text-[10px] font-bold uppercase tracking-wider text-text-muted">{label}</p><p className={`mt-1 font-mono text-lg font-bold ${emphasis ? 'text-primary' : 'text-text-main'}`}>{value}</p></div>;
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
