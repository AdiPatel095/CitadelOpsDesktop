import React, { useState, useEffect } from 'react';
import { X, Save, Plus, Trash2, Settings } from 'lucide-react';
import { FrontendWebsocket } from '../../websocket';
import { showTroopPicker } from '../../components/TroopPickerModal';
import type { UnitWithQuantity } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import { TROOP_DEFINITIONS } from '../../config/constants';

interface RecruitTroopsSettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
}

interface Castle {
    id: number;
    name: string;
    type: string;
}

interface RecruitItem {
    id: number;
    amount: number;
}

export const RecruitTroopsSettingsModal: React.FC<RecruitTroopsSettingsModalProps> = ({ isOpen, onClose }) => {
    const [castles, setCastles] = useState<Castle[]>([]);

    // State: CastleID -> Array of { id: troopId, amount: number }
    const [settings, setSettings] = useState<Record<string, RecruitItem[]>>({});
    const [isSaving, setIsSaving] = useState(false);

    // Edit Modal State
    const [editingUnit, setEditingUnit] = useState<{ castleId: string, item: RecruitItem } | null>(null);
    const [editAmount, setEditAmount] = useState<string>('');

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

    const handleAddItem = async (castleId: string) => {
        // Get current items to pre-fill
        const currentItems = settings[castleId] || [];
        const preselectedQuantities: Record<number, number> = {};
        currentItems.forEach(item => {
            if (item.id) preselectedQuantities[item.id] = item.amount;
        });

        const result = await showTroopPicker({
            mode: 'multi',
            title: `Select Units to Recruit - ${castles.find(c => c.id === parseInt(castleId))?.name}`,
            allowQuantity: true,
            preselected: currentItems.map(i => i.id),
            preselectedQuantities
        });

        if (Array.isArray(result)) {
            const newItems: RecruitItem[] = (result as UnitWithQuantity[]).map(u => ({
                id: u.unitId,
                amount: u.quantity
            }));
            setSettings(prev => ({
                ...prev,
                [castleId]: newItems
            }));
        }
    };

    const openEditModal = (castleId: string, item: RecruitItem) => {
        setEditingUnit({ castleId, item });
        setEditAmount(item.amount.toLocaleString());
    };

    const closeEditModal = () => {
        setEditingUnit(null);
        setEditAmount('');
    };

    const handleQuantityChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        // Allow digits and commas
        const raw = e.target.value.replace(/,/g, '');
        if (!/^\d*$/.test(raw)) return;

        const num = parseInt(raw);
        const formatted = num ? num.toLocaleString() : '';
        setEditAmount(formatted);
    };

    const saveEditModal = () => {
        if (!editingUnit) return;

        const parsedAmount = parseInt(editAmount.replace(/,/g, '')) || 0;
        const updatedItem = { ...editingUnit.item, amount: parsedAmount };

        // Update the item in the list
        const castleItems = settings[editingUnit.castleId] || [];
        const updatedItems = castleItems.map(item =>
            item.id === editingUnit.item.id ? updatedItem : item
        );

        setSettings(prev => ({
            ...prev,
            [editingUnit.castleId]: updatedItems
        }));
        closeEditModal();
    };

    const deleteFromEditModal = () => {
        if (!editingUnit) return;

        const castleItems = settings[editingUnit.castleId]?.filter(item => item.id !== editingUnit.item.id) || [];

        setSettings(prev => ({
            ...prev,
            [editingUnit.castleId]: castleItems
        }));
        closeEditModal();
    };

    const handleSave = () => {
        setIsSaving(true);
        localStorage.setItem('recruitTroopsSettings', JSON.stringify(settings));
        FrontendWebsocket.sendMessage({
            type: 'saveRecruitTroopsSettings',
            payload: settings
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
                        const castleId = castle.id.toString();
                        const castleItems = settings[castleId] || [];
                        const hasItems = castleItems.length > 0;

                        return (
                            <div key={castle.id} className="bg-bg-app rounded-xl border border-border-base overflow-hidden">
                                <div className="px-4 py-3 bg-bg-card-hover border-b border-border-base flex items-center justify-between">
                                    <div className="flex items-center gap-3">
                                        <span className="font-semibold text-text-main">{castle.name}</span>
                                        {hasItems && (
                                            <span className="px-2 py-0.5 rounded-full bg-primary/20 text-primary text-xs font-bold">
                                                {castleItems.length} unit{castleItems.length !== 1 ? 's' : ''}
                                            </span>
                                        )}
                                    </div>
                                </div>

                                {/* Units Grid Area */}
                                <div className="flex-1 overflow-y-auto bg-bg-app/30 rounded-lg p-4 border-t border-border-base/50">
                                    {hasItems ? (
                                        <div className="flex flex-wrap gap-3 content-start">
                                            {castleItems.map((item, index) => (
                                                <div
                                                    key={index}
                                                    className="relative group/unit transition-transform hover:scale-105 cursor-pointer"
                                                    title={`${TROOP_DEFINITIONS[item.id] || 'Unknown Unit'} (x${item.amount})`}
                                                    onClick={() => openEditModal(castleId, item)}
                                                >
                                                    <UnitImage unitId={item.id} size={60} showLevel={true} />
                                                    <div className="absolute -bottom-2 -right-2 bg-text-main border-2 border-bg-card rounded-full px-2 py-0.5 text-[10px] font-black text-bg-app shadow-md z-10 min-w-[24px] text-center">
                                                        {item.amount.toLocaleString()}
                                                    </div>

                                                    {/* Hover overlay hint */}
                                                    <div className="absolute inset-0 bg-black/20 rounded-lg opacity-0 group-hover/unit:opacity-100 transition-opacity flex items-center justify-center text-white pointer-events-none">
                                                        <Settings className="w-4 h-4 drop-shadow-md" />
                                                    </div>
                                                </div>
                                            ))}
                                            {/* Add Button Card */}
                                            <button
                                                onClick={() => handleAddItem(castleId)}
                                                className="flex flex-col items-center justify-center w-[60px] h-[60px] rounded-global border-2 border-dashed border-border-base text-text-muted hover:text-primary hover:border-primary hover:bg-primary/5 transition-all group/add"
                                                title="Add Unit"
                                            >
                                                <Plus className="w-6 h-6 group-hover/add:scale-110 transition-transform" />
                                            </button>
                                        </div>
                                    ) : (
                                        <div className="h-full flex flex-col items-center justify-center py-8">
                                            <div className="text-text-muted/40 text-xs italic text-center mb-4">
                                                No recruitment targets
                                            </div>
                                            <button
                                                onClick={() => handleAddItem(castleId)}
                                                className="flex items-center gap-2 px-4 py-2 rounded-global bg-bg-card border border-border-light hover:border-primary text-text-muted hover:text-primary transition-all group/empty-add"
                                            >
                                                <Plus className="w-4 h-4" />
                                                <span className="font-bold text-sm">Add Unit</span>
                                            </button>
                                        </div>
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

            {/* Edit Unit Modal */}
            {editingUnit && (
                <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
                    <div className="bg-bg-app border border-border-light rounded-global p-6 w-full max-w-sm shadow-2xl animate-scale-in" onClick={e => e.stopPropagation()}>
                        <h3 className="text-lg font-bold text-primary mb-4 text-center truncate">
                            Edit {TROOP_DEFINITIONS[editingUnit.item.id] || 'Unit'}
                        </h3>

                        <div className="flex flex-col items-center gap-6 mb-6">
                            <UnitImage unitId={editingUnit.item.id} size={80} showLevel={true} />

                            <div className="w-full">
                                <label className="text-xs text-text-muted font-bold uppercase mb-1 block text-center">Target Amount</label>
                                <input
                                    type="text"
                                    value={editAmount}
                                    onChange={handleQuantityChange}
                                    className="w-full bg-bg-input border border-border-base rounded-global px-4 py-3 text-xl font-bold text-center focus:border-primary focus:outline-none placeholder-text-muted/20"
                                    autoFocus
                                    placeholder="0"
                                />
                            </div>
                        </div>

                        <div className="flex gap-3">
                            <button
                                onClick={deleteFromEditModal}
                                className="flex-1 py-3 rounded-global bg-red-500/10 text-red-500 hover:bg-red-500 hover:text-white font-bold transition-colors flex items-center justify-center gap-2"
                            >
                                <Trash2 className="w-4 h-4" />
                                Remove
                            </button>
                            <button
                                onClick={saveEditModal}
                                className="flex-[2] py-3 rounded-global bg-primary text-bg-app font-bold hover:brightness-110 transition-colors"
                            >
                                Save
                            </button>
                        </div>
                    </div>
                </div>
            )}
        </div >
    );
};
