import React, { useState, useEffect } from 'react';
import { X, Plus, Trash2, Search, Settings, AlertTriangle, Copy, Link as LinkIcon, Unlink, CheckSquare, Square } from 'lucide-react';
import { TROOP_DEFINITIONS, TOOL_DEFINITIONS, getUnitBaseAndLevel, getUnitIdForLevel, UNIT_LEVEL_MAP } from '../../config/constants';
import { FrontendWebsocket } from '../../websocket';
import { showTroopPicker } from '../../components/TroopPickerModal';
import { UnitWithQuantity } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';

interface AutoBirdSettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
}

interface IgnoreItem {
    id: number;
    amount: number;
}

type IgnoreList = Record<number, IgnoreItem[]>; // Key is CastleID (number)
type CastleLinks = Record<number, number>; // Key is ChildID, Value is ParentID

interface Castle {
    id: number;
    name: string;
    type: string;
}

export const AutoBirdSettingsModal: React.FC<AutoBirdSettingsModalProps> = ({ isOpen, onClose }) => {
    const [ignoreList, setIgnoreList] = useState<IgnoreList>({});
    const [castles, setCastles] = useState<Castle[]>([]);
    const [castleLinks, setCastleLinks] = useState<CastleLinks>({});
    const [loading, setLoading] = useState(true);
    const [delaySettings, setDelaySettings] = useState({ min: 6, max: 12 });


    // Edit Modal State
    // keys: castleId, originalId (to track replacement), item (current state in modal)
    const [editingUnit, setEditingUnit] = useState<{ castleId: number, originalId: number, item: IgnoreItem } | null>(null);
    const [editAmount, setEditAmount] = useState<string>('');

    // Management Modal State
    const [managementMode, setManagementMode] = useState<'copy' | 'link' | null>(null);
    const [managementSource, setManagementSource] = useState<number | null>(null);
    const [selectedTargets, setSelectedTargets] = useState<number[]>([]);

    // Helper: Persist Links
    const persistLinks = (newLinks: CastleLinks) => {
        setCastleLinks(newLinks);
        localStorage.setItem('autoBird_links', JSON.stringify(newLinks));
    };

    // Helper: Update ignore list and persist
    const updateIgnoreList = (newList: IgnoreList) => {
        setIgnoreList(newList);
        // We do strictly local update here, assuming main Save persists to server/storage.
        // BUT logic requires offline persistence too.
        localStorage.setItem('autoBird_ignoreList', JSON.stringify(newList));
    };

    // Management Handlers
    const openManagement = (mode: 'copy' | 'link', castleId: number) => {
        setManagementMode(mode);
        setManagementSource(castleId);
        setSelectedTargets([]);
    };

    const closeManagement = () => {
        setManagementMode(null);
        setManagementSource(null);
        setSelectedTargets([]);
    };

    const handleToggleTarget = (id: number) => {
        setSelectedTargets(prev =>
            prev.includes(id) ? prev.filter(t => t !== id) : [...prev, id]
        );
    };

    const handleSelectAll = () => {
        if (!managementSource) return;
        // Filter out source and already linked (if linking)
        // For Linking: Can only link castles that are not ALREADY linked (or overwrite?) -> Overwrite is fine.
        // But preventing circular links is good.
        // Only select castles that are NOT the source.
        const targets = castles
            .filter(c => c.id !== managementSource)
            .map(c => c.id);
        setSelectedTargets(targets);
    };

    const handleApplyManagement = () => {
        if (!managementSource) return;

        const sourceItems = ignoreList[managementSource] || [];

        if (managementMode === 'copy') {
            // Deep Copy Source -> Targets
            const newList = { ...ignoreList };
            selectedTargets.forEach(targetId => {
                // Determine if target is a Child. If so, should we allow Copy?
                // If target is Linked, it should follow Parent. Copying to it is temp override or broken link?
                // Let's assume Copying OVERWRITES any link? Or just simple data copy.
                // Simple data copy. If it's linked later, it will overwrite again.
                // But if it IS linked, copying to it might be weird if it's read-only.
                // For now, allow copy.

                newList[targetId] = sourceItems.map(item => ({ ...item }));
            });
            updateIgnoreList(newList);
        }

        if (managementMode === 'link') {
            // Set Link: Targets -> Source (Parent)
            const newLinks = { ...castleLinks };
            const newList = { ...ignoreList };

            selectedTargets.forEach(targetId => {
                newLinks[targetId] = managementSource;
                // Auto-sync immediately
                newList[targetId] = sourceItems.map(item => ({ ...item }));
            });

            persistLinks(newLinks);
            updateIgnoreList(newList);
        }

        closeManagement();
    };

    const handleUnlink = (childId: number) => {
        const newLinks = { ...castleLinks };
        delete newLinks[childId];
        persistLinks(newLinks);
    };

    // Helper: Enforce links (Child follows Parent)
    const enforceLinks = (list: IgnoreList, links: CastleLinks): IgnoreList => {
        const enforced = { ...list };
        Object.entries(links).forEach(([childIdStr, parentId]) => {
            const childId = parseInt(childIdStr);
            // Overwrite child with parent data if parent exists in list
            if (enforced[parentId]) {
                enforced[childId] = enforced[parentId].map(item => ({ ...item }));
            }
        });
        return enforced;
    };

    // Fetch data on open
    useEffect(() => {
        if (!isOpen) return;

        const handleMessage = (message: any) => {
            if (message.type === 'castleList') {
                const list = message.payload as Castle[];
                // Only update if we have a valid list, otherwise keep cached version (Offline Mode)
                if (list && list.length > 0) {
                    setCastles(list);
                    // Cache for offline use
                    localStorage.setItem('autoBird_castleList', JSON.stringify(list));
                }
            }
            if (message.type === 'birdSettings') {
                // payload: Map<int, Map<int, int>>
                // Convert to Record<number, IgnoreItem[]>
                const rawSettings = message.payload as Record<string, Record<string, number>>;
                let processed: IgnoreList = {};

                Object.entries(rawSettings).forEach(([castleIdStr, itemsMap]) => {
                    const castleId = parseInt(castleIdStr);
                    const items: IgnoreItem[] = [];
                    Object.entries(itemsMap).forEach(([unitIdStr, amount]) => {
                        items.push({ id: parseInt(unitIdStr), amount: Number(amount) });
                    });
                    processed[castleId] = items;
                });

                // Enforce Links on Server Data
                // Server doesn't know about links, so we must re-apply them to ensure consistency
                try {
                    const currentLinksRaw = localStorage.getItem('autoBird_links');
                    if (currentLinksRaw) {
                        const currentLinks = JSON.parse(currentLinksRaw);
                        processed = enforceLinks(processed, currentLinks);
                    }
                } catch (e) {
                    console.error("Failed to enforce links on server data", e);
                }

                // Only update if we have data or if the server explicitly sends empty but valid config
                // But for offline persistence, let's treat empty server response as "not loaded" if we have cache?
                // Actually, settings CAN be empty. But usually valid response.
                if (Object.keys(processed).length > 0) {
                    setIgnoreList(processed);
                    // Cache for offline use
                    localStorage.setItem('autoBird_ignoreList', JSON.stringify(processed));
                    setLoading(false);
                }
            }
        };

        FrontendWebsocket.addMessageListener(handleMessage);

        // Load cached data first (Offline Persistence)
        const cachedCastles = localStorage.getItem('autoBird_castleList');
        const cachedSettings = localStorage.getItem('autoBird_ignoreList');
        const cachedLinks = localStorage.getItem('autoBird_links');

        if (cachedCastles) {
            try {
                setCastles(JSON.parse(cachedCastles));
            } catch (e) {
                console.error("Failed to parse cached castles", e);
            }
        }

        let loadedLinks: CastleLinks = {};
        if (cachedLinks) {
            try {
                loadedLinks = JSON.parse(cachedLinks);
                setCastleLinks(loadedLinks);
            } catch (e) {
                console.error("Failed to parse cached links", e);
            }
        }

        if (cachedSettings) {
            try {
                let settings = JSON.parse(cachedSettings);
                // Enforce links on cached data too
                if (Object.keys(loadedLinks).length > 0) {
                    settings = enforceLinks(settings, loadedLinks);
                }
                setIgnoreList(settings);
            } catch (e) {
                console.error("Failed to parse cached settings", e);
            }
        }

        const cachedDelays = localStorage.getItem('autoBird_delaySettings');
        if (cachedDelays) {
            try {
                const delays = JSON.parse(cachedDelays);
                setDelaySettings({ min: delays.min || 6, max: delays.max || 12 });
            } catch (e) {
                console.error("Failed to parse cached delays", e);
            }
        }

        // Always finish loading state after cache check so UI can render
        setLoading(false);

        // Request Data (will update cache if successful)
        // Request Data (castles only - settings managed locally)
        FrontendWebsocket.sendMessage({ type: 'getCastleList' });
        // FrontendWebsocket.sendMessage({ type: 'getBirdSettings' }); // Removed - Frontend managed

        return () => {
            FrontendWebsocket.removeMessageListener(handleMessage);
        };
    }, [isOpen]);

    if (!isOpen) return null;

    const handleAddItem = async (castleId: number) => {
        // Get current items to pre-fill
        const currentItems = ignoreList[castleId] || [];
        const preselectedQuantities: Record<number, number> = {};
        currentItems.forEach(item => {
            if (item.id) preselectedQuantities[item.id] = item.amount;
        });

        const result = await showTroopPicker({
            mode: 'multi',
            title: `Select Units to Ignore - ${castles.find(c => c.id === castleId)?.name}`,
            allowQuantity: true,
            preselected: currentItems.map(i => i.id),
            preselectedQuantities
        });

        if (Array.isArray(result)) {
            // Map result back to IgnoreItem
            const newItems: IgnoreItem[] = (result as UnitWithQuantity[]).map(u => ({
                id: u.unitId,
                amount: u.quantity
            }));

            // Compute new state
            const updates: IgnoreList = {};
            const children = Object.entries(castleLinks)
                .filter(([_, parentId]) => parentId === castleId)
                .map(([childId]) => parseInt(childId));

            children.forEach(childId => {
                updates[childId] = newItems.map(i => ({ ...i }));
            });

            const newList = {
                ...ignoreList,
                [castleId]: newItems,
                ...updates
            };

            updateIgnoreList(newList);
        }
    };

    const handleRemoveItem = (castleId: number, index: number) => {
        setIgnoreList(prev => ({
            ...prev,
            [castleId]: prev[castleId]?.filter((_, i) => i !== index) || []
        }));
    };

    const handleUpdateItem = (castleId: number, index: number, field: keyof IgnoreItem, value: number) => {
        setIgnoreList(prev => ({
            ...prev,
            [castleId]: prev[castleId]?.map((item, i) =>
                i === index ? { ...item, [field]: value } : item
            ) || []
        }));
    };

    const handleSave = () => {
        // Transform back to map structure needed by backend
        // Backend expects CastleID(string) -> List of {id, amount}
        // Logic in parser expects this to check valid entries

        // Actually, backend expects payload to be `map[string]interface{}` where items are list of objects
        // My backend change: `payloadRaw` is the map.
        // So I send object keyed by castleID.

        // Prepare payload
        const payload: Record<string, any[]> = {};
        Object.entries(ignoreList).forEach(([castleId, items]) => {
            payload[castleId] = items.map(item => ({ id: item.id, amount: item.amount }));
        });

        // Persist to local storage immediately (Offline Save)
        localStorage.setItem('autoBird_ignoreList', JSON.stringify(ignoreList));
        localStorage.setItem('autoBird_delaySettings', JSON.stringify(delaySettings));

        // FrontendWebsocket.sendMessage({ type: 'saveBirdSettings', payload }); // Removed - Frontend managed
        onClose();
    };

    // Combine definitions for search
    const allDefinitions = { ...TROOP_DEFINITIONS, ...TOOL_DEFINITIONS };

    // Handle Edit Modal Actions
    const openEditModal = (castleId: number, item: IgnoreItem) => {
        setEditingUnit({ castleId, originalId: item.id, item });
        setEditAmount(item.amount >= 100000000 ? '0' : item.amount.toLocaleString());
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
        setEditAmount(isNaN(num) ? '' : num.toLocaleString());
    };

    const handleLevelChange = (newLevel: number) => {
        if (!editingUnit) return;

        // Find base ID to determine family
        const currentId = editingUnit.item.id;
        const info = getUnitBaseAndLevel(currentId);

        let newItemId = currentId;
        if (info) {
            newItemId = getUnitIdForLevel(info.baseId, newLevel);
        }

        setEditingUnit({
            ...editingUnit,
            item: { ...editingUnit.item, id: newItemId }
        });
    };

    const saveEditModal = () => {
        if (!editingUnit) return;

        let newAmount = parseInt(editAmount.replace(/,/g, '')) || 0;
        if (newAmount === 0) newAmount = 100000000;
        const castleItems = ignoreList[editingUnit.castleId] || [];
        let updatedItems: IgnoreItem[];

        if (editingUnit.originalId !== editingUnit.item.id) {
            // Unit ID changed (e.g., level switch)
            // Remove the original item and add the new one
            updatedItems = castleItems.filter(item => item.id !== editingUnit.originalId);
            updatedItems.push({ id: editingUnit.item.id, amount: newAmount });
        } else {
            // Unit ID is the same, just update the amount
            updatedItems = castleItems.map(item =>
                item.id === editingUnit.item.id ? { ...item, amount: newAmount } : item
            );
        }

        // Check for Linked Children and propagate changes
        const children = Object.entries(castleLinks)
            .filter(([_, parentId]) => parentId === editingUnit.castleId)
            .map(([childId]) => parseInt(childId));

        const updates: IgnoreList = {};
        if (children.length > 0) {
            children.forEach(childId => {
                updates[childId] = updatedItems.map(i => ({ ...i }));
            });
        }

        const newList = {
            ...ignoreList,
            [editingUnit.castleId]: updatedItems,
            ...updates
        };

        updateIgnoreList(newList);
        closeEditModal();
    };

    const deleteFromEditModal = () => {
        if (!editingUnit) return;

        const parentList = ignoreList[editingUnit.castleId]?.filter(item => item.id !== editingUnit.item.id) || [];
        const updates: IgnoreList = {};

        // Apply to Children
        const children = Object.entries(castleLinks)
            .filter(([_, parentId]) => parentId === editingUnit.castleId)
            .map(([childId]) => parseInt(childId));

        children.forEach(childId => {
            updates[childId] = parentList.map(i => ({ ...i }));
        });

        const newList = {
            ...ignoreList,
            [editingUnit.castleId]: parentList,
            ...updates
        };

        updateIgnoreList(newList);
        closeEditModal();
    };

    // Calculate level info for render
    const levelInfo = editingUnit ? getUnitBaseAndLevel(editingUnit.item.id) : null;
    const availableLevels = levelInfo && UNIT_LEVEL_MAP[levelInfo.baseId]
        ? Object.keys(UNIT_LEVEL_MAP[levelInfo.baseId]).map(Number).sort((a, b) => a - b)
        : [];

    return (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-sm animate-fade-in">
            {/* Edit Unit Modal */}
            {editingUnit && (
                <div className="fixed inset-0 z-[60] flex items-center justify-center bg-black/50 backdrop-blur-sm p-4">
                    <div className="bg-bg-app border border-border-light rounded-global p-6 w-full max-w-sm shadow-2xl animate-scale-in" onClick={e => e.stopPropagation()}>
                        <h3 className="text-lg font-bold text-primary mb-4 text-center truncate">
                            Edit {TROOP_DEFINITIONS[editingUnit.item.id] || 'Unit'}
                        </h3>

                        <div className="flex flex-col items-center gap-6 mb-6">
                            <UnitImage unitId={editingUnit.item.id} size={80} showLevel={true} />

                            {/* Level Switcher */}
                            {availableLevels.length > 0 && levelInfo && (
                                <div className="w-full">
                                    <label className="text-xs text-text-muted font-bold uppercase mb-1 block text-center">Level</label>
                                    <select
                                        value={levelInfo.level}
                                        onChange={(e) => handleLevelChange(parseInt(e.target.value))}
                                        className="w-full bg-bg-input border border-border-base rounded-global px-3 py-2 text-sm text-center focus:border-primary focus:outline-none appearance-none cursor-pointer hover:border-primary/50 transition-colors"
                                    >
                                        {availableLevels.map(lvl => (
                                            <option key={lvl} value={lvl}>Level {lvl}</option>
                                        ))}
                                    </select>
                                </div>
                            )}

                            <div className="w-full">
                                <label className="text-xs text-text-muted font-bold uppercase mb-1 block text-center">Quantity to Ignore</label>
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
                                Save Changes
                            </button>
                        </div>

                        <button
                            onClick={closeEditModal}
                            className="absolute top-4 right-4 text-text-muted hover:text-white"
                        >
                            <X className="w-5 h-5" />
                        </button>
                    </div>
                </div>
            )}

            {/* Management Modal (Copy / Link) */}
            {managementMode && managementSource && (
                <div className="fixed inset-0 z-[70] flex items-center justify-center bg-black/50 backdrop-blur-sm p-4 animate-fade-in">
                    <div className="bg-bg-app border border-border-light rounded-global p-6 w-full max-w-lg shadow-2xl animate-scale-in flex flex-col max-h-[80vh]" onClick={e => e.stopPropagation()}>
                        <h3 className="text-xl font-bold text-white mb-2">
                            {managementMode === 'copy' ? 'Batch Copy Settings' : 'Link Castles'}
                        </h3>
                        <p className="text-text-muted text-sm mb-4">
                            {managementMode === 'copy'
                                ? `Select castles to copy settings FROM ${castles.find(c => c.id === managementSource)?.name}.`
                                : `Select castles to LINK to ${castles.find(c => c.id === managementSource)?.name}. Linked castles will automatically mirror these settings.`
                            }
                        </p>

                        <div className="flex items-center justify-between mb-2">
                            <div className="text-xs text-text-muted uppercase font-bold">Targets ({selectedTargets.length})</div>
                            <button onClick={handleSelectAll} className="text-xs text-primary hover:text-white font-bold">Select All Eligible</button>
                        </div>

                        <div className="flex-1 overflow-y-auto bg-bg-input rounded-global border border-border-base p-2 space-y-1 mb-6">
                            {castles.filter(c => c.id !== managementSource).map(castle => {
                                const isSelected = selectedTargets.includes(castle.id);
                                const isLinked = !!castleLinks[castle.id];
                                // If Linking mode: Cannot link a castle that is already linked (unless we overwrite, which is fine, but maybe warn?)
                                // For simplicity, let's allow overwrite.

                                return (
                                    <div
                                        key={castle.id}
                                        onClick={() => handleToggleTarget(castle.id)}
                                        className={`flex items-center gap-3 p-3 rounded-lg cursor-pointer transition-colors ${isSelected ? 'bg-primary/10 border border-primary/30' : 'hover:bg-white/5 border border-transparent'}`}
                                    >
                                        <div className={`w-5 h-5 rounded flex items-center justify-center ${isSelected ? 'bg-primary text-bg-app' : 'bg-bg-app border border-border-base'}`}>
                                            {isSelected && <CheckSquare className="w-3.5 h-3.5" />}
                                        </div>
                                        <div className="flex-1 min-w-0">
                                            <div className="font-bold text-sm truncate text-text-main">{castle.name}</div>
                                            {isLinked && <div className="text-[10px] text-blue-400">Currently Linked</div>}
                                        </div>
                                    </div>
                                );
                            })}
                        </div>

                        <div className="flex justify-end gap-3 pt-4 border-t border-border-base">
                            <button
                                onClick={closeManagement}
                                className="px-5 py-2 rounded-global text-text-muted hover:text-white font-bold transition-colors"
                            >
                                Cancel
                            </button>
                            <button
                                onClick={handleApplyManagement}
                                disabled={selectedTargets.length === 0}
                                className="px-6 py-2 rounded-global bg-primary text-bg-app font-bold hover:brightness-110 disabled:opacity-50 disabled:cursor-not-allowed transition-all"
                            >
                                {managementMode === 'copy' ? 'Copy Settings' : 'Link Castles'}
                            </button>
                        </div>
                    </div>
                </div>
            )}

            <div className="w-full h-full bg-bg-app flex flex-col">
                {/* Header */}
                <div className="h-16 border-b border-border-base flex items-center justify-between px-6 bg-glass-gradient">
                    <div className="flex items-center gap-3">
                        <div className="w-2 h-8 rounded-full bg-primary shadow-[0_0_10px] shadow-primary/50" />
                        <h2 className="heading-1">Auto Bird Settings</h2>
                    </div>
                    <button
                        onClick={onClose}
                        className="w-10 h-10 rounded-full flex items-center justify-center hover:bg-white/10 transition-colors"
                    >
                        <X className="w-6 h-6 text-text-muted hover:text-white" />
                    </button>
                </div>

                {/* Content */}
                <div className="flex-1 overflow-y-auto p-8 relative">
                    <div className="max-w-[1800px] mx-auto space-y-6">
                        <p className="text-text-muted">
                            Configure troops to keep (ignore) for each castle when auto-birding.
                            These units will NOT be sent.
                        </p>

                        {/* Global Config Section */}
                        <div className="glass-panel p-5 bg-bg-card/30 border border-border-base/50">
                            <h3 className="text-lg font-bold text-white mb-4 flex items-center gap-2">
                                <Settings className="w-5 h-5 text-primary" />
                                Global Settings
                            </h3>
                            <div className="flex items-end gap-6 flex-wrap">
                                <div>
                                    <label className="text-xs text-text-muted font-bold uppercase mb-2 block">Random Delay Range (Hours)</label>
                                    <div className="flex items-center gap-3">
                                        <div className="relative">
                                            <input
                                                type="number"
                                                min="1"
                                                max="12"
                                                value={delaySettings.min}
                                                onChange={e => {
                                                    const val = Math.max(1, Math.min(12, parseInt(e.target.value) || 1));
                                                    setDelaySettings(prev => ({ ...prev, min: val, max: Math.max(prev.max, val) }));
                                                }}
                                                className="w-24 bg-bg-input border border-border-base rounded-global px-3 py-2 text-center font-bold focus:border-primary focus:outline-none"
                                            />
                                            <span className="absolute right-2 top-2.5 text-xs text-text-muted">Min</span>
                                        </div>
                                        <span className="text-text-muted font-bold">-</span>
                                        <div className="relative">
                                            <input
                                                type="number"
                                                min="1"
                                                max="12"
                                                value={delaySettings.max}
                                                onChange={e => {
                                                    const val = Math.max(1, Math.min(12, parseInt(e.target.value) || 12));
                                                    setDelaySettings(prev => ({ ...prev, max: Math.max(prev.min, val) }));
                                                }}
                                                className="w-24 bg-bg-input border border-border-base rounded-global px-3 py-2 text-center font-bold focus:border-primary focus:outline-none"
                                            />
                                            <span className="absolute right-2 top-2.5 text-xs text-text-muted">Max</span>
                                        </div>
                                    </div>
                                </div>
                                <div className="pb-2 text-sm text-text-muted italic max-w-md">
                                    Birds will be sent with a random return delay between these hours (Max 12h).
                                </div>
                            </div>
                        </div>

                        {loading ? (
                            <div className="text-center py-10 text-text-muted">Loading configuration...</div>
                        ) : !castles || castles.length === 0 ? (
                            <div className="flex flex-col items-center justify-center py-12 px-4 text-center h-full animate-fade-in">
                                <div className="w-20 h-20 rounded-full bg-yellow-500/10 flex items-center justify-center mb-6 border border-yellow-500/20 shadow-[0_0_15px_rgba(234,179,8,0.1)]">
                                    <AlertTriangle className="w-10 h-10 text-yellow-500" />
                                </div>
                                <h3 className="heading-2 text-white mb-3">No Data Found</h3>
                                <p className="text-text-muted max-w-md mb-8 leading-relaxed">
                                    Please start the bot to load your castle data. <br />
                                    <span className="text-text-muted/60 text-sm">Once loaded, data will be saved for offline editing.</span>
                                </p>
                                <button
                                    onClick={onClose}
                                    className="px-8 py-3 rounded-global bg-primary/10 border border-primary/30 text-primary font-bold hover:bg-primary hover:text-bg-app transition-all hover:scale-105 active:scale-95"
                                >
                                    Close Settings
                                </button>
                            </div>
                        ) : (
                            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
                                {castles.map(castle => {
                                    const castleItems = ignoreList[castle.id] || [];
                                    const hasItems = castleItems.length > 0;
                                    const parentLink = castleLinks[castle.id];
                                    const parentName = parentLink ? castles.find(c => c.id === parentLink)?.name : null;
                                    const isLinked = !!parentLink;

                                    return (
                                        <div
                                            key={castle.id}
                                            className="glass-panel p-4 flex flex-col h-[320px] relative group hover:border-primary/50 transition-colors"
                                        >
                                            {/* Castle Name Header */}
                                            <div className="flex items-center justify-between mb-3 px-1 h-8">
                                                <div className="flex items-center gap-2 overflow-hidden">
                                                    <h3 className="text-lg font-bold text-primary truncate" title={castle.name}>
                                                        {castle.name}
                                                    </h3>
                                                    {isLinked && (
                                                        <span className="px-1.5 py-0.5 rounded-md bg-blue-500/20 text-blue-400 text-[10px] font-bold border border-blue-500/30 whitespace-nowrap">
                                                            Linked
                                                        </span>
                                                    )}
                                                </div>

                                                {/* Header Actions */}
                                                <div className="flex items-center gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                                                    {isLinked ? (
                                                        <button
                                                            onClick={() => handleUnlink(castle.id)}
                                                            className="p-1.5 rounded-md hover:bg-white/10 text-text-muted hover:text-red-400 transition-colors"
                                                            title={`Unlink from ${parentName}`}
                                                        >
                                                            <Unlink className="w-4 h-4" />
                                                        </button>
                                                    ) : (
                                                        <>
                                                            <button
                                                                onClick={() => openManagement('copy', castle.id)}
                                                                className="p-1.5 rounded-md hover:bg-white/10 text-text-muted hover:text-primary transition-colors"
                                                                title="Copy Settings To..."
                                                            >
                                                                <Copy className="w-4 h-4" />
                                                            </button>
                                                            <button
                                                                onClick={() => openManagement('link', castle.id)}
                                                                className="p-1.5 rounded-md hover:bg-white/10 text-text-muted hover:text-blue-400 transition-colors"
                                                                title="Link To..."
                                                            >
                                                                <LinkIcon className="w-4 h-4" />
                                                            </button>
                                                        </>
                                                    )}
                                                </div>
                                            </div>

                                            {/* Units Grid Area */}
                                            <div className={`flex-1 overflow-y-auto bg-bg-app/30 rounded-lg p-3 border ${isLinked ? 'border-blue-500/30 bg-blue-500/5' : 'border-border-base/50'}`}>
                                                {hasItems ? (
                                                    <div className="flex flex-wrap gap-3 content-start">
                                                        {castleItems.map((item, index) => (
                                                            <div
                                                                key={index}
                                                                className={`relative group/unit transition-transform hover:scale-105 cursor-pointer ${isLinked ? 'cursor-default pointer-events-none opacity-90' : ''}`}
                                                                title={`${allDefinitions[item.id] || 'Unknown Unit'} (x${item.amount})`}
                                                                onClick={() => !isLinked && openEditModal(castle.id, item)}
                                                            >
                                                                <UnitImage unitId={item.id} size={60} showLevel={true} />
                                                                {item.amount < 100000000 && (
                                                                    <div className="absolute -bottom-2 -right-2 bg-text-main border-2 border-bg-card rounded-full px-2 py-0.5 text-[10px] font-black text-bg-app shadow-md z-10 min-w-[24px] text-center">
                                                                        {item.amount.toLocaleString()}
                                                                    </div>
                                                                )}

                                                                {/* Hover overlay hint */}
                                                                {!isLinked && (
                                                                    <div className="absolute inset-0 bg-black/20 rounded-lg opacity-0 group-hover/unit:opacity-100 transition-opacity flex items-center justify-center text-white pointer-events-none">
                                                                        <Settings className="w-4 h-4 drop-shadow-md" />
                                                                    </div>
                                                                )}
                                                            </div>
                                                        ))}
                                                        {/* Add Button Card */}
                                                        {!isLinked && (
                                                            <button
                                                                onClick={() => handleAddItem(castle.id)}
                                                                className="flex flex-col items-center justify-center w-[60px] h-[60px] rounded-global border-2 border-dashed border-border-base text-text-muted hover:text-primary hover:border-primary hover:bg-primary/5 transition-all group/add"
                                                                title="Add Unit"
                                                            >
                                                                <Plus className="w-6 h-6 group-hover/add:scale-110 transition-transform" />
                                                            </button>
                                                        )}
                                                    </div>
                                                ) : (
                                                    <div className="h-full flex flex-col items-center justify-center">
                                                        <div className="text-text-muted/40 text-xs italic text-center mb-4">
                                                            No ignored units
                                                        </div>
                                                        {!isLinked && (
                                                            <button
                                                                onClick={() => handleAddItem(castle.id)}
                                                                className="flex items-center gap-2 px-4 py-2 rounded-global bg-bg-card border border-border-light hover:border-primary text-text-muted hover:text-primary transition-all group/empty-add"
                                                            >
                                                                <Plus className="w-4 h-4" />
                                                                <span className="font-bold text-sm">Add Unit</span>
                                                            </button>
                                                        )}
                                                    </div>
                                                )}
                                            </div>
                                        </div>
                                    );
                                })}
                            </div>
                        )}
                    </div>
                </div>

                {/* Footer */}
                <div className="h-20 border-t border-border-base flex items-center justify-end px-8 bg-glass-gradient gap-4">
                    <button
                        onClick={onClose}
                        className="px-6 py-2.5 rounded-global text-text-muted hover:text-white font-bold transition-colors"
                    >
                        CANCEL
                    </button>
                    <button
                        onClick={handleSave}
                        className="px-8 py-2.5 rounded-global bg-primary text-bg-app font-bold hover:brightness-110 transition-all shadow-lg shadow-primary/20 hover:shadow-primary/40 active:scale-95 flex items-center gap-2"
                    >
                        SAVE CHANGES
                    </button>
                </div>
            </div>
        </div>
    );
};
