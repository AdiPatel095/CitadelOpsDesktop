import React, { useState, useEffect, useMemo } from 'react';
import { Icons } from '../components/Icons';
import PriorityModal from '../components/PriorityModal';
import { FrontendWebsocket } from '../Websocket';
import { Card, CardHeader, CardTitle, CardContent, Button, Input } from '../components/ui';

const SettingsView: React.FC = () => {
  const [minTimer, setMinTimer] = useState<string>('4.0');
  const [maxTimer, setMaxTimer] = useState<string>('6.0');
  const [upgradeEreDelayMs, setUpgradeEreDelayMs] = useState<string>('50');
  const [upgradeCoinThreshold, setUpgradeCoinThreshold] = useState<string>('0');
  const [isPriorityModalOpen, setIsPriorityModalOpen] = useState(false);

  useEffect(() => {
    const handleMessage = (msg: any) => {
      if (msg.type === 'schedulerSettings' && msg.payload) {
        setMinTimer(msg.payload.minAttackDelay?.toFixed(1) || '4.0');
        setMaxTimer(msg.payload.maxAttackDelay?.toFixed(1) || '6.0');
        setUpgradeEreDelayMs(String(msg.payload.upgradeEreDelayMs ?? 50));
        setUpgradeCoinThreshold(String(msg.payload.upgradeCoinThreshold ?? 0));
      }
    };

    FrontendWebsocket.addMessageListener(handleMessage);
    FrontendWebsocket.sendGetSchedulerSettings();

    return () => {
      FrontendWebsocket.removeMessageListener(handleMessage);
    };
  }, []);

  const saveSettings = (
    min: string,
    max: string,
    ereDelayMs?: string,
    coinThreshold?: string,
  ) => {
    FrontendWebsocket.sendSaveSchedulerSettings({
      minAttackDelay: parseFloat(min),
      maxAttackDelay: parseFloat(max),
      upgradeEreDelayMs: parseInt(ereDelayMs ?? upgradeEreDelayMs, 10),
      upgradeCoinThreshold: parseFloat(coinThreshold ?? upgradeCoinThreshold),
    });
  };

  const parsedCoinThreshold = useMemo(() => {
    const num = parseFloat(upgradeCoinThreshold);
    return Number.isFinite(num) && num >= 0 ? num : 0;
  }, [upgradeCoinThreshold]);

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
              <div className="w-8 h-8 rounded-lg bg-indigo-500/10 flex items-center justify-center">
                <Icons.Activity className="w-4 h-4 text-indigo-400" />
              </div>
              <CardTitle className="text-lg">Attack Scheduler</CardTitle>
            </div>
          </CardHeader>

          <CardContent className="liquid-prominent-header-content p-6 space-y-8">
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
                  <label className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
                    Min Delay (Sec)
                  </label>
                  <Input
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
                  <label className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
                    Max Delay (Sec)
                  </label>
                  <Input
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
              <label className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
                Delay (ms)
              </label>
              <Input
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
              <label className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
                Minimum Coins
              </label>
              <Input
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
