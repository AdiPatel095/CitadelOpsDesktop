import { LocalizedRichText } from '../../i18n/LocalizedRichText';
import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
import { useEffect, useMemo, useState } from 'react';
import { ArrowUpCircle, Gem, RefreshCw, Shield, Sparkles, Trash2, TriangleAlert } from 'lucide-react';
import { Badge, Button, Input, Modal, PillSelector, Switch } from '../../components/ui';
import {
	equipmentEventTierOrder,
	type EquipmentEventAvailability,
	type EquipmentEventKey,
	type EquipmentEventTier,
} from '../EquipmentEventLoadouts';
import { officialUpgradeLevelCap } from '../EquipmentUpgradeCaps';
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
  const { t: localizeStatic } = useStaticLocale();
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
			title={<PillSelector ariaLabel={localizeStatic("ui.equipment.components.equipmentModals.ariaLabel.equipment.category.d378a1be")} value={relicTab} onChange={(value) => setRelicTab(value as RelicTab)} options={['Non Relic', 'Relic 1.0', 'Relic 2.0']} size="header" fullWidth />}
			footer={(
				<>
					<Button variant="ghost" onClick={onClose}><LocalizedText messageKey="game.cancel" /></Button>
					<Button variant="danger" onClick={confirm} isLoading={busy}><LocalizedText messageKey="ui.equipment.components.equipmentModals.confirm.sell.827ff403" /></Button>
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
					<p className="mt-2 text-xs text-error"><LocalizedText messageKey="ui.equipment.components.equipmentModals.this.game.action.cannot.be.reversed.43e31498" /></p>
				</div>

				{relicTab === 'Non Relic' && (
					<div className="space-y-3">
						<label className="flex cursor-pointer items-center justify-between rounded-global border border-border-base bg-bg-app/50 p-3">
							<span>
								<span className="block text-sm font-medium text-text-main"><LocalizedText messageKey="ui.equipment.components.equipmentModals.sell.post.2026.definitions.81a124f5" /></span>
								<span className="block text-[11px] text-text-muted"><LocalizedText messageKey="ui.equipment.components.equipmentModals.includes.newly.introduced.catalog.ranges.835d450c" /></span>
							</span>
							<Switch checked={sellPost2026} onChange={setSellPost2026} ariaLabel={localizeStatic("ui.equipment.components.equipmentModals.ariaLabel.sell.post.2026.definitions.81a124f5")} />
						</label>
						{itemType === 'Equipment' && (
							<label className="flex cursor-pointer items-center justify-between rounded-global border border-border-base bg-bg-app/50 p-3">
								<span className="text-sm font-medium text-text-main"><LocalizedText messageKey="ui.equipment.components.equipmentModals.sell.look.items.3837c1a3" /></span>
								<Switch checked={sellLookItems} onChange={setSellLookItems} ariaLabel={localizeStatic("ui.equipment.components.equipmentModals.ariaLabel.sell.look.items.3837c1a3")} />
							</label>
						)}
					</div>
				)}

				{relicTab === 'Relic 2.0' && (
					<div className="rounded-global border border-border-base bg-bg-app/50 p-4">
						<div className="mb-3 flex items-center justify-between">
							<span className="text-sm font-medium text-text-main"><LocalizedText messageKey="ui.equipment.components.equipmentModals.keep.total.stars.and.above.21f1668e" /></span>
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
  const { t: localizeStatic } = useStaticLocale();
	const [otherID, setOtherID] = useState<number | null>(null);
	useEffect(() => {
		if (isOpen) setOtherID(null);
	}, [isOpen]);
	const available = leaders.filter((candidate) => candidate.id !== leader?.id);
	return (
		<Modal
			isOpen={isOpen}
			onClose={onClose}
			title={localizeStatic("ui.equipment.components.equipmentModals.title.swap.base.equipment.57096c41")}
			maxWidth="2xl"
			footer={(
				<>
					<Button variant="ghost" onClick={onClose}><LocalizedText messageKey="game.cancel" /></Button>
					<Button disabled={otherID == null} onClick={() => otherID != null && onConfirm(otherID)} isLoading={busy}><LocalizedText messageKey="ui.equipment.components.equipmentModals.swap.pieces.86121fb1" /></Button>
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

export function EquipmentEventModal({
	isOpen,
	leader,
	availability,
	onClose,
	onConfirm,
	busy,
}: {
	isOpen: boolean;
	leader: EquipmentLeader | null;
	availability: EquipmentEventAvailability[];
	onClose: () => void;
	onConfirm: (event: EquipmentEventKey, tier?: EquipmentEventTier) => void;
	busy: boolean;
}) {
  const { t: localizeStatic } = useStaticLocale();
	const [selectedEvent, setSelectedEvent] = useState<EquipmentEventKey | null>(null);
	const [selectedTier, setSelectedTier] = useState<EquipmentEventTier | null>(null);
	useEffect(() => {
		if (!isOpen) return;
		setSelectedEvent(null);
		setSelectedTier(null);
	}, [isOpen, leader?.id]);
	const selected = availability.find((entry) => entry.option.value === selectedEvent);
	const tierRequired = (selected?.sets.length ?? 0) > 1;
	const selectedSet = tierRequired
		? selected?.sets.find((candidate) => candidate.tier === selectedTier)
		: selected;
	const canApply = selectedSet != null && selectedSet.equipmentCount > 0 && leader?.available === true;
	const selectEvent = (entry: EquipmentEventAvailability) => {
		setSelectedEvent(entry.option.value);
		setSelectedTier(entry.sets.length > 1 ? entry.tier ?? null : null);
	};

	return (
		<Modal
			isOpen={isOpen}
			onClose={onClose}
			title={localizeStatic("ui.equipment.components.equipmentModals.title.equip.event.set.16c25631")}
			maxWidth="2xl"
			footer={(
				<>
					<Button variant="ghost" onClick={onClose}><LocalizedText messageKey="game.cancel" /></Button>
					<Button
						disabled={!canApply}
						onClick={() => selectedEvent && onConfirm(selectedEvent, selectedTier ?? undefined)}
						isLoading={busy}
					>
						{localizeStatic('equipment.event.apply',{tier:selectedTier ?? 'none'})}
					</Button>
				</>
			)}
		>
			<div className="space-y-4">
				<div className="mx-auto flex h-14 w-14 items-center justify-center rounded-full bg-primary/10 text-primary">
					<Sparkles className="h-7 w-7" />
				</div>
				<div className="text-center">
					<p className="text-sm text-text-muted">
						<LocalizedRichText messageKey="equipment.event.chooseForLeader" params={{name:leader?.name ?? ''}} tags={{leader:children=><span className="font-semibold text-text-main">{children}</span>}} />
					</p>
					<p className="mt-1 text-xs text-text-muted">
						<LocalizedText messageKey="ui.equipment.components.equipmentModals.only.storage.and.pieces.already.on.this.9ae35d7d" /></p>
				</div>

				<div className="flex items-start gap-2 rounded-global border border-warning/30 bg-warning/10 p-3 text-xs text-text-muted">
					<TriangleAlert className="mt-0.5 h-4 w-4 shrink-0 text-warning" />
					<span>
						<LocalizedText messageKey="ui.equipment.components.equipmentModals.applying.removes.all.five.base.equipment.slots.d95f2a94" /></span>
				</div>

				<div className="space-y-2">
					{availability.map((entry) => {
						const isSelected = selectedEvent === entry.option.value;
						const displayedSet = isSelected && selectedSet ? selectedSet : entry;
						const hasTierChoice = entry.sets.length > 1;
						return (
							<div
								key={entry.option.value}
								className={`overflow-hidden rounded-global border transition-colors ${isSelected ? 'border-primary/50 bg-primary/10' : 'border-border-base bg-bg-app/50 hover:bg-bg-card-hover'}`}
							>
								<button type="button" onClick={() => selectEvent(entry)} className="w-full p-3 text-left">
									<span className="flex items-start gap-3">
										<span className={`mt-0.5 h-4 w-4 shrink-0 rounded-full border-2 ${isSelected ? 'border-primary bg-primary shadow-[inset_0_0_0_3px_var(--bg-app)]' : 'border-border-base'}`} />
										<span className="min-w-0 flex-1">
											<span className="flex flex-wrap items-center gap-2">
												<span className="text-sm font-semibold text-text-main">{localizeStatic(entry.option.labelKey)}</span>
												{hasTierChoice && <Badge variant="secondary">{isSelected && selectedTier ? localizeStatic('equipment.event.tier',{tier:selectedTier}) : localizeStatic('equipment.event.tiers',{count:entry.sets.length})}</Badge>}
												{displayedSet.complete && <Badge variant="success"><LocalizedText messageKey="ui.equipment.components.equipmentModals.complete.143b270a" /></Badge>}
											</span>
											<span className="mt-1 block text-xs text-text-muted">{localizeStatic(entry.option.descriptionKey)}</span>
											<span className="mt-2 flex flex-wrap gap-2">
												<Badge variant={displayedSet.equipmentCount === 5 ? 'success' : displayedSet.equipmentCount > 0 ? 'warning' : 'outline'}>
													{localizeStatic('equipment.event.equipmentCount',{count:displayedSet.equipmentCount,maximum:5})}
												</Badge>
												<Badge variant={displayedSet.gemCount === 4 ? 'success' : displayedSet.gemCount > 0 ? 'warning' : 'outline'}>
													{localizeStatic('equipment.event.gemCount',{count:displayedSet.gemCount,maximum:4})}
												</Badge>
											</span>
										</span>
									</span>
								</button>
								{isSelected && hasTierChoice && (
									<div className="border-t border-primary/20 px-3 pb-3 pt-2.5">
										<div className="mb-2 flex items-center justify-between gap-3">
											<span className="text-[11px] font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.equipment.components.equipmentModals.set.tier.9c00987e" /></span>
											<span className="text-[11px] text-text-muted"><LocalizedText messageKey="ui.equipment.components.equipmentModals.equip.only.this.tier.593e5e5f" /></span>
										</div>
										<PillSelector
											ariaLabel={localizeStatic('equipment.event.tierLabel',{event:localizeStatic(entry.option.labelKey)})}
											value={selectedTier ?? ''}
											onChange={(value) => setSelectedTier(value as EquipmentEventTier)}
											options={equipmentEventTierOrder.map(tier=>({value:tier,label:localizeStatic('equipment.event.tier',{tier})}))}
											size="body"
											fullWidth
										/>
									</div>
								)}
							</div>
						);
					})}
				</div>

				{selected && selectedSet && selectedSet.equipmentCount === 0 && (
					<p className="text-center text-xs text-warning">{localizeStatic('equipment.event.unavailable',{event:localizeStatic(selected.option.labelKey),tier:selectedTier ?? 'none'})}</p>
				)}
				{leader && !leader.available && (
					<p className="text-center text-xs text-warning"><LocalizedText messageKey="ui.equipment.components.equipmentModals.this.commander.is.busy.and.cannot.be.41116e12" /></p>
				)}
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
					<Button variant="ghost" onClick={onClose}><LocalizedText messageKey="game.cancel" /></Button>
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
	const candidates = useMemo(() => rows.flatMap((row) => {
		const item = kind === 'equipment' ? row.item : row.gem;
		if (item == null) return [];
		return [{
			row,
			item,
			levelCap: officialUpgradeLevelCap(kind, kind === 'equipment' ? {
				rarityID: row.item?.rarityId,
				relic: row.item?.relic,
				relicKnown: row.item?.relicKnown,
				slot: row.item?.slot,
			} : {}),
		}];
	}), [kind, rows]);
	const [selectedID, setSelectedID] = useState<number | null>(null);
	const [targetLevel, setTargetLevel] = useState(1);
	useEffect(() => {
		if (!isOpen) return;
		setSelectedID(null);
		setTargetLevel(1);
	}, [isOpen, kind]);
	const selected = candidates.find((candidate) => candidate.item.id === selectedID);
	const currentLevel = selected?.item.level ?? 0;
	const selectedLevelCap = selected?.levelCap ?? null;
	useEffect(() => {
		if (selectedID != null && selectedLevelCap != null) {
			setTargetLevel(Math.min(selectedLevelCap, currentLevel + 1));
		}
	}, [currentLevel, selectedID, selectedLevelCap]);
	return (
		<Modal
			isOpen={isOpen}
			onClose={onClose}
			title={`Upgrade ${kind === 'equipment' ? 'Equipment' : 'Gem'}`}
			footer={(
				<>
					<Button variant="ghost" onClick={onClose}><LocalizedText messageKey="game.cancel" /></Button>
					<Button
						disabled={selectedID == null || selectedLevelCap == null || currentLevel >= selectedLevelCap || targetLevel <= currentLevel || targetLevel > selectedLevelCap || coinBlocked}
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
					{candidates.map(({ row, item, levelCap }) => {
						const level = item.level ?? 0;
						const capped = levelCap != null && level >= levelCap;
						return (
							<button
								type="button"
								key={`${row.slot}-${item.id}`}
								disabled={levelCap == null || capped || item.id <= 0}
								onClick={() => setSelectedID(item.id)}
								className={`flex w-full items-center gap-3 rounded-global border p-3 text-left disabled:opacity-50 ${selectedID === item.id ? 'border-primary/50 bg-primary/10' : 'border-border-base bg-bg-app/50 hover:bg-bg-card-hover'}`}
							>
								<span className="flex-1 text-sm font-medium text-text-main">{row.label}</span>
								<Badge variant={levelCap == null ? 'warning' : capped ? 'success' : 'secondary'}>
									{levelCap == null ? 'Unknown rarity' : `Level ${level} / ${levelCap}`}
								</Badge>
								<span className="font-mono text-[10px] text-text-muted">{item.id}</span>
							</button>
						);
					})}
				</div>
				{selected && selectedLevelCap != null && currentLevel < selectedLevelCap && (
					<div className="rounded-global border border-border-base bg-bg-app/50 p-4">
						<label className="mb-2 block text-sm font-medium text-text-main">Target level ({currentLevel + 1}–{selectedLevelCap})</label>
						<Input
							type="number"
							min={currentLevel + 1}
							max={selectedLevelCap}
							value={targetLevel}
							onChange={(event) => setTargetLevel(Math.max(currentLevel + 1, Math.min(selectedLevelCap, Number(event.target.value))))}
						/>
					</div>
				)}
				{coinBlocked && <p className="text-center text-xs text-warning"><LocalizedText messageKey="ui.equipment.components.equipmentModals.the.configured.coin.reserve.currently.blocks.upgrades.587e3e2f" /></p>}
			</div>
		</Modal>
	);
}
