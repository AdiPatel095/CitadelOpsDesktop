import { LocalizedRichText } from "../../i18n/LocalizedRichText";
import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
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
import type { EquipmentEffectTotalV2, EquipmentLoadoutV2, EquipmentOptimizeResponse, EquipmentPriorityV2 } from '../../api/Contracts';
import { useCitadelAPI } from '../../api/ApiContext';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Input, MetricTile, Modal, ModalTitle } from '../../components/ui';
import { useMetadata } from '../../context/MetadataContext';
import type { EquipmentLeader } from './EquipmentTypes';
import {
	cacheEquipmentPriorityProfile,
	descriptiveEquipmentEffectLabel,
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
import {
	equipmentExtractionApplyLabel,
	equipmentAlternativeApplyDisabled,
	equipmentExtractionCostNotices,
	equipmentOptimizerInitializationChange,
	equipmentOptimizerSnapshotKey,
	equipmentPriorityCatalogKey,
	equipmentPriorityProfileKey,
} from './EquipmentOptimizerLifecycle';

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

interface PreviewEnvelope {
	response: EquipmentOptimizeResponse;
	localSnapshotKey: string;
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
  const { t: localizeStatic } = useStaticLocale();
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
						description={localizeStatic("ui.equipment.components.equipmentOptimizer.description.choose.the.battle.family.this.relic.loadout.9185ee43")}
					>
						<LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.reconfiguration.target.06c7dc57" /></ModalTitle>
				)}
			>
				<div className="space-y-4">
					<p className="text-sm leading-relaxed text-text-muted">
						<LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.pvp.and.pve.cover.their.broadly.applicable.eb6b01ee" /></p>
					{isLoading ? (
						<div className="rounded-global border border-border-base bg-bg-app/35 px-4 py-8 text-center text-sm text-text-muted">
							<LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.loading.official.equipment.targets.4e64c51a" /></div>
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
  const { t: localizeStatic } = useStaticLocale();
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
				<MetricTile size="sm" label={localizeStatic("ui.equipment.components.equipmentOptimizer.label.usable.battle.stats.bd169e1e")} value={availableGroups.toLocaleString()} />
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
  const { t: localizeStatic } = useStaticLocale();
	const { state, catalogs, configuration, submitIntent, optimizeEquipment, updateConfiguration } = useCitadelAPI();
	const { effects, troops } = useMetadata();
	const [priorityProfile, setPriorityProfile] = useState<EquipmentPriorityProfile>({ tier1: [], tier2: [] });
	const [showPicker, setShowPicker] = useState(false);
	const [showInfo, setShowInfo] = useState(false);
	const [search, setSearch] = useState('');
	const [optimizing, setOptimizing] = useState(false);
	const [applying, setApplying] = useState(false);
	const [preview, setPreview] = useState<PreviewEnvelope | null>(null);
	const [selectedAlternative, setSelectedAlternative] = useState(0);
	const [optimizeError, setOptimizeError] = useState<string | null>(null);
	const [applyError, setApplyError] = useState<string | null>(null);
	const [dragState, setDragState] = useState<DragState | null>(null);
	const [dropTarget, setDropTarget] = useState<DropTarget | null>(null);
	const priorityRef = useRef<EquipmentPriorityProfile>({ tier1: [], tier2: [] });
	const optimisticProfiles = useRef<Record<string, EquipmentPriorityProfile>>({});
	const optimizeRequest = useRef(0);
	const initializedSection = useRef<string | null | undefined>(undefined);
	const initializedCatalogKey = useRef('');
	const initializedProfileKey = useRef('');
	const optimizeInFlight = useRef(false);
	const applyInFlight = useRef(false);
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
	const priorityGroups = officialGroups;
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
	const legacyStoredProfile = useMemo(
		() => readProfileJSON(legacyProfileJSON, priorityGroups, effects),
		[effects, legacyProfileJSON, priorityGroups],
	);
	const inferredProfile = useMemo(
		() => inferredEquipmentPriorityProfile(priorityGroups, leader?.kind),
		[leader?.kind, priorityGroups],
	);
	const priorityCatalogKey = equipmentPriorityCatalogKey(priorityGroups);
	const currentSnapshotKey = useMemo(() => `${equipmentOptimizerSnapshotKey(
		state, leader, target.combatMode.toLowerCase() as 'pvp' | 'pve',
	)}|catalog:${state?.catalogVersion ?? ''}:${catalogs?.metadata.digestSha256 ?? ''}:${priorityCatalogKey}`, [catalogs?.metadata.digestSha256, leader, priorityCatalogKey, state, target.combatMode]);
	const previewStale = preview != null && preview.localSnapshotKey !== currentSnapshotKey;

	useEffect(() => {
		const initial = prioritySection && priorityGroups.length > 0
			? optimisticProfiles.current[prioritySection]
				?? readCachedEquipmentPriorityProfile(prioritySection, priorityGroups, effects)
				?? storedProfile
				?? readFirstCachedEquipmentPriorityProfile(legacySections, priorityGroups, effects)
				?? legacyStoredProfile
				?? inferredProfile
			: { tier1: [], tier2: [] };
		const next = normalizeEquipmentPriorityProfile(initial, priorityGroups);
		const nextProfileKey = equipmentPriorityProfileKey(next);
		const change = equipmentOptimizerInitializationChange(
			initializedSection.current, prioritySection,
			initializedCatalogKey.current, priorityCatalogKey,
			initializedProfileKey.current, nextProfileKey,
		);
		if (change === 'unchanged') return;
		initializedSection.current = prioritySection;
		initializedCatalogKey.current = priorityCatalogKey;
		initializedProfileKey.current = nextProfileKey;
		priorityRef.current = next;
		setPriorityProfile(next);
		if (change === 'retain-preview') return;
		optimizeRequest.current += 1;
		optimizeInFlight.current = false;
		setOptimizing(false);
		setPreview(null);
		setOptimizeError(null);
		setApplyError(null);
	}, [effects, inferredProfile, legacyProfileJSON, legacySections, legacyStoredProfile, priorityCatalogKey, priorityGroups, prioritySection, storedProfile, storedProfileJSON]);

	const tier1 = priorityProfile.tier1;
	const tier2 = priorityProfile.tier2;
	const used = useMemo(() => new Set([...tier1, ...tier2]), [tier1, tier2]);
	const availableGroups = useMemo(() => priorityGroups
		.filter((group) => !used.has(group.key))
		.filter((group) => availableGroupKeys.has(group.key))
		.filter((group) => {
			const query = search.trim().toLowerCase();
			if (!query) return true;
			return group.label.toLowerCase().includes(query)
				|| group.categoryLabel.toLowerCase().includes(query)
				|| group.effectIDs.some((id) => {
					const effect = effects[id];
					return String(effect?.name ?? '').toLowerCase().includes(query)
						|| String(effect?.internalName ?? '').toLowerCase().includes(query)
						|| String(id).includes(query);
				});
		}), [availableGroupKeys, effects, priorityGroups, search, used]);
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
		initializedProfileKey.current = equipmentPriorityProfileKey(next);
		optimizeRequest.current += 1;
		optimizeInFlight.current = false;
		setOptimizing(false);
		setPreview(null);
		setSelectedAlternative(0);
		setOptimizeError(null);
		setApplyError(null);
		if (prioritySection) {
			optimisticProfiles.current[prioritySection] = next;
			cacheEquipmentPriorityProfile(prioritySection, next);
			void updateConfiguration(prioritySection, storedEquipmentPriorityProfile(next)).catch(() => undefined);
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
		if (disabled || !leader || priorities.length === 0 || optimizeInFlight.current) return;
		optimizeInFlight.current = true;
		const requestID = ++optimizeRequest.current;
		const requestSnapshotKey = currentSnapshotKey;
		setOptimizing(true);
		setOptimizeError(null);
		setApplyError(null);
		try {
			const result = await withTimeout(optimizeEquipment({
				leaderKind: leader.kind,
				leaderId: leader.id,
				combatMode: target.combatMode.toLowerCase() as 'pvp' | 'pve',
				targetAreaTypeIds: target.areaTypeIDs,
				priorities,
				resultCount: 10,
			}), 8_000, 'The preview request expired. Check the connection and try again.');
			if (requestID !== optimizeRequest.current) return;
			const alternatives = result.alternatives?.length ? result.alternatives : [result.proposed];
			setPreview({ response: { ...result, alternatives, proposed: alternatives[0]! }, localSnapshotKey: requestSnapshotKey });
			setSelectedAlternative(0);
		} catch (error) {
			if (requestID !== optimizeRequest.current) return;
			setOptimizeError(error instanceof Error ? error.message : 'Could not optimize this loadout. Try again.');
		} finally {
			if (requestID === optimizeRequest.current) {
				optimizeInFlight.current = false;
				setOptimizing(false);
			}
		}
	};
	const cancelPendingOptimize = () => {
		optimizeRequest.current += 1;
		optimizeInFlight.current = false;
		setOptimizing(false);
	};
	const closeEditor = () => {
		if (applying) return;
		cancelPendingOptimize();
		setPreview(null);
		setOptimizeError(null);
		setApplyError(null);
		onClose();
	};
	const changeTarget = () => {
		cancelPendingOptimize();
		setPreview(null);
		setOptimizeError(null);
		setApplyError(null);
		onBack();
	};
	const closePreview = () => {
		if (applying) return;
		cancelPendingOptimize();
		setPreview(null);
		setOptimizeError(null);
	};

	const apply = async () => {
		if (disabled || !preview || previewStale || applyInFlight.current) return;
		const selected = preview.response.alternatives[selectedAlternative];
		if (!selected) return;
		applyInFlight.current = true;
		setApplying(true);
		setApplyError(null);
		try {
			await submitIntent('equipment.reconfigure', {
				leaderKind: preview.response.leaderKind,
				leaderId: preview.response.leaderId,
				combatMode: target.combatMode.toLowerCase(),
				snapshotFingerprint: preview.response.snapshotFingerprint,
				equipment: selected.equipment,
				gems: selected.gems,
				quoteFingerprint: selected.extractionCost.fingerprint,
				maximumRubySpend: selected.extractionCost.maximumRubySpend,
			});
			Notifications.success(`Reconfigured ${leader?.name ?? preview.response.leaderKind}`);
			setPreview(null);
		} catch (error) {
			setApplyError(error instanceof Error ? error.message : 'The selected loadout could not be applied. Review the current equipment and try again.');
		} finally {
			applyInFlight.current = false;
			setApplying(false);
		}
	};

	return (
		<>
			<Modal
				isOpen={isOpen}
				onClose={closeEditor}
				title={(
					<ModalTitle
						icon={<Activity className="h-5 w-5" />}
						description={`${target.label} · Effective Battle Report stat priority`}
					>
						<LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.stat.priority.reconfigure.c9997f27" /></ModalTitle>
				)}
				maxWidth="5xl"
				footer={(
					<>
						<Button variant="ghost" onClick={closeEditor}><LocalizedText messageKey="game.cancel" /></Button>
						<Button
							onClick={optimize}
							disabled={disabled || !leader || priorities.length === 0}
							isLoading={optimizing}
							leftIcon={<RefreshCw className="h-4 w-4" />}
						>
							<LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.preview.reconfiguration.62efee53" /></Button>
					</>
				)}
			>
				<div className="space-y-4">
					{priorities.length === 0 && (
						<p className="rounded-global border border-warning/30 bg-warning/10 px-3 py-2 text-sm text-warning">
							<LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.add.at.least.one.battle.stat.to.7e402aa8" /></p>
					)}
					{optimizeError && (
						<p role="alert" className="rounded-global border border-error/30 bg-error/10 px-3 py-2 text-sm text-error">
							{optimizeError}
						</p>
					)}
					<div className="grid gap-3 rounded-global border border-border-base bg-bg-app/45 p-3 sm:grid-cols-[minmax(0,1fr)_auto]">
						<div className="flex min-w-0 items-start gap-2">
							<Button size="icon" variant="ghost" onClick={changeTarget} aria-label={localizeStatic("ui.equipment.components.equipmentOptimizer.aria-label.change.reconfiguration.target.1172fad5")} title={localizeStatic("ui.equipment.components.equipmentOptimizer.title.change.target.36d36e6b")}>
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
							<Button size="icon" variant="ghost" onClick={() => setShowInfo(true)} aria-label={localizeStatic("ui.equipment.components.equipmentOptimizer.aria-label.explain.battle.stat.priority.20ba9a01")}><Info className="h-4 w-4" /></Button>
							<Button
								size="sm"
								variant="outline"
								onClick={() => { setSearch(''); setShowPicker(true); }}
								disabled={availableGroups.length === 0}
								leftIcon={<Plus className="h-4 w-4" />}
							>
								<LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.add.battle.stat.302ba69c" /></Button>
						</div>
					</div>

					<div className="grid gap-4 md:grid-cols-2">
						<PriorityTier
							title={localizeStatic("ui.equipment.components.equipmentOptimizer.title.max.stat.f25df34f")}
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
							title={localizeStatic("ui.equipment.components.equipmentOptimizer.title.have.in.random.slots.d286f8b3")}
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

			<Modal isOpen={showPicker} onClose={() => setShowPicker(false)} title={localizeStatic("ui.equipment.components.equipmentOptimizer.title.add.effective.battle.stat.9bdc52fe")} maxWidth="3xl">
				<div className="space-y-3">
					<Input value={search} onChange={(event) => setSearch(event.target.value)} placeholder={localizeStatic("ui.equipment.components.equipmentOptimizer.placeholder.search.battle.stats.6d2a48bf")} leftIcon={<Search className="h-4 w-4" />} />
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
											<Button size="sm" variant="danger" onClick={() => addGroup(group, 1)}><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.max.stat.f25df34f" /></Button>
											<Button size="sm" variant="outline" onClick={() => addGroup(group, 2)}><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.random.slots.c4c4ae0d" /></Button>
										</div>
									))}
								</div>
							</section>
						))}
						{availableGroups.length === 0 && <p className="py-6 text-center text-sm text-text-muted"><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.no.matching.unused.battle.stats.613bcf21" /></p>}
					</div>
				</div>
			</Modal>

			<Modal isOpen={showInfo} onClose={() => setShowInfo(false)} title={localizeStatic("ui.equipment.components.equipmentOptimizer.title.how.battle.stat.priority.works.5c727707")} maxWidth="2xl">
				<div className="space-y-3 text-sm text-text-muted">
						<p><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.each.draggable.row.is.the.same.official.8bf29a83" /></p>
						<p><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.when.previewing.citadelops.expands.that.group.into.bc9f6b4c" /></p>
					<p><LocalizedRichText messageKey="ui.rich.equipment.components.equipmentOptimizer.max.stat.groups.receive.the.strongest.position.583645f6" params={{}} tags={{span0: children => <span className="font-semibold text-error">{children}</span>, span1: children => <span className="font-semibold text-primary">{children}</span>}} /></p>
					<p><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.the.server.searches.storage.plus.the.selected.09fac1e7" /></p>
				</div>
			</Modal>

			<OptimizerPreview
				preview={preview?.response ?? null}
				priorityGroups={selectedGroups}
				getEffectName={(id) => descriptiveEquipmentEffectLabel(id, effects[id])}
				getArgumentName={(id) => troops[id]?.name?.trim() || `Unit ${id}`}
				selectedAlternative={selectedAlternative}
				onSelectAlternative={(index) => { setSelectedAlternative(index); setApplyError(null); }}
				onClose={closePreview}
				onApply={apply}
				onRegenerate={optimize}
				applying={applying}
				applyDisabled={!canApply || previewStale}
				stale={previewStale}
				applyError={applyError}
				optimizing={optimizing}
				optimizeError={optimizeError}
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
				{keys.length === 0 && <p className="py-5 text-center text-xs text-text-muted"><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.drag.battle.stats.here.6d8b8785" /></p>}
			</div>
		</div>
	);
}

function OptimizerPreview({
	preview,
	priorityGroups,
	getEffectName,
	getArgumentName,
	selectedAlternative,
	onSelectAlternative,
	onClose,
	onApply,
	onRegenerate,
	applying,
	applyDisabled,
	stale,
	applyError,
	optimizing,
	optimizeError,
}: {
	preview: EquipmentOptimizeResponse | null;
	priorityGroups: EquipmentPriorityGroup[];
	getEffectName: (id: number) => string;
	getArgumentName: (id: number) => string;
	selectedAlternative: number;
	onSelectAlternative: (index: number) => void;
	onClose: () => void;
	onApply: () => void;
	onRegenerate: () => void;
	applying: boolean;
	applyDisabled: boolean;
	stale: boolean;
	applyError: string | null;
	optimizing: boolean;
	optimizeError: string | null;
}) {
  const { t: localizeStatic } = useStaticLocale();
	const selected = preview?.alternatives[selectedAlternative] ?? preview?.proposed ?? null;
	const effectRows = (() => {
		if (!preview || !selected) return [];
		const keyFor = (effect: EquipmentEffectTotalV2) => effect.semanticKey || `${effect.definitionId}:${effect.argumentId ?? 0}`;
		const current = new Map(preview.current.effects.map((effect) => [keyFor(effect), effect]));
		const proposed = new Map(selected.effects.map((effect) => [keyFor(effect), effect]));
		const keys = Array.from(new Set([...current.keys(), ...proposed.keys()]));
		const rows = keys.map((key) => {
			const before = current.get(key);
			const after = proposed.get(key);
			const definitionId = after?.definitionId ?? before?.definitionId ?? 0;
			const argumentId = after?.argumentId ?? before?.argumentId;
			const priorityIndex = priorityGroups.findIndex((group) => group.effectIDs.includes(definitionId));
			const label = `${getEffectName(definitionId)}${argumentId ? ` · ${getArgumentName(argumentId)}` : ''}`;
			return {
				key, definitionId, label, priorityIndex,
				current: before?.value ?? 0,
				proposed: after?.value ?? 0,
				unit: after?.unit ?? before?.unit ?? 'percent',
				precision: after?.precision ?? before?.precision ?? 1,
				categorical: Boolean(after?.categorical ?? before?.categorical),
				argumentId,
				cap: after?.cap ?? before?.cap,
			};
		});
		for (let index = 0; index < priorityGroups.length; index += 1) {
			const group = priorityGroups[index];
			if (rows.some((row) => row.priorityIndex === index)) continue;
			rows.push({ key: `priority:${group.key}`, definitionId: 0, label: group.label, priorityIndex: index, current: 0, proposed: 0, unit: 'percent', precision: 1, categorical: false, argumentId: undefined, cap: undefined });
		}
		return rows.sort((left, right) => (
			(left.priorityIndex < 0 ? Number.MAX_SAFE_INTEGER : left.priorityIndex) - (right.priorityIndex < 0 ? Number.MAX_SAFE_INTEGER : right.priorityIndex)
			|| left.label.localeCompare(right.label)
			|| left.key.localeCompare(right.key)
		));
	})();
	const unavailableGroups = (() => {
		if (!preview || !selected) return [];
		return priorityGroups.filter((_, index) => effectRows.filter((row) => row.priorityIndex === index).every((row) => row.proposed === 0));
	})();
	const extractionNotices = selected ? equipmentExtractionCostNotices(selected.extractionCost) : [];
	const pointlessApply = Boolean(preview && selected && equipmentAlternativeApplyDisabled(preview.current, selected, preview.noUsefulChange));
	const pointlessApplyMessage = preview?.noUsefulChange
		? 'Your current loadout already has the strongest useful outcome for these priorities. Apply is disabled because this batch offers no useful stat or cost change.'
		: 'This selected alternative matches your current loadout. Choose a different useful alternative to apply a change.';
	return (
		<Modal
			isOpen={preview != null}
			onClose={onClose}
			title={localizeStatic("ui.equipment.components.equipmentOptimizer.title.reconfiguration.preview.36b7dacc")}
			maxWidth="5xl"
			footer={(
				<>
					<Button variant="ghost" onClick={onClose} disabled={applying}><LocalizedText messageKey="game.cancel" /></Button>
					<Button
						onClick={onApply}
						isLoading={applying}
						disabled={applyDisabled || pointlessApply}
						title={stale ? 'Regenerate this preview before applying it' : applyDisabled ? 'Connect the game before applying this loadout' : undefined}
					>
						{selected ? equipmentExtractionApplyLabel(selectedAlternative + 1, selected.extractionCost) : `Apply Alternative ${selectedAlternative + 1}`}
					</Button>
				</>
			)}
		>
			{preview && selected && (
				<div className="space-y-5">
					<div className="rounded-global border border-warning/40 bg-warning/10 px-3 py-2 text-sm text-warning">
						<p className="font-semibold"><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.review.extraction.costs.before.applying.4034bdf3" /></p>
						{extractionNotices.map((notice) => <p key={notice} className="mt-1 text-xs">{notice}</p>)}
					</div>
					{stale && (
						<div role="alert" className="flex flex-wrap items-center justify-between gap-3 rounded-global border border-warning/40 bg-warning/10 px-3 py-2 text-sm text-warning">
							<span><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.equipment.or.official.metadata.changed.after.this.78ae51fb" /></span>
							<Button size="sm" variant="outline" onClick={onRegenerate} disabled={applying || optimizing} isLoading={optimizing}><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.regenerate.1651031b" /></Button>
						</div>
					)}
					{optimizeError && <p role="alert" className="rounded-global border border-error/30 bg-error/10 px-3 py-2 text-sm text-error">{optimizeError}</p>}
					{applyError && <p role="alert" className="rounded-global border border-error/30 bg-error/10 px-3 py-2 text-sm text-error">{applyError}</p>}
					{unavailableGroups.length > 0 && (
						<p className="rounded-global border border-warning/30 bg-warning/10 px-3 py-2 text-xs text-warning">
							No eligible value was found for {unavailableGroups.map((group) => group.label).join(' · ')}. This is still the best available loadout.
						</p>
					)}
					{pointlessApply && (
						<p className="rounded-global border border-border-base bg-bg-app/40 px-3 py-2 text-sm text-text-muted">{pointlessApplyMessage}</p>
					)}
					<div>
						<div className="mb-2 flex items-center justify-between gap-3">
							<h4 className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.ranked.alternatives.3ebb915e" /></h4>
							<span className="text-xs text-text-muted">{preview.alternatives.filter((alternative) => alternative.useful !== false).length} useful {preview.alternatives.filter((alternative) => alternative.useful !== false).length === 1 ? 'choice' : 'choices'} · {preview.alternatives.length} shown · switching is instant</span>
						</div>
						<div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-5">
							{preview.alternatives.map((alternative, index) => (
								<button
									key={assignmentKey(alternative)}
									type="button"
									onClick={() => onSelectAlternative(index)}
									aria-pressed={selectedAlternative === index}
									className={`rounded-global border px-3 py-2 text-left transition ${selectedAlternative === index ? 'border-primary bg-primary/10' : 'border-border-base bg-bg-app/40 hover:border-primary/40'}`}
								>
									<span className="block text-xs font-bold text-text-main">#{index + 1}</span>
									<span className="mt-0.5 block text-[11px] leading-snug text-text-muted">{alternativeReason(alternative, preview.alternatives[0], preview.current, getEffectName, getArgumentName)}</span>
								</button>
							))}
						</div>
					</div>
					<div className="overflow-hidden rounded-global border border-border-base">
						<div className="grid grid-cols-[minmax(0,1fr)_6rem_6rem_6rem] bg-bg-card-hover px-3 py-2 text-[10px] font-bold uppercase tracking-wider text-text-muted"><span><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.stat.194535a5" /></span><span className="text-right"><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.current.total.c291c1ae" /></span><span className="text-right"><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.new.total.02a63a5e" /></span><span className="text-right"><LocalizedText messageKey="ui.equipment.components.equipmentOptimizer.difference.4d280a45" /></span></div>
						<div className="max-h-80 overflow-y-auto custom-scrollbar">
							{effectRows.map((row) => (
								<div key={row.key} className="grid grid-cols-[minmax(0,1fr)_6rem_6rem_6rem] border-t border-border-base/50 px-3 py-2 text-xs">
									<span className="min-w-0 text-text-main"><span className="block truncate">{row.label}</span><span className="block truncate text-[10px] text-text-muted">{row.priorityIndex >= 0 ? `Priority ${row.priorityIndex + 1}` : 'Other applicable stat'}{row.cap ? ` · Official cap ${formatStatValue(row.cap, row.unit, row.precision)}` : ''}</span></span>
									<span className="text-right font-mono text-text-muted">{formatStatValue(row.current, row.unit, row.precision, row.categorical, row.argumentId, getArgumentName)}</span>
									<span className="text-right font-mono font-semibold text-text-main">{formatStatValue(row.proposed, row.unit, row.precision, row.categorical, row.argumentId, getArgumentName)}</span>
									<span className={`text-right font-mono font-semibold ${row.proposed >= row.current ? 'text-success' : 'text-warning'}`}>{formatStatDifference(row.proposed - row.current, row.unit, row.precision, row.categorical)}</span>
								</div>
							))}
						</div>
					</div>
					<p className="text-xs text-text-muted">Weighted priority score (secondary): {formatNumber(preview.current.score)} → {formatNumber(selected.score)} ({formatSignedNumber(selected.score - preview.current.score)}).</p>
					<p className="text-xs text-text-muted">Candidates: {Object.entries(preview.candidates.equipmentBySlot).map(([slot, count]) => `slot ${slot}: ${count}`).join(' · ')} · gems: {preview.candidates.gems}</p>
				</div>
			)}
		</Modal>
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

function formatNumber(value: number): string {
	return Number.isInteger(value) ? value.toLocaleString() : value.toLocaleString(undefined, { maximumFractionDigits: 1 });
}

function formatSignedNumber(value: number): string {
	if (value === 0) return '0';
	return `${value > 0 ? '+' : ''}${formatNumber(value)}`;
}

function assignmentKey(loadout: EquipmentLoadoutV2): string {
	return [1, 2, 3, 4, 6].map((slot) => `e${slot}:${loadout.equipment[String(slot)] ?? 0}`).join('|')
		+ [1, 2, 3, 4].map((slot) => `|g${slot}:${loadout.gems[String(slot)] ?? 0}`).join('');
}

function formatStatValue(
	value: number,
	unit: EquipmentEffectTotalV2['unit'] | string,
	precision: number,
	categorical = false,
	argumentId?: number,
	getArgumentName: (id: number) => string = (id) => `Unit ${id}`,
): string {
	if (categorical || unit === 'categorical') return value === 0 ? '—' : argumentId ? getArgumentName(argumentId) : 'Present';
	const formatted = value.toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: Math.max(0, precision) });
	return `${formatted}${unit === 'percent' ? '%' : ''}`;
}

function formatStatDifference(value: number, unit: EquipmentEffectTotalV2['unit'] | string, precision: number, categorical = false): string {
	if (categorical || unit === 'categorical') return value > 0 ? 'Gained' : value < 0 ? 'Lost' : '—';
	const formatted = Math.abs(value).toLocaleString(undefined, { minimumFractionDigits: 0, maximumFractionDigits: Math.max(0, precision) });
	if (value === 0) return unit === 'percent' ? '0 pp' : '0';
	return `${value > 0 ? '+' : '-'}${formatted}${unit === 'percent' ? ' pp' : ''}`;
}

function alternativeReason(
	alternative: EquipmentLoadoutV2,
	best: EquipmentLoadoutV2,
	current: EquipmentLoadoutV2,
	getEffectName: (id: number) => string,
	getArgumentName: (id: number) => string,
): string {
	const baseline = alternative === best ? current : best;
	const keyFor = (effect: EquipmentEffectTotalV2) => effect.semanticKey || `${effect.definitionId}:${effect.argumentId ?? 0}`;
	const before = new Map(baseline.effects.map((effect) => [keyFor(effect), effect]));
	const after = new Map(alternative.effects.map((effect) => [keyFor(effect), effect]));
	const changes = Array.from(new Set([...before.keys(), ...after.keys()])).map((key) => {
		const oldEffect = before.get(key);
		const newEffect = after.get(key);
		const effect = newEffect ?? oldEffect!;
		const difference = (newEffect?.value ?? 0) - (oldEffect?.value ?? 0);
		const name = `${getEffectName(effect.definitionId)}${effect.argumentId ? ` · ${getArgumentName(effect.argumentId)}` : ''}`;
		return { name, difference, unit: effect.unit ?? 'percent', precision: effect.precision ?? 1, categorical: Boolean(effect.categorical) };
	}).filter((change) => change.difference !== 0)
		.sort((left, right) => Math.abs(right.difference) - Math.abs(left.difference) || left.name.localeCompare(right.name));
	const parts = changes.slice(0, 2).map((change) => `${formatStatDifference(change.difference, change.unit, change.precision, change.categorical)} ${change.name}`);
	const rubySaving = baseline.extractionCost.maximumRubySpend - alternative.extractionCost.maximumRubySpend;
	if (rubySaving > 0) parts.push(`saves up to ${rubySaving.toLocaleString()} rubies`);
	return parts.join(' · ') || alternative.reason || (alternative === best ? 'Strongest configured-priority result' : 'Distinct useful outcome');
}

function withTimeout<T>(promise: Promise<T>, timeoutMs: number, message: string): Promise<T> {
	return new Promise<T>((resolve, reject) => {
		const timer = window.setTimeout(() => reject(new Error(message)), timeoutMs);
		promise.then(
			(value) => { window.clearTimeout(timer); resolve(value); },
			(error) => { window.clearTimeout(timer); reject(error); },
		);
	});
}
