import React, { useState, useEffect } from 'react';
import { Trash2, Save, Plus, Settings } from 'lucide-react';
import { FrontendWebsocket } from '../../Websocket';
import { showTroopPicker } from '../../components/TroopPickerModal';
import type { UnitWithQuantity } from '../../components/TroopPickerModal';
import UnitImage from '../../components/UnitImage';
import { TROOP_DEFINITIONS } from '../../config/Constants';
import { Modal, Button, Input, Card, CardHeader, CardTitle, CardContent, Badge } from '../../components/ui';

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
  const [settings, setSettings] = useState<Record<string, RecruitItem[]>>({});
  const [isSaving, setIsSaving] = useState(false);

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

  return (
    <>
      <Modal
        isOpen={isOpen}
        onClose={onClose}
        maxWidth="full"
        title={
          <div className="flex flex-col">
            <span className="text-warning flex items-center gap-2">
              <span className="w-2 h-2 rounded-full bg-warning shadow-[0_0_8px_var(--color-warning)]" />
              Recruit Troops Settings
            </span>
            <p className="mt-1 text-sm text-text-muted font-normal">
              Configure target troop counts per castle. The bot will automatically recruit to reach these counts.
            </p>
          </div>
        }
        footer={
          <>
            <Button variant="ghost" onClick={onClose} className="px-6">Cancel</Button>
            <Button variant="primary" onClick={handleSave} disabled={isSaving} className="px-8" leftIcon={<Save className="w-4 h-4" />}>
              {isSaving ? 'Saving...' : 'Save Settings'}
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-6 max-w-6xl mx-auto w-full h-[calc(100vh-14rem)] overflow-y-auto custom-scrollbar pb-6 pr-2">
          {castles.map((castle) => {
            const castleId = castle.id.toString();
            const castleItems = settings[castleId] || [];
            const hasItems = castleItems.length > 0;

            return (
              <Card key={castle.id} variant="solid" className="border-border-base bg-bg-app flex flex-col min-h-0">
                <CardHeader className="bg-bg-card-hover py-3 border-b border-border-base flex flex-row items-center justify-between rounded-t-[calc(var(--radius-global)-1px)]">
                  <div className="flex items-center gap-3">
                    <CardTitle className="text-base text-text-main">{castle.name}</CardTitle>
                    {hasItems && (
                      <Badge variant="primary">{castleItems.length} unit{castleItems.length !== 1 ? 's' : ''}</Badge>
                    )}
                  </div>
                </CardHeader>

                <CardContent className="p-4 bg-bg-app/30">
                  {hasItems ? (
                    <div className="flex flex-wrap gap-4 content-start">
                      {castleItems.map((item, index) => (
                        <div
                          key={index}
                          className="relative group/unit transition-transform hover:-translate-y-1 cursor-pointer"
                          title={`${TROOP_DEFINITIONS[item.id] || 'Unknown Unit'} (x${item.amount})`}
                          onClick={() => openEditModal(castleId, item)}
                        >
                          <UnitImage unitId={item.id} size={64} showLevel={true} className="rounded-xl shadow-sm" />
                          <div className="absolute -bottom-2 left-1/2 -translate-x-1/2 bg-text-main border border-bg-card rounded-full px-2 py-0.5 text-[10px] font-black text-bg-app shadow-md z-10 min-w-[32px] text-center">
                            {item.amount.toLocaleString()}
                          </div>

                          <div className="absolute inset-0 bg-black/30 rounded-xl opacity-0 group-hover/unit:opacity-100 transition-opacity flex items-center justify-center text-white pointer-events-none">
                            <Settings className="w-5 h-5 drop-shadow-md" />
                          </div>
                        </div>
                      ))}
                      <button
                        onClick={() => handleAddItem(castleId)}
                        className="flex flex-col items-center justify-center w-[64px] h-[64px] rounded-xl border-2 border-dashed border-border-base text-text-muted hover:text-primary hover:border-primary hover:bg-primary/5 transition-all group/add"
                        title="Add Unit"
                      >
                        <Plus className="w-6 h-6 group-hover/add:scale-110 transition-transform" />
                      </button>
                    </div>
                  ) : (
                    <div className="flex flex-col items-center justify-center py-6">
                      <div className="text-text-muted/60 text-xs font-bold uppercase tracking-wider text-center mb-3">
                        No recruitment targets
                      </div>
                      <Button
                        variant="outline"
                        size="sm"
                        onClick={() => handleAddItem(castleId)}
                        leftIcon={<Plus className="w-4 h-4" />}
                      >
                        Add Unit
                      </Button>
                    </div>
                  )}
                </CardContent>
              </Card>
            );
          })}
        </div>
      </Modal>

      <Modal
        isOpen={!!editingUnit}
        onClose={closeEditModal}
        maxWidth="sm"
        title={`Edit ${editingUnit ? TROOP_DEFINITIONS[editingUnit.item.id] || 'Unit' : ''}`}
        footer={
          <>
            <Button variant="danger" onClick={deleteFromEditModal} leftIcon={<Trash2 className="w-4 h-4" />}>Remove</Button>
            <Button variant="primary" onClick={saveEditModal} className="flex-[2]">Save</Button>
          </>
        }
      >
        <div className="flex flex-col items-center gap-6 py-4">
          {editingUnit && (
            <UnitImage unitId={editingUnit.item.id} size={80} showLevel={true} className="rounded-2xl shadow-lg" />
          )}

          <div className="w-full">
            <label className="text-[10px] text-text-muted font-bold uppercase tracking-wider mb-2 block text-center">Target Amount</label>
            <Input
              type="text"
              value={editAmount}
              onChange={handleQuantityChange}
              className="text-center text-xl font-bold font-mono py-3"
              autoFocus
              placeholder="0"
            />
          </div>
        </div>
      </Modal>
    </>
  );
};
