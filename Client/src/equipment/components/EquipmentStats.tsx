import React, { useMemo, useState, useEffect } from 'react';

import { statDisplayName, commanderStatGroups, castellanStatGroups, statGroupDisplayName, type CommStat, type CastStat } from '../models/Equipment';
import { FrontendWebsocket } from '../../Websocket';
import { useResources } from '../../currency/context/ResourceContext';
import { coinsUnderUpgradeReserve } from '../../utils/UpgradeCoinReserve';
import { Button, Badge, Modal, Input } from '../../components/ui';

// Equipment slot definitions
const EQUIPMENT_SLOTS = [
  { slot: 1, name: 'Armor' },
  { slot: 2, name: 'Weapon' },
  { slot: 3, name: 'Helmet' },
  { slot: 4, name: 'Artifact' },
  { slot: 6, name: 'Hero' },
];

const GEM_SLOTS = [
  { slot: 1, name: 'Gem Slot 1' },
  { slot: 2, name: 'Gem Slot 2' },
  { slot: 3, name: 'Gem Slot 3' },
  { slot: 4, name: 'Gem Slot 4' },
];

// Unequip Equipment Modal - Multi-select
interface UnequipEquipmentModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: (selections: Array<{ slotNumber: number; equipmentId: number }>) => void;
  selectedItem: CommStat | CastStat | null;
}

const UnequipEquipmentModal: React.FC<UnequipEquipmentModalProps> = ({ isOpen, onClose, onConfirm, selectedItem }) => {
  const [selectedSlots, setSelectedSlots] = useState<Set<number>>(new Set());

  if (!isOpen) return null;

  const getEquipmentId = (slot: number): number => {
    if (!selectedItem) return 0;
    switch (slot) {
      case 1: return selectedItem.equip1;
      case 2: return selectedItem.equip2;
      case 3: return selectedItem.equip3;
      case 4: return selectedItem.equip4;
      case 6: return selectedItem.hero;
      default: return 0;
    }
  };

  const toggleSlot = (slot: number) => {
    const newSet = new Set(selectedSlots);
    if (newSet.has(slot)) {
      newSet.delete(slot);
    } else {
      newSet.add(slot);
    }
    setSelectedSlots(newSet);
  };

  const handleConfirm = () => {
    if (selectedSlots.size > 0) {
      const selections = Array.from(selectedSlots).map(slot => ({
        slotNumber: slot,
        equipmentId: getEquipmentId(slot)
      }));
      onConfirm(selections);
      setSelectedSlots(new Set());
    }
  };

  const handleClose = () => {
    setSelectedSlots(new Set());
    onClose();
  };

  // Filter to only show slots that have equipment
  const availableSlots = EQUIPMENT_SLOTS.filter(({ slot }) => getEquipmentId(slot) !== 0);

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={
        <div className="flex flex-col items-center pt-2">
          <div className="w-16 h-16 rounded-full bg-primary/20 flex items-center justify-center mb-4">
            <svg className="w-8 h-8 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
          </div>
          Unequip Equipment
        </div>
      }
      footer={
        <>
          <Button variant="ghost" onClick={handleClose} className="flex-1">
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleConfirm}
            disabled={selectedSlots.size === 0}
            className="flex-1"
          >
            Unequip {selectedSlots.size > 0 ? `(${selectedSlots.size})` : ''}
          </Button>
        </>
      }
    >
      <div className="flex flex-col items-center">
        <p className="text-text-muted text-center mb-6">
          Select equipment to unequip from <span className="text-primary font-semibold">{selectedItem?.name}</span>
        </p>

        <div className="w-full space-y-2">
          {availableSlots.length === 0 ? (
            <p className="text-text-muted text-center py-4">No equipment to unequip</p>
          ) : (
            availableSlots.map(({ slot, name }) => {
              const equipId = getEquipmentId(slot);
              const isSelected = selectedSlots.has(slot);
              return (
                <div
                  key={slot}
                  className={`
                    flex items-center gap-3 p-3 rounded-global border cursor-pointer transition-all duration-200
                    ${isSelected
                      ? 'bg-primary/10 border-primary/50 text-text-main'
                      : 'bg-bg-app/50 border-border-base hover:bg-bg-card-hover text-text-muted'
                    }
                  `}
                  onClick={() => toggleSlot(slot)}
                >
                  <div className={`
                    w-5 h-5 rounded border-2 flex items-center justify-center transition-all duration-200 shrink-0
                    ${isSelected ? 'bg-primary border-primary' : 'bg-bg-app/50 border-border-base group-hover:border-text-muted'}
                  `}>
                    {isSelected && (
                      <svg className="w-3 h-3 text-text-inverted" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
                      </svg>
                    )}
                  </div>
                  <span className="flex-1 font-medium">{name}</span>
                  <span className="text-xs font-mono text-text-muted opacity-75">
                    ID: {equipId}
                  </span>
                </div>
              );
            })
          )}
        </div>
      </div>
    </Modal>
  );
};

// Unequip Gem Modal - Multi-select
interface UnequipGemModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: (selections: Array<{ slotNumber: number; gemId: number; equipmentId: number }>) => void;
  selectedItem: CommStat | CastStat | null;
}

const UnequipGemModal: React.FC<UnequipGemModalProps> = ({ isOpen, onClose, onConfirm, selectedItem }) => {
  const [selectedSlots, setSelectedSlots] = useState<Set<number>>(new Set());

  if (!isOpen) return null;

  const getGemId = (slot: number): number => {
    if (!selectedItem) return 0;
    switch (slot) {
      case 1: return selectedItem.gem1;
      case 2: return selectedItem.gem2;
      case 3: return selectedItem.gem3;
      case 4: return selectedItem.gem4;
      default: return 0;
    }
  };

  const getEquipmentId = (slot: number): number => {
    if (!selectedItem) return 0;
    switch (slot) {
      case 1: return selectedItem.equip1;
      case 2: return selectedItem.equip2;
      case 3: return selectedItem.equip3;
      case 4: return selectedItem.equip4;
      default: return 0;
    }
  };

  const toggleSlot = (slot: number) => {
    const newSet = new Set(selectedSlots);
    if (newSet.has(slot)) {
      newSet.delete(slot);
    } else {
      newSet.add(slot);
    }
    setSelectedSlots(newSet);
  };

  const handleConfirm = () => {
    if (selectedSlots.size > 0) {
      const selections = Array.from(selectedSlots).map(slot => ({
        slotNumber: slot,
        gemId: getGemId(slot),
        equipmentId: getEquipmentId(slot)
      }));
      onConfirm(selections);
      setSelectedSlots(new Set());
    }
  };

  const handleClose = () => {
    setSelectedSlots(new Set());
    onClose();
  };

  const availableSlots = GEM_SLOTS.filter(({ slot }) => getGemId(slot) !== 0);

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={
        <div className="flex flex-col items-center pt-2">
          <div className="w-16 h-16 rounded-full bg-purple-500/20 flex items-center justify-center mb-4">
            <svg className="w-8 h-8 text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
            </svg>
          </div>
          Unequip Gem
        </div>
      }
      footer={
        <>
          <Button variant="ghost" onClick={handleClose} className="flex-1">
            Cancel
          </Button>
          <Button
            onClick={handleConfirm}
            disabled={selectedSlots.size === 0}
            className={`flex-1 bg-purple-500 text-white hover:bg-purple-600 ${selectedSlots.size === 0 ? 'opacity-50 cursor-not-allowed' : ''}`}
          >
            Unequip {selectedSlots.size > 0 ? `(${selectedSlots.size})` : ''}
          </Button>
        </>
      }
    >
      <div className="flex flex-col items-center">
        <p className="text-text-muted text-center mb-6">
          Select gems to unequip from <span className="text-purple-400 font-semibold">{selectedItem?.name}</span>
        </p>

        <div className="w-full space-y-2">
          {availableSlots.length === 0 ? (
            <p className="text-text-muted text-center py-4">No gems to unequip</p>
          ) : (
            availableSlots.map(({ slot, name }) => {
              const gemId = getGemId(slot);
              const isSelected = selectedSlots.has(slot);
              return (
                <div
                  key={slot}
                  className={`
                    flex items-center gap-3 p-3 rounded-global border cursor-pointer transition-all duration-200
                    ${isSelected
                      ? 'bg-purple-500/10 border-purple-500/50 text-text-main'
                      : 'bg-bg-app/50 border-border-base hover:bg-bg-card-hover text-text-muted'
                    }
                  `}
                  onClick={() => toggleSlot(slot)}
                >
                  <div className={`
                    w-5 h-5 rounded border-2 flex items-center justify-center transition-all duration-200 shrink-0
                    ${isSelected ? 'bg-purple-500 border-purple-500' : 'bg-bg-app/50 border-border-base group-hover:border-text-muted'}
                  `}>
                    {isSelected && (
                      <svg className="w-3 h-3 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
                        <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={3} d="M5 13l4 4L19 7" />
                      </svg>
                    )}
                  </div>
                  <span className="flex-1 font-medium">{name}</span>
                  <span className="text-xs font-mono text-text-muted opacity-75">
                    Gem ID: {gemId}
                  </span>
                </div>
              );
            })
          )}
        </div>
      </div>
    </Modal>
  );
};

interface SlotUpgradeInfo {
  slotNumber: number;
  itemId: number;
  level: number;
  label: string;
}

const UPGRADE_COINS_LOCKED_LABEL = 'Coins under threshold';

interface UpgradeModalProps {
  kind: 'equipment' | 'gem';
  isOpen: boolean;
  onClose: () => void;
  onConfirm: (selection: { slotNumber: number; itemId: number; targetLevel: number }) => void;
  selectedItem: CommStat | CastStat | null;
  equipmentMode: 'Commander' | 'Castellan';
  selectedIndex: number | null;
  coinsUnderThreshold: boolean;
}

const MAX_UPGRADE_LEVEL = 50;

const UpgradeModal: React.FC<UpgradeModalProps> = ({
  kind,
  isOpen,
  onClose,
  onConfirm,
  selectedItem,
  equipmentMode,
  selectedIndex,
  coinsUnderThreshold,
}) => {
  const [slots, setSlots] = useState<SlotUpgradeInfo[]>([]);
  const [loading, setLoading] = useState(false);
  const [selectedSlot, setSelectedSlot] = useState<number | null>(null);
  const [targetLevel, setTargetLevel] = useState<number>(1);

  const selectedSlotInfo = slots.find(s => s.slotNumber === selectedSlot) ?? null;
  const minTarget = selectedSlotInfo ? Math.min(MAX_UPGRADE_LEVEL, Math.floor(selectedSlotInfo.level) + 1) : 1;
  const isSlotAtMaxLevel = (level: number) => Math.floor(level) >= MAX_UPGRADE_LEVEL;
  const allSlotsAtMax = slots.length > 0 && slots.every(({ level }) => isSlotAtMaxLevel(level));

  useEffect(() => {
    if (!isOpen) {
      setSlots([]);
      setSelectedSlot(null);
      setTargetLevel(1);
      setLoading(false);
      return;
    }
    if (selectedIndex === null) return;

    const handleMessage = (message: { type?: string; payload?: { equipment?: SlotUpgradeInfo[]; gems?: SlotUpgradeInfo[] } }) => {
      if (message.type !== 'equipmentUpgradeInfo') return;
      const list = kind === 'equipment' ? message.payload?.equipment : message.payload?.gems;
      setSlots(list ?? []);
      setLoading(false);
    };

    FrontendWebsocket.addMessageListener(handleMessage);
    setLoading(true);
    FrontendWebsocket.sendMessage({
      type: 'getEquipmentUpgradeInfo',
      payload: { equipmentMode, targetIndex: selectedIndex },
    });

    return () => FrontendWebsocket.removeMessageListener(handleMessage);
  }, [isOpen, kind, equipmentMode, selectedIndex]);

  useEffect(() => {
    if (selectedSlotInfo) {
      setTargetLevel(minTarget);
    }
  }, [selectedSlotInfo?.slotNumber, minTarget]);

  if (!isOpen) return null;

  const title = kind === 'equipment' ? 'Upgrade Equipment' : 'Upgrade Gem';

  const handleConfirm = () => {
    if (coinsUnderThreshold) return;
    if (!selectedSlotInfo) return;
    if (isSlotAtMaxLevel(selectedSlotInfo.level)) return;
    if (targetLevel < minTarget || targetLevel > MAX_UPGRADE_LEVEL) return;
    onConfirm({
      slotNumber: selectedSlotInfo.slotNumber,
      itemId: selectedSlotInfo.itemId,
      targetLevel,
    });
    setSelectedSlot(null);
    setTargetLevel(1);
  };

  const handleClose = () => {
    setSelectedSlot(null);
    setTargetLevel(1);
    onClose();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      title={
        <div className="flex flex-col items-center pt-2">
          <div className={`w-16 h-16 rounded-full flex items-center justify-center mb-4 ${kind === 'equipment' ? 'bg-primary/20' : 'bg-purple-500/20'}`}>
            <svg className={`w-8 h-8 ${kind === 'equipment' ? 'text-primary' : 'text-purple-400'}`} fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M5 10l7-7m0 0l7 7m-7-7v18" />
            </svg>
          </div>
          {title}
        </div>
      }
      footer={
        <>
          <Button variant="ghost" onClick={handleClose} className="flex-1">
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleConfirm}
            disabled={
              coinsUnderThreshold
              || !selectedSlotInfo
              || loading
              || targetLevel < minTarget
              || isSlotAtMaxLevel(selectedSlotInfo?.level ?? 0)
            }
            className={`flex-1 ${kind === 'gem' ? 'bg-purple-500 hover:bg-purple-600 border-purple-500' : ''}`}
          >
            {coinsUnderThreshold ? UPGRADE_COINS_LOCKED_LABEL : `Upgrade to ${targetLevel}`}
          </Button>
        </>
      }
    >
      <div className="flex flex-col items-center">
        <p className="text-text-muted text-center mb-6">
          Select one {kind === 'equipment' ? 'piece' : 'gem'} on{' '}
          <span className={`font-semibold ${kind === 'equipment' ? 'text-primary' : 'text-purple-400'}`}>{selectedItem?.name}</span>
        </p>

        <div className="w-full space-y-2 mb-6">
          {loading ? (
            <p className="text-text-muted text-center py-4">Loading levels…</p>
          ) : slots.length === 0 ? (
            <p className="text-text-muted text-center py-4">No {kind === 'equipment' ? 'equipment' : 'gems'} to upgrade</p>
          ) : allSlotsAtMax ? (
            <p className="text-text-muted text-center py-4">All {kind === 'equipment' ? 'equipment' : 'gems'} are already at max level ({MAX_UPGRADE_LEVEL})</p>
          ) : (
            slots.map(({ slotNumber, label, itemId, level }) => {
              const isSelected = selectedSlot === slotNumber;
              const atMaxLevel = isSlotAtMaxLevel(level);
              return (
                <div
                  key={slotNumber}
                  aria-disabled={atMaxLevel}
                  className={`
                    flex items-center gap-3 p-3 rounded-global border transition-all duration-200
                    ${atMaxLevel
                      ? 'bg-bg-app/30 border-border-base/50 text-text-muted opacity-60 cursor-not-allowed'
                      : isSelected
                        ? kind === 'equipment'
                          ? 'bg-primary/10 border-primary/50 text-text-main cursor-pointer'
                          : 'bg-purple-500/10 border-purple-500/50 text-text-main cursor-pointer'
                        : 'bg-bg-app/50 border-border-base hover:bg-bg-card-hover text-text-muted cursor-pointer'
                    }
                  `}
                  onClick={() => {
                    if (!atMaxLevel) setSelectedSlot(slotNumber);
                  }}
                >
                  <div className={`
                    w-5 h-5 rounded-full border-2 flex items-center justify-center transition-all duration-200 shrink-0
                    ${atMaxLevel
                      ? 'bg-bg-app/50 border-border-base/70'
                      : isSelected
                        ? kind === 'equipment' ? 'border-primary bg-primary' : 'border-purple-500 bg-purple-500'
                        : 'bg-bg-app/50 border-border-base'
                    }
                  `}>
                    {isSelected && !atMaxLevel && <div className="w-2 h-2 rounded-full bg-white" />}
                  </div>
                  <span className="flex-1 font-medium">{label}</span>
                  {atMaxLevel ? (
                    <Badge variant="outline" className="text-[10px] uppercase tracking-wide">
                      Max level
                    </Badge>
                  ) : (
                    <span className="text-xs font-mono text-text-muted">Lv {Math.floor(level)}</span>
                  )}
                  <span className="text-xs font-mono text-text-muted opacity-75">ID: {itemId}</span>
                </div>
              );
            })
          )}
        </div>

        {selectedSlotInfo && (
          <div className="w-full bg-bg-app border border-border-base rounded-global p-4">
            <label className="text-sm font-medium text-text-main block mb-2">
              Target level ({minTarget}–{MAX_UPGRADE_LEVEL})
            </label>
            <Input
              type="number"
              min={minTarget}
              max={MAX_UPGRADE_LEVEL}
              value={targetLevel}
              onChange={(e) => {
                const parsed = parseInt(e.target.value, 10);
                if (!Number.isNaN(parsed)) {
                  setTargetLevel(Math.max(minTarget, Math.min(MAX_UPGRADE_LEVEL, parsed)));
                }
              }}
            />
            <p className="text-xs text-text-muted mt-2">
              Current level: {Math.floor(selectedSlotInfo.level)}. Upgrades stop on insufficient shards/coins or errors.
            </p>
          </div>
        )}
      </div>
    </Modal>
  );
};

interface EquipmentStatsProps {
  equipmentMode: 'Commander' | 'Castellan';
  combatMode: 'PvP' | 'PvE';
  selectedItem: CommStat | CastStat | null;
  selectedIndex: number | null;
}

function resolveEquippedItemId(selectedItem: CommStat | CastStat | null, slot: number): number {
  if (!selectedItem) return 0;
  switch (slot) {
    case 1: return selectedItem.equip1;
    case 2: return selectedItem.equip2;
    case 3: return selectedItem.equip3;
    case 4: return selectedItem.equip4;
    case 6: return selectedItem.hero;
    default: return 0;
  }
}

function resolveEquippedGemId(selectedItem: CommStat | CastStat | null, slot: number): number {
  if (!selectedItem) return 0;
  switch (slot) {
    case 1: return selectedItem.gem1;
    case 2: return selectedItem.gem2;
    case 3: return selectedItem.gem3;
    case 4: return selectedItem.gem4;
    default: return 0;
  }
}

const EquipmentStats: React.FC<EquipmentStatsProps> = ({ equipmentMode, combatMode, selectedItem, selectedIndex }) => {
  const [showEquipmentModal, setShowEquipmentModal] = useState(false);
  const [showGemModal, setShowGemModal] = useState(false);
  const [showUpgradeEquipmentModal, setShowUpgradeEquipmentModal] = useState(false);
  const [showUpgradeGemModal, setShowUpgradeGemModal] = useState(false);
  const [upgradeCoinThreshold, setUpgradeCoinThreshold] = useState(0);
  const { resources } = useResources();
  const stats = selectedItem;
  const name = selectedItem?.name;

  useEffect(() => {
    const handleMessage = (msg: any) => {
      if (msg.type === 'schedulerSettings' && msg.payload) {
        setUpgradeCoinThreshold(Number(msg.payload.upgradeCoinThreshold ?? 0));
      }
    };

    FrontendWebsocket.addMessageListener(handleMessage);
    FrontendWebsocket.sendGetSchedulerSettings();

    return () => {
      FrontendWebsocket.removeMessageListener(handleMessage);
    };
  }, []);

  const coinsUnderThreshold =
    resources != null && coinsUnderUpgradeReserve(resources.coins, upgradeCoinThreshold);

  // Handler for unequip equipment - accepts multiple selections
  const handleUnequipEquipment = (selections: Array<{ slotNumber: number; equipmentId: number }>) => {
    if (!selectedItem || selectedIndex === null) return;

    FrontendWebsocket.sendMessage({
      type: 'unequipEquipment',
      payload: {
        equipmentMode,
        targetIndex: selectedIndex,
        selections, // Array of { slotNumber, equipmentId }
      }
    });
    setShowEquipmentModal(false);
  };

  // Handler for unequip gem - accepts multiple selections
  const handleUnequipGem = (selections: Array<{ slotNumber: number; gemId: number; equipmentId: number }>) => {
    if (!selectedItem || selectedIndex === null) return;

    FrontendWebsocket.sendMessage({
      type: 'unequipGem',
      payload: {
        equipmentMode,
        targetIndex: selectedIndex,
        selections, // Array of { slotNumber, gemId, equipmentId }
      }
    });
    setShowGemModal(false);
  };

  const handleUpgradeEquipment = (selection: { slotNumber: number; itemId: number; targetLevel: number }) => {
    if (coinsUnderThreshold) return;
    if (selectedIndex === null || !selectedItem) return;
    const equipmentId = resolveEquippedItemId(selectedItem, selection.slotNumber) || selection.itemId;
    if (!equipmentId) return;
    FrontendWebsocket.sendMessage({
      type: 'upgradeEquipment',
      payload: {
        equipmentMode,
        targetIndex: selectedIndex,
        slotNumber: selection.slotNumber,
        equipmentId,
        targetLevel: selection.targetLevel,
      },
    });
    setShowUpgradeEquipmentModal(false);
  };

  const handleUpgradeGem = (selection: { slotNumber: number; itemId: number; targetLevel: number }) => {
    if (coinsUnderThreshold) return;
    if (selectedIndex === null || !selectedItem) return;
    const gemId = resolveEquippedGemId(selectedItem, selection.slotNumber) || selection.itemId;
    if (!gemId) return;
    FrontendWebsocket.sendMessage({
      type: 'upgradeGem',
      payload: {
        equipmentMode,
        targetIndex: selectedIndex,
        slotNumber: selection.slotNumber,
        gemId,
        targetLevel: selection.targetLevel,
      },
    });
    setShowUpgradeGemModal(false);
  };

  const processedStats = useMemo(() => {
    if (!stats) return {};

    const newStats: { [key: string]: number } = {};
    const statGroups = equipmentMode === 'Commander' ? commanderStatGroups : castellanStatGroups;
    const allKeys = Object.values(statGroups).flat();

    for (const key of allKeys) {
      const isSpecialStat = ['glory', 'later', 'fire', 'early'].includes(key);
      let baseKey = key;
      if (isSpecialStat) {
        baseKey = combatMode === 'PvP' ? `CL${key.charAt(0).toUpperCase() + key.slice(1)}` : `NPC${key.charAt(0).toUpperCase() + key.slice(1)}`;
      }

      let finalValue = (stats as any)[baseKey] || 0;

      if (!isSpecialStat) {
        let suffix = key;

        if (key === 'frontLimit') {
          suffix = 'Front';
        } else if (key === 'flankLimit') {
          suffix = 'Flank';
        } else if (key.endsWith('CbtStr')) {
          suffix = key.replace('CbtStr', '');
        } else if (key.endsWith('Str')) {
          suffix = key.replace('Str', '');
        }

        const capitalizedSuffix = suffix.charAt(0).toUpperCase() + suffix.slice(1);

        if (key === 'frontCbtStr' || key === 'flankCbtStr') {
          // Do nothing
        } else {
          if (combatMode === 'PvP') {
            const clKey = `CL${capitalizedSuffix}`;
            if ((stats as any)[clKey]) {
              finalValue += (stats as any)[clKey];
            }
          } else { // PvE
            const npcKey = `NPC${capitalizedSuffix}`;
            if ((stats as any)[npcKey]) {
              finalValue += (stats as any)[npcKey];
            }
          }
        }
      }

      newStats[key] = finalValue;
    }

    return newStats;
  }, [stats, combatMode, equipmentMode]);

  const renderStat = (statKey: string, value: number) => {
    const label = statDisplayName[statKey] || statKey;
    return (
      <div className="flex items-center justify-between py-1.5 px-2 rounded hover:bg-bg-card-hover transition-colors" key={statKey}>
        <span className="text-sm text-text-muted">{label}</span>
        <span className="text-sm font-mono font-medium text-primary">{value.toFixed(2)}</span>
      </div>
    );
  };

  const statGroups = equipmentMode === 'Commander' ? commanderStatGroups : castellanStatGroups;

  return (
    <div className="p-4">
      <UnequipEquipmentModal
        isOpen={showEquipmentModal}
        onClose={() => setShowEquipmentModal(false)}
        onConfirm={handleUnequipEquipment}
        selectedItem={selectedItem}
      />
      <UnequipGemModal
        isOpen={showGemModal}
        onClose={() => setShowGemModal(false)}
        onConfirm={handleUnequipGem}
        selectedItem={selectedItem}
      />
      <UpgradeModal
        kind="equipment"
        isOpen={showUpgradeEquipmentModal}
        onClose={() => setShowUpgradeEquipmentModal(false)}
        onConfirm={handleUpgradeEquipment}
        selectedItem={selectedItem}
        equipmentMode={equipmentMode}
        selectedIndex={selectedIndex}
        coinsUnderThreshold={coinsUnderThreshold}
      />
      <UpgradeModal
        kind="gem"
        isOpen={showUpgradeGemModal}
        onClose={() => setShowUpgradeGemModal(false)}
        onConfirm={handleUpgradeGem}
        selectedItem={selectedItem}
        equipmentMode={equipmentMode}
        selectedIndex={selectedIndex}
        coinsUnderThreshold={coinsUnderThreshold}
      />

      <div className="mb-4">
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-lg font-semibold text-text-main">
            {name ? name : `Select a ${equipmentMode}`}
          </h3>
        </div>

        {name && (
          <div className="flex items-center gap-2 flex-wrap">
            <Badge variant={combatMode === 'PvP' ? 'danger' : 'info'}>
              {combatMode} Stats
            </Badge>

            <div className="flex gap-2 ml-auto flex-wrap justify-end">
              <Button
                variant="outline"
                size="sm"
                disabled={coinsUnderThreshold}
                onClick={() => setShowUpgradeEquipmentModal(true)}
              >
                {coinsUnderThreshold ? UPGRADE_COINS_LOCKED_LABEL : 'Upgrade Equipment'}
              </Button>
              <Button
                size="sm"
                className="bg-emerald-500/10 text-emerald-400 border-emerald-500/30 border hover:bg-emerald-500/20 hover:border-emerald-500/50 disabled:opacity-50 disabled:cursor-not-allowed"
                disabled={coinsUnderThreshold}
                onClick={() => setShowUpgradeGemModal(true)}
              >
                {coinsUnderThreshold ? UPGRADE_COINS_LOCKED_LABEL : 'Upgrade Gem'}
              </Button>
              <Button
                variant="outline"
                size="sm"
                onClick={() => setShowEquipmentModal(true)}
              >
                Unequip Equipment
              </Button>
              <Button
                size="sm"
                className="bg-purple-500/10 text-purple-400 border-purple-500/30 border hover:bg-purple-500/20 hover:border-purple-500/50"
                onClick={() => setShowGemModal(true)}
              >
                Unequip Gem
              </Button>
            </div>
          </div>
        )}
      </div>

      <div className="space-y-4">
        {!stats && (
          <div className="text-center py-8 text-text-muted">
            <p>No stats available for this selection.</p>
          </div>
        )}

        {stats && Object.entries(statGroups).map(([groupName, statKeys]) => {
          const visibleStats = statKeys.filter(key => {
            const value = processedStats[key];
            return !(value === undefined || value === 0);
          });

          if (visibleStats.length === 0) return null;

          return (
            <div key={groupName} className="rounded-global bg-bg-app border border-border-base p-3">
              <h4 className="text-xs font-bold text-text-muted uppercase tracking-wider mb-2">{statGroupDisplayName[groupName] || groupName}</h4>
              <div className="space-y-0.5">
                {visibleStats.map(key => renderStat(key, processedStats[key]))}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
};

export default EquipmentStats;
