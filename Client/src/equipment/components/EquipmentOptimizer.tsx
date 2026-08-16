import { useEffect, useMemo, useRef, useState, type DragEvent, type ReactNode } from 'react';
import {
	Activity,
	ArrowDown,
	ArrowLeft,
	ArrowUp,
	Crown,
	GripVertical,
	Info,
	Plus,
	RefreshCw,
	Search,
	Shield,
	Sparkles,
	Swords,
	Target,
	X,
} from 'lucide-react';
import type { EquipmentOptimizeResponse, EquipmentPriorityV2 } from '../../api/Contracts';
import { useCitadelAPI } from '../../api/ApiContext';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Input, MetricTile, Modal, ModalTitle } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';
import type { EquipmentLeader } from './EquipmentTypes';
import {
	cacheEquipmentPriorityProfile,
	equipmentPrioritySection,
	equipmentTargetProfiles,
	groupEquipmentPriorityEffects,
	inferredEquipmentPriorityProfile,
	legacyEquipmentPrioritySections,
	normalizeEquipmentPriorityProfile,
	readCachedEquipmentPriorityProfile,
	readEquipmentPriorityProfile,
	readFirstCachedEquipmentPriorityProfile,
	storedEquipmentPriorityProfile,
	targetProfileEffectIDs,
	type EquipmentPriorityGroup,
	type EquipmentPriorityProfile,
	type EquipmentTargetProfile,
} from './EquipmentOptimizerState';

type Tier = 1 | 2;

interface TargetProfileChoice {
	profile: EquipmentTargetProfile;
	availableGroups: number;
}

interface DragState {
	key: string;
	tier: Tier;
}

interface DropTarget {
	tier: Tier;
	key: string | null;
}

export default function EquipmentOptimizer({
	isOpen,
	onClose,
	leader,
	candidateEffectIDsByMode,
	disabled,
}: {
	isOpen: boolean;
	onClose: () => void;
	leader: EquipmentLeader | null;
	candidateEffectIDsByMode: Record<EquipmentTargetProfile['combatMode'], number[]>;
	disabled: boolean;
}) {
	const { effects, isLoading } = useMetadata();
	const [targetID, setTargetID] = useState<string | null>(null);
	const targetProfiles = useMemo(() => equipmentTargetProfiles(effects), [effects]);
	const targetChoices = useMemo<TargetProfileChoice[]>(() => {
		return targetProfiles.map((profile) => {
			const availableEffects = targetProfileEffectIDs(candidateEffectIDsByMode[profile.combatMode], effects, profile);
			const availableKeys = new Set(groupEquipmentPriorityEffects(availableEffects, effects).map((group) => group.key));
			return {
				profile,
				availableGroups: availableKeys.size,
			};
		});
	}, [candidateEffectIDsByMode, effects, targetProfiles]);
	const target = targetProfiles.find((profile) => profile.id === targetID) ?? null;

	const closeFlow = () => {
		setTargetID(null);
		onClose();
	};

	return (
		<>
			<Modal
				isOpen={isOpen && target == null}
				onClose={closeFlow}
				maxWidth="5xl"
				title={(
					<ModalTitle
						icon={<Target className="h-5 w-5" />}
						description="Choose the battle family this relic loadout should optimize for."
					>
						Reconfiguration target
					</ModalTitle>
				)}
			>
				<div className="space-y-4">
					<p className="text-sm leading-relaxed text-text-muted">
						PvP and PvE cover their broadly applicable effects. Event cards add official effects restricted to that target family, and new complete families appear automatically from current game data.
					</p>
					{isLoading ? (
						<div className="rounded-global border border-border-base bg-bg-app/35 px-4 py-8 text-center text-sm text-text-muted">
							Loading official equipment targets…
						</div>
					) : (
						<div className="grid gap-3 md:grid-cols-2 xl:grid-cols-3">
							{targetChoices.map((choice) => (
								<TargetProfileCard
									key={choice.profile.id}
									choice={choice}
									disabled={disabled || !leader || choice.availableGroups === 0}
									onSelect={() => setTargetID(choice.profile.id)}
								/>
							))}
						</div>
					)}
				</div>
			</Modal>

			{target && (
				<EquipmentOptimizerEditor
					key={`${leader?.kind ?? 'none'}-${leader?.id ?? 0}-${target.id}`}
					isOpen={isOpen}
					onClose={closeFlow}
					onBack={() => setTargetID(null)}
					leader={leader}
					target={target}
					candidateEffectIDs={candidateEffectIDsByMode[target.combatMode]}
					disabled={disabled}
				/>
			)}
		</>
	);
}

function TargetProfileCard({
	choice,
	disabled,
	onSelect,
}: {
	choice: TargetProfileChoice;
	disabled: boolean;
	onSelect: () => void;
}) {
	const { profile, availableGroups } = choice;
	const tone = profile.combatMode === 'PvP' ? 'primary' : profile.kind === 'event' ? 'warning' : 'success';
	return (
		<button
			type="button"
			onClick={onSelect}
			disabled={disabled}
			className="group flex min-h-64 flex-col rounded-global border border-border-base bg-bg-card/65 p-5 text-left shadow-[var(--shadow-raised)] transition enabled:hover:-translate-y-0.5 enabled:hover:border-primary/55 enabled:hover:bg-primary/8 focus:outline-none focus:ring-2 focus:ring-primary/45 disabled:cursor-not-allowed disabled:opacity-45"
		>
			<div className="flex items-start justify-between gap-3">
				<div className="flex min-w-0 items-center gap-3">
					<span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-global border ${targetIconTone(profile)}`}>
						{targetIcon(profile)}
					</span>
					<div className="min-w-0">
						<div className="truncate text-base font-black text-text-main transition-colors group-hover:text-primary">{profile.label}</div>
						<div className="mt-1 flex flex-wrap gap-1.5">
							<Badge variant={tone}>{profile.combatMode}</Badge>
							{profile.kind === 'event' && <Badge variant="secondary">{profile.discovered ? 'Official discovery' : 'Event profile'}</Badge>}
						</div>
					</div>
				</div>
			</div>
			<p className="mt-4 flex-1 text-sm leading-relaxed text-text-muted">{profile.description}</p>
			<div className="mt-4 grid grid-cols-2 gap-2">
				<MetricTile
					size="sm"
					label={profile.kind === 'event' ? 'Target battle stats' : 'Official scope'}
					value={profile.kind === 'event' ? profile.officialGroupCount.toLocaleString() : 'Broad'}
				/>
				<MetricTile size="sm" label="Usable battle stats" value={availableGroups.toLocaleString()} />
			</div>
			<div className="mt-4 text-xs font-black uppercase tracking-wide text-primary">
				{availableGroups > 0 ? `Configure ${profile.label}` : 'No matching effects available'}
			</div>
		</button>
	);
}

function targetIcon(profile: EquipmentTargetProfile): ReactNode {
	if (profile.id === 'pvp') return <Swords className="h-5 w-5" />;
	if (profile.id === 'pve') return <Shield className="h-5 w-5" />;
	if (profile.id === 'glory-invasion') return <Crown className="h-5 w-5" />;
	if (profile.discovered) return <Sparkles className="h-5 w-5" />;
	return <Target className="h-5 w-5" />;
}

function targetIconTone(profile: EquipmentTargetProfile): string {
	if (profile.combatMode === 'PvP') return 'border-primary/40 bg-primary/12 text-primary';
	if (profile.kind === 'event') return 'border-warning/40 bg-warning/12 text-warning';
	return 'border-success/40 bg-success/12 text-success';
}

function EquipmentOptimizerEditor({
	isOpen,
	onClose,
	onBack,
	leader,
	target,
	candidateEffectIDs,
	disabled,
}: {
	isOpen: boolean;
	onClose: () => void;
	onBack: () => void;
	leader: EquipmentLeader | null;
	target: EquipmentTargetProfile;
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
	const [dragState, setDragState] = useState<DragState | null>(null);
	const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);
	const priorityRef = useRef<EquipmentPriorityProfile>({ tier1: [], tier2: [] });
	const optimisticProfiles = useRef<Record<string, EquipmentPriorityProfile>>({});
	const optimizeRequest = useRef(0);
	const canApply = state?.session.loggedIn === true && state.session.socketReady === true;
	const candidateEffectKey = candidateEffectIDs.join(',');
	const candidateEffects = useMemo(() => candidateEffectKey === '' ? [] : candidateEffectKey.split(',').map(Number), [candidateEffectKey]);
	const officialEffectIDs = useMemo(() => Object.keys(effects).map(Number).filter((id) => id > 0), [effects]);
	const targetEffects = useMemo(
		() => targetProfileEffectIDs(officialEffectIDs, effects, target),
		[effects, officialEffectIDs, target],
	);
	const availableTargetEffects = useMemo(
		() => targetProfileEffectIDs(candidateEffects, effects, target),
		[candidateEffects, effects, target],
	);
	const officialGroups = useMemo(() => groupEquipmentPriorityEffects(targetEffects, effects), [effects, targetEffects]);
	const availableGroupKeys = useMemo(() => new Set(
		groupEquipmentPriorityEffects(availableTargetEffects, effects).map((group) => group.key),
	), [availableTargetEffects, effects]);
	const priorityGroups = useMemo(
		() => officialGroups.filter((group) => availableGroupKeys.has(group.key)),
		[availableGroupKeys, officialGroups],
	);
	const groupsByKey = useMemo(() => new Map(priorityGroups.map((group) => [group.key, group])), [priorityGroups]);
	const prioritySection = useMemo(
		() => equipmentPrioritySection(state?.player.id, leader, target.id),
		[leader, state?.player.id, target.id],
	);
	const legacySections = useMemo(
		() => legacyEquipmentPrioritySections(state?.player.id, leader, target, effects),
		[effects, leader, state?.player.id, target],
	);
	const storedProfileJSON = useMemo(() => safeJSONStringify(
		prioritySection ? configuration?.sections[prioritySection] ?? null : null,
	), [configuration?.sections, prioritySection]);
	const legacyProfileJSON = useMemo(() => {
		for (const section of legacySections) {
			const raw = configuration?.sections[section];
			if (raw != null) return safeJSONStringify(raw);
		}
		return 'null';
	}, [configuration?.sections, legacySections]);
	const storedProfile = useMemo(
		() => readProfileJSON(storedProfileJSON, priorityGroups, effects),
		[effects, priorityGroups, storedProfileJSON],
	);
	const cachedProfile = useMemo(
		() => readCachedEquipmentPriorityProfile(prioritySection, priorityGroups, effects),
		[effects, priorityGroups, prioritySection],
	);
	const legacyStoredProfile = useMemo(
		() => readProfileJSON(legacyProfileJSON, priorityGroups, effects),
		[effects, legacyProfileJSON, priorityGroups],
	);
	const legacyCachedProfile = useMemo(
		() => readFirstCachedEquipmentPriorityProfile(legacySections, priorityGroups, effects),
		[effects, legacySections, priorityGroups],
	);
	const inferredProfile = useMemo(
		() => inferredEquipmentPriorityProfile(priorityGroups, leader?.kind),
		[leader?.kind, priorityGroups],
	);

	useEffect(() => {
		optimizeRequest.current += 1;
		const initial = prioritySection && priorityGroups.length > 0
			? optimisticProfiles.current[prioritySection]
				?? cachedProfile
				?? storedProfile
				?? legacyCachedProfile
				?? legacyStoredProfile
				?? inferredProfile
			: { tier1: [], tier2: [] };
		const next = normalizeEquipmentPriorityProfile(initial, priorityGroups);
		priorityRef.current = next;
		setPriorityProfile(next);
		setPreview(null);
	}, [cachedProfile, inferredProfile, legacyCachedProfile, legacyStoredProfile, priorityGroups, prioritySection, storedProfile]);

	const tier1 = priorityProfile.tier1;
	const tier2 = priorityProfile.tier2;
	const used = useMemo(() => new Set([...tier1, ...tier2]), [tier1, tier2]);
	const availableGroups = useMemo(() => priorityGroups
		.filter((group) => !used.has(group.key))
		.filter((group) => {
			const query = search.trim().toLowerCase();
			if (!query) return true;
			return group.label.toLowerCase().includes(query)
				|| group.categoryLabel.toLowerCase().includes(query)
				|| group.effectIDs.some((id) => {
					const effect = getEffect(id);
					return String(effect?.name ?? '').toLowerCase().includes(query)
						|| String(effect?.internalName ?? '').toLowerCase().includes(query)
						|| String(id).includes(query);
				});
		}), [getEffect, priorityGroups, search, used]);
	const pickerSections = useMemo(() => {
		const sections = new Map<string, { label: string; category: number; groups: EquipmentPriorityGroup[] }>();
		for (const group of availableGroups) {
			const key = `${group.category}:${group.categoryLabel}`;
			const section = sections.get(key) ?? { label: group.categoryLabel, category: group.category, groups: [] };
			section.groups.push(group);
			sections.set(key, section);
		}
		return [...sections.values()].sort((left, right) => left.category - right.category || left.label.localeCompare(right.label));
	}, [availableGroups]);

	const updatePriorities = (change: (current: EquipmentPriorityProfile) => EquipmentPriorityProfile) => {
		const next = normalizeEquipmentPriorityProfile(change(priorityRef.current), priorityGroups);
		priorityRef.current = next;
		setPriorityProfile(next);
		optimizeRequest.current += 1;
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
			? { ...current, tier1: [...current.tier1, group.key] }
			: { ...current, tier2: [...current.tier2, group.key] });
		setSearch('');
	};
	const remove = (key: string) => {
		updatePriorities((current) => ({
			tier1: current.tier1.filter((value) => value !== key),
			tier2: current.tier2.filter((value) => value !== key),
		}));
	};
	const moveTier = (key: string, from: Tier) => {
		updatePriorities((current) => from === 1
			? { tier1: current.tier1.filter((value) => value !== key), tier2: [...current.tier2.filter((value) => value !== key), key] }
			: { tier1: [...current.tier1.filter((value) => value !== key), key], tier2: current.tier2.filter((value) => value !== key) });
	};
	const reorder = (tier: Tier, index: number, direction: -1 | 1) => {
		updatePriorities((current) => {
			const values = tier === 1 ? current.tier1 : current.tier2;
			const targetIndex = index + direction;
			if (targetIndex < 0 || targetIndex >= values.length) return current;
			const next = [...values];
			[next[index], next[targetIndex]] = [next[targetIndex], next[index]];
			return tier === 1 ? { ...current, tier1: next } : { ...current, tier2: next };
		});
	};
	const moveGroup = (key: string, from: Tier, to: Tier, beforeKey: string | null) => {
		updatePriorities((current) => {
			const source = (from === 1 ? current.tier1 : current.tier2).filter((value) => value !== key);
			const destination = (to === from ? source : (to === 1 ? current.tier1 : current.tier2).filter((value) => value !== key));
			const insertAt = beforeKey == null ? destination.length : Math.max(0, destination.indexOf(beforeKey));
			const moved = [...destination];
			moved.splice(insertAt, 0, key);
			if (from === to) return to === 1 ? { ...current, tier1: moved } : { ...current, tier2: moved };
			return to === 1 ? { tier1: moved, tier2: source } : { tier1: source, tier2: moved };
		});
	};
	const finishDrag = () => {
		setDragState(null);
		setDropTarget(null);
	};
	const dropGroup = (event: DragEvent, tier: Tier, beforeKey: string | null) => {
		event.preventDefault();
		const fallback = parseDragState(event.dataTransfer.getData('text/plain'));
		const source = dragState ?? fallback;
		if (source) moveGroup(source.key, source.tier, tier, beforeKey);
		finishDrag();
	};

	const selectedGroups = useMemo(() => [...tier1, ...tier2]
		.map((key) => groupsByKey.get(key))
		.filter((group): group is EquipmentPriorityGroup => group != null), [groupsByKey, tier1, tier2]);
	const priorities = useMemo<EquipmentPriorityV2[]>(() => [
		...tier1.flatMap((key, position) => (groupsByKey.get(key)?.effectIDs ?? []).map((effectId) => ({ effectId, tier: 1 as const, position }))),
		...tier2.flatMap((key, position) => (groupsByKey.get(key)?.effectIDs ?? []).map((effectId) => ({ effectId, tier: 2 as const, position }))),
	], [groupsByKey, tier1, tier2]);

	const optimize = async () => {
		if (!leader || priorities.length === 0) return;
		const requestID = ++optimizeRequest.current;
		setOptimizing(true);
		try {
			if (canApply) await submitIntent('equipment.refresh');
			try {
				const result = await optimizeEquipment({
					leaderKind: leader.kind,
					leaderId: leader.id,
					combatMode: target.combatMode.toLowerCase() as 'pvp' | 'pve',
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
					<ModalTitle
						icon={<Activity className="h-5 w-5" />}
						description={`${target.label} · Effective Battle Report stat priority`}
					>
						Stat Priority &amp; Reconfigure
					</ModalTitle>
				)}
				maxWidth="5xl"
				footer={(
					<>
						<Button variant="ghost" onClick={onClose}>Cancel</Button>
						<Button
							onClick={optimize}
							disabled={disabled || !leader || priorities.length === 0}
							isLoading={optimizing}
							leftIcon={<RefreshCw className="h-4 w-4" />}
						>
							Preview Reconfiguration
						</Button>
					</>
				)}
			>
				<div className="space-y-4">
					<div className="grid gap-3 rounded-global border border-border-base bg-bg-app/45 p-3 sm:grid-cols-[minmax(0,1fr)_auto]">
						<div className="flex min-w-0 items-start gap-2">
							<Button size="icon" variant="ghost" onClick={onBack} aria-label="Change reconfiguration target" title="Change target">
								<ArrowLeft className="h-4 w-4" />
							</Button>
							<div className="min-w-0 flex-1">
								<div className="flex flex-wrap items-center gap-2">
									<span className="truncate text-sm font-semibold text-text-main">{leader?.name ?? 'No loadout selected'}</span>
									<Badge variant={target.combatMode === 'PvP' ? 'danger' : 'success'}>{target.label}</Badge>
								</div>
									<p className="mt-1 text-xs leading-relaxed text-text-muted">{target.description} Drag rows within or between tiers; priorities are saved per leader and target profile.</p>
							</div>
						</div>
						<div className="flex items-center justify-end gap-2 sm:shrink-0">
							<Button size="icon" variant="ghost" onClick={() => setShowInfo(true)} aria-label="Explain battle stat priority"><Info className="h-4 w-4" /></Button>
							<Button
								size="sm"
								variant="outline"
								onClick={() => { setSearch(''); setShowPicker(true); }}
								disabled={availableGroups.length === 0}
								leftIcon={<Plus className="h-4 w-4" />}
							>
								Add Battle Stat
							</Button>
						</div>
					</div>

					<div className="grid gap-4 md:grid-cols-2">
						<PriorityTier
							title="Max Stat"
							tier={1}
							keys={tier1}
							groupsByKey={groupsByKey}
							dragState={dragState}
							dropTarget={dropTarget}
							onDragStart={(event, key) => {
								event.dataTransfer.effectAllowed = 'move';
								event.dataTransfer.setData('text/plain', `1|${key}`);
								setDragState({ key, tier: 1 });
							}}
							onDragOver={(key) => setDropTarget({ tier: 1, key })}
							onDrop={(event, key) => dropGroup(event, 1, key)}
							onDragEnd={finishDrag}
							onRemove={remove}
							onMoveTier={moveTier}
							onReorder={reorder}
						/>
						<PriorityTier
							title="Have in Random Slots"
							tier={2}
							keys={tier2}
							groupsByKey={groupsByKey}
							dragState={dragState}
							dropTarget={dropTarget}
							onDragStart={(event, key) => {
								event.dataTransfer.effectAllowed = 'move';
								event.dataTransfer.setData('text/plain', `2|${key}`);
								setDragState({ key, tier: 2 });
							}}
							onDragOver={(key) => setDropTarget({ tier: 2, key })}
							onDrop={(event, key) => dropGroup(event, 2, key)}
							onDragEnd={finishDrag}
							onRemove={remove}
							onMoveTier={moveTier}
							onReorder={reorder}
						/>
					</div>
				</div>
			</Modal>

			<Modal isOpen={showPicker} onClose={() => setShowPicker(false)} title="Add Effective Battle Stat" maxWidth="3xl">
				<div className="space-y-3">
					<Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder="Search battle stats" leftIcon={<Search className="h-4 w-4" />} />
					<div className="max-h-[65vh] space-y-4 overflow-y-auto custom-scrollbar">
						{pickerSections.map((section) => (
							<section key={`${section.category}:${section.label}`}>
								<div className="sticky top-0 z-10 mb-2 border-b border-border-base bg-bg-card/95 px-1 py-2 text-[10px] font-black uppercase tracking-wider text-text-muted backdrop-blur-sm">
									{section.label}
								</div>
								<div className="space-y-2">
									{section.groups.map((group) => (
										<div key={group.key} className="flex flex-wrap items-center gap-2 rounded-global border border-border-base bg-bg-app/50 p-3">
											<span className="min-w-48 flex-1">
												<span className="block text-sm font-medium text-text-main">{group.label}</span>
												<span className="mt-0.5 block text-[10px] text-text-muted">{group.effectIDs.length} target-compatible official effect definition{group.effectIDs.length === 1 ? '' : 's'}</span>
											</span>
											<Button size="sm" variant="danger" onClick={() => addGroup(group, 1)}>Max Stat</Button>
											<Button size="sm" variant="outline" onClick={() => addGroup(group, 2)}>Random Slots</Button>
										</div>
									))}
								</div>
							</section>
						))}
						{availableGroups.length === 0 && <p className="py-6 text-center text-sm text-text-muted">No matching unused battle stats.</p>}
					</div>
				</div>
			</Modal>

			<Modal isOpen={showInfo} onClose={() => setShowInfo(false)} title="How Battle Stat Priority Works" maxWidth="2xl">
				<div className="space-y-3 text-sm text-text-muted">
						<p>Each draggable row is the same official game-data effect group shown in the Equipment view's Effective Battle Report.</p>
						<p>When previewing, CitadelOps expands that group into every target-compatible official effect definition. New definitions therefore join their catalog group automatically.</p>
					<p><span className="font-semibold text-error">Max Stat</span> groups receive the strongest position-decayed score. <span className="font-semibold text-primary">Have in Random Slots</span> groups receive a presence bonus and lower weighted score.</p>
					<p>The server searches storage plus the selected leader’s current pieces, respects official caps and set bonuses, and never borrows gear from another leader.</p>
				</div>
			</Modal>

			<OptimizerPreview
				preview={preview}
				priorityGroups={selectedGroups}
				getEffectName={(id) => effectName(id, getEffect)}
				getEquipmentName={(id) => getEquipment(state?.inventory.equipment[String(id)]?.definitionId ?? 0)?.name ?? `Equipment ${id}`}
				getGemName={(id) => getGem(state?.inventory.gems[String(id)]?.definitionId ?? 0)?.name ?? `Gem ${id}`}
				onClose={() => setPreview(null)}
				onApply={apply}
				applying={applying}
				applyDisabled={!canApply}
			/>
		</>
	);
}

function PriorityTier({
	title,
	tier,
	keys,
	groupsByKey,
	dragState,
	dropTarget,
	onDragStart,
	onDragOver,
	onDrop,
	onDragEnd,
	onRemove,
	onMoveTier,
	onReorder,
}: {
	title: string;
	tier: Tier;
	keys: string[];
	groupsByKey: Map<string, EquipmentPriorityGroup>;
	dragState: DragState | null;
	dropTarget: DropTarget | null;
	onDragStart: (event: DragEvent, key: string) => void;
	onDragOver: (key: string | null) => void;
	onDrop: (event: DragEvent, key: string | null) => void;
	onDragEnd: () => void;
	onRemove: (key: string) => void;
	onMoveTier: (key: string, tier: Tier) => void;
	onReorder: (tier: Tier, index: number, direction: -1 | 1) => void;
}) {
	return (
		<div
			className={`overflow-hidden rounded-global border ${tier === 1 ? 'border-error/30 bg-error/5' : 'border-primary/30 bg-primary/5'}`}
			onDragOver={(event) => {
				event.preventDefault();
				event.dataTransfer.dropEffect = 'move';
				onDragOver(null);
			}}
			onDrop={(event) => onDrop(event, null)}
		>
			<div className="flex items-center justify-between border-b border-border-base/50 px-3 py-2">
				<span className={`text-xs font-bold uppercase tracking-wider ${tier === 1 ? 'text-error' : 'text-primary'}`}>{tier}. {title}</span>
				<Badge variant={tier === 1 ? 'danger' : 'primary'}>{keys.length}</Badge>
			</div>
			<div className={`min-h-24 space-y-1.5 p-2 ${dropTarget?.tier === tier && dropTarget.key == null ? 'bg-primary/5' : ''}`}>
				{keys.map((key, index) => {
					const group = groupsByKey.get(key);
					if (!group) return null;
					const dragging = dragState?.key === key;
					const targeted = dropTarget?.tier === tier && dropTarget.key === key;
					return (
						<div
							key={key}
							draggable
							onDragStart={(event) => onDragStart(event, key)}
							onDragOver={(event) => {
								event.preventDefault();
								event.stopPropagation();
								event.dataTransfer.dropEffect = 'move';
								onDragOver(key);
							}}
							onDrop={(event) => {
								event.stopPropagation();
								onDrop(event, key);
							}}
							onDragEnd={onDragEnd}
							className={`flex cursor-grab items-center gap-2 rounded-global border bg-bg-card px-2 py-2 transition-colors active:cursor-grabbing ${
								dragging ? 'border-primary/40 opacity-45' : targeted ? 'border-primary bg-primary/10' : 'border-border-base/50 hover:border-primary/30'
							}`}
						>
							<GripVertical className="h-4 w-4 shrink-0 text-text-muted" aria-hidden="true" />
							<span className="flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-bg-app text-[10px] font-bold text-text-muted">{index + 1}</span>
							<span className="min-w-0 flex-1">
								<span className="block truncate text-xs font-semibold text-text-main" title={group.label}>{group.label}</span>
								<span className="block truncate text-[10px] text-text-muted">{group.categoryLabel} · {group.effectIDs.length} official effects</span>
							</span>
							<button type="button" disabled={index === 0} onClick={() => onReorder(tier, index, -1)} className="rounded p-1 text-text-muted hover:bg-primary/10 hover:text-primary disabled:opacity-25" aria-label={`Move ${group.label} up`}><ArrowUp className="h-3.5 w-3.5" /></button>
							<button type="button" disabled={index === keys.length - 1} onClick={() => onReorder(tier, index, 1)} className="rounded p-1 text-text-muted hover:bg-primary/10 hover:text-primary disabled:opacity-25" aria-label={`Move ${group.label} down`}><ArrowDown className="h-3.5 w-3.5" /></button>
							<button type="button" onClick={() => onMoveTier(key, tier)} className="rounded px-1.5 py-1 text-[9px] font-bold uppercase text-text-muted hover:bg-primary/10 hover:text-primary" aria-label={`Move ${group.label} to tier ${tier === 1 ? 2 : 1}`}>T{tier === 1 ? 2 : 1}</button>
							<button type="button" onClick={() => onRemove(key)} className="rounded p-1 text-text-muted hover:bg-error/10 hover:text-error" aria-label={`Remove ${group.label}`}><X className="h-3.5 w-3.5" /></button>
						</div>
					);
				})}
				{keys.length === 0 && <p className="py-5 text-center text-xs text-text-muted">Drag battle stats here</p>}
			</div>
		</div>
	);
}

function OptimizerPreview({
	preview,
	priorityGroups,
	getEffectName,
	getEquipmentName,
	getGemName,
	onClose,
	onApply,
	applying,
	applyDisabled,
}: {
	preview: EquipmentOptimizeResponse | null;
	priorityGroups: EquipmentPriorityGroup[];
	getEffectName: (id: number) => string;
	getEquipmentName: (id: number) => string;
	getGemName: (id: number) => string;
	onClose: () => void;
	onApply: () => void;
	applying: boolean;
	applyDisabled: boolean;
}) {
	const effectRows = (() => {
		if (!preview) return [];
		const current = new Map(preview.current.effects.map((effect) => [effect.definitionId, effect]));
		const proposed = new Map(preview.proposed.effects.map((effect) => [effect.definitionId, effect]));
		const ids = Array.from(new Set([...current.keys(), ...proposed.keys()]));
		return ids.map((id) => ({ id, current: current.get(id)?.value ?? 0, proposed: proposed.get(id)?.value ?? 0, cap: proposed.get(id)?.cap ?? current.get(id)?.cap }))
			.filter((row) => row.current !== row.proposed)
			.sort((left, right) => Math.abs(right.proposed - right.current) - Math.abs(left.proposed - left.current));
	})();
	const unavailableGroups = (() => {
		if (!preview) return [];
		const proposed = new Map(preview.proposed.effects.map((effect) => [effect.definitionId, effect.value]));
		return priorityGroups.filter((group) => group.effectIDs.every((id) => (proposed.get(id) ?? 0) === 0));
	})();
	return (
		<Modal
			isOpen={preview != null}
			onClose={onClose}
			title="Reconfiguration Preview"
			maxWidth="5xl"
			footer={(
				<>
					<Button variant="ghost" onClick={onClose}>Cancel</Button>
					<Button
						onClick={onApply}
						isLoading={applying}
						disabled={applyDisabled}
						title={applyDisabled ? 'Connect the game before applying this loadout' : undefined}
					>
						Apply Loadout
					</Button>
				</>
			)}
		>
			{preview && (
				<div className="space-y-5">
					{unavailableGroups.length > 0 && (
						<p className="rounded-global border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">
							No eligible value was found for {unavailableGroups.map((group) => group.label).join(' · ')}. This is still the best available loadout.
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

function readProfileJSON(
	value: string,
	groups: readonly EquipmentPriorityGroup[],
	effects: Parameters<typeof readEquipmentPriorityProfile>[2],
): EquipmentPriorityProfile | null {
	try {
		return readEquipmentPriorityProfile(JSON.parse(value), groups, effects);
	} catch {
		return null;
	}
}

function safeJSONStringify(value: unknown): string {
	try {
		return JSON.stringify(value ?? null);
	} catch {
		return 'null';
	}
}

function parseDragState(value: string): DragState | null {
	const [tier, ...keyParts] = value.split('|');
	const key = keyParts.join('|');
	if ((tier !== '1' && tier !== '2') || !key) return null;
	return { key, tier: Number(tier) as Tier };
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
