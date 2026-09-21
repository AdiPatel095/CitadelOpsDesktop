import { useLocale as useStaticLocale } from "../../i18n/LocaleContext";
import { LocalizedText } from "../../i18n/LocalizedText";
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
	activateAutoBeriBlueprint,
	parseAutoBeriBlueprintDocument,
	parseAutoBeriWorldSettings,
	saveAutoBeriBlueprint,
	type AutoBeriWorldSettings,
} from '../AutoBeriWorldClientState';
import HorseTravelBoostSelect from './HorseTravelBoostSelect';
import { DailyAttackLimitField } from './DailyAttackLimitField';

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
  const { t: localizeStatic } = useStaticLocale();
	const { state, configuration, updateConfiguration, captureBuildingTarget } = useCitadelAPI();
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
			const savedBlueprints = configuration?.sections[AUTO_BERI_WORLD_BLUEPRINTS_SECTION];
			await updateConfiguration(
				AUTO_BERI_WORLD_BLUEPRINTS_SECTION,
				saveAutoBeriBlueprint(savedBlueprints, preview.target),
				savedBlueprints === undefined ? undefined : { expectedValue: savedBlueprints },
			);
			setBlueprintPreview(preview);
			Notifications.success(`${captureModeLabel(mode)} Berimond target passed preflight and was saved.`);
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
			const savedBlueprints = configuration?.sections[AUTO_BERI_WORLD_BLUEPRINTS_SECTION];
			await updateConfiguration(
				AUTO_BERI_WORLD_BLUEPRINTS_SECTION,
				activateAutoBeriBlueprint(savedBlueprints, id),
				savedBlueprints === undefined ? undefined : { expectedValue: savedBlueprints },
			);
			setCaptureCastleId(blueprintDocument.blueprints[id].target.castleId);
			setBlueprintPreview(null);
			Notifications.success(`${blueprintDocument.blueprints[id].name} activated.`);
		} catch (error) {
			Notifications.error(error instanceof Error ? error.message : 'Could not activate the Berimond blueprint.');
		} finally {
			setBlueprintBusy(false);
		}
	};

	const deactivateBlueprint = async () => {
		if (capturing || blueprintBusy) return;
		setBlueprintBusy(true);
		try {
			const savedBlueprints = configuration?.sections[AUTO_BERI_WORLD_BLUEPRINTS_SECTION];
			await updateConfiguration(
				AUTO_BERI_WORLD_BLUEPRINTS_SECTION,
				activateAutoBeriBlueprint(savedBlueprints, ''),
				savedBlueprints === undefined ? undefined : { expectedValue: savedBlueprints },
			);
			setBlueprintPreview(null);
			Notifications.success('The built-in Berimond target is active. Saved custom targets were retained.');
		} catch (error) {
			Notifications.error(error instanceof Error ? error.message : 'Could not activate the built-in Berimond target.');
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
			title={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.title.auto.beri.world.a579a63b")}
			icon={<Swords className="h-5 w-5" />}
			description={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.description.attack.berimond.towers.bring.the.loot.home.ff1b05e8")}
			titleTrailing={(
					<Button
						variant="outline"
						size="sm"
						className="shrink-0"
						onClick={() => onOpenFeatureSchedule('autoBeriWorld', 'Auto Beri World')}
						leftIcon={<CalendarDays className="h-4 w-4" />}
					>
						<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.calendar.d5d0a30b" /></Button>
			)}
			maxWidth="4xl"
			onSave={save}
			saveLabel="Save"
		>
			<div className="space-y-5">
				<SettingsToggleRow
					title={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.title.only.run.with.a.gallantry.booster.358a7233")}
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
							<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.uses.the.built.in.exact.camp.layout.6c194563" /></p>
					</div>

					<SettingsToggleRow
						title={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.title.auto.beri.builder.lane.10891189")}
						description={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.description.choose.whether.auto.beri.may.build.and.eecf5076")}
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
									<span className="text-sm font-bold text-text-main"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.built.in.exact.camp.target.696aa37c" /></span>
								</div>
								<p className="mt-1 text-xs text-text-muted">
									<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.17.ground.tiles.92.functional.buildings.64.9a88ca2d" /></p>
							</div>
							<label className="block w-40 shrink-0">
								<span className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.stable.target.6115bc56" /></span>
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
							<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.the.stable.level.is.resolved.to.its.7d6ad5dd" /></p>
					</div>

					<div className="grid gap-3 lg:grid-cols-[minmax(0,1fr)_repeat(3,auto)] lg:items-end">
						<label className="block">
							<span className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.optional.custom.camp.target.08f12769" /></span>
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
							<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.functional.b6656595" /></Button>
						<Button
							variant="outline"
							disabled={!captureCastle || capturing != null || blueprintBusy}
							isLoading={capturing === 'layout'}
							onClick={() => void captureBlueprint('layout')}
							leftIcon={<Castle className="h-4 w-4" />}
						>
							<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.layout.a5119091" /></Button>
						<Button
							variant="outline"
							disabled={!captureCastle || capturing != null || blueprintBusy}
							isLoading={capturing === 'exact'}
							onClick={() => void captureBlueprint('exact')}
							leftIcon={<Camera className="h-4 w-4" />}
						>
							<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.exact.clone.0174197d" /></Button>
					</div>

					{savedBlueprints.length > 0 ? (
						<div className="flex flex-wrap items-center gap-2">
							<span className="text-[11px] font-semibold text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.saved.targets.1b41cb7c" /></span>
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
									<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.use.built.in.default.e396d794" /></Button>
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
							<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.the.built.in.exact.target.is.active.07b82a6b" /></p>
					)}

					<div className="grid gap-3 lg:grid-cols-3">
						<SettingsToggleRow
							title={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.title.use.construction.time.skips.c7ae0ffb")}
							description={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.description.advance.a.confirmed.build.timer.while.preserving.3e2f7894")}
							icon={<FastForward className="h-4 w-4" />}
							checked={settings.build.allowTimeSkips}
							onChange={(allowTimeSkips) => setSettings((current) => ({
								...current,
								build: { ...current.build, allowTimeSkips },
							}))}
						/>
						<SettingsToggleRow
							title={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.title.allow.premium.costs.fd72d704")}
							description={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.description.permit.built.in.or.captured.target.steps.95dd7222")}
							icon={<Zap className="h-4 w-4" />}
							checked={settings.build.allowPremium}
							onChange={(allowPremium) => setSettings((current) => ({
								...current,
								build: { ...current.build, allowPremium },
							}))}
							tone="warning"
						/>
						<SettingsToggleRow
							title={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.title.allow.demolition.b9a49e66")}
							description={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.description.permit.exact.reconciliation.to.remove.unmanaged.buildings.93936bb7")}
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
							<p className="mt-2 text-[11px] text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.the.builder.spends.only.the.amount.above.d96d3273" /></p>
						</div>

						{settings.build.allowTimeSkips ? (
							<div>
								<div className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.construction.skips.kept.in.reserve.c78ae698" /></div>
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
								<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.construction.time.skips.are.off.active.build.c8682826" /></p>
						)}
					</div>
				</div>

				<div className="space-y-4 rounded-xl border border-border-base bg-bg-elevated/40 p-4">
					<div>
						<div className="flex items-center gap-2 text-sm font-black text-text-main">
							<Crosshair className="h-4 w-4 text-primary" /> Tower attack
						</div>
						<p className="mt-1 text-xs text-text-muted">
							<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.uses.berimond.s.find.next.tower.command.f09dfe35" /></p>
					</div>
					<div className="grid gap-4 md:grid-cols-2">
						<label className="block">
							<span className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.attack.preset.407b93e9" /></span>
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
							<span className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.attack.check.interval.bc3e388d" /></span>
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
							description={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.description.the.exact.berimond.hbw.id.and.speed.6618c35f")}
						/>
					</div>
					{presetSummary ? (
						<div className="flex flex-wrap items-center gap-2 border-t border-border-base pt-3">
							<span className="mr-1 text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.preset.loadout.4c1e2d30" /></span>
							<Badge variant="outline">{presetSummary.waves} waves</Badge>
							<Badge variant="outline">{presetSummary.troops.toLocaleString()} troops</Badge>
							<Badge variant="outline">{presetSummary.tools.toLocaleString()} tools</Badge>
						</div>
					) : null}
				</div>

				<DailyAttackLimitField
					value={settings.dailyAttackLimit}
					onChange={(dailyAttackLimit) => setSettings((current) => ({ ...current, dailyAttackLimit }))}
					serverState={state?.dailyAttacks}
				/>

				<div className="space-y-4 rounded-xl border border-border-base bg-bg-elevated/40 p-4">
					<div>
						<div className="flex items-center gap-2 text-sm font-black text-text-main">
							<Hammer className="h-4 w-4 text-primary" /> Armorer tool minimums
						</div>
						<p className="mt-1 text-xs text-text-muted">
							<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.an.independent.auto.beri.lane.buys.the.72da5b70" /></p>
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
						<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.only.scaling.ladders.battering.rams.and.mantlets.72f4a748" /></p>
				</div>

				<div className="space-y-3">
					<SettingsToggleRow
						title={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.title.use.troop.transport.time.skips.54aebc99")}
						description={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.description.apply.the.selected.skip.after.a.berimond.0e2de10a")}
						icon={<FastForward className="h-4 w-4" />}
						checked={settings.useTroopTransportTimeSkips}
						onChange={(checked) => setSettings((current) => ({ ...current, useTroopTransportTimeSkips: checked }))}
					/>
					<label className="block">
						<span className="mb-1.5 block text-xs font-bold uppercase tracking-wider text-text-muted">
							<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.troop.transport.skip.5fc75be9" /></span>
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
						CitadelOps sends the exact <span className="font-mono"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.fuc.63ca2042" /></span> capacity with <span className="font-mono"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.kut.341e7a8f" /></span>.
						When skipping is enabled, it applies the selected <span className="font-mono"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.msk.3521dbfc" /></span> immediately, then checks a still-travelling transfer once per minute.
						The selection stays saved while skipping is off.
					</p>
				</div>

				<div className="space-y-1.5">
					<label className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.berimond.castle.id.a5595160" /></label>
					<Input
						type="number"
						min={0}
						value={settings.beriCastleId || ''}
						onChange={(event) => updateNumber('beriCastleId', event.target.value)}
						placeholder={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.placeholder.auto.detect.owned.kingdom.10.camp.cabbe880")}
					/>
					<p className="text-xs text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.leave.blank.to.use.the.owned.berimond.31b2b0ae" /></p>
				</div>

				<div className="grid gap-4 sm:grid-cols-2">
					<div className="space-y-1.5">
						<label className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.check.interval.9a5266be" /></label>
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
						<label className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.minimum.transfer.ef38f61d" /></label>
						<Input
							type="number"
							min={1}
							value={settings.minTroopsToTransfer}
							onChange={(event) => updateNumber('minTroopsToTransfer', event.target.value)}
						/>
					</div>
				</div>

				<div className="space-y-2">
					<label className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.transfer.troop.2a96547d" /></label>
					<div className="flex gap-2">
						<Input readOnly value={settings.transferTroopId || ''} placeholder={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.placeholder.official.unit.id.60e3619e")} />
						<Button variant="outline" leftIcon={<Users className="h-4 w-4" />} onClick={pickTroop}><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.pick.unit.8ff02f40" /></Button>
					</div>
					<p className="text-xs text-text-muted">
						<LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.only.troops.whose.official.upkeep.is.food.29743507" /></p>
				</div>

				<div className="space-y-1.5">
					<label className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.source.castle.86d5a48e" /></label>
					<Select
						value={effectiveSourceID > 0 ? String(effectiveSourceID) : ''}
						options={sourceOptions}
						onChange={(value) => updateNumber('sourceCastleId', value)}
						placeholder={localizeStatic("ui.settings.components.autoBeriWorldSettingsModal.placeholder.main.castle.0e106dcc")}
					/>
				</div>

				<div className="space-y-1.5">
					<label className="text-xs font-bold uppercase tracking-wider text-text-muted"><LocalizedText messageKey="ui.settings.components.autoBeriWorldSettingsModal.kut.cid.field.580caaae" /></label>
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
