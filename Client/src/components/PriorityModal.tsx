import React, { useState, useEffect } from 'react';
import { Icons } from './Icons';
import { FrontendWebsocket } from '../websocket';
import { Modal, Button, Card, CardHeader, CardContent } from './ui';

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

  const handleDragStart = (e: React.DragEvent, id: string) => {
    setDraggedTabId(id);
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', id);

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
    { id: 'P1', label: 'Priority 1 (High)', colorClass: 'text-error bg-error/10', borderClass: 'border-error/30' },
    { id: 'P2', label: 'Priority 2 (Medium)', colorClass: 'text-warning bg-warning/10', borderClass: 'border-warning/30' },
    { id: 'P3', label: 'Priority 3 (Low)', colorClass: 'text-success bg-success/10', borderClass: 'border-success/30' },
    { id: 'Ignored', label: 'Ignored', colorClass: 'text-text-muted bg-bg-input', borderClass: 'border-border-light' },
  ];

  const handleSave = () => {
    const priorityMap: Record<string, string> = {};
    tabs.forEach(t => priorityMap[t.id] = t.group);
    FrontendWebsocket.sendSaveSchedulerSettings({ tabPriorities: priorityMap });
    onClose();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      maxWidth="full"
      title={
        <div className="flex flex-col">
          <span className="flex items-center gap-2">
            <Icons.List className="w-5 h-5 text-primary" />
            Tab Priority Categorization
          </span>
          <p className="text-xs text-text-muted mt-1 font-normal">Drag and drop tabs to assign them to different attack scheduler priority queues.</p>
        </div>
      }
      footer={
        <Button variant="primary" onClick={handleSave} className="ml-auto">
          Save & Close
        </Button>
      }
    >
      <div className="flex gap-4 h-[calc(100vh-14rem)] min-w-max p-2 pb-6 custom-scrollbar overflow-x-auto">
        {columns.map(col => (
          <Card
            key={col.id}
            variant="solid"
            className={`w-[280px] flex flex-col ${col.borderClass} shrink-0 min-h-0`}
            onDragOver={handleDragOver}
            onDrop={(e) => handleDrop(e, col.id)}
          >
            <CardHeader className={`px-4 py-3 flex flex-row items-center justify-between border-b rounded-t-[calc(var(--radius-global)-1px)] ${col.borderClass} ${col.colorClass}`}>
              <span className="font-bold text-sm">{col.label}</span>
              <span className="text-xs font-mono bg-black/20 px-2 py-0.5 rounded-full text-current">
                {tabs.filter(t => t.group === col.id).length}
              </span>
            </CardHeader>

            <CardContent className="flex-1 p-3 overflow-y-auto space-y-3 relative group custom-scrollbar bg-bg-card/50">
              {tabs.filter(t => t.group === col.id).length === 0 && (
                <div className="absolute inset-0 flex flex-col items-center justify-center text-text-muted/50 p-4 text-center pointer-events-none">
                  <Icons.ArrowRight className="w-8 h-8 mb-2 opacity-20" />
                  <span className="text-xs font-medium">Drag tabs here</span>
                </div>
              )}

              {tabs.filter(t => t.group === col.id).map(tab => (
                <div
                  key={tab.id}
                  id={`tab-${tab.id}`}
                  draggable
                  onDragStart={(e) => handleDragStart(e, tab.id)}
                  onDragEnd={(e) => handleDragEnd(e, tab.id)}
                  className="group/item relative bg-bg-card border border-border-base p-3 rounded-global shadow-sm cursor-grab active:cursor-grabbing hover:border-primary/50 hover:shadow-[0_0_15px_var(--color-primary-glow)] transition-all flex items-start gap-3"
                >
                  <div className="mt-0.5 text-text-muted/50 cursor-grab group-hover/item:text-text-muted transition-colors">
                    <Icons.GripVertical className="w-4 h-4" />
                  </div>
                  <div className="flex-1 min-w-0">
                    <div className="text-sm font-semibold text-text-main truncate" title={tab.name}>
                      {tab.name}
                    </div>
                    <div className="text-[10px] text-text-muted mt-1 uppercase tracking-wider font-mono font-medium">
                      ID: {tab.id}
                    </div>
                  </div>
                </div>
              ))}
            </CardContent>
          </Card>
        ))}
      </div>
    </Modal>
  );
};

export default PriorityModal;
