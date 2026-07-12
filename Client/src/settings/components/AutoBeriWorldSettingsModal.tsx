import React, { useEffect, useMemo, useState } from 'react';
import { CalendarDays, Save, Swords, Users } from 'lucide-react';
import { showTroopPicker } from '../../components/TroopPickerModal';
import { Button, Input, Modal, Select } from '../../components/ui';
import { useCitadelAPI } from '../../api/ApiContext';
import { configurationSection } from '../Configuration';
import {
	DEFAULT_AUTO_BERI_WORLD_SETTINGS,
	parseAutoBeriWorldSettings,
	type AutoBeriWorldSettings,
} from '../AutoBeriWorldClientState';

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
	const saved = useMemo(
		() => parseAutoBeriWorldSettings(configurationSection(configuration, 'automation.autoBeriWorld')),
		[configuration?.sections['automation.autoBeriWorld']],
	);
	const [settings, setSettings] = useState<AutoBeriWorldSettings>(DEFAULT_AUTO_BERI_WORLD_SETTINGS);
	const [saveError, setSaveError] = useState('');

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

	const updateNumber = (field: keyof AutoBeriWorldSettings, value: string) => {
		setSettings((current) => ({ ...current, [field]: Number.parseInt(value, 10) || 0 }));
	};

	const pickTroop = async () => {
		const result = await showTroopPicker({
			mode: 'single',
			title: 'Troop type to transfer to Berimond',
			preselected: settings.transferTroopId > 0 ? [settings.transferTroopId] : [],
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
		<Modal
			isOpen={isOpen}
			onClose={onClose}
			title={(
				<div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
					<div className="min-w-0">
						<span className="flex items-center gap-2 text-primary"><Swords className="h-5 w-5" />Auto Beri World</span>
						<p className="mt-1 text-sm font-normal text-text-muted">
							Configure deterministic Berimond capacity checks and troop transfers.
						</p>
					</div>
					<Button
						variant="outline"
						size="sm"
						className="shrink-0"
						onClick={() => onOpenFeatureSchedule('autoBeriWorld', 'Auto Beri World')}
						leftIcon={<CalendarDays className="h-4 w-4" />}
					>
						Calendar
					</Button>
				</div>
			)}
			maxWidth="md"
			footer={(
				<>
					<Button variant="ghost" onClick={onClose}>Cancel</Button>
					<Button variant="primary" leftIcon={<Save className="h-4 w-4" />} onClick={save}>Save</Button>
				</>
			)}
		>
			<div className="space-y-5">
				<p className="text-sm text-text-muted">
					CitadelOps refreshes transfer capacity with <span className="font-mono">fuc</span>, sends the exact returned amount with <span className="font-mono">kut</span>, then applies the fixed <span className="font-mono">msk</span> speed-up.
				</p>

				<div className="space-y-1.5">
					<label className="text-xs font-bold uppercase tracking-wider text-text-muted">Berimond castle ID</label>
					<Input
						type="number"
						min={1}
						value={settings.beriCastleId || ''}
						onChange={(event) => updateNumber('beriCastleId', event.target.value)}
						placeholder="Active Berimond castle CID"
					/>
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
		</Modal>
	);
};

export default AutoBeriWorldSettingsModal;
