import React, { useState, useEffect, useMemo, useRef } from 'react';
import {
	ArrowDown,
	ArrowUp,
	CheckCircle2,
	Download,
	FileJson,
	Microscope,
	MonitorPlay,
	Server,
	TriangleAlert,
	Upload,
} from 'lucide-react';
import { Icons } from '../components/Icons';
import { CitadelAPI } from '../api/CitadelClient';
import { useCitadelAPI } from '../api/ApiContext';
import type {
	GameServerEntry, BackgroundLoginStatus, BrowserInventory, SettingsBundleV1 } from '../api/Contracts';
import { Badge, Button, Input, PageHeader, SectionCard, Select, SettingsToggleRow } from '../components/ui';
import { asRecord, configurationSection, numericSetting } from '../settings/Configuration';
import {
	applyPortableClientPreferences,
	parseSettingsBundle,
	withPortableClientPreferences,
} from '../settings/SettingsTransfer';

interface AttackPriorityFeature {
	id: string;
	label: string;
	detail: string;
	defaultWeight: number;
}

type GameConnectionMode = 'full' | 'background';

const defaultAttackPriorityFeatures: AttackPriorityFeature[] = [
	{ id: 'autoTowers', label: 'Auto Towers', detail: 'Robber-baron and kingdom tower attacks', defaultWeight: 50 },
	{ id: 'riftMaiden', label: 'Rift Maiden Waves', detail: 'Shield-maiden probe and wave launches', defaultWeight: 50 },
	{ id: 'riftReplay', label: 'Rift Replays', detail: 'Captured Rift attack templates', defaultWeight: 50 },
];

function normalizedAttackPriority(value: unknown, fallback = 50): number {
	const numeric = Number(value);
	if (!Number.isFinite(numeric)) return fallback;
	return Math.min(100, Math.max(1, Math.round(numeric)));
}

function orderedAttackPriorityIDs(features: AttackPriorityFeature[], storedPriorities: Record<string, unknown>): string[] {
	return features
		.map((feature, index) => ({
			id: feature.id,
			index,
			weight: normalizedAttackPriority(storedPriorities[feature.id], feature.defaultWeight),
		}))
		.sort((left, right) => right.weight - left.weight || left.index - right.index)
		.map((feature) => feature.id);
}

function rankedAttackPriorities(featureIDs: string[]): Record<string, number> {
	return Object.fromEntries(featureIDs.map((featureID, index) => [featureID, Math.max(1, 100 - index)]));
}

const SettingsView: React.FC = () => {
	const { state, configuration, refreshConfiguration, submitIntent, updateConfiguration } = useCitadelAPI();
  const [minTimer, setMinTimer] = useState<string>('4.0');
  const [maxTimer, setMaxTimer] = useState<string>('6.0');
  const [upgradeEreDelayMs, setUpgradeEreDelayMs] = useState<string>('50');
  const [upgradeCoinThreshold, setUpgradeCoinThreshold] = useState<string>('0');
	const [attackPriorityOrder, setAttackPriorityOrder] = useState<string[]>([]);
	const [attackPriorityFeatures, setAttackPriorityFeatures] = useState<AttackPriorityFeature[]>(defaultAttackPriorityFeatures);
	const [draggedAttackPriorityID, setDraggedAttackPriorityID] = useState<string | null>(null);
	const [attackPriorityDropTargetID, setAttackPriorityDropTargetID] = useState<string | null>(null);
	const [browserInventory, setBrowserInventory] = useState<BrowserInventory | null>(null);
	const [browserSelectionPending, setBrowserSelectionPending] = useState(false);
	const [browserSelectionError, setBrowserSelectionError] = useState('');
	const [connectionModePending, setConnectionModePending] = useState(false);
	const [connectionModeError, setConnectionModeError] = useState('');
	const [backgroundLogin, setBackgroundLogin] = useState<BackgroundLoginStatus | null>(null);
	const [backgroundUsername, setBackgroundUsername] = useState('');
	const [backgroundPassword, setBackgroundPassword] = useState('');
	const [backgroundServer, setBackgroundServer] = useState('');
	const [gameServers, setGameServers] = useState<GameServerEntry[]>([]);

	useEffect(() => {
		let cancelled = false;
		// The official world directory, from the runtime's catalog, so the
		// server field offers real choices instead of a bare code box.
		CitadelAPI.getGameServers()
			.then((catalog) => { if (!cancelled) setGameServers(catalog.servers ?? []); })
			.catch(() => { /* free-text entry still works without the list */ });
		return () => { cancelled = true; };
	}, []);
	const [backgroundLoginPending, setBackgroundLoginPending] = useState(false);
	const [backgroundLoginError, setBackgroundLoginError] = useState('');
	const [backgroundLoginMessage, setBackgroundLoginMessage] = useState('');
	const [customBrowserPath, setCustomBrowserPath] = useState('');
	const [relogDelayMinutes, setRelogDelayMinutes] = useState('5');
	const [relogDelayError, setRelogDelayError] = useState('');
	const [settingsSaveError, setSettingsSaveError] = useState('');
	const [battleResearchPending, setBattleResearchPending] = useState(false);
	const [battleResearchError, setBattleResearchError] = useState('');
	const settingsFileInputRef = useRef<HTMLInputElement>(null);
	const [settingsTransferPending, setSettingsTransferPending] = useState<'export' | 'import' | null>(null);
	const [settingsTransferError, setSettingsTransferError] = useState('');
	const [settingsTransferStatus, setSettingsTransferStatus] = useState('');
	const schedulerConfiguration = useMemo(
		() => configurationSection(configuration, 'scheduler'),
		[configuration?.sections.scheduler],
	);
	const reconnectConfiguration = useMemo(
		() => configurationSection(configuration, 'session.reconnect'),
		[configuration?.sections['session.reconnect']],
	);
	const connectionConfiguration = useMemo(
		() => configurationSection(configuration, 'session.connection'),
		[configuration],
	);
	const battleResearchConfiguration = useMemo(
		() => configurationSection(configuration, 'research.battlePredictionBeta'),
		[configuration?.sections['research.battlePredictionBeta']],
	);
	const battleResearchEnabled = battleResearchConfiguration.enabled === true
		&& Number(battleResearchConfiguration.consentVersion) === 1;

	useEffect(() => {
		let active = true;
		void CitadelAPI.getBrowsers()
			.then((inventory) => {
				if (active) setBrowserInventory(inventory);
			})
			.catch((error) => {
				if (active) setBrowserSelectionError(error instanceof Error ? error.message : 'Could not discover browsers');
			});
		return () => {
			active = false;
		};
	}, []);

	useEffect(() => {
		let active = true;
		void CitadelAPI.getBackgroundLoginStatus()
			.then((status) => {
				if (!active) return;
				setBackgroundLogin(status);
				if (status.server) setBackgroundServer(status.server);
			})
			.catch((error) => {
				if (active) {
					setBackgroundLoginError(error instanceof Error ? error.message : 'Could not read the saved background login');
				}
			});
		return () => {
			active = false;
		};
	}, []);

  useEffect(() => {
		setMinTimer(numericSetting(schedulerConfiguration.minAttackDelay, 4).toFixed(1));
		setMaxTimer(numericSetting(schedulerConfiguration.maxAttackDelay, 6).toFixed(1));
		setUpgradeEreDelayMs(String(numericSetting(schedulerConfiguration.upgradeEreDelayMs, 50)));
		setUpgradeCoinThreshold(String(numericSetting(schedulerConfiguration.upgradeCoinThreshold, 0)));
		const storedPriorities = asRecord(schedulerConfiguration.attackPriorities);
		setAttackPriorityOrder(orderedAttackPriorityIDs(attackPriorityFeatures, storedPriorities));
	}, [attackPriorityFeatures, schedulerConfiguration]);

	useEffect(() => {
		const seconds = numericSetting(reconnectConfiguration.relogDelaySec, 300);
		setRelogDelayMinutes(String(Math.min(1_440, Math.max(1, Math.round(seconds / 60)))));
	}, [reconnectConfiguration]);

	useEffect(() => {
		if (!backgroundLogin?.server && typeof connectionConfiguration.server === 'string') {
			setBackgroundServer(connectionConfiguration.server);
		}
	}, [backgroundLogin?.server, connectionConfiguration.server]);

	const orderedAttackPriorityFeatures = useMemo(() => {
		const features = new Map(attackPriorityFeatures.map((feature) => [feature.id, feature]));
		return attackPriorityOrder.map((featureID) => features.get(featureID)).filter((feature): feature is AttackPriorityFeature => feature != null);
	}, [attackPriorityFeatures, attackPriorityOrder]);

	useEffect(() => {
		let active = true;
		void CitadelAPI.getIntentDefinitions()
			.then((definitions) => {
				if (!active) return;
				const modules = new Map<string, AttackPriorityFeature>();
				definitions.forEach((definition) => {
					const module = definition.attackModule;
					if (!module?.id || modules.has(module.id)) return;
					modules.set(module.id, {
						id: module.id,
						label: module.label || module.id,
						detail: module.description || definition.description,
						defaultWeight: normalizedAttackPriority(module.defaultWeight),
					});
				});
				if (modules.size > 0) {
					setAttackPriorityFeatures([...modules.values()].sort((left, right) => left.label.localeCompare(right.label)));
				}
			})
			.catch(() => undefined);
		return () => {
			active = false;
		};
	}, []);

  const saveSettings = (
    min: string,
    max: string,
    ereDelayMs?: string,
    coinThreshold?: string,
  ) => {
		setSettingsSaveError('');
		void updateConfiguration('scheduler', {
			...schedulerConfiguration,
      minAttackDelay: parseFloat(min),
      maxAttackDelay: parseFloat(max),
      upgradeEreDelayMs: parseInt(ereDelayMs ?? upgradeEreDelayMs, 10),
      upgradeCoinThreshold: parseFloat(coinThreshold ?? upgradeCoinThreshold),
		}).catch((error) => {
			setSettingsSaveError(error instanceof Error ? error.message : 'Could not save settings');
		});
  };

  const parsedCoinThreshold = useMemo(() => {
    const num = parseFloat(upgradeCoinThreshold);
    return Number.isFinite(num) && num >= 0 ? num : 0;
  }, [upgradeCoinThreshold]);

	const selectedBrowserID = browserInventory?.selected?.id ?? state?.session.browserId ?? '';
	const selectedBrowser = browserInventory?.available.find((browser) => browser.id === selectedBrowserID)
		?? browserInventory?.selected;
	const currentBrowserName = browserInventory?.current?.name ?? state?.session.browserName ?? 'the current browser';
	const browserOptions = useMemo(() => {
		const options = (browserInventory?.available ?? []).map((browser) => ({
			value: browser.id,
			label: browser.isDefault ? `${browser.name} (System default)` : browser.name,
		}));
		if (selectedBrowserID && !options.some((option) => option.value === selectedBrowserID)) {
			options.unshift({
				value: selectedBrowserID,
				label: browserInventory?.selected?.name ?? selectedBrowserID,
			});
		}
		return options;
	}, [browserInventory, selectedBrowserID]);
	const browserPlaceholder = browserInventory == null
		? 'Discovering browsers…'
		: browserOptions.length > 0
			? 'Select a browser'
			: 'No compatible browser detected';
	const configuredConnectionMode: GameConnectionMode = connectionConfiguration.mode === 'background'
		? 'background'
		: 'full';
	const activeConnectionMode: GameConnectionMode = state?.session.mode === 'background'
		? 'background'
		: 'full';
	const connectionModeRestartRequired = configuredConnectionMode !== activeConnectionMode;
	const backgroundLoginNeedsReauthorization = configuredConnectionMode === 'background'
		&& activeConnectionMode === 'background'
		&& !state?.session.loggedIn
		&& state?.session.detail?.toLowerCase().includes('saved login that has been disabled');

	const selectConnectionMode = (mode: GameConnectionMode) => {
		if (mode === configuredConnectionMode || connectionModePending) return;
		setConnectionModePending(true);
		setConnectionModeError('');
		void updateConfiguration('session.connection', {
			...connectionConfiguration,
			mode,
		})
			.catch((error) => {
				setConnectionModeError(error instanceof Error ? error.message : 'Could not save the game connection mode');
			})
			.finally(() => setConnectionModePending(false));
	};

	const reauthorizeBackgroundLogin = () => {
		if (connectionModePending) return;
		setConnectionModePending(true);
		setConnectionModeError('');
		void submitIntent('session.background.prepare')
			.then(() => submitIntent('session.start'))
			.catch((error) => {
				setConnectionModeError(error instanceof Error ? error.message : 'Could not re-enable the saved game login');
			})
			.finally(() => setConnectionModePending(false));
	};

	const setBattleResearchEnabled = (enabled: boolean) => {
		if (battleResearchPending || enabled === battleResearchEnabled) return;
		if (enabled && !window.confirm(
			'Enable Experimental Battle Research (Beta)?\n\n'
			+ 'For outgoing PvP attacks you launch, CitadelOps will poll movements, record your account/world/player identifiers plus the exact formation and combat context, '
			+ 'automatically send one military spy before and after battle, save an experimental pre-impact prediction and the final report, '
			+ 'then upload the completed bundle to CitadelOps training data. Spy missions use game resources and may be detected. '
			+ 'The calculator is incomplete and may be wrong. This feature respects Bot Lock and never launches an attack.',
		)) return;
		setBattleResearchPending(true);
		setBattleResearchError('');
		void updateConfiguration('research.battlePredictionBeta', {
			...battleResearchConfiguration,
			enabled,
			consentVersion: enabled ? 1 : Number(battleResearchConfiguration.consentVersion ?? 0),
			spyCount: 1,
		})
			.catch((error) => {
				setBattleResearchError(error instanceof Error ? error.message : 'Could not update battle research consent');
			})
			.finally(() => setBattleResearchPending(false));
	};

	const selectBrowser = (browser: string) => {
		if (!browser || browser === selectedBrowserID) return;
		setBrowserSelectionPending(true);
		setBrowserSelectionError('');
		void submitIntent('session.select_browser', { browser })
			.then(async () => {
				setBrowserInventory(await CitadelAPI.getBrowsers());
			})
			.catch((error) => {
				setBrowserSelectionError(error instanceof Error ? error.message : 'Could not select browser');
			})
			.finally(() => setBrowserSelectionPending(false));
	};

	const saveBackgroundLogin = (event: React.FormEvent<HTMLFormElement>) => {
		event.preventDefault();
		const username = backgroundUsername.trim();
		const server = backgroundServer.trim();
		if (!username || !backgroundPassword || !server || backgroundLoginPending) return;
		setBackgroundLoginPending(true);
		setBackgroundLoginError('');
		setBackgroundLoginMessage('');
		void CitadelAPI.configureBackgroundLogin({
			username,
			password: backgroundPassword,
			server,
		})
			.then(async (status) => {
				const savedServer = status.server ?? server.toUpperCase();
				await updateConfiguration('session.connection', {
					...connectionConfiguration,
					server: savedServer,
				});
				setBackgroundLogin(status);
				setBackgroundServer(savedServer);
				setBackgroundPassword('');
				setBackgroundLoginMessage('Background login saved. Start Bot can now connect without opening Full application mode.');
			})
			.catch((error) => {
				setBackgroundLoginError(error instanceof Error ? error.message : 'Could not save the background login');
			})
			.finally(() => setBackgroundLoginPending(false));
	};

	const selectCustomBrowser = () => {
		const executable = customBrowserPath.trim();
		if (!executable) return;
		selectBrowser(executable);
	};

	const saveRelogDelay = () => {
		const parsed = Number(relogDelayMinutes);
		const minutes = Math.min(1_440, Math.max(1, Number.isFinite(parsed) ? Math.round(parsed) : 5));
		setRelogDelayMinutes(String(minutes));
		setRelogDelayError('');
		void updateConfiguration('session.reconnect', {
			...reconnectConfiguration,
			relogDelaySec: minutes * 60,
		}).catch((error) => {
			setRelogDelayError(error instanceof Error ? error.message : 'Could not save the relog delay');
		});
	};

  const handleMinChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    let val = e.target.value;
    setMinTimer(val);
  };

  const handleMaxChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    let val = e.target.value;
    setMaxTimer(val);
  };

  const handleMinBlur = () => {
    let num = parseFloat(minTimer);
    let newVal = '4.0';
    if (isNaN(num) || num < 4.0) {
      newVal = '4.0';
    } else {
      newVal = num.toFixed(1);
    }
    setMinTimer(newVal);
    saveSettings(newVal, maxTimer);
  };

  const handleMaxBlur = () => {
    let num = parseFloat(maxTimer);
    let currentMin = parseFloat(minTimer);
    if (isNaN(currentMin)) currentMin = 4.0;

    let newVal = '6.0';
    if (isNaN(num) || num < currentMin) {
      newVal = Math.max(currentMin, 4.0).toFixed(1);
    } else {
      newVal = num.toFixed(1);
    }
    setMaxTimer(newVal);
    saveSettings(minTimer, newVal);
  };

  const handleUpgradeDelayBlur = () => {
    let num = parseInt(upgradeEreDelayMs, 10);
    let newVal = '50';
    if (isNaN(num) || num < 10) {
      newVal = '10';
    } else if (num > 5000) {
      newVal = '5000';
    } else {
      newVal = String(num);
    }
    setUpgradeEreDelayMs(newVal);
    saveSettings(minTimer, maxTimer, newVal);
  };

  const handleUpgradeCoinThresholdBlur = () => {
    let num = parseFloat(upgradeCoinThreshold);
    let newVal = '0';
    if (isNaN(num) || num < 0) {
      newVal = '0';
    } else {
      newVal = String(Math.floor(num));
    }
    setUpgradeCoinThreshold(newVal);
    saveSettings(minTimer, maxTimer, upgradeEreDelayMs, newVal);
  };

	const saveAttackPriorityOrder = (featureIDs: string[]) => {
		setSettingsSaveError('');
		setAttackPriorityOrder(featureIDs);
		void updateConfiguration('scheduler', {
			...schedulerConfiguration,
			attackPriorities: {
				...asRecord(schedulerConfiguration.attackPriorities),
				...rankedAttackPriorities(featureIDs),
			},
		}).catch((error) => {
			setSettingsSaveError(error instanceof Error ? error.message : 'Could not save attack priorities');
		});
	};

	const moveAttackPriority = (featureID: string, targetFeatureID: string) => {
		const sourceIndex = attackPriorityOrder.indexOf(featureID);
		const targetIndex = attackPriorityOrder.indexOf(targetFeatureID);
		if (sourceIndex < 0 || targetIndex < 0 || sourceIndex === targetIndex) return;
		const next = [...attackPriorityOrder];
		const [moved] = next.splice(sourceIndex, 1);
		next.splice(targetIndex, 0, moved);
		saveAttackPriorityOrder(next);
	};

	const moveAttackPriorityBy = (featureID: string, direction: -1 | 1) => {
		const sourceIndex = attackPriorityOrder.indexOf(featureID);
		const targetFeatureID = attackPriorityOrder[sourceIndex + direction];
		if (sourceIndex < 0 || targetFeatureID == null) return;
		moveAttackPriority(featureID, targetFeatureID);
	};

	const finishAttackPriorityDrag = () => {
		setDraggedAttackPriorityID(null);
		setAttackPriorityDropTargetID(null);
	};

	const exportSettings = async () => {
		setSettingsTransferPending('export');
		setSettingsTransferError('');
		setSettingsTransferStatus('');
		try {
			const bundle = withPortableClientPreferences(await CitadelAPI.exportSettings());
			downloadSettingsBundle(bundle);
			const sectionCount = Object.keys(bundle.configuration.sections).length;
			const preferenceCount = Object.keys(bundle.clientPreferences ?? {}).length;
			setSettingsTransferStatus(`Exported ${sectionCount} settings sections and ${preferenceCount} local preferences.`);
		} catch (error) {
			setSettingsTransferError(error instanceof Error ? error.message : 'Could not export settings.');
		} finally {
			setSettingsTransferPending(null);
		}
	};

	const importSettings = async (event: React.ChangeEvent<HTMLInputElement>) => {
		const file = event.currentTarget.files?.[0];
		event.currentTarget.value = '';
		if (!file) return;
		setSettingsTransferError('');
		setSettingsTransferStatus('');
		try {
			if (file.size > 32 * 1024 * 1024) {
				throw new Error('The selected settings file is larger than 32 MB.');
			}
			const bundle = parseSettingsBundle(await file.text());
			const sectionCount = Object.keys(bundle.configuration.sections).length;
			const preferenceCount = Object.keys(bundle.clientPreferences ?? {}).length;
			if (!window.confirm(
				`Import “${file.name}” with ${sectionCount} settings sections and ${preferenceCount} local preferences? `
				+ 'Existing values will be overwritten, and enabled automations will re-evaluate immediately.',
			)) return;

			setSettingsTransferPending('import');
			const result = await CitadelAPI.importSettings(bundle);
			const appliedPreferences = applyPortableClientPreferences(bundle.clientPreferences);
			await refreshConfiguration();
			setSettingsTransferStatus(
				`Imported ${result.importedSections} settings sections and ${appliedPreferences} local preferences. Reloading…`,
			);
			window.setTimeout(() => window.location.reload(), 800);
		} catch (error) {
			setSettingsTransferError(error instanceof Error ? error.message : 'Could not import settings.');
			setSettingsTransferPending(null);
		}
	};

  return (
    <div className="space-y-6 max-w-4xl mx-auto pb-12">
      <PageHeader
        className="mb-6"
        title="System Settings"
        description="Configure system behaviors, attack scheduling, and portable app preferences."
      />

      <div className="grid grid-cols-1 gap-6">
		<SectionCard
			variant="solid"
			title="Settings Import & Export"
			description="Move your CitadelOps setup between installations with one JSON file."
			icon={<span className="flex h-8 w-8 items-center justify-center rounded-lg bg-violet-500/10"><FileJson className="h-4 w-4 text-violet-400" /></span>}
			contentClassName="p-6 space-y-5"
		>
			<div className="grid gap-4 sm:grid-cols-2">
				<div className="rounded-global border border-border-base bg-bg-app/35 p-4">
					<h3 className="text-sm font-semibold text-text-main">Export this setup</h3>
					<p className="mt-1 text-xs leading-relaxed text-text-muted">
						Downloads automation settings, enabled states, schedules, priorities, presets, and portable interface preferences.
					</p>
					<Button
						type="button"
						variant="secondary"
						className="mt-4 w-full"
						leftIcon={<Download className="h-4 w-4" />}
						isLoading={settingsTransferPending === 'export'}
						disabled={settingsTransferPending != null}
						onClick={() => void exportSettings()}
					>
						Export settings
					</Button>
				</div>
				<div className="rounded-global border border-border-base bg-bg-app/35 p-4">
					<h3 className="text-sm font-semibold text-text-main">Import another setup</h3>
					<p className="mt-1 text-xs leading-relaxed text-text-muted">
						Validates the complete file before replacing matching settings on this installation.
					</p>
					<input
						ref={settingsFileInputRef}
						type="file"
						accept=".json,application/json"
						className="hidden"
						onChange={(event) => void importSettings(event)}
					/>
					<Button
						type="button"
						variant="outline"
						className="mt-4 w-full"
						leftIcon={<Upload className="h-4 w-4" />}
						isLoading={settingsTransferPending === 'import'}
						disabled={settingsTransferPending != null}
						onClick={() => settingsFileInputRef.current?.click()}
					>
						Import settings
					</Button>
				</div>
			</div>
			<div className="rounded-global border border-warning/25 bg-warning/5 px-4 py-3 text-xs leading-relaxed text-text-muted">
				Imported enabled automations and schedules take effect immediately. Login credentials, browser selection,
				logs, reports, and live game state stay on this computer and are never included.
			</div>
			{settingsTransferError && <p role="alert" className="text-xs font-medium text-error">{settingsTransferError}</p>}
			{settingsTransferStatus && <p role="status" className="text-xs font-medium text-success">{settingsTransferStatus}</p>}
		</SectionCard>

		<SectionCard
			variant="solid"
			title="Experimental Battle Research"
			description="Opt in to forward-tested battle predictions and privacy-bounded training capture."
			icon={<span className="flex h-8 w-8 items-center justify-center rounded-lg bg-fuchsia-500/10"><Microscope className="h-4 w-4 text-fuchsia-400" /></span>}
			actions={<Badge variant="warning">Beta</Badge>}
			contentClassName="p-6 space-y-4"
		>
			<SettingsToggleRow
				title="Contribute outgoing PvP battle trials"
				description="Observe attacks you already launch, save an untouched prediction before impact, and upload the completed pre-spy, formation, report, and post-spy bundle."
				icon={<Microscope className="h-4 w-4" />}
				checked={battleResearchEnabled}
				onChange={setBattleResearchEnabled}
				disabled={battleResearchPending}
				disabledReason="Saving consent…"
				tone="warning"
				ariaLabel="Contribute outgoing PvP battle trials"
			/>
			<div className="grid gap-3 md:grid-cols-3">
				<div className="rounded-global border border-border-base bg-bg-app/35 p-3">
					<p className="text-xs font-bold text-text-main">What is recorded</p>
					<p className="mt-1 text-[11px] leading-relaxed text-text-muted">Your account, world, and player identifiers; exact waves, troops, tools, commander context, target identity, two spy snapshots, the pre-impact estimate, and the final report.</p>
				</div>
				<div className="rounded-global border border-border-base bg-bg-app/35 p-3">
					<p className="text-xs font-bold text-text-main">What it does in-game</p>
					<p className="mt-1 text-[11px] leading-relaxed text-text-muted">Polls movements every five seconds and automatically sends one-agent military espionage before and after an eligible battle. It respects Bot Lock and never launches attacks.</p>
				</div>
				<div className="rounded-global border border-border-base bg-bg-app/35 p-3">
					<p className="text-xs font-bold text-text-main">Calculator maturity</p>
					<p className="mt-1 text-[11px] leading-relaxed text-text-muted">The visible baseline is intentionally labeled low confidence. It records unsupported effects for later models but does not claim exact accuracy today.</p>
				</div>
			</div>
			<p className="text-[11px] leading-relaxed text-text-muted">
				Disabling stops new capture, automatic spies, and pending uploads. Consent stays on this installation and is never included in settings export or import. Existing local trial records remain available for audit; completed uploads are retained as training data.
			</p>
			{battleResearchError && <p role="alert" className="text-xs font-medium text-error">{battleResearchError}</p>}
		</SectionCard>

		<SectionCard variant="solid" title="Game Connection" icon={<span className="flex h-8 w-8 items-center justify-center rounded-lg bg-sky-500/10"><Icons.Monitor className="h-4 w-4 text-sky-400" /></span>} contentClassName="p-6 space-y-6">
			<div>
				<h3 className="text-sm font-semibold text-text-main">How CitadelOps connects</h3>
				<p className="mt-1 max-w-3xl text-xs leading-relaxed text-text-muted">
					Choose whether CitadelOps opens the complete game or connects quietly in the background.
					The saved choice is used the next time CitadelOps starts.
				</p>
				<div role="radiogroup" aria-label="Game connection mode" className="mt-4 grid gap-3 lg:grid-cols-2">
					<button
						type="button"
						role="radio"
						aria-checked={configuredConnectionMode === 'full'}
						disabled={connectionModePending}
						onClick={() => selectConnectionMode('full')}
						className={`group rounded-global border p-4 text-left transition-colors disabled:cursor-wait disabled:opacity-70 ${
							configuredConnectionMode === 'full'
								? 'border-primary/60 bg-primary/10 ring-1 ring-primary/20'
								: 'border-border-base bg-surface-muted/30 hover:border-primary/35 hover:bg-surface-muted/60'
						}`}
					>
						<div className="flex items-start gap-3">
							<span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-sky-500/10 text-sky-400">
								<MonitorPlay className="h-5 w-5" />
							</span>
							<span className="min-w-0 flex-1">
								<span className="flex items-center justify-between gap-2">
									<span className="text-sm font-semibold text-text-main">Full application</span>
									{configuredConnectionMode === 'full' && <CheckCircle2 className="h-4 w-4 shrink-0 text-primary" />}
								</span>
								<span className="mt-1 block text-xs leading-relaxed text-text-muted">
									Opens the game tab so you can play along, watch actions happen, and use the complete game interface.
								</span>
								<span className="mt-3 flex items-start gap-2 rounded-lg border border-warning/20 bg-warning/5 px-3 py-2 text-[11px] leading-relaxed text-warning">
									<TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
									Uses considerably more processor and memory resources and may make your computer feel slower.
								</span>
							</span>
						</div>
					</button>

					<button
						type="button"
						role="radio"
						aria-checked={configuredConnectionMode === 'background'}
						disabled={connectionModePending}
						onClick={() => selectConnectionMode('background')}
						className={`group rounded-global border p-4 text-left transition-colors disabled:cursor-wait disabled:opacity-70 ${
							configuredConnectionMode === 'background'
								? 'border-primary/60 bg-primary/10 ring-1 ring-primary/20'
								: 'border-border-base bg-surface-muted/30 hover:border-primary/35 hover:bg-surface-muted/60'
						}`}
					>
						<div className="flex items-start gap-3">
							<span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-emerald-500/10 text-emerald-400">
								<Server className="h-5 w-5" />
							</span>
							<span className="min-w-0 flex-1">
								<span className="flex items-center justify-between gap-2">
									<span className="text-sm font-semibold text-text-main">Background only</span>
									{configuredConnectionMode === 'background' && <CheckCircle2 className="h-4 w-4 shrink-0 text-primary" />}
								</span>
								<span className="mt-1 block text-xs leading-relaxed text-text-muted">
									Connects directly to the game server without opening Chromium or a game tab. Automations and live state continue with much lower resource use.
								</span>
				<span className="mt-3 block text-[11px] leading-relaxed text-text-muted">
					Uses the login and server selection saved on this computer, then derives the current client build and remaining WebSocket handshake automatically.
				</span>
							</span>
						</div>
					</button>
				</div>
				{connectionModeRestartRequired ? (
					<p role="status" className="mt-3 text-xs font-medium text-warning">
						Currently running {activeConnectionMode === 'full' ? 'Full application' : 'Background only'} mode.
						Restart CitadelOps to use {configuredConnectionMode === 'full' ? 'Full application' : 'Background only'} mode.
					</p>
				) : (
					<p className="mt-3 text-xs text-text-muted">Connection mode changes are applied after restarting CitadelOps.</p>
				)}
				{backgroundLoginNeedsReauthorization && (
					<div className="mt-3 flex flex-wrap items-center gap-3 rounded-lg border border-warning/25 bg-warning/5 px-3 py-2.5">
						<p className="min-w-0 flex-1 text-xs leading-relaxed text-warning">
							The protected saved login is present but disabled. Re-enable it explicitly to retry Background mode without exposing or re-entering the saved password.
						</p>
						<Button type="button" variant="secondary" disabled={connectionModePending} onClick={reauthorizeBackgroundLogin}>
							{connectionModePending ? 'Re-enabling…' : 'Re-enable saved login'}
						</Button>
					</div>
				)}
				{connectionModeError && <p role="alert" className="mt-2 text-xs font-medium text-error">{connectionModeError}</p>}
				{configuredConnectionMode === 'background' && (
					<form onSubmit={saveBackgroundLogin} className="mt-5 rounded-global border border-border-base bg-bg-app/45 p-4">
						<div className="flex flex-col gap-2 sm:flex-row sm:items-start sm:justify-between">
							<div>
								<h3 className="text-sm font-semibold text-text-main">Background game login</h3>
								<p className="mt-1 max-w-3xl text-xs leading-relaxed text-text-muted">
									Enter the login and server explicitly. The server code determines the official WebSocket address;
									CitadelOps derives only the remaining non-secret handshake values.
								</p>
							</div>
							{backgroundLogin?.configured && (
								<span className="inline-flex shrink-0 items-center gap-1.5 rounded-full border border-success/25 bg-success/10 px-2.5 py-1 text-[11px] font-semibold text-success">
									<CheckCircle2 className="h-3.5 w-3.5" />
									Saved for {backgroundLogin.server}
								</span>
							)}
						</div>
						<div className="mt-4 grid gap-3 lg:grid-cols-3">
							<div>
								<label htmlFor="background-login-username" className="mb-1.5 block text-[10px] font-bold uppercase tracking-wider text-text-muted">
									Username
								</label>
								<Input
									id="background-login-username"
									name="username"
									autoComplete="username"
									value={backgroundUsername}
									onChange={(event) => setBackgroundUsername(event.target.value)}
									placeholder="Game username"
									disabled={backgroundLoginPending}
								/>
							</div>
							<div>
								<label htmlFor="background-login-password" className="mb-1.5 block text-[10px] font-bold uppercase tracking-wider text-text-muted">
									Password
								</label>
								<Input
									id="background-login-password"
									name="password"
									type="password"
									autoComplete="current-password"
									value={backgroundPassword}
									onChange={(event) => setBackgroundPassword(event.target.value)}
									placeholder="Game password"
									disabled={backgroundLoginPending}
								/>
							</div>
							<div>
								<label htmlFor="background-login-server" className="mb-1.5 block text-[10px] font-bold uppercase tracking-wider text-text-muted">
									Server
								</label>
								<Input
									id="background-login-server"
									name="server"
									autoCapitalize="characters"
									value={backgroundServer}
									onChange={(event) => setBackgroundServer(event.target.value)}
									placeholder="US1"
									className="font-mono uppercase"
									disabled={backgroundLoginPending}
									list="background-login-server-options"
								/>
								<datalist id="background-login-server-options">
									{gameServers.map((server) => (
										<option key={server.code} value={server.code}>{server.label}</option>
									))}
								</datalist>
								<p className="mt-1.5 text-[11px] text-text-muted">
									{gameServers.length > 0
										? 'Pick the world you play on; the directory resolves its connection address automatically.'
										: 'Use the world code shown by the game, such as US1, GB1, or DE1.'}
								</p>
							</div>
						</div>
						<div className="mt-4 flex flex-col gap-3 sm:flex-row sm:items-center">
							<Button
								type="submit"
								variant="secondary"
								isLoading={backgroundLoginPending}
								disabled={backgroundLoginPending || !backgroundUsername.trim() || !backgroundPassword || !backgroundServer.trim()}
							>
								Save background login
							</Button>
							<p className="text-[11px] leading-relaxed text-text-muted">
								Saved only in this profile's protected session file and excluded from settings exports and operation receipts.
							</p>
						</div>
						{backgroundLoginError && <p role="alert" className="mt-3 text-xs font-medium text-error">{backgroundLoginError}</p>}
						{backgroundLoginMessage && <p role="status" className="mt-3 text-xs font-medium text-success">{backgroundLoginMessage}</p>}
					</form>
				)}
			</div>

			<div className="border-t border-border-base pt-5">
				<div>
					<h3 className="text-sm font-semibold text-text-main mb-1">Full application browser</h3>
						<p className="text-xs text-text-muted mb-4">
							Full application mode starts with your system-default compatible Chromium browser, or the only compatible
							browser when one is installed. A saved choice is used after the next app restart, with a
							dedicated CitadelOps profile that leaves your normal browser profile untouched.
						</p>
					</div>

				<div className="w-full sm:max-w-[520px]">
					<label className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
						Browser
					</label>
						<Select
							value={selectedBrowserID}
							options={browserOptions}
							onChange={selectBrowser}
							placeholder={browserPlaceholder}
							icon={<Icons.Monitor className="w-4 h-4" />}
							disabled={browserSelectionPending || browserInventory == null}
						/>
						{selectedBrowser?.executablePath && (
							<p className="mt-2 text-[11px] text-text-muted font-mono break-all">{selectedBrowser.executablePath}</p>
					)}
					{browserInventory != null && browserInventory.available.length > 0 && !selectedBrowser && (
						<p className="mt-2 text-xs text-text-muted">
							Detected: {browserInventory.available.map((browser) => browser.name).join(', ')}
						</p>
					)}
						{browserInventory?.restartRequired && (
							<p role="status" className="mt-2 text-xs text-warning">
								Currently using {currentBrowserName}. Restart CitadelOps to switch to {browserInventory.selected?.name}.
							</p>
						)}
						{browserSelectionError && (
						<p className="mt-2 text-xs text-error">{browserSelectionError}</p>
					)}
						<div className="mt-4 border-t border-border-base pt-4">
						<label className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
							Custom Chromium executable
						</label>
						<div className="flex flex-col gap-2 sm:flex-row">
							<Input
								aria-label="Custom Chromium executable"
								value={customBrowserPath}
								onChange={(event) => setCustomBrowserPath(event.target.value)}
								placeholder="Absolute path or executable command"
								className="font-mono"
								disabled={browserSelectionPending}
							/>
							<Button
								variant="secondary"
								onClick={selectCustomBrowser}
								disabled={browserSelectionPending || !customBrowserPath.trim()}
								className="shrink-0"
							>
								Use executable
							</Button>
						</div>
							<p className="mt-2 text-xs text-text-muted">
								Use this for Chromium-based builds that are not detected automatically.
							</p>
						</div>
						<div className="mt-4 border-t border-border-base pt-4">
							<h3 className="text-sm font-semibold text-text-main">Relog Attempt Delay</h3>
							<p className="mt-1 text-xs leading-relaxed text-text-muted">
								Wait this long after an automatic socket loss, or after a game login cooldown ends,
								before reconnecting and attempting the saved login again.
							</p>
							<div className="mt-3 w-full sm:max-w-[200px]">
								<label htmlFor="relog-attempt-delay" className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
									Delay (Minutes)
								</label>
								<Input
									id="relog-attempt-delay"
									type="number"
									min="1"
									max="1440"
									step="1"
									value={relogDelayMinutes}
									onChange={(event) => setRelogDelayMinutes(event.target.value)}
									onBlur={saveRelogDelay}
									className="font-mono"
									rightIcon={<span className="text-xs">min</span>}
								/>
							</div>
							<p className="mt-2 text-xs text-text-muted">Default: 5 minutes. Allowed range: 1 minute to 24 hours.</p>
							{relogDelayError && <p role="alert" className="mt-2 text-xs font-medium text-error">{relogDelayError}</p>}
						</div>
					</div>
				</div>
		</SectionCard>

        <SectionCard variant="solid" title="Attack Scheduler" icon={<span className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/10"><Icons.Activity className="h-4 w-4 text-indigo-400" /></span>} contentClassName="p-6 space-y-8">
			{settingsSaveError && <p className="text-xs text-error">{settingsSaveError}</p>}
            <div className="space-y-4">
              <div>
                <h3 className="text-sm font-semibold text-text-main mb-1">Random Attack Timer Range</h3>
                <p className="text-xs text-text-muted mb-4">
                  Set the minimum and maximum delay (in seconds) between sent attacks.
                  Minimum allowed value is 4.0s to avoid rate limiting.
                </p>
              </div>

              <div className="flex flex-col sm:flex-row sm:items-center gap-4">
                <div className="relative flex-1 w-full sm:max-w-[200px]">
                  <label htmlFor="min-attack-delay" className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
                    Min Delay (Sec)
                  </label>
                  <Input
					id="min-attack-delay"
                    type="number"
                    step="0.1"
                    min="4.0"
                    value={minTimer}
                    onChange={handleMinChange}
                    onBlur={handleMinBlur}
                    className="font-mono"
                    rightIcon={<span className="text-xs">s</span>}
                  />
                </div>

                <div className="hidden sm:block mt-6 text-text-muted font-bold">-</div>

                <div className="relative flex-1 w-full sm:max-w-[200px]">
                  <label htmlFor="max-attack-delay" className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
                    Max Delay (Sec)
                  </label>
                  <Input
					id="max-attack-delay"
                    type="number"
                    step="0.1"
                    min={minTimer}
                    value={maxTimer}
                    onChange={handleMaxChange}
                    onBlur={handleMaxBlur}
                    className="font-mono"
                    rightIcon={<span className="text-xs">s</span>}
                  />
                </div>
              </div>
            </div>

            <div className="h-px bg-border-base w-full"></div>

			<div className="space-y-4">
				<div>
					<h3 className="text-sm font-semibold text-text-main mb-1">Automated Attack Priority</h3>
					<p className="text-xs text-text-muted mb-4">
						Drag modules into priority order, highest first. Waiting time gradually raises older work; manual and scheduled attacks retain protected priority.
					</p>
				</div>

				<div className="space-y-2" role="list" aria-label="Automated attack priority order">
					{orderedAttackPriorityFeatures.map((feature, index) => (
						<div
							key={feature.id}
							role="listitem"
							draggable
							onDragStart={(event) => {
								event.dataTransfer.effectAllowed = 'move';
								event.dataTransfer.setData('text/plain', feature.id);
								setDraggedAttackPriorityID(feature.id);
							}}
							onDragOver={(event) => {
								event.preventDefault();
								event.dataTransfer.dropEffect = 'move';
								if (feature.id !== draggedAttackPriorityID) setAttackPriorityDropTargetID(feature.id);
							}}
							onDragLeave={(event) => {
								if (!event.currentTarget.contains(event.relatedTarget as Node | null)) setAttackPriorityDropTargetID(null);
							}}
							onDrop={(event) => {
								event.preventDefault();
								const sourceID = draggedAttackPriorityID ?? event.dataTransfer.getData('text/plain');
								if (sourceID) moveAttackPriority(sourceID, feature.id);
								finishAttackPriorityDrag();
							}}
							onDragEnd={finishAttackPriorityDrag}
							className={`flex cursor-grab items-center gap-3 rounded-global border bg-bg-app/45 p-3 transition-colors active:cursor-grabbing ${
								draggedAttackPriorityID === feature.id
									? 'border-primary/40 opacity-45'
									: attackPriorityDropTargetID === feature.id
										? 'border-primary bg-primary/10'
										: 'border-border-base hover:border-primary/30'
							}`}
						>
							<Icons.GripVertical className="h-5 w-5 shrink-0 text-text-muted" aria-hidden="true" />
							<span className="flex h-7 w-7 shrink-0 items-center justify-center rounded-full bg-bg-card text-xs font-bold tabular-nums text-primary ring-1 ring-border-base">
								{index + 1}
							</span>
							<span className="min-w-0 flex-1">
								<span className="block text-xs font-bold text-text-main">{feature.label}</span>
								<span className="mt-0.5 block text-[11px] leading-4 text-text-muted">{feature.detail}</span>
							</span>
							<span className="flex shrink-0 items-center gap-1">
								<button
									type="button"
									disabled={index === 0}
									onClick={() => moveAttackPriorityBy(feature.id, -1)}
									className="rounded-md p-1.5 text-text-muted transition-colors hover:bg-primary/10 hover:text-primary disabled:pointer-events-none disabled:opacity-25"
									aria-label={`Move ${feature.label} up`}
								>
									<ArrowUp className="h-3.5 w-3.5" />
								</button>
								<button
									type="button"
									disabled={index === orderedAttackPriorityFeatures.length - 1}
									onClick={() => moveAttackPriorityBy(feature.id, 1)}
									className="rounded-md p-1.5 text-text-muted transition-colors hover:bg-primary/10 hover:text-primary disabled:pointer-events-none disabled:opacity-25"
									aria-label={`Move ${feature.label} down`}
								>
									<ArrowDown className="h-3.5 w-3.5" />
								</button>
							</span>
						</div>
					))}
				</div>
			</div>

        </SectionCard>

        <SectionCard variant="solid" title="Equipment Upgrades" icon={<span className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-500/10"><Icons.Shield className="h-4 w-4 text-emerald-400" /></span>} contentClassName="p-6 space-y-4">
            <div>
              <h3 className="text-sm font-semibold text-text-main mb-1">Upgrade Step Delay</h3>
              <p className="text-xs text-text-muted mb-4">
                Pause between each <span className="font-mono">ere</span> command when bulk-upgrading equipment or gems (10–5000 ms).
              </p>
            </div>

            <div className="relative flex-1 w-full sm:max-w-[200px]">
              <label htmlFor="upgrade-step-delay" className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
                Delay (ms)
              </label>
              <Input
				id="upgrade-step-delay"
                type="number"
                step="1"
                min="10"
                max="5000"
                value={upgradeEreDelayMs}
                onChange={(e) => setUpgradeEreDelayMs(e.target.value)}
                onBlur={handleUpgradeDelayBlur}
                className="font-mono"
                rightIcon={<span className="text-xs">ms</span>}
              />
            </div>

            <div>
              <h3 className="text-sm font-semibold text-text-main mb-1">Coin Reserve Threshold</h3>
              <p className="text-xs text-text-muted mb-4">
                Block equipment and gem upgrades when your coin balance is at or below this reserve.
              </p>
            </div>

            <div className="relative flex-1 w-full sm:max-w-[200px]">
              <label htmlFor="upgrade-coin-reserve" className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
                Minimum Coins
              </label>
              <Input
				id="upgrade-coin-reserve"
                type="number"
                step="1"
                min="0"
                value={upgradeCoinThreshold}
                onChange={(e) => setUpgradeCoinThreshold(e.target.value)}
                onBlur={handleUpgradeCoinThresholdBlur}
                className="font-mono"
              />
              <p className="mt-2 text-xs text-text-muted">
                Reserve: <span className="font-mono font-semibold text-text-main">{parsedCoinThreshold.toLocaleString()}</span> coins
              </p>
            </div>
        </SectionCard>
      </div>
    </div>
  );
};

export default SettingsView;

function downloadSettingsBundle(bundle: SettingsBundleV1): void {
	const blob = new Blob([`${JSON.stringify(bundle, null, 2)}\n`], { type: 'application/json' });
	const url = URL.createObjectURL(blob);
	const link = document.createElement('a');
	link.href = url;
	link.download = `CitadelOps-Settings-${new Date().toISOString().replace(/[:.]/g, '-')}.json`;
	document.body.appendChild(link);
	link.click();
	link.remove();
	URL.revokeObjectURL(url);
}
