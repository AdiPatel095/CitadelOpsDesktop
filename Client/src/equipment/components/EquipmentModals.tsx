import { useEffect, useMemo, useState } from 'react';
import { ArrowUpCircle, Gem, RefreshCw, Shield, Trash2, TriangleAlert } from 'lucide-react';
import { Badge, Button, Input, Modal, PillSelector, Switch } from '../../components/ui';
import type { EquipmentLeader, EquipmentSlotRow } from './EquipmentTypes';

export type SaleCategory =
	| 'non_relic_equipment'
	| 'relic1_equipment'
	| 'relic2_equipment'
	| 'non_relic_gems'
	| 'relic1_gems'
	| 'relic2_gems';

export interface SaleRequest {
	category: SaleCategory;
	sellLookItems?: boolean;
	sellPost2026?: boolean;
	keepStars?: number;
}

type RelicTab = 'Non Relic' | 'Relic 1.0' | 'Relic 2.0';

export function EquipmentSellModal({
	isOpen,
	itemType,
	onClose,
	onConfirm,
	busy,
}: {
	isOpen: boolean;
	itemType: 'Equipment' | 'Gems';
	onClose: () => void;
	onConfirm: (request: SaleRequest) => void;
	busy: boolean;
}) {
	const [relicTab, setRelicTab] = useState<RelicTab>('Non Relic');
	const [sellLookItems, setSellLookItems] = useState(false);
	const [sellPost2026, setSellPost2026] = useState(false);
	const [keepStars, setKeepStars] = useState(12);

	useEffect(() => {
		if (!isOpen) return;
		setRelicTab('Non Relic');
		setSellLookItems(false);
		setSellPost2026(false);
		setKeepStars(12);
	}, [isOpen]);

	const confirm = () => {
		const prefix = itemType === 'Equipment' ? 'equipment' : 'gems';
		const category = relicTab === 'Non Relic'
			? `non_relic_${prefix}`
			: relicTab === 'Relic 1.0'
				? `relic1_${prefix}`
				: `relic2_${prefix}`;
		onConfirm({
			category: category as SaleCategory,
			sellLookItems,
			sellPost2026,
			keepStars,
		});
	};

	return (
		<Modal
			isOpen={isOpen}
			onClose={onClose}
			title={<PillSelector ariaLabel="Equipment category" value={relicTab} onChange={(value) => setRelicTab(value as RelicTab)} options={['Non Relic', 'Relic 1.0', 'Relic 2.0']} size="header" fullWidth />}
			footer={(
				<>
					<Button variant="ghost" onClick={onClose}>Cancel</Button>
					<Button variant="danger" onClick={confirm} isLoading={busy}>Confirm Sell</Button>
				</>
			)}
		>
			<div className="space-y-5">
				<div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-error/10 text-error">
					<Trash2 className="h-7 w-7" />
				</div>
				<div className="rounded-global border border-error/30 bg-error/10 p-4 text-center">
					<p className="text-sm font-semibold text-text-main">
						{relicTab === 'Non Relic'
							? `Sell eligible non-relic ${itemType.toLowerCase()}`
							: relicTab === 'Relic 1.0'
								? `Sell all Relic 1.0 ${itemType.toLowerCase()}`
								: `Sell Relic 2.0 ${itemType.toLowerCase()} below ${keepStars} total stars`}
					</p>
					<p className="mt-2 text-xs text-error">This game action cannot be reversed.</p>
				</div>

				{relicTab === 'Non Relic' && (
					<div className="space-y-3">
						<label className="flex cursor-pointer items-center justify-between rounded-global border border-border-base bg-bg-app/50 p-3">
							<span>
								<span className="block text-sm font-medium text-text-main">Sell post-2026 definitions</span>
								<span className="block text-[11px] text-text-muted">Includes newly introduced catalog ranges.</span>
							</span>
							<Switch checked={sellPost2026} onChange={setSellPost2026} />
						</label>
						{itemType === 'Equipment' && (
							<label className="flex cursor-pointer items-center justify-between rounded-global border border-border-base bg-bg-app/50 p-3">
								<span className="text-sm font-medium text-text-main">Sell look items</span>
								<Switch checked={sellLookItems} onChange={setSellLookItems} />
							</label>
						)}
					</div>
				)}

				{relicTab === 'Relic 2.0' && (
					<div className="rounded-global border border-border-base bg-bg-app/50 p-4">
						<div className="mb-3 flex items-center justify-between">
							<span className="text-sm font-medium text-text-main">Keep total stars and above</span>
							<Badge variant="warning">{keepStars} stars</Badge>
						</div>
						<input
							type="range"
							min={4}
							max={42}
							value={keepStars}
							onChange={(event) => setKeepStars(Number(event.target.value))}
							className="w-full accent-primary"
						/>
						<div className="mt-1 flex justify-between text-[10px] text-text-muted"><span>4</span><span>42</span></div>
					</div>
				)}

				<div className="flex items-start gap-2 text-xs text-text-muted">
					<TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
					The server refreshes storage first, freezes the exact selection, and verifies every game response before reporting success.
				</div>
			</div>
		</Modal>
	);
}

export function EquipmentSwapModal({
	isOpen,
	leader,
	leaders,
	onClose,
	onConfirm,
	busy,
}: {
	isOpen: boolean;
	leader: EquipmentLeader | null;
	leaders: EquipmentLeader[];
	onClose: () => void;
	onConfirm: (otherLeaderID: number) => void;
	busy: boolean;
}) {
	const [otherID, setOtherID] = useState<number | null>(null);
	useEffect(() => {
		if (isOpen) setOtherID(null);
	}, [isOpen]);
	const available = leaders.filter((candidate) => candidate.id !== leader?.id);
	return (
		<Modal
			isOpen={isOpen}
			onClose={onClose}
			title="Swap Base Equipment"
			maxWidth="2xl"
			footer={(
				<>
					<Button variant="ghost" onClick={onClose}>Cancel</Button>
					<Button disabled={otherID == null} onClick={() => otherID != null && onConfirm(otherID)} isLoading={busy}>Swap Pieces</Button>
				</>
			)}
		>
			<div className="space-y-4">
				<div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-primary/10 text-primary"><RefreshCw className="h-7 w-7" /></div>
				<p className="text-center text-sm text-text-muted">
					Move base equipment and heroes between <span className="font-semibold text-primary">{leader?.name}</span> and another {leader?.kind}. Socketed gems remain on their equipment.
				</p>
				<div className="max-h-[50vh] space-y-2 overflow-y-auto custom-scrollbar">
					{available.map((candidate) => {
						const equipped = Object.values(candidate.equipment).filter(Boolean).length;
						return (
							<button
								type="button"
								key={candidate.id}
								onClick={() => setOtherID(candidate.id)}
								className={`flex w-full items-center gap-3 rounded-global border p-3 text-left ${otherID === candidate.id ? 'border-primary/50 bg-primary/10' : 'border-border-base bg-bg-app/50 hover:bg-bg-card-hover'}`}
							>
								<span className="flex h-7 w-7 items-center justify-center rounded-full bg-bg-card text-xs font-bold text-text-muted">{candidate.position}</span>
								<span className="min-w-0 flex-1 truncate text-sm font-medium text-text-main">{candidate.name}</span>
								<Badge variant={candidate.available ? 'success' : 'warning'}>{candidate.available ? `${equipped}/5` : 'Busy'}</Badge>
							</button>
						);
					})}
				</div>
			</div>
		</Modal>
	);
}

export function UnequipModal({
	isOpen,
	kind,
	leader,
	rows,
	onClose,
	onConfirm,
	busy,
}: {
	isOpen: boolean;
	kind: 'equipment' | 'gems';
	leader: EquipmentLeader | null;
	rows: EquipmentSlotRow[];
	onClose: () => void;
	onConfirm: (slots: number[]) => void;
	busy: boolean;
}) {
	const [selected, setSelected] = useState<Set<number>>(new Set());
	useEffect(() => {
		if (isOpen) setSelected(new Set());
	}, [isOpen]);
	const available = rows.filter((row) => kind === 'equipment' ? row.item != null : row.gem != null);
	const toggle = (slot: number) => setSelected((current) => {
		const next = new Set(current);
		if (next.has(slot)) next.delete(slot);
		else next.add(slot);
		return next;
	});
	return (
		<Modal
			isOpen={isOpen}
			onClose={onClose}
			title={`Unequip ${kind === 'equipment' ? 'Equipment' : 'Gems'}`}
			footer={(
				<>
					<Button variant="ghost" onClick={onClose}>Cancel</Button>
					<Button disabled={selected.size === 0} onClick={() => onConfirm(Array.from(selected))} isLoading={busy}>Unequip {selected.size ? `(${selected.size})` : ''}</Button>
				</>
			)}
		>
			<div className="space-y-4">
				<div className={`mx-auto flex h-14 w-14 items-center justify-center rounded-full ${kind === 'equipment' ? 'bg-primary/10 text-primary' : 'bg-purple-500/10 text-purple-300'}`}>
					{kind === 'equipment' ? <Shield className="h-7 w-7" /> : <Gem className="h-7 w-7" />}
				</div>
				<p className="text-center text-sm text-text-muted">Select {kind} to remove from <span className="font-semibold text-text-main">{leader?.name}</span>.</p>
				<div className="space-y-2">
					{available.map((row) => {
						const id = kind === 'equipment' ? row.item?.id : row.gem?.id;
						return (
							<button
								type="button"
								key={row.slot}
								onClick={() => toggle(row.slot)}
								className={`flex w-full items-center gap-3 rounded-global border p-3 text-left ${selected.has(row.slot) ? 'border-primary/50 bg-primary/10' : 'border-border-base bg-bg-app/50 hover:bg-bg-card-hover'}`}
							>
								<span className={`h-5 w-5 rounded border-2 ${selected.has(row.slot) ? 'border-primary bg-primary' : 'border-border-base'}`} />
								<span className="flex-1 text-sm font-medium text-text-main">{row.label}</span>
								<span className="font-mono text-xs text-text-muted">ID {id}</span>
							</button>
						);
					})}
					{available.length === 0 && <p className="py-5 text-center text-sm text-text-muted">No {kind} are equipped.</p>}
				</div>
			</div>
		</Modal>
	);
}

export function UpgradeModal({
	isOpen,
	kind,
	leader,
	rows,
	coinBlocked,
	onClose,
	onConfirm,
	busy,
}: {
	isOpen: boolean;
	kind: 'equipment' | 'gem';
	leader: EquipmentLeader | null;
	rows: EquipmentSlotRow[];
	coinBlocked: boolean;
	onClose: () => void;
	onConfirm: (itemID: number, targetLevel: number) => void;
	busy: boolean;
}) {
	const candidates = useMemo(() => rows
		.map((row) => ({ row, item: kind === 'equipment' ? row.item : row.gem }))
		.filter((entry): entry is { row: EquipmentSlotRow; item: NonNullable<EquipmentSlotRow['item'] | EquipmentSlotRow['gem']> } => entry.item != null), [kind, rows]);
	const [selectedID, setSelectedID] = useState<number | null>(null);
	const [targetLevel, setTargetLevel] = useState(1);
	useEffect(() => {
		if (!isOpen) return;
		setSelectedID(null);
		setTargetLevel(1);
	}, [isOpen, kind]);
	const selected = candidates.find((candidate) => candidate.item.id === selectedID);
	const currentLevel = selected?.item.level ?? 0;
	useEffect(() => {
		if (selected) setTargetLevel(Math.min(50, currentLevel + 1));
	}, [currentLevel, selectedID]);
	return (
		<Modal
			isOpen={isOpen}
			onClose={onClose}
			title={`Upgrade ${kind === 'equipment' ? 'Equipment' : 'Gem'}`}
			footer={(
				<>
					<Button variant="ghost" onClick={onClose}>Cancel</Button>
					<Button
						disabled={selectedID == null || targetLevel <= currentLevel || targetLevel > 50 || coinBlocked}
						onClick={() => selectedID != null && onConfirm(selectedID, targetLevel)}
						isLoading={busy}
					>
						{coinBlocked ? 'Coins under threshold' : `Upgrade to ${targetLevel}`}
					</Button>
				</>
			)}
		>
			<div className="space-y-4">
				<div className={`mx-auto flex h-14 w-14 items-center justify-center rounded-full ${kind === 'equipment' ? 'bg-primary/10 text-primary' : 'bg-purple-500/10 text-purple-300'}`}>
					<ArrowUpCircle className="h-7 w-7" />
				</div>
				<p className="text-center text-sm text-text-muted">Choose one {kind} on <span className="font-semibold text-text-main">{leader?.name}</span>.</p>
				<div className="max-h-[42vh] space-y-2 overflow-y-auto custom-scrollbar">
					{candidates.map(({ row, item }) => (
						<button
							type="button"
							key={`${row.slot}-${item.id}`}
							disabled={(item.level ?? 0) >= 50 || item.id <= 0}
							onClick={() => setSelectedID(item.id)}
							className={`flex w-full items-center gap-3 rounded-global border p-3 text-left disabled:opacity-50 ${selectedID === item.id ? 'border-primary/50 bg-primary/10' : 'border-border-base bg-bg-app/50 hover:bg-bg-card-hover'}`}
						>
							<span className="flex-1 text-sm font-medium text-text-main">{row.label}</span>
							<Badge variant={(item.level ?? 0) >= 50 ? 'success' : 'secondary'}>Level {item.level ?? 0}</Badge>
							<span className="font-mono text-[10px] text-text-muted">{item.id}</span>
						</button>
					))}
				</div>
				{selected && (
					<div className="rounded-global border border-border-base bg-bg-app/50 p-4">
						<label className="mb-2 block text-sm font-medium text-text-main">Target level ({currentLevel + 1}–50)</label>
						<Input
							type="number"
							min={currentLevel + 1}
							max={50}
							value={targetLevel}
							onChange={(event) => setTargetLevel(Math.max(currentLevel + 1, Math.min(50, Number(event.target.value))))}
						/>
					</div>
				)}
				{coinBlocked && <p className="text-center text-xs text-warning">The configured coin reserve currently blocks upgrades.</p>}
			</div>
		</Modal>
	);
}
