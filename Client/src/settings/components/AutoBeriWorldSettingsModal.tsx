import React, { useEffect, useMemo, useState } from 'react';
import { CalendarDays, Crosshair, FastForward, Hammer, Swords, Users } from 'lucide-react';
import { showTroopPicker } from '../../components/TroopPickerModal';
import { ATTACK_PRESETS_SECTION, parseAttackPresetDocument, summarizeAttackPreset } from '../../attackPresets/AttackPresetTypes';
import { Badge, Button, Input, Select, SettingsModal, SettingsToggleRow } from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import { useMetadata } from '../../context/MetadataContext';
import { configurationSection } from '../Configuration';
import {
	AUTO_BERI_COIN_ATTACK_TOOLS,
	AUTO_BERI_TROOP_TRANSPORT_TIME_SKIPS,
	DEFAULT_AUTO_BERI_WORLD_SETTINGS,
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
	const { state, configuration, updateConfiguration } = useCitadelAPI();
	const { troops } = useMetadata();
	const saved = useMemo(
		() => parseAutoBeriWorldSettings(configurationSection(configuration, 'automation.autoBeriWorld')),
		[configuration?.sections['automation.autoBeriWorld']],
	);
	const [settings, setSettings] = useState<AutoBeriWorldSettings>(DEFAULT_AUTO_BERI_WORLD_SETTINGS);
	const [saveError, setSaveError] = useState('');
	const presetDocument = useMemo(
		() => parseAttackPresetDocument(configuration?.sections[ATTACK_PRESETS_SECTION]),
		[configuration?.sections],
	);
	const selectedPreset = presetDocument.presets.find((preset) => preset.id === settings.presetId);
	const presetSummary = selectedPreset ? summarizeAttackPreset(selectedPreset) : null;
	const foodTroopIDs = useMemo(() => Object.entries(troops).flatMap(([rawID, unit]) => {
		const unitID = Number(rawID);
		const foodSupply = metadataNumber(unit.foodSupply);
		const meadSupply = metadataNumber(unit.meadSupply);
		const beefSupply = metadataNumber(unit.beefSupply);
		return Number.isInteger(unitID) && unitID > 0 && foodSupply > 0 && meadSupply <= 0 && beefSupply <= 0
			? [unitID]
			: [];
	}), [troops]);

	useEffect(() => {
		if (isOpen) setSettings(saved);
	}, [isOpen, saved]);

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
			description="Transfer troops, replenish selected coin tools, open a free-resource camp when needed, and attack the next available Berimond tower."
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
			maxWidth="2xl"
			onSave={save}
			saveLabel="Save"
		>
			<div className="space-y-5">
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

export default AutoBeriWorldSettingsModal;
