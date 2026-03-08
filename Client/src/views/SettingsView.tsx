import React, { useState, useEffect } from 'react';
import { Icons } from '../components/Icons';
import PriorityModal from '../components/PriorityModal';
import { FrontendWebsocket } from '../websocket';

const SettingsView: React.FC = () => {
    const [minTimer, setMinTimer] = useState<string>('4.0');
    const [maxTimer, setMaxTimer] = useState<string>('6.0');
    const [isPriorityModalOpen, setIsPriorityModalOpen] = useState(false);

    useEffect(() => {
        const handleMessage = (msg: any) => {
            if (msg.type === 'schedulerSettings' && msg.payload) {
                setMinTimer(msg.payload.minAttackDelay?.toFixed(1) || '4.0');
                setMaxTimer(msg.payload.maxAttackDelay?.toFixed(1) || '6.0');
            }
        };

        FrontendWebsocket.addMessageListener(handleMessage);
        FrontendWebsocket.sendGetSchedulerSettings();

        return () => {
            FrontendWebsocket.removeMessageListener(handleMessage);
        };
    }, []);

    const saveSettings = (min: string, max: string) => {
        FrontendWebsocket.sendSaveSchedulerSettings({
            minAttackDelay: parseFloat(min),
            maxAttackDelay: parseFloat(max)
        });
    };

    const handleMinChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        // Keep it as string while typing to allow backspacing to empty, 
        // validation happens on blur or save
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

    return (
        <div className="space-y-6 max-w-4xl mx-auto pb-12">
            {/* Header */}
            <div className="flex justify-between items-center mb-6">
                <div>
                    <h1 className="text-2xl font-bold bg-gradient-to-r from-text-main to-text-main/70 bg-clip-text text-transparent">
                        System Settings
                    </h1>
                    <p className="text-text-muted mt-1 text-sm">Configure system behaviors and attack scheduling</p>
                </div>
            </div>

            <div className="grid grid-cols-1 gap-6">

                {/* Attack Scheduler Settings Group */}
                <div className="bg-bg-card rounded-global border border-border-base overflow-hidden">
                    <div className="px-6 py-4 border-b border-border-base bg-bg-card-hover/50 flex items-center gap-3">
                        <div className="w-8 h-8 rounded-lg bg-indigo-500/10 flex items-center justify-center">
                            <Icons.Activity className="w-4 h-4 text-indigo-400" />
                        </div>
                        <h2 className="text-lg font-bold">Attack Scheduler</h2>
                    </div>

                    <div className="p-6 space-y-8">
                        {/* Timer Settings */}
                        <div className="space-y-4">
                            <div>
                                <h3 className="text-sm font-semibold text-text-main mb-1">Random Attack Timer Range</h3>
                                <p className="text-xs text-text-muted mb-4">
                                    Set the minimum and maximum delay (in seconds) between sent attacks.
                                    Minimum allowed value is 4.0s to avoid rate limiting.
                                </p>
                            </div>

                            <div className="flex items-center gap-4">
                                {/* Min Input */}
                                <div className="relative flex-1 max-w-[200px]">
                                    <label className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
                                        Min Delay (Sec)
                                    </label>
                                    <div className="relative">
                                        <input
                                            type="number"
                                            step="0.1"
                                            min="4.0"
                                            value={minTimer}
                                            onChange={handleMinChange}
                                            onBlur={handleMinBlur}
                                            className="w-full bg-bg-input border border-border-light rounded-global py-2 px-3 text-sm text-text-main focus:outline-none focus:border-primary focus:ring-1 focus:ring-primary/50 transition-all font-mono"
                                        />
                                        <div className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted text-xs">s</div>
                                    </div>
                                </div>

                                <div className="mt-6 text-text-muted font-bold">-</div>

                                {/* Max Input */}
                                <div className="relative flex-1 max-w-[200px]">
                                    <label className="block text-[10px] font-bold text-text-muted uppercase tracking-wider mb-1.5">
                                        Max Delay (Sec)
                                    </label>
                                    <div className="relative">
                                        <input
                                            type="number"
                                            step="0.1"
                                            min={minTimer}
                                            value={maxTimer}
                                            onChange={handleMaxChange}
                                            onBlur={handleMaxBlur}
                                            className="w-full bg-bg-input border border-border-light rounded-global py-2 px-3 text-sm text-text-main focus:outline-none focus:border-primary focus:ring-1 focus:ring-primary/50 transition-all font-mono"
                                        />
                                        <div className="absolute right-3 top-1/2 -translate-y-1/2 text-text-muted text-xs">s</div>
                                    </div>
                                </div>
                            </div>
                        </div>

                        <div className="h-px bg-border-base w-full"></div>

                        {/* Priority Settings */}
                        <div className="space-y-4">
                            <div className="flex items-center justify-between">
                                <div>
                                    <h3 className="text-sm font-semibold text-text-main mb-1">Priority Categorization</h3>
                                    <p className="text-xs text-text-muted">
                                        Manage which tabs fall into which priority buckets (P1, P2, P3, Ignored).
                                    </p>
                                </div>
                                <button
                                    onClick={() => setIsPriorityModalOpen(true)}
                                    className="px-4 py-2 bg-primary hover:bg-primary-hover text-bg-app rounded-global text-sm font-bold transition-colors flex items-center gap-2 shadow-lg shadow-primary/20"
                                >
                                    <Icons.List className="w-4 h-4" />
                                    Manage Priorities
                                </button>
                            </div>
                        </div>

                    </div>
                </div>

            </div>

            <PriorityModal
                isOpen={isPriorityModalOpen}
                onClose={() => setIsPriorityModalOpen(false)}
            />
        </div>
    );
};

export default SettingsView;
