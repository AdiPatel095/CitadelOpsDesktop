import React, { useState, useEffect, useMemo } from 'react';
import { Icons } from '../components/Icons';
import { CitadelAPI } from '../api/CitadelClient';
import { useCitadelAPI } from '../api/ApiContext';
import type { BrowserInventory } from '../api/Contracts';
import { Button, Input, PageHeader, SectionCard, Select } from '../components/ui';
import { asRecord, configurationSection, numericSetting } from '../settings/Configuration';

interface AttackPriorityFeature {
	id: string;
	label: string;
	detail: string;
	defaultWeight: number;
}

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

const SettingsView: React.FC = () => {
	const { state, configuration, submitIntent, updateConfiguration } = useCitadelAPI();
  const [minTimer, setMinTimer] = useState<string>('4.0');
  const [maxTimer, setMaxTimer] = useState<string>('6.0');
  const [upgradeEreDelayMs, setUpgradeEreDelayMs] = useState<string>('50');
  const [upgradeCoinThreshold, setUpgradeCoinThreshold] = useState<string>('0');
	const [attackPriorities, setAttackPriorities] = useState<Record<string, string>>({});
	const [attackPriorityFeatures, setAttackPriorityFeatures] = useState<AttackPriorityFeature[]>(defaultAttackPriorityFeatures);
	const [browserInventory, setBrowserInventory] = useState<BrowserInventory | null>(null);
	const [browserSelectionPending, setBrowserSelectionPending] = useState(false);
	const [browserSelectionError, setBrowserSelectionError] = useState('');
	const [customBrowserPath, setCustomBrowserPath] = useState('');
	const [settingsSaveError, setSettingsSaveError] = useState('');
	const schedulerConfiguration = useMemo(
		() => configurationSection(configuration, 'scheduler'),
		[configuration?.sections.scheduler],
	);

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
		setMinTimer(numericSetting(schedulerConfiguration.minAttackDelay, 4).toFixed(1));
		setMaxTimer(numericSetting(schedulerConfiguration.maxAttackDelay, 6).toFixed(1));
		setUpgradeEreDelayMs(String(numericSetting(schedulerConfiguration.upgradeEreDelayMs, 50)));
		setUpgradeCoinThreshold(String(numericSetting(schedulerConfiguration.upgradeCoinThreshold, 0)));
		const storedPriorities = asRecord(schedulerConfiguration.attackPriorities);
		setAttackPriorities(Object.fromEntries(attackPriorityFeatures.map((feature) => [
			feature.id,
			String(normalizedAttackPriority(storedPriorities[feature.id], feature.defaultWeight)),
		])));
	}, [attackPriorityFeatures, schedulerConfiguration]);

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

	const selectedBrowserID = state?.session.browserId ?? browserInventory?.selected?.id ?? '';
	const selectedBrowser = browserInventory?.available.find((browser) => browser.id === selectedBrowserID);
	const browserOptions = useMemo(() => {
		const options = (browserInventory?.available ?? []).map((browser) => ({
			value: browser.id,
			label: browser.name,
		}));
		if (selectedBrowserID && !options.some((option) => option.value === selectedBrowserID)) {
			options.unshift({
				value: selectedBrowserID,
				label: state?.session.browserName ?? browserInventory?.selected?.name ?? selectedBrowserID,
			});
		}
		return options;
	}, [browserInventory, selectedBrowserID, state?.session.browserName]);
	const browserPlaceholder = browserInventory == null
		? 'Discovering browsers…'
		: browserOptions.length > 0
			? 'Select a browser'
			: 'No compatible browser detected';
	const browserCanChange = state?.session.status === 'stopped' || state?.session.status === 'unavailable';

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

	const selectCustomBrowser = () => {
		const executable = customBrowserPath.trim();
		if (!executable) return;
		selectBrowser(executable);
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

	const saveAttackPriority = (featureID: string) => {
		const fallback = attackPriorityFeatures.find((feature) => feature.id === featureID)?.defaultWeight ?? 50;
		const priority = normalizedAttackPriority(attackPriorities[featureID], fallback);
		setAttackPriorities((current) => ({ ...current, [featureID]: String(priority) }));
		void updateConfiguration('scheduler', {
			...schedulerConfiguration,
			attackPriorities: {
				...asRecord(schedulerConfiguration.attackPriorities),
				[featureID]: priority,
			},
		}).catch((error) => {
			setSettingsSaveError(error instanceof Error ? error.message : 'Could not save attack priorities');
		});
	};

  return (
    <div className="space-y-6 max-w-4xl mx-auto pb-12">
      <PageHeader
        className="mb-6"
        title="System Settings"
        description="Configure system behaviors and attack scheduling."
      />

      <div className="grid grid-cols-1 gap-6">
		<SectionCard variant="glass" title="Game Browser" icon={<span className="flex h-8 w-8 items-center justify-center rounded-lg bg-sky-500/10"><Icons.Monitor className="h-4 w-4 text-sky-400" /></span>} contentClassName="p-6 space-y-4">
				<div>
					<h3 className="text-sm font-semibold text-text-main mb-1">Chromium Browser</h3>
					<p className="text-xs text-text-muted mb-4">
						Choose any detected CDP-capable browser. CitadelOps uses a dedicated profile for each browser,
						so your normal browser profile remains untouched.
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
						disabled={!browserCanChange || browserSelectionPending || browserInventory == null}
					/>
					{selectedBrowser?.executablePath && (
						<p className="mt-2 text-[11px] text-text-muted font-mono break-all">{selectedBrowser.executablePath}</p>
					)}
					{browserInventory != null && browserInventory.available.length > 0 && !selectedBrowser && (
						<p className="mt-2 text-xs text-text-muted">
							Detected: {browserInventory.available.map((browser) => browser.name).join(', ')}
						</p>
					)}
					{!browserCanChange && (
						<p className="mt-2 text-xs text-warning">Stop the game session before changing browsers.</p>
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
								disabled={!browserCanChange || browserSelectionPending}
							/>
							<Button
								variant="secondary"
								onClick={selectCustomBrowser}
								disabled={!browserCanChange || browserSelectionPending || !customBrowserPath.trim()}
								className="shrink-0"
							>
								Use executable
							</Button>
						</div>
						<p className="mt-2 text-xs text-text-muted">
							Use this for Chromium-based builds that are not detected automatically.
						</p>
					</div>
				</div>
		</SectionCard>

        <SectionCard variant="glass" title="Attack Scheduler" icon={<span className="flex h-8 w-8 items-center justify-center rounded-lg bg-indigo-500/10"><Icons.Activity className="h-4 w-4 text-indigo-400" /></span>} contentClassName="p-6 space-y-8">
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
						Choose which automated attack modules receive open launch slots first. Waiting time gradually raises older work; manual and scheduled attacks retain protected priority.
					</p>
				</div>

				<div className="grid grid-cols-1 gap-3 md:grid-cols-3">
					{attackPriorityFeatures.map((feature) => (
						<label key={feature.id} className="rounded-xl border border-border-base bg-bg-app/45 p-3">
							<span className="block text-xs font-bold text-text-main">{feature.label}</span>
							<span className="mt-0.5 block min-h-8 text-[11px] leading-4 text-text-muted">{feature.detail}</span>
							<Input
								type="number"
								min={1}
								max={100}
								value={attackPriorities[feature.id] ?? '50'}
								onChange={(event) => setAttackPriorities((current) => ({ ...current, [feature.id]: event.target.value }))}
								onBlur={() => saveAttackPriority(feature.id)}
								className="mt-3 text-center font-mono"
								rightIcon={<span className="text-[10px] text-text-muted">1–100</span>}
							/>
						</label>
					))}
				</div>
			</div>

        </SectionCard>

        <SectionCard variant="glass" title="Equipment Upgrades" icon={<span className="flex h-8 w-8 items-center justify-center rounded-lg bg-emerald-500/10"><Icons.Shield className="h-4 w-4 text-emerald-400" /></span>} contentClassName="p-6 space-y-4">
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
