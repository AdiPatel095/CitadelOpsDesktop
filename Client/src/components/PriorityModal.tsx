import React, { useState, useEffect } from 'react';
import { createPortal } from 'react-dom';
import { Icons } from './Icons';
import { FrontendWebsocket } from '../websocket';

interface PriorityModalProps {
    isOpen: boolean;
    onClose: () => void;
}

type PriorityGroup = 'P1' | 'P2' | 'P3' | 'Ignored';

interface TabItem {
    id: string;
    name: string;
    group: PriorityGroup;
}

const PriorityModal: React.FC<PriorityModalProps> = ({ isOpen, onClose }) => {
    // Some mock base tabs. Dynamic generation later.
    const [tabs, setTabs] = useState<TabItem[]>([
        { id: 't1', name: 'Main Castle Defense', group: 'Ignored' },
        { id: 't2', name: 'Outpost 1 Farm', group: 'Ignored' },
        { id: 't3', name: 'Resource Gathering', group: 'Ignored' },
        { id: 't4', name: 'Random Scouting', group: 'Ignored' },
        { id: 't5', name: 'Alliance Event', group: 'Ignored' },
    ]);

    useEffect(() => {
        const handleMessage = (msg: any) => {
            if (msg.type === 'schedulerSettings' && msg.payload && msg.payload.tabPriorities) {
                const priorities = msg.payload.tabPriorities as Record<string, string>;
                setTabs(prev => prev.map(t => ({
                    ...t,
                    group: (priorities[t.id] as PriorityGroup) || t.group
                })));
            }
        };

        if (isOpen) {
            FrontendWebsocket.addMessageListener(handleMessage);
            FrontendWebsocket.sendGetSchedulerSettings();
        }

        return () => {
            FrontendWebsocket.removeMessageListener(handleMessage);
        };
    }, [isOpen]);

    const [draggedTabId, setDraggedTabId] = useState<string | null>(null);

    useEffect(() => {
        if (isOpen) {
            document.body.style.overflow = 'hidden';
        } else {
            document.body.style.overflow = 'unset';
        }
        return () => {
            document.body.style.overflow = 'unset';
        };
    }, [isOpen]);

    if (!isOpen) return null;

    const handleDragStart = (e: React.DragEvent, id: string) => {
        setDraggedTabId(id);
        e.dataTransfer.effectAllowed = 'move';
        // Required for Firefox compatibility
        e.dataTransfer.setData('text/plain', id);

        // Slight delay to allow ghost image to capture correct state before modifying appearance
        setTimeout(() => {
            const el = document.getElementById(`tab-${id}`);
            if (el) el.classList.add('opacity-50');
        }, 0);
    };

    const handleDragEnd = (e: React.DragEvent, id: string) => {
        setDraggedTabId(null);
        const el = document.getElementById(`tab-${id}`);
        if (el) el.classList.remove('opacity-50');
    };

    const handleDragOver = (_e: React.DragEvent) => {
        _e.preventDefault();
        _e.dataTransfer.dropEffect = 'move';
    };

    const handleDrop = (e: React.DragEvent, targetGroup: PriorityGroup) => {
        e.preventDefault();
        const tabId = e.dataTransfer.getData('text/plain') || draggedTabId;

        if (tabId) {
            setTabs(prev => prev.map(tab =>
                tab.id === tabId ? { ...tab, group: targetGroup } : tab
            ));
        }
    };

    const columns: { id: PriorityGroup; label: string; colorClass: string; borderClass: string }[] = [
        { id: 'P1', label: 'Priority 1 (High)', colorClass: 'text-red-400 bg-red-400/10', borderClass: 'border-red-400/30' },
        { id: 'P2', label: 'Priority 2 (Medium)', colorClass: 'text-orange-400 bg-orange-400/10', borderClass: 'border-orange-400/30' },
        { id: 'P3', label: 'Priority 3 (Low)', colorClass: 'text-yellow-400 bg-yellow-400/10', borderClass: 'border-yellow-400/30' },
        { id: 'Ignored', label: 'Ignored', colorClass: 'text-text-muted bg-bg-input', borderClass: 'border-border-light' },
    ];

    return createPortal(
        <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/60 backdrop-blur-sm animate-fade-in p-4">
            <div className="bg-bg-card border border-border-base rounded-2xl shadow-2xl w-full max-w-5xl max-h-[90vh] flex flex-col overflow-hidden">
                {/* Header */}
                <div className="flex justify-between items-center px-6 py-4 border-b border-border-base bg-bg-card-hover/50 shrink-0">
                    <div>
                        <h2 className="text-xl font-bold text-text-main flex items-center gap-2">
                            <Icons.List className="w-5 h-5 text-primary" />
                            Tab Priority Categorization
                        </h2>
                        <p className="text-xs text-text-muted mt-1">Drag and drop tabs to assign them to different attack scheduler priority queues.</p>
                    </div>
                    <button
                        onClick={onClose}
                        className="p-2 text-text-muted hover:text-text-main hover:bg-bg-input rounded-full transition-colors focus:outline-none focus:ring-2 focus:ring-primary/50"
                        aria-label="Close modal"
                    >
                        <Icons.X className="w-5 h-5" />
                    </button>
                </div>

                {/* Board Body */}
                <div className="flex-1 p-6 overflow-x-auto overflow-y-hidden bg-bg-app/50">
                    <div className="flex gap-4 h-full min-w-max">
                        {columns.map(col => (
                            <div
                                key={col.id}
                                className={`w-[260px] flex flex-col bg-bg-card rounded-xl border ${col.borderClass} shadow-md overflow-hidden`}
                                onDragOver={handleDragOver}
                                onDrop={(e) => handleDrop(e, col.id)}
                            >
                                {/* Column Header */}
                                <div className={`px-4 py-3 flex items-center justify-between border-b ${col.borderClass} ${col.colorClass}`}>
                                    <span className="font-bold text-sm">{col.label}</span>
                                    <span className="text-xs font-mono bg-black/20 px-2 py-0.5 rounded-full">
                                        {tabs.filter(t => t.group === col.id).length}
                                    </span>
                                </div>

                                {/* Column Content */}
                                <div className="flex-1 p-3 overflow-y-auto space-y-3 relative group">
                                    {tabs.filter(t => t.group === col.id).length === 0 && (
                                        <div className="absolute inset-0 flex flex-col items-center justify-center text-text-muted/50 p-4 text-center pointer-events-none">
                                            <Icons.ArrowRight className="w-8 h-8 mb-2 opacity-20" />
                                            <span className="text-xs">Drag tabs here</span>
                                        </div>
                                    )}

                                    {tabs.filter(t => t.group === col.id).map(tab => (
                                        <div
                                            key={tab.id}
                                            id={`tab-${tab.id}`}
                                            draggable
                                            onDragStart={(e) => handleDragStart(e, tab.id)}
                                            onDragEnd={(e) => handleDragEnd(e, tab.id)}
                                            className="group/item relative bg-bg-card-hover border border-border-light p-3 rounded-lg shadow-sm cursor-grab active:cursor-grabbing hover:border-primary/50 hover:shadow-primary/10 transition-all flex items-start gap-3"
                                        >
                                            <div className="mt-0.5 text-text-muted/50 cursor-grab group-hover/item:text-text-muted transition-colors">
                                                <Icons.GripVertical className="w-4 h-4" />
                                            </div>
                                            <div className="flex-1 min-w-0">
                                                <div className="text-sm font-medium text-text-main truncate" title={tab.name}>
                                                    {tab.name}
                                                </div>
                                                <div className="text-[10px] text-text-muted mt-1 uppercase tracking-wider font-mono">
                                                    ID: {tab.id}
                                                </div>
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            </div>
                        ))}
                    </div>
                </div>

                {/* Footer */}
                <div className="px-6 py-4 border-t border-border-base bg-bg-card-hover/50 flex justify-end shrink-0">
                    <button
                        onClick={() => {
                            const priorityMap: Record<string, string> = {};
                            tabs.forEach(t => priorityMap[t.id] = t.group);
                            FrontendWebsocket.sendSaveSchedulerSettings({ tabPriorities: priorityMap });
                            onClose();
                        }}
                        className="px-6 py-2 bg-bg-input hover:bg-border-light border border-border-base text-text-main rounded-global font-medium transition-colors text-sm"
                    >
                        Save & Close
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
};

export default PriorityModal;
