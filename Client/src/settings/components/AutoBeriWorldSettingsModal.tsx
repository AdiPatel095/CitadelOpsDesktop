import React, { useEffect, useMemo, useState } from 'react';
import { CalendarDays, Camera, Castle, Crosshair, FastForward, Hammer, Shield, Swords, Trash2, Users, Zap } from 'lucide-react';
import type { BuildingBlueprintDiffResponse, BuildingTargetCaptureMode } from '../../api/Contracts';
import { CitadelAPI } from '../../api/CitadelClient';
import { showTroopPicker } from '../../components/TroopPickerModal';
import { ATTACK_PRESETS_SECTION, parseAttackPresetDocument, summarizeAttackPreset } from '../../attackPresets/AttackPresetTypes';
import { Notifications } from '../../components/Notifications';
import { Badge, Button, Input, Select, SettingsModal, SettingsToggleRow } from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import { useMetadata } from '../../context/MetadataContext';
import { configurationSection } from '../Configuration';
import {
	AUTO_BERI_COIN_ATTACK_TOOLS,
	AUTO_BERI_DEFAULT_STABLE_LEVEL,
	AUTO_BERI_MAXIMUM_STABLE_LEVEL,
	AUTO_BERI_MINIMUM_STABLE_LEVEL,
	AUTO_BERI_TROOP_TRANSPORT_TIME_SKIPS,
	AUTO_BERI_WORLD_BLUEPRINTS_SECTION,
	DEFAULT_AUTO_BERI_WORLD_SETTINGS,
	parseAutoBeriBlueprintDocument,
	parseAutoBeriWorldSettings,
	type AutoBeriWorldSettings,
} from '../AutoBeriWorldClientState';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';

interface AutoBeriWorldSettingsModalProps {
	isOpen: boolean;
	onClose: () => void;
	onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
}

export const AutoBeriWorldSettingsModal: React.FC<AutoBeriWorldSettingsModalProps> = ({
	isOpen,
	onClose,
	onOpenFeatureSchedule,
}) => {
	const { state, configuration, updateConfiguration, captureBuildingTarget, submitIntent } = useCitadelAPI();
	const { troops } = useMetadata();
	const saved = useMemo(
		() => parseAutoBeriWorldSettings(configurationSection(configuration, 'automation.autoBeriWorld')),
		[configuration?.sections['automation.autoBeriWorld']],
	);
	const [settings, setSettings] = useState<AutoBeriWorldSettings>(DEFAULT_AUTO_BERI_WORLD_SETTINGS);
	const [saveError, setSaveError] = useState('');
	const [captureCastleId, setCaptureCastleId] = useState(0);
	const [capturing, setCapturing] = useState<BuildingTargetCaptureMode | null>(null);
	const [blueprintPreview, setBlueprintPreview] = useState<BuildingBlueprintDiffResponse | null>(null);
	const [blueprintBusy, setBlueprintBusy] = useState(false);
	const presetDocument = useMemo(
		() => parseAttackPresetDocument(configuration?.sections[ATTACK_PRESETS_SECTION]),
		[configuration?.sections],
	);
	const selectedPreset = presetDocument.presets.find((preset) => preset.id === settings.presetId);
	const presetSummary = selectedPreset ? summarizeAttackPreset(selectedPreset) : null;
	const gallantryBooster = state?.market?.boosters?.['24'];
	const gallantryBoosterExpiresAt = gallantryBooster?.expiresAt ? Date.parse(gallantryBooster.expiresAt) : 0;
	const gallantryBoosterActive = gallantryBooster?.permanent === true ||
		(Number.isFinite(gallantryBoosterExpiresAt) && gallantryBoosterExpiresAt > Date.now());
	const gallantryBoosterStatus = gallantryBoosterActive
		? `${gallantryBooster?.bonusPercent ? `+${gallantryBooster.bonusPercent}% · ` : ''}${gallantryBooster?.permanent
			? 'active'
			: `${formatBoosterRemaining(gallantryBoosterExpiresAt - Date.now())} left`}`
		: state?.market?.boostersObservedAt
			? 'No active boi ID 24 booster detected'
			: 'Waiting for the first authoritative boi booster snapshot';
	const foodTroopIDs = useMemo(() => Object.entries(troops).flatMap(([rawID, unit]) => {
		const unitID = Number(rawID);
		const foodSupply = metadataNumber(unit.foodSupply);
		const meadSupply = metadataNumber(unit.meadSupply);
		const beefSupply = metadataNumber(unit.beefSupply);
		return Number.isInteger(unitID) && unitID > 0 && foodSupply > 0 && meadSupply <= 0 && beefSupply <= 0
			? [unitID]
			: [];
	}), [troops]);
	const beriCastles = useMemo(() => Object.values(state?.castles ?? {})
		.filter((castle) => castle.kingdomId === 10)
		.sort((left, right) => left.id - right.id), [state?.castles]);
	const blueprintDocument = useMemo(
		() => parseAutoBeriBlueprintDocument(configuration?.sections[AUTO_BERI_WORLD_BLUEPRINTS_SECTION]),
		[configuration?.sections],
	);
	const activeBlueprint = blueprintDocument.blueprints[blueprintDocument.activeId];
	const savedBlueprints = Object.values(blueprintDocument.blueprints)
		.sort((left, right) => left.id.localeCompare(right.id));
	const captureCastle = beriCastles.find((castle) => castle.id === captureCastleId) ?? beriCastles[0];
	const target = activeBlueprint?.target;
	const targetCastle = target
		? beriCastles.find((castle) => castle.id === target.castleId)
		: undefined;

	useEffect(() => {
		if (!isOpen) return;
		setSettings(saved);
		setCaptureCastleId(activeBlueprint?.target.castleId ?? 0);
		setBlueprintPreview(null);
	}, [activeBlueprint?.target.castleId, isOpen, saved]);

	useEffect(() => {
		if (!isOpen || captureCastleId > 0 || beriCastles.length === 0) return;
		setCaptureCastleId(beriCastles[0].id);
	}, [beriCastles, captureCastleId, isOpen]);

	const castles = useMemo(() => Object.values(state?.castles ?? {})
		.filter((castle) => castle.kingdomId === 0)
		.sort((left, right) => left.id - right.id), [state?.castles]);
	const sourceOptions = castles.map((castle) => ({
		value: String(castle.id),
		label: castle.name || `Castle ${castle.id}`,
	}));
	const effectiveSourceID = settings.sourceCastleId || castles.find((castle) => castle.slotType === 1)?.id || 0;

	const updateNumber = (
		field: 'minTroopsToTransfer' | 'beriCastleId' | 'transferTroopId' | 'sourceCastleId' |
			'wireCastleId' | 'troopSpaceCheckIntervalSec' | 'attackCheckIntervalSec',
		value: string,
	) => {
		setSettings((current) => ({ ...current, [field]: Number.parseInt(value, 10) || 0 }));
	};

	const updateToolMinimum = (toolID: number, value: string) => {
		const minimum = Math.max(0, Number.parseInt(value, 10) || 0);
		setSettings((current) => ({
			...current,
			toolMinimums: { ...current.toolMinimums, [String(toolID)]: minimum },
		}));
	};

	const updateBuildNumberMap = (
		field: 'resourceReserves' | 'timeSkipReserve',
		key: string,
		value: string,
	) => {
		const amount = Math.max(0, Number.parseInt(value, 10) || 0);
		setSettings((current) => {
			const next = { ...current.build[field] };
			if (amount > 0) next[key] = amount;
			else delete next[key];
			return { ...current, build: { ...current.build, [field]: next } };
		});
	};

	const captureBlueprint = async (mode: BuildingTargetCaptureMode) => {
		if (!captureCastle || capturing || blueprintBusy) return;
		setCapturing(mode);
		try {
			const capturedTarget = await captureBuildingTarget({
				castleId: captureCastle.id,
				mode,
				expectedRevision: state?.revision,
			});
			const policy = {
				allowPremium: settings.build.allowPremium,
				resourceReserves: settings.build.resourceReserves,
			};
			const preview = await CitadelAPI.previewBuildingBlueprint({ target: capturedTarget, policy });
			if (!preview.compilable) {
				const issue = [...preview.normal.issues, ...preview.fixed.issues]
					.find((candidate) => candidate.severity === 'error');
				throw new Error(issue?.message ?? 'The captured Berimond blueprint cannot be compiled safely.');
			}
			await submitIntent('beri.blueprint.save', { target: capturedTarget, policy });
			setBlueprintPreview(preview);
			Notifications.success(`${captureModeLabel(mode)} Berimond target passed preflight and was queued for durable storage.`);
		} catch (error) {
			Notifications.error(error instanceof Error ? error.message : 'Could not capture the Berimond camp target.');
		} finally {
			setCapturing(null);
		}
	};

	const activateBlueprint = async (id: string) => {
		if (capturing || blueprintBusy || !blueprintDocument.blueprints[id]) return;
		setBlueprintBusy(true);
		try {
			await submitIntent('beri.blueprint.activate', { id });
			setCaptureCastleId(blueprintDocument.blueprints[id].target.castleId);
			setBlueprintPreview(null);
			Notifications.success(`${blueprintDocument.blueprints[id].name} activation queued.`);
		} catch {
			// submitIntent reports the operation error.
		} finally {
			setBlueprintBusy(false);
		}
	};

	const deactivateBlueprint = async () => {
		if (capturing || blueprintBusy) return;
		setBlueprintBusy(true);
		try {
			await submitIntent('beri.blueprint.activate', { id: '' });
			setBlueprintPreview(null);
			Notifications.success('The built-in Berimond target is being activated. Saved custom targets were retained.');
		} catch {
			// submitIntent reports the operation error.
		} finally {
			setBlueprintBusy(false);
		}
	};

	const pickTroop = async () => {
		const result = await showTroopPicker({
			mode: 'single',
			title: 'Troop type to transfer to Berimond',
			preselected: foodTroopIDs.includes(settings.transferTroopId) ? [settings.transferTroopId] : [],
			allowedUnitIds: foodTroopIDs,
		});
		if (typeof result === 'number' && result > 0) {
			setSettings((current) => ({ ...current, transferTroopId: result }));
		}
	};

	const save = () => {
		const normalized = parseAutoBeriWorldSettings({ ...settings, sourceCastleId: effectiveSourceID });
		setSaveError('');
		void updateConfiguration('automation.autoBeriWorld', normalized)
			.then(onClose)
			.catch((error) => setSaveError(error instanceof Error ? error.message : 'Could not save Berimond settings'));
	};

	return (
		<SettingsModal
			isOpen={isOpen}
			onClose={onClose}
			title="Auto Beri World"
			icon={<Swords className="h-5 w-5" />}
			description="Attack Berimond towers, bring the loot home, and spend only the camp's confirmed resources on the built-in or a captured build target."
			titleTrailing={(
					<Button
						variant="outline"
						size="sm"
						className="shrink-0"
						onClick={() => onOpenFeatureSchedule('autoBeriWorld', 'Auto Beri World')}
						leftIcon={<CalendarDays className="h-4 w-4" />}
					>
						Calendar
					</Button>
			)}
			maxWidth="4xl"
			onSave={save}
			saveLabel="Save"
		>
			<div className="space-y-5">
				<SettingsToggleRow
					title="Only run with a Gallantry booster"
					description={(
						<>
							Gates transfers, armorer purchases, camp setup, tower attacks, and construction unless boi booster ID 24 is active.
							<span className={`mt-1 block font-bold ${gallantryBoosterActive ? 'text-success' : 'text-text-muted'}`}>
								{gallantryBoosterStatus}
							</span>
						</>
					)}
					icon={<Zap className="h-4 w-4" />}
					checked={settings.requireActiveGallantryBooster}
					onChange={(checked) => setSettings((current) => ({
						...current,
						requireActiveGallantryBooster: checked,
					}))}
					tone={settings.requireActiveGallantryBooster && !gallantryBoosterActive ? 'warning' : 'default'}
				/>

				<div className="space-y-4 rounded-xl border border-border-base bg-bg-elevated/40 p-4">
					<div>
						<div className="flex items-center gap-2 text-sm font-black text-text-main">
							<Castle className="h-4 w-4 text-primary" /> Loot-funded camp construction
						</div>
						<p className="mt-1 text-xs text-text-muted">
							Uses the built-in exact camp layout by default, then builds and upgrades only after returned attacks increase the authoritative Berimond wood and stone balances. This lane never transports resources from another kingdom.
						</p>
					</div>

					<SettingsToggleRow
						title="Build and upgrade from returned loot"
						description="Reconcile the built-in target or an active custom target one confirmed construction operation at a time. When loot is short, the builder waits while attacks continue."
						icon={<Hammer className="h-4 w-4" />}
						checked={settings.build.enabled}
						onChange={(enabled) => setSettings((current) => ({
							...current,
							build: { ...current.build, enabled },
						}))}
					/>

					<div className="rounded-xl border border-primary/20 bg-primary/5 p-3">
						<div className="flex flex-wrap items-start justify-between gap-3">
							<div className="min-w-0 flex-1">
								<div className="flex flex-wrap items-center gap-2">
									<Badge variant={target ? 'outline' : 'success'}>{target ? 'Built-in available' : 'Active default'}</Badge>
									<span className="text-sm font-bold text-text-main">Built-in exact camp target</span>
								</div>
								<p className="mt-1 text-xs text-text-muted">
									17 ground tiles, 92 functional buildings, 64 decorations, and 22 fixed targets. All 84 small and large tents plus the Auxiliaries&apos; headquarters resolve to the terminal WoD in current official data.
								</p>
							</div>
							<label className="block w-40 shrink-0">
								<span className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-text-muted">Stable target</span>
								<Select
									value={String(settings.build.stableLevel || AUTO_BERI_DEFAULT_STABLE_LEVEL)}
									onChange={(value) => setSettings((current) => ({
										...current,
										build: {
											...current.build,
											stableLevel: Math.min(
												AUTO_BERI_MAXIMUM_STABLE_LEVEL,
												Math.max(AUTO_BERI_MINIMUM_STABLE_LEVEL, Number(value) || AUTO_BERI_DEFAULT_STABLE_LEVEL),
											),
										},
									}))}
									options={Array.from(
										{ length: AUTO_BERI_MAXIMUM_STABLE_LEVEL - AUTO_BERI_MINIMUM_STABLE_LEVEL + 1 },
										(_, index) => {
											const level = AUTO_BERI_MINIMUM_STABLE_LEVEL + index;
											return { value: String(level), label: `Level ${level}` };
										},
									)}
									menuGrowToViewport
								/>
							</label>
						</div>
						<p className="mt-2 text-[11px] text-text-muted">
							The Stable level is resolved to its official Berimond WoD when the built-in target is active. An already-higher Stable is retained rather than demolished or downgraded. Maximum Large-tent steps with official premium costs stay gated by “Allow premium costs.” Custom captured targets retain their own Stable definition.
						</p>
					</div>

					<div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_repeat(3,auto)] lg:items-end">
						<label className="block">
							<span className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-text-muted">Optional custom camp target</span>
							<Select
								value={captureCastle ? String(captureCastle.id) : ''}
								onChange={(value) => setCaptureCastleId(Number(value) || 0)}
								options={beriCastles.map((castle) => ({
									value: String(castle.id),
									label: `${castle.name?.trim() || `Camp ${castle.id}`} · ${castle.x}:${castle.y}`,
								}))}
								placeholder={beriCastles.length > 0 ? 'Choose Berimond camp' : 'Waiting for an owned Berimond camp'}
								disabled={beriCastles.length === 0 || capturing != null || blueprintBusy}
								menuGrowToViewport
							/>
						</label>
						<Button
							variant="outline"
							disabled={!captureCastle || capturing != null || blueprintBusy}
							isLoading={capturing === 'functional'}
							onClick={() => void captureBlueprint('functional')}
							leftIcon={<Hammer className="h-4 w-4" />}
						>
							Functional
						</Button>
						<Button
							variant="outline"
							disabled={!captureCastle || capturing != null || blueprintBusy}
							isLoading={capturing === 'layout'}
							onClick={() => void captureBlueprint('layout')}
							leftIcon={<Castle className="h-4 w-4" />}
						>
							Layout
						</Button>
						<Button
							variant="outline"
							disabled={!captureCastle || capturing != null || blueprintBusy}
							isLoading={capturing === 'exact'}
							onClick={() => void captureBlueprint('exact')}
							leftIcon={<Camera className="h-4 w-4" />}
						>
							Exact clone
						</Button>
					</div>

					{savedBlueprints.length > 0 ? (
						<div className="flex flex-wrap items-center gap-2">
							<span className="text-[11px] font-semibold text-text-muted">Saved targets:</span>
							{savedBlueprints.map((blueprint) => (
								<Button
									key={blueprint.id}
									size="sm"
									variant={blueprint.id === blueprintDocument.activeId ? 'primary' : 'ghost'}
									disabled={capturing != null || blueprintBusy}
									onClick={() => void activateBlueprint(blueprint.id)}
								>
									{blueprint.name}
								</Button>
							))}
						</div>
					) : null}

					{target ? (
						<div className="rounded-xl border border-primary/20 bg-primary/5 p-3">
							<div className="flex flex-wrap items-center justify-between gap-3">
								<div>
									<div className="flex flex-wrap items-center gap-2">
										<Badge variant="success">{captureModeLabel(target.mode)}</Badge>
										<span className="text-sm font-bold text-text-main">
											{targetCastle?.name?.trim() || `Camp ${target.castleId}`}
										</span>
									</div>
									<p className="mt-1 text-xs text-text-muted">
										Captured {formatDate(target.capturedAt)} from revision {target.revision.toLocaleString()}.
									</p>
								</div>
								<Button
									size="sm"
									variant="ghost"
									disabled={capturing != null || blueprintBusy}
									onClick={() => void deactivateBlueprint()}
									leftIcon={<Hammer className="h-3.5 w-3.5" />}
								>
									Use built-in default
								</Button>
							</div>
							<div className="mt-3 flex flex-wrap gap-2">
								<Badge variant="outline">{target.summary.groundCount} ground tiles</Badge>
								<Badge variant="outline">{target.summary.buildingCount} buildings</Badge>
								<Badge variant="outline">{target.summary.fixedCount} fixed</Badge>
								<Badge variant="outline">{target.summary.decorationCount} decorations</Badge>
								{blueprintPreview ? (
									<Badge variant="outline">
										Preflight: {blueprintPreview.satisfiedCount}/{blueprintPreview.targetCount} satisfied · {blueprintPreview.actionCount} actions
									</Badge>
								) : null}
							</div>
						</div>
					) : (
						<p className="rounded-xl border border-border-base bg-bg-app/35 px-3 py-2 text-xs text-text-muted">
							The built-in exact target is active. Capture a custom target only when you want to replace it; combat, transfers, and tool purchases remain independent.
						</p>
					)}

					<div className="grid gap-3 lg:grid-cols-3">
						<SettingsToggleRow
							title="Use construction time skips"
							description="Advance a confirmed build timer while preserving the selected skip reserves."
							icon={<FastForward className="h-4 w-4" />}
							checked={settings.build.allowTimeSkips}
							onChange={(allowTimeSkips) => setSettings((current) => ({
								...current,
								build: { ...current.build, allowTimeSkips },
							}))}
						/>
						<SettingsToggleRow
							title="Allow premium costs"
							description="Permit built-in or captured target steps that spend premium currency, including eligible Large-tent upgrades."
							icon={<Zap className="h-4 w-4" />}
							checked={settings.build.allowPremium}
							onChange={(allowPremium) => setSettings((current) => ({
								...current,
								build: { ...current.build, allowPremium },
							}))}
							tone="warning"
						/>
						<SettingsToggleRow
							title="Allow demolition"
							description="Permit exact reconciliation to remove unmanaged buildings when they cannot be moved or stored."
							icon={<Trash2 className="h-4 w-4" />}
							checked={settings.build.allowDemolition}
							onChange={(allowDemolition) => setSettings((current) => ({
								...current,
								build: { ...current.build, allowDemolition },
							}))}
							tone="warning"
						/>
					</div>

					<div className="grid gap-4 border-t border-border-base pt-4 lg:grid-cols-2">
						<div>
							<div className="flex items-center gap-2 text-xs font-bold uppercase tracking-wider text-text-muted">
								<Shield className="h-3.5 w-3.5" /> Camp resources kept in reserve
							</div>
							<div className="mt-2 grid grid-cols-2 gap-3">
								{[
									{ key: '3', label: 'Wood' },
									{ key: '4', label: 'Stone' },
								].map((resource) => (
									<label key={resource.key} className="block">
										<span className="mb-1 block text-[10px] font-semibold text-text-muted">{resource.label}</span>
										<Input
											type="number"
											min={0}
											value={settings.build.resourceReserves[resource.key] ?? 0}
											onChange={(event) => updateBuildNumberMap('resourceReserves', resource.key, event.target.value)}
										/>
									</label>
								))}
							</div>
							<p className="mt-2 text-[11px] text-text-muted">The builder spends only the amount above these Berimond camp floors.</p>
						</div>

						{settings.build.allowTimeSkips ? (
							<div>
								<div className="text-xs font-bold uppercase tracking-wider text-text-muted">Construction skips kept in reserve</div>
								<div className="mt-2 grid grid-cols-4 gap-2 sm:grid-cols-7">
									{AUTO_BERI_TROOP_TRANSPORT_TIME_SKIPS.map((skip) => (
										<label key={skip.id} className="block">
											<span className="mb-1 block text-center text-[10px] font-semibold text-text-muted">{skip.label}</span>
											<Input
												type="number"
												min={0}
												value={settings.build.timeSkipReserve[skip.id] ?? 0}
												onChange={(event) => updateBuildNumberMap('timeSkipReserve', skip.id, event.target.value)}
												className="px-2 text-center font-mono"
											/>
										</label>
									))}
								</div>
							</div>
						) : (
							<p className="self-center rounded-xl border border-border-base bg-bg-app/35 px-3 py-2 text-xs text-text-muted">
								Construction time skips are off. Active build timers are allowed to finish normally.
							</p>
						)}
					</div>
				</div>

				<div className="space-y-4 rounded-xl border border-border-base bg-bg-elevated/40 p-4">
					<div>
						<div className="flex items-center gap-2 text-sm font-black text-text-main">
							<Crosshair className="h-4 w-4 text-primary" /> Tower attack
						</div>
						<p className="mt-1 text-xs text-text-muted">
							Uses Berimond&apos;s find-next-tower command and the commanders assigned under Movement → Features.
						</p>
					</div>
					<div className="grid gap-4 md:grid-cols-2">
						<label className="block">
							<span className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-text-muted">Attack preset</span>
							<Select
								value={settings.presetId}
								onChange={(presetId) => setSettings((current) => ({ ...current, presetId }))}
								options={presetDocument.presets.map((preset) => ({ value: preset.id, label: preset.name }))}
								placeholder={presetDocument.presets.length > 0 ? 'Choose a CitadelOps preset' : 'Create an Attack Preset first'}
								disabled={presetDocument.presets.length === 0}
								menuGrowToViewport
							/>
						</label>
						<label className="block">
							<span className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-text-muted">Attack check interval</span>
							<Input
								type="number"
								min={30}
								max={3600}
								value={settings.attackCheckIntervalSec}
								onChange={(event) => updateNumber('attackCheckIntervalSec', event.target.value)}
								rightIcon={<span className="text-xs">s</span>}
							/>
						</label>
						<HorseTravelBoostSelect
							className="block md:col-span-2"
							value={settings.horseTravelBoostId}
							onChange={(horseTravelBoostId) => setSettings((current) => ({ ...current, horseTravelBoostId }))}
							description="The exact Berimond HBW ID and speed are resolved from the current Faction Stable level. Travel feather remains HBW -1."
						/>
					</div>
					{presetSummary ? (
						<div className="flex flex-wrap items-center gap-2 border-t border-border-base pt-3">
							<span className="mr-1 text-xs text-text-muted">Preset loadout</span>
							<Badge variant="outline">{presetSummary.waves} waves</Badge>
							<Badge variant="outline">{presetSummary.troops.toLocaleString()} troops</Badge>
							<Badge variant="outline">{presetSummary.tools.toLocaleString()} tools</Badge>
						</div>
					) : null}
				</div>

				<div className="space-y-4 rounded-xl border border-border-base bg-bg-elevated/40 p-4">
					<div>
						<div className="flex items-center gap-2 text-sm font-black text-text-main">
							<Hammer className="h-4 w-4 text-primary" /> Armorer tool minimums
						</div>
						<p className="mt-1 text-xs text-text-muted">
							An independent Auto Beri lane buys the shortage with coins in game-capped batches of up to 1,000. Set a tool to 0 to leave it unmanaged.
						</p>
					</div>
					<div className="grid gap-4 sm:grid-cols-3">
						{AUTO_BERI_COIN_ATTACK_TOOLS.map((tool) => (
							<label key={tool.id} className="block">
								<span className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-text-muted">
									{tool.name}
								</span>
								<Input
									type="number"
									min={0}
									value={settings.toolMinimums[String(tool.id)] ?? 0}
									onChange={(event) => updateToolMinimum(tool.id, event.target.value)}
									rightIcon={<span className="text-[10px] font-mono">#{tool.id}</span>}
								/>
							</label>
						))}
					</div>
					<p className="text-xs text-text-muted">
						Only scaling ladders, battering rams, and mantlets from the coin armorer are eligible.
					</p>
				</div>

				<div className="space-y-3">
					<SettingsToggleRow
						title="Use troop transport time skips"
						description="Apply the selected skip after a Berimond transfer, one command per confirmed response, until the troops arrive."
						icon={<FastForward className="h-4 w-4" />}
						checked={settings.useTroopTransportTimeSkips}
						onChange={(checked) => setSettings((current) => ({ ...current, useTroopTransportTimeSkips: checked }))}
					/>
					<label className="block">
						<span className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-text-muted">
							Troop transport skip
						</span>
						<Select
							value={settings.troopTransportTimeSkipId}
							onChange={(troopTransportTimeSkipId) => setSettings((current) => parseAutoBeriWorldSettings({
								...current,
								troopTransportTimeSkipId,
							}))}
							options={AUTO_BERI_TROOP_TRANSPORT_TIME_SKIPS.map((skip) => ({
								value: skip.id,
								label: `${skip.label} · ${skip.id}`,
							}))}
							menuGrowToViewport
						/>
					</label>
					<p className="text-xs text-text-muted">
						CitadelOps sends the exact <span className="font-mono">fuc</span> capacity with <span className="font-mono">kut</span>.
						When skipping is enabled, it applies the selected <span className="font-mono">msk</span> immediately, then checks a still-travelling transfer once per minute.
						The selection stays saved while skipping is off.
					</p>
				</div>

				<div className="space-y-1.5">
					<label className="text-xs font-bold uppercase tracking-wider text-text-muted">Berimond castle ID</label>
					<Input
						type="number"
						min={0}
						value={settings.beriCastleId || ''}
						onChange={(event) => updateNumber('beriCastleId', event.target.value)}
						placeholder="Auto-detect owned kingdom 10 camp"
					/>
					<p className="text-xs text-text-muted">Leave blank to use the owned Berimond camp automatically.</p>
				</div>

				<div className="grid gap-4 sm:grid-cols-2">
					<div className="space-y-1.5">
						<label className="text-xs font-bold uppercase tracking-wider text-text-muted">Check interval</label>
						<Input
							type="number"
							min={5}
							max={3600}
							value={settings.troopSpaceCheckIntervalSec}
							onChange={(event) => updateNumber('troopSpaceCheckIntervalSec', event.target.value)}
							rightIcon={<span className="text-xs">s</span>}
						/>
					</div>
					<div className="space-y-1.5">
						<label className="text-xs font-bold uppercase tracking-wider text-text-muted">Minimum transfer</label>
						<Input
							type="number"
							min={1}
							value={settings.minTroopsToTransfer}
							onChange={(event) => updateNumber('minTroopsToTransfer', event.target.value)}
						/>
					</div>
				</div>

				<div className="space-y-2">
					<label className="text-xs font-bold uppercase tracking-wider text-text-muted">Transfer troop</label>
					<div className="flex gap-2">
						<Input readOnly value={settings.transferTroopId || ''} placeholder="Official unit ID" />
						<Button variant="outline" leftIcon={<Users className="h-4 w-4" />} onClick={pickTroop}>Pick unit</Button>
					</div>
					<p className="text-xs text-text-muted">
						Only troops whose official upkeep is Food are eligible. Mead- and Beef-consuming troops are excluded.
					</p>
				</div>

				<div className="space-y-1.5">
					<label className="text-xs font-bold uppercase tracking-wider text-text-muted">Source castle</label>
					<Select
						value={effectiveSourceID > 0 ? String(effectiveSourceID) : ''}
						options={sourceOptions}
						onChange={(value) => updateNumber('sourceCastleId', value)}
						placeholder="Main castle"
					/>
				</div>

				<div className="space-y-1.5">
					<label className="text-xs font-bold uppercase tracking-wider text-text-muted">kut CID field</label>
					<Input
						type="number"
						value={settings.wireCastleId}
						onChange={(event) => setSettings((current) => ({
							...current,
							wireCastleId: Number.isFinite(Number(event.target.value)) ? Math.trunc(Number(event.target.value)) : -1,
						}))}
					/>
					<p className="text-xs text-text-muted">The game normally expects <span className="font-mono">-1</span>.</p>
				</div>

				{saveError && <p className="text-xs text-error">{saveError}</p>}
			</div>
		</SettingsModal>
	);
};

function metadataNumber(value: unknown): number {
	const parsed = Number(value);
	return Number.isFinite(parsed) ? parsed : 0;
}

function formatBoosterRemaining(milliseconds: number): string {
	const totalMinutes = Math.max(1, Math.ceil(milliseconds / 60_000));
	const hours = Math.floor(totalMinutes / 60);
	const minutes = totalMinutes % 60;
	if (hours > 0) return `${hours}h${minutes > 0 ? ` ${minutes}m` : ''}`;
	return `${minutes}m`;
}

function captureModeLabel(mode: string): string {
	if (mode === 'functional') return 'Functional';
	if (mode === 'layout' || mode === 'buildings') return 'Layout';
	return 'Exact clone';
}

function formatDate(value: string): string {
	const timestamp = Date.parse(value);
	return Number.isFinite(timestamp) ? new Date(timestamp).toLocaleString() : value;
}

export default AutoBeriWorldSettingsModal;
