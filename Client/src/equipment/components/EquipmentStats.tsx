import React, { useMemo, useState } from 'react';

import { statDisplayName, commanderStatGroups, castellanStatGroups, statGroupDisplayName, type CommStat, type CastStat } from '../models/equipment';
import { FrontendWebsocket } from '../../websocket';
import GameButton from '../../components/GameButton';

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
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={handleClose}
      />

      {/* Modal Content */}
      <div className="relative glass-panel p-6 max-w-md mx-4 animate-fade-in bg-bg-card">
        {/* Icon */}
        <div className="flex justify-center mb-4">
          <div className="w-16 h-16 rounded-full bg-primary/20 flex items-center justify-center">
            <svg className="w-8 h-8 text-primary" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 7l-.867 12.142A2 2 0 0116.138 21H7.862a2 2 0 01-1.995-1.858L5 7m5 4v6m4-6v6m1-10V4a1 1 0 00-1-1h-4a1 1 0 00-1 1v3M4 7h16" />
            </svg>
          </div>
        </div>

        {/* Title */}
        <h3 className="text-xl font-bold text-text-main text-center mb-3">
          Unequip Equipment
        </h3>

        {/* Description */}
        <p className="text-text-muted text-center mb-4">
          Select equipment to unequip from <span className="text-primary font-semibold">{selectedItem?.name}</span>
        </p>

        {/* Slot Selection */}
        <div className="space-y-2 mb-6">
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
                  {/* Custom Checkbox */}
                  <div className={`
                    w-5 h-5 rounded border-2 flex items-center justify-center transition-all duration-200
                    ${isSelected
                      ? 'bg-primary border-primary'
                      : 'bg-bg-app/50 border-gray-500 hover:border-text-muted'
                    }
                  `}>
                    {isSelected && (
                      <svg className="w-3 h-3 text-bg-app" fill="none" stroke="currentColor" viewBox="0 0 24 24">
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

        {/* Buttons */}
        <div className="flex gap-3">
          <button
            onClick={handleClose}
            className="flex-1 btn-ghost border border-border-base"
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            disabled={selectedSlots.size === 0}
            className={`
              flex-1 px-4 py-2 font-semibold rounded-global transition-all duration-200
              ${selectedSlots.size > 0
                ? 'bg-primary text-bg-app hover:bg-primary/80 active:scale-95'
                : 'bg-bg-card-hover text-text-muted cursor-not-allowed'
              }
            `}
          >
            Unequip {selectedSlots.size > 0 ? `(${selectedSlots.size})` : ''}
          </button>
        </div>
      </div>
    </div>
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

  // Filter to only show slots that have gems (gem ID != 0)
  const availableSlots = GEM_SLOTS.filter(({ slot }) => getGemId(slot) !== 0);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/60 backdrop-blur-sm"
        onClick={handleClose}
      />

      {/* Modal Content */}
      <div className="relative glass-panel p-6 max-w-md mx-4 animate-fade-in bg-bg-card">
        {/* Icon */}
        <div className="flex justify-center mb-4">
          <div className="w-16 h-16 rounded-full bg-purple-500/20 flex items-center justify-center">
            <svg className="w-8 h-8 text-purple-400" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M20.354 15.354A9 9 0 018.646 3.646 9.003 9.003 0 0012 21a9.003 9.003 0 008.354-5.646z" />
            </svg>
          </div>
        </div>

        {/* Title */}
        <h3 className="text-xl font-bold text-text-main text-center mb-3">
          Unequip Gem
        </h3>

        {/* Description */}
        <p className="text-text-muted text-center mb-4">
          Select gems to unequip from <span className="text-purple-400 font-semibold">{selectedItem?.name}</span>
        </p>

        {/* Slot Selection */}
        <div className="space-y-2 mb-6">
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
                  {/* Custom Checkbox */}
                  <div className={`
                    w-5 h-5 rounded border-2 flex items-center justify-center transition-all duration-200
                    ${isSelected
                      ? 'bg-purple-500 border-purple-500'
                      : 'bg-bg-app/50 border-gray-500 hover:border-gray-400'
                    }
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

        {/* Buttons */}
        <div className="flex gap-3">
          <button
            onClick={handleClose}
            className="flex-1 btn-ghost border border-border-base"
          >
            Cancel
          </button>
          <button
            onClick={handleConfirm}
            disabled={selectedSlots.size === 0}
            className={`
              flex-1 px-4 py-2 font-semibold rounded-global transition-all duration-200
              ${selectedSlots.size > 0
                ? 'bg-purple-500 text-white hover:bg-purple-600 active:scale-95'
                : 'bg-bg-card-hover text-text-muted cursor-not-allowed'
              }
            `}
          >
            Unequip {selectedSlots.size > 0 ? `(${selectedSlots.size})` : ''}
          </button>
        </div>
      </div>
    </div>
  );
};

interface EquipmentStatsProps {
  equipmentMode: 'Commander' | 'Castellan';
  combatMode: 'PvP' | 'PvE';
  selectedItem: CommStat | CastStat | null;
  selectedIndex: number | null;
}

const EquipmentStats: React.FC<EquipmentStatsProps> = ({ equipmentMode, combatMode, selectedItem, selectedIndex }) => {
  const [showEquipmentModal, setShowEquipmentModal] = useState(false);
  const [showGemModal, setShowGemModal] = useState(false);
  const stats = selectedItem;
  const name = selectedItem?.name;

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

        // Special handling for Front/Flank limits
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

        // Skip adding CL/NPC stats to frontCbtStr/flankCbtStr since they go to limits now
        if (key === 'frontCbtStr' || key === 'flankCbtStr') {
          // Do nothing, these don't get CL/NPC additions anymore
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
      {/* Modals */}
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

      {/* Header */}
      <div className="mb-4">
        <div className="flex items-center justify-between mb-2">
          <h3 className="text-lg font-semibold text-white">
            {name ? name : `Select a ${equipmentMode}`}
          </h3>
        </div>

        {name && (
          <div className="flex items-center gap-2 flex-wrap">
            <span className={`
              inline-flex items-center px-2 py-0.5 rounded-global text-xs font-medium
              ${combatMode === 'PvP'
                ? 'bg-red-500/10 text-red-400 border border-red-500/20'
                : 'bg-blue-500/10 text-blue-400 border border-blue-500/20'
              }
            `}>
              {combatMode} Stats
            </span>

            {/* Unequip Buttons */}
            <div className="flex gap-2 ml-auto">
              <GameButton
                onClick={() => setShowEquipmentModal(true)}
                className="px-3 py-1 bg-primary/20 text-primary border border-primary/30 font-medium text-xs rounded-global hover:bg-primary/30 hover:border-primary/50 active:scale-95 transition-all duration-200"
              >
                Unequip Equipment
              </GameButton>
              <GameButton
                onClick={() => setShowGemModal(true)}
                className="px-3 py-1 bg-purple-500/20 text-purple-400 border border-purple-500/30 font-medium text-xs rounded-global hover:bg-purple-500/30 hover:border-purple-500/50 active:scale-95 transition-all duration-200"
              >
                Unequip Gem
              </GameButton>
            </div>
          </div>
        )}
      </div>

      {/* Stats List */}
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
            <div key={groupName} className="rounded-global bg-bg-app/50 p-3 border border-border-base/50">
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
