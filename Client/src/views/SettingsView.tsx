import React, { useState, useEffect, useMemo } from 'react';
import { Icons } from '../components/Icons';
import PriorityModal from '../components/PriorityModal';
import { CitadelAPI } from '../api/CitadelClient';
import { useCitadelAPI } from '../api/ApiContext';
import type { BrowserInventory } from '../api/Contracts';
import { Card, CardHeader, CardTitle, CardContent, Button, Input, Select } from '../components/ui';
import { configurationSection, numericSetting } from '../settings/Configuration';

const SettingsView: React.FC = () => {
	const { state, configuration, submitIntent, updateConfiguration } = useCitadelAPI();
  const [minTimer, setMinTimer] = useState<string>('4.0');
  const [maxTimer, setMaxTimer] = useState<string>('6.0');
  const [upgradeEreDelayMs, setUpgradeEreDelayMs] = useState<string>('50');
  const [upgradeCoinThreshold, setUpgradeCoinThreshold] = useState<string>('0');
  const [manualFocusIdleSec, setManualFocusIdleSec] = useState<string>('30');
  const [isPriorityModalOpen, setIsPriorityModalOpen] = useState(false);
	const [browserInventory, setBrowserInventory] = useState<BrowserInventory | null>(null);
	const [browserSelectionPending, setBrowserSelectionPending] = useState(false);
	const [browserSelectionError, setBrowserSelectionError] = useState('');
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
		setManualFocusIdleSec(String(numericSetting(schedulerConfiguration.manualFocusIdleSec, 30)));
	}, [schedulerConfiguration]);

  const saveSettings = (
    min: string,
    max: string,
    ereDelayMs?: string,
    coinThreshold?: string,
    focusIdleSec?: string,
  ) => {
		setSettingsSaveError('');
		void updateConfiguration('scheduler', {
			...schedulerConfiguration,
      minAttackDelay: parseFloat(min),
      maxAttackDelay: parseFloat(max),
      upgradeEreDelayMs: parseInt(ereDelayMs ?? upgradeEreDelayMs, 10),
      upgradeCoinThreshold: parseFloat(coinThreshold ?? upgradeCoinThreshold),
      manualFocusIdleSec: parseInt(focusIdleSec ?? manualFocusIdleSec, 10),
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

  const handleManualFocusIdleBlur = () => {
    let num = parseInt(manualFocusIdleSec, 10);
    let newVal = '30';
    if (isNaN(num) || num <= 0) {
      newVal = '30';
    } else if (num < 5) {
      newVal = '5';
    } else if (num > 300) {
      newVal = '300';
    } else {
      newVal = String(num);
    }
    setManualFocusIdleSec(newVal);
    saveSettings(minTimer, maxTimer, upgradeEreDelayMs, upgradeCoinThreshold, newVal);
  };

  return (
    <div className="space-y-6 max-w-4xl mx-auto pb-12">
      {/* Header */}
      <div className="responsive-page-heading mb-6">
        <div>
          <h1 className="text-2xl font-bold bg-gradient-to-r from-text-main to-text-main/70 bg-clip-text text-transparent">
            System Settings
          </h1>
          <p className="text-text-muted mt-1 text-sm">Configure system behaviors and attack scheduling</p>
        </div>
      </div>

      <div className="grid grid-cols-1 gap-6">
		<Card className="liquid-prominent-header-card">
			<CardHeader className="liquid-card-header-prominent">
				<div className="flex items-center gap-3">
					<div className="w-8 h-8 rounded-lg bg-sky-500/10 flex items-center justify-center">
						<Icons.Monitor className="w-4 h-4 text-sky-400" />
					</div>
					<CardTitle className="text-lg">Game Browser</CardTitle>
				</div>
			</CardHeader>

			<CardContent className="liquid-prominent-header-content p-6 space-y-4">
				<div>
					<h3 className="text-sm font-semibold text-text-main mb-1">Chromium Browser</h3>
					<p className="text-xs text-text-muted mb-4">
						Choose any detected CDP-capable browser. CitadelOps uses a dedicated profile for each browser,
						so your normal browser profile remains untouched.
					</p>
				</div>

				<div className="w-full sm:max-w-[360px]">
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
				</div>
			</CardContent>
		</Card>

        <Card className="liquid-prominent-header-card">
          <CardHeader className="liquid-card-header-prominent">
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 rounded-lg bg-indigo-500/10 flex items-center justify-center">
                <Icons.Activity className="w-4 h-4 text-indigo-400" />
              </div>
              <CardTitle className="text-lg">Attack Scheduler</CardTitle>
            </div>
          </CardHeader>

          <CardContent className="liquid-prominent-header-content p-6 space-y-8">
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
                <h3 className="text-sm font-semibold text-text-main mb-1">Manual Focus Hold</h3>
                <p className="text-xs text-text-muted mb-4">
                  Pause automation focus leases after game-tab input (5-300 seconds).
                </p>
              </div>

              <div className="relative flex-1 w-full sm:max-w-[200px]">
                <label htmlFor="manual-focus-idle" className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
                  Idle Timeout
                </label>
                <Input
				  id="manual-focus-idle"
                  type="number"
                  step="1"
                  min="5"
                  max="300"
                  value={manualFocusIdleSec}
                  onChange={(e) => setManualFocusIdleSec(e.target.value)}
                  onBlur={handleManualFocusIdleBlur}
                  className="font-mono"
                  rightIcon={<span className="text-xs">s</span>}
                />
              </div>
            </div>

            <div className="h-px bg-border-base w-full"></div>

            <div className="space-y-4">
              <div className="flex flex-col gap-4 sm:flex-row sm:items-center sm:justify-between">
                <div>
                  <h3 className="text-sm font-semibold text-text-main mb-1">Priority Categorization</h3>
                  <p className="text-xs text-text-muted">
                    Manage which tabs fall into which priority buckets (P1, P2, P3, Ignored).
                  </p>
                </div>
                <Button
                  onClick={() => setIsPriorityModalOpen(true)}
                  leftIcon={<Icons.List className="w-4 h-4" />}
                  className="w-full sm:w-auto"
                >
                  Manage Priorities
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card className="liquid-prominent-header-card">
          <CardHeader className="liquid-card-header-prominent">
            <div className="flex items-center gap-3">
              <div className="w-8 h-8 rounded-lg bg-emerald-500/10 flex items-center justify-center">
                <Icons.Shield className="w-4 h-4 text-emerald-400" />
              </div>
              <CardTitle className="text-lg">Equipment Upgrades</CardTitle>
            </div>
          </CardHeader>

          <CardContent className="liquid-prominent-header-content p-6 space-y-4">
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
          </CardContent>
        </Card>
      </div>

      <PriorityModal
        isOpen={isPriorityModalOpen}
        onClose={() => setIsPriorityModalOpen(false)}
      />
    </div>
  );
};

export default SettingsView;
