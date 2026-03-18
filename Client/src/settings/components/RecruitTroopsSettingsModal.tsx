import React, { useState, useEffect } from 'react';
import { X, Save, Plus, Trash2 } from 'lucide-react';
import { FrontendWebsocket } from '../../websocket';

interface RecruitTroopsSettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
}

// Troop mapping derived from Models.go
const TROOP_OPTIONS = [
    { id: 5, name: 'Veteran Saber Cleaver' },
    { id: 6, name: 'Veteran Slingshot' },
    { id: 9, name: 'Veteran Demon Horror' },
    { id: 10, name: 'Veteran Deathly Horror' },
    { id: 11, name: 'Veteran Flame Bearer' },
    { id: 12, name: 'Veteran Composite Bowman' },
    { id: 409, name: 'Star-Spangled Veteran Demon Horror' },
    { id: 410, name: 'Star-Spangled Veteran Deathly Horror' },
    { id: 308, name: 'Veteran Halberdier' },
    { id: 309, name: 'Veteran Two-Handed Swordsman' },
    { id: 311, name: 'Veteran Longbowman' },
    { id: 312, name: 'Veteran Heavy Crossbowman' },
];

interface Castle {
    id: number;
    name: string;
    type: string;
}

export const RecruitTroopsSettingsModal: React.FC<RecruitTroopsSettingsModalProps> = ({ isOpen, onClose }) => {
    const [castles, setCastles] = useState<Castle[]>([]);

    // State: CastleID -> Array of { id: troopId, amount: number }
    const [settings, setSettings] = useState<Record<string, { id: number; amount: number }[]>>({});
    const [isSaving, setIsSaving] = useState(false);

    useEffect(() => {
        if (isOpen) {
            FrontendWebsocket.sendMessage({ type: 'getRecruitTroopsSettings' });
            FrontendWebsocket.sendMessage({ type: 'getCastleList' });
        }
    }, [isOpen]);

    useEffect(() => {
        const handleMessage = (msg: any) => {
            if (msg.type === 'castleList') {
                const list = msg.payload as Castle[];
                if (list && list.length > 0) {
                    setCastles(list);
                }
            }

            if (msg.type === 'recruitTroopsSettings') {
                const payload = msg.payload || {};
                const formattedSettings: Record<string, { id: number; amount: number }[]> = {};

                // Transform backend map[castleID]map[unitID]amount to our local state array
                Object.keys(payload).forEach(castleId => {
                    const troopMap = payload[castleId];
                    formattedSettings[castleId] = Object.keys(troopMap).map(unitId => ({
                        id: parseInt(unitId),
                        amount: troopMap[unitId]
                    }));
                });

                setSettings(formattedSettings);
                setIsSaving(false);
            }
        };

        FrontendWebsocket.addMessageListener(handleMessage);
        return () => FrontendWebsocket.removeMessageListener(handleMessage);
    }, []);

    const handleSave = () => {
        setIsSaving(true);
        localStorage.setItem('recruitTroopsSettings', JSON.stringify(settings));
        FrontendWebsocket.sendMessage({
            type: 'saveRecruitTroopsSettings',
            payload: settings
        });
    };

    const addTroopRule = (castleId: string) => {
        setSettings(prev => {
            const castleRules = prev[castleId] || [];
            return {
                ...prev,
                [castleId]: [...castleRules, { id: TROOP_OPTIONS[0].id, amount: 100 }]
            };
        });
    };

    const removeTroopRule = (castleId: string, index: number) => {
        setSettings(prev => {
            const castleRules = [...(prev[castleId] || [])];
            castleRules.splice(index, 1);
            return {
                ...prev,
                [castleId]: castleRules
            };
        });
    };

    const updateTroopRule = (castleId: string, index: number, field: 'id' | 'amount', value: number) => {
        setSettings(prev => {
            const castleRules = [...(prev[castleId] || [])];
            castleRules[index] = { ...castleRules[index], [field]: value };
            return {
                ...prev,
                [castleId]: castleRules
            };
        });
    };

    if (!isOpen) return null;

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 sm:p-6">
            <div
                className="absolute inset-0 bg-black/60 backdrop-blur-sm transition-opacity"
                onClick={onClose}
            />

            <div className="bg-bg-card border border-border-base rounded-2xl shadow-2xl w-full max-w-4xl max-h-[90vh] flex flex-col relative z-10 animate-in fade-in zoom-in duration-200">
                <div className="flex items-center justify-between px-6 py-4 border-b border-border-base bg-bg-card-hover/50 rounded-t-2xl">
                    <div>
                        <h2 className="text-xl font-bold text-text-main flex items-center gap-2">
                            <span className="w-2 h-2 rounded-full bg-amber-500 shadow-[0_0_8px_rgba(245,158,11,0.6)]" />
                            Recruit Troops Settings
                        </h2>
                        <p className="text-sm text-text-muted mt-1">
                            Configure target troop counts per castle. The bot will automatically recruit to reach these counts.
                        </p>
                    </div>
                    <button
                        onClick={onClose}
                        className="p-2 text-text-muted hover:text-red-400 hover:bg-red-400/10 rounded-global transition-colors"
                    >
                        <X className="w-5 h-5" />
                    </button>
                </div>

                <div className="flex-1 overflow-y-auto p-6 space-y-6 hidden-scrollbar">
                    {castles.map((castle) => {
                        const rules = settings[castle.id] || [];

                        return (
                            <div key={castle.id} className="bg-bg-app rounded-xl border border-border-base overflow-hidden">
                                <div className="px-4 py-3 bg-bg-card-hover border-b border-border-base flex items-center justify-between">
                                    <div className="flex items-center gap-3">
                                        <span className="font-semibold text-text-main">{castle.name}</span>
                                    </div>
                                    <button
                                        onClick={() => addTroopRule(castle.id)}
                                        className="flex items-center gap-1.5 px-3 py-1.5 rounded-global bg-primary/10 text-primary hover:bg-primary/20 text-sm font-medium transition-colors"
                                    >
                                        <Plus className="w-4 h-4" />
                                        Add Troop
                                    </button>
                                </div>
                                <div className="p-4 space-y-3">
                                    {rules.length === 0 ? (
                                        <div className="text-center py-4 text-text-muted/60 text-sm">
                                            No recruitment rules for this castle
                                        </div>
                                    ) : (
                                        rules.map((rule, idx) => (
                                            <div key={idx} className="flex flex-col sm:flex-row gap-3 items-center bg-bg-card p-3 rounded-lg border border-border-light">
                                                <div className="flex-1 w-full">
                                                    <label className="block text-xs text-text-muted mb-1 ml-1">Troop Type</label>
                                                    <select
                                                        value={rule.id}
                                                        onChange={(e) => updateTroopRule(castle.id, idx, 'id', parseInt(e.target.value))}
                                                        className="w-full bg-bg-input border border-border-base text-text-main text-sm rounded-global focus:ring-1 focus:ring-primary focus:border-primary block p-2 transition-colors"
                                                    >
                                                        {TROOP_OPTIONS.map(opt => (
                                                            <option key={opt.id} value={opt.id}>{opt.name}</option>
                                                        ))}
                                                    </select>
                                                </div>

                                                <div className="w-full sm:w-32">
                                                    <label className="block text-xs text-text-muted mb-1 ml-1">Target Amount</label>
                                                    <input
                                                        type="number"
                                                        min="0"
                                                        step="100"
                                                        value={rule.amount || 0}
                                                        onChange={(e) => updateTroopRule(castle.id, idx, 'amount', parseInt(e.target.value) || 0)}
                                                        className="w-full bg-bg-input border border-border-base text-text-main text-sm rounded-global focus:ring-1 focus:ring-primary focus:border-primary block p-2 transition-colors"
                                                    />
                                                </div>

                                                <div className="sm:self-end mt-2 sm:mt-0">
                                                    <button
                                                        onClick={() => removeTroopRule(castle.id, idx)}
                                                        className="p-2 text-text-muted hover:text-red-400 hover:bg-red-400/10 rounded-global transition-colors"
                                                        title="Remove Rule"
                                                    >
                                                        <Trash2 className="w-4 h-4" />
                                                    </button>
                                                </div>
                                            </div>
                                        ))
                                    )}
                                </div>
                            </div>
                        );
                    })}
                </div>

                <div className="p-4 sm:p-6 border-t border-border-base bg-bg-card-hover/50 rounded-b-2xl flex justify-end gap-3">
                    <button
                        onClick={onClose}
                        className="px-6 py-2 rounded-global font-medium text-text-muted hover:text-text-main transition-colors"
                    >
                        Cancel
                    </button>
                    <button
                        onClick={handleSave}
                        disabled={isSaving}
                        className="flex items-center gap-2 px-6 py-2 rounded-global font-medium bg-primary text-bg-app hover:bg-primary/90 transition-colors disabled:opacity-50 disabled:cursor-not-allowed shadow-[0_0_15px_rgba(52,211,153,0.2)]"
                    >
                        <Save className="w-4 h-4" />
                        {isSaving ? 'Saving...' : 'Save Settings'}
                    </button>
                </div>
            </div>
        </div >
    );
};
