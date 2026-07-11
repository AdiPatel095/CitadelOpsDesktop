import React, { useState } from 'react';
import EquipmentStats from './EquipmentStats';
import { FrontendWebsocket } from '../../Websocket';
import { Icons } from '../../components/Icons';
import { Button, Modal, Card, CardHeader, CardContent, PillSelector, Switch, Input } from '../../components/ui';

const img1Star = '/game-data/stars/images/1Star.webp';
const img4Star = '/game-data/stars/images/4Star.webp';
const img7Star = '/game-data/stars/images/7Star.webp';
const img12Star = '/game-data/stars/images/12Star.webp';

import { type CommStat, type CastStat } from '../models/Equipment';

export type EquipmentMode = 'Commander' | 'Castellan';
export type CombatMode = 'PvP' | 'PvE';

interface EquipmentSelectionProps {
  equipmentMode: EquipmentMode;
  setEquipmentMode: (mode: EquipmentMode) => void;
  combatMode: CombatMode;
  setCombatMode: (mode: CombatMode) => void;
  selectedIndex: number | null;
  setSelectedIndex: (index: number) => void;
  fullArray: (CommStat | CastStat | null)[];  // Full array with nulls
  selectedItem: CommStat | CastStat | null;
}

type SellItemType = 'Equipment' | 'Gems';
type RelicTab = 'Non Relic' | 'Relic 1.0' | 'Relic 2.0';

const BASE_EQUIPMENT_SLOTS = [
  { key: 'equip1', label: 'Armor' },
  { key: 'equip2', label: 'Weapon' },
  { key: 'equip3', label: 'Helmet' },
  { key: 'equip4', label: 'Artifact' },
  { key: 'hero', label: 'Hero' },
] as const;

const AUTO_SELL_EQUIPMENT_STORAGE_KEY = 'equipmentAutoSellNonRelicEquipment';
const AUTO_SELL_EQUIPMENT_INTERVAL_STORAGE_KEY = 'equipmentAutoSellNonRelicEquipmentIntervalMinutes';
const DEFAULT_AUTO_SELL_EQUIPMENT_INTERVAL_MIN = 1;
const MIN_AUTO_SELL_EQUIPMENT_INTERVAL_MIN = 1;
const MAX_AUTO_SELL_EQUIPMENT_INTERVAL_MIN = 1440;

interface SellConfig {
  relicType: RelicTab;
  sellLookItems?: boolean;
  sellSpecialPost2026?: boolean;
  keepStars?: number;
}

interface SwapEquipmentModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: (firstIndex: number, secondIndex: number) => void;
  equipmentMode: EquipmentMode;
  fullArray: (CommStat | CastStat | null)[];
  selectedIndex: number | null;
}

const baseEquipmentCount = (item: CommStat | CastStat): number => {
  return BASE_EQUIPMENT_SLOTS.reduce((count, slot) => count + (item[slot.key] !== 0 ? 1 : 0), 0);
};

const baseEquipmentSummary = (item: CommStat | CastStat): string => {
  const labels = BASE_EQUIPMENT_SLOTS
    .filter(slot => item[slot.key] !== 0)
    .map(slot => slot.label);
  return labels.length > 0 ? labels.join(', ') : 'No base pieces';
};

const clampAutoSellEquipmentIntervalMinutes = (value: number): number => {
  if (!Number.isFinite(value)) return DEFAULT_AUTO_SELL_EQUIPMENT_INTERVAL_MIN;
  return Math.min(
    MAX_AUTO_SELL_EQUIPMENT_INTERVAL_MIN,
    Math.max(MIN_AUTO_SELL_EQUIPMENT_INTERVAL_MIN, Math.round(value)),
  );
};

const loadAutoSellEquipmentIntervalMinutes = (): number => {
  try {
    const raw = localStorage.getItem(AUTO_SELL_EQUIPMENT_INTERVAL_STORAGE_KEY);
    return clampAutoSellEquipmentIntervalMinutes(raw ? Number(raw) : DEFAULT_AUTO_SELL_EQUIPMENT_INTERVAL_MIN);
  } catch {
    return DEFAULT_AUTO_SELL_EQUIPMENT_INTERVAL_MIN;
  }
};

const SwapEquipmentModal: React.FC<SwapEquipmentModalProps> = ({
  isOpen,
  onClose,
  onConfirm,
  equipmentMode,
  fullArray,
  selectedIndex,
}) => {
  const [selectedIndexes, setSelectedIndexes] = useState<number[]>([]);
  const availableItems = fullArray
    .map((item, index) => ({ item, index }))
    .filter((entry): entry is { item: CommStat | CastStat; index: number } => entry.item !== null);
  const selectedFirst = selectedIndexes[0] ?? null;
  const selectedSecond = selectedIndexes[1] ?? null;

  React.useEffect(() => {
    if (!isOpen) return;
    if (selectedIndex !== null && fullArray[selectedIndex] !== null) {
      setSelectedIndexes([selectedIndex]);
    } else {
      setSelectedIndexes([]);
    }
  }, [isOpen, selectedIndex]);

  if (!isOpen) return null;

  const toggleIndex = (index: number) => {
    setSelectedIndexes(prev => {
      if (prev.includes(index)) {
        return prev.filter(value => value !== index);
      }
      if (prev.length === 0) {
        return [index];
      }
      if (prev.length === 1) {
        return [prev[0], index];
      }
      return [prev[0], index];
    });
  };

  const handleClose = () => {
    setSelectedIndexes([]);
    onClose();
  };

  const handleConfirm = () => {
    if (selectedFirst === null || selectedSecond === null) return;
    onConfirm(selectedFirst, selectedSecond);
    setSelectedIndexes([]);
  };

  const leaderLabel = equipmentMode.toLowerCase();

  return (
    <Modal
      isOpen={isOpen}
      onClose={handleClose}
      maxWidth="2xl"
      title={
        <div className="flex flex-col items-center pt-2">
          <div className="w-16 h-16 rounded-full bg-primary/20 flex items-center justify-center mb-4">
            <Icons.RefreshCw className="w-8 h-8 text-primary" />
          </div>
          Swap Base Equipment
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
            disabled={selectedFirst === null || selectedSecond === null}
            className="flex-1"
          >
            Swap Pieces
          </Button>
        </>
      }
    >
      <div className="flex flex-col items-center">
        <p className="text-text-muted text-center mb-4">
          Select two {leaderLabel}s. Base equipment and heroes move across; socketed gems stay on those pieces.
        </p>

        <div className="w-full rounded-global border border-border-base bg-bg-app p-3 mb-4">
          <div className="grid grid-cols-2 gap-3">
            <div className="min-w-0">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-1">First</div>
              <div className="text-sm font-semibold text-text-main truncate">
                {selectedFirst !== null ? fullArray[selectedFirst]?.name : 'Select first'}
              </div>
            </div>
            <div className="min-w-0">
              <div className="text-[10px] uppercase tracking-wider text-text-muted mb-1">Second</div>
              <div className="text-sm font-semibold text-text-main truncate">
                {selectedSecond !== null ? fullArray[selectedSecond]?.name : 'Select second'}
              </div>
            </div>
          </div>
        </div>

        <div className="w-full max-h-[55dvh] overflow-y-auto custom-scrollbar space-y-2 pr-1">
          {availableItems.length < 2 ? (
            <p className="text-text-muted text-center py-4">At least two {leaderLabel}s are required</p>
          ) : (
            availableItems.map(({ item, index }) => {
              const selectionNumber = selectedIndexes.indexOf(index) + 1;
              const isSelected = selectionNumber > 0;
              const equippedCount = baseEquipmentCount(item);

              return (
                <button
                  key={index}
                  type="button"
                  className={`
                    w-full text-left flex items-center gap-3 p-3 rounded-global border transition-all duration-200
                    ${isSelected
                      ? 'bg-primary/10 border-primary/50 text-text-main'
                      : 'bg-bg-app/50 border-border-base hover:bg-bg-card-hover text-text-muted'
                    }
                  `}
                  onClick={() => toggleIndex(index)}
                >
                  <span className={`
                    rounded-full w-7 h-7 flex items-center justify-center text-xs font-bold shrink-0
                    ${isSelected ? 'bg-primary text-bg-app' : 'bg-bg-app border border-border-base text-text-muted'}
                  `}>
                    {isSelected ? selectionNumber : index + 1}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block text-sm font-medium truncate">{item.name}</span>
                    <span className="block text-xs text-text-muted truncate">{baseEquipmentSummary(item)}</span>
                  </span>
                  <span className="text-xs font-mono text-text-muted shrink-0">
                    {equippedCount}/5
                  </span>
                </button>
              );
            })
          )}
        </div>
      </div>
    </Modal>
  );
};

interface CautionModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: (config: SellConfig) => void;
  itemType: SellItemType;
}

const CautionModal: React.FC<CautionModalProps> = ({ isOpen, onClose, onConfirm, itemType }) => {
  const [relicTab, setRelicTab] = useState<RelicTab>('Non Relic');
  const [sellLookItems, setSellLookItems] = useState(false);
  const [sellSpecialPost2026, setSellSpecialPost2026] = useState(false);
  const [keepStars, setKeepStars] = useState(12);
  const [showInfoModal, setShowInfoModal] = useState(false);

  React.useEffect(() => {
    if (isOpen) {
      setRelicTab('Non Relic');
      setSellLookItems(false);
      setSellSpecialPost2026(false);
      setKeepStars(12);
      setShowInfoModal(false);
    }
  }, [isOpen]);

  if (!isOpen) return null;

  const isGems = itemType === 'Gems';
  const singularItem = isGems ? 'gem' : 'item';
  const action = isGems ? 'socket it to an equipment piece' : 'equip it to a commander or castellan';

  const handleConfirm = () => {
    onConfirm({
      relicType: relicTab,
      sellLookItems,
      sellSpecialPost2026,
      keepStars
    });
  };

  const renderCautionMessage = (message: React.ReactNode, subMessage?: React.ReactNode) => (
    <div className="p-4 bg-error/10 border border-error/30 rounded-global text-center mb-6">
      <p className="text-text-main text-sm font-medium">
        {message}
      </p>
      {subMessage && (
        <p className="text-xs text-text-muted mt-2">
          {subMessage}
        </p>
      )}
    </div>
  );

  const renderContent = () => {
    switch (relicTab) {
      case 'Non Relic':
        return (
          <>
            {renderCautionMessage(
              <>This will sell all <span className="font-bold text-warning">Old Red (Pre-2026)</span> {isGems ? 'Gems' : 'Equipment'}.</>,
              <>This action <span className="font-bold text-error">CANNOT</span> be reversed.</>
            )}
            <div className="space-y-3 mb-6">
              <div
                className="flex items-center justify-between p-3 rounded-global bg-bg-app border border-border-base cursor-pointer hover:bg-bg-card-hover transition-colors group"
                onClick={() => setSellSpecialPost2026(!sellSpecialPost2026)}
              >
                <div className="flex flex-col">
                  <span className="text-sm font-medium text-text-main">Sell Special Post-2026 Sets</span>
                  <span className="text-[10px] text-text-muted">Includes Rift, Spore, and Victorious sets</span>
                </div>
                <Switch checked={sellSpecialPost2026} onChange={setSellSpecialPost2026} />
              </div>

              {!isGems && (
                <div
                  className="flex items-center justify-between p-3 rounded-global bg-bg-app border border-border-base cursor-pointer hover:bg-bg-card-hover transition-colors group"
                  onClick={() => setSellLookItems(!sellLookItems)}
                >
                  <span className="text-sm font-medium text-text-main">Sell Look Items</span>
                  <Switch checked={sellLookItems} onChange={setSellLookItems} />
                </div>
              )}
            </div>
            <p className="text-xs text-text-muted text-center mb-4 flex items-center justify-center gap-1.5">
              <Icons.Shield className="w-4 h-4 text-success" />
              Future items not yet in our database are <span className="text-success font-bold tracking-tight">SAFE</span> by default.
            </p>
          </>
        );
      case 'Relic 1.0':
        return renderCautionMessage(
          <>This will sell <span className="font-bold text-error">ALL Relic 1.0</span> {isGems ? 'gems' : 'equipment'}.</>,
          <>This action <span className="font-bold text-error">CANNOT</span> be reversed.</>
        );
      case 'Relic 2.0':
        return (
          <div className="space-y-4 mb-6">
            {renderCautionMessage(
              <>This will sell {isGems ? 'gems' : 'items'} below <span className="font-bold text-text-inverted">{keepStars}</span> stars.</>,
              <>This action <span className="font-bold text-error">CANNOT</span> be reversed.</>
            )}

            <div className="bg-bg-app border border-border-base rounded-global p-4 relative">
              <div className="flex items-center justify-between mb-2">
                <label className="text-sm font-medium text-text-main">Keep Total Stars & Above</label>
                <button
                  onClick={() => setShowInfoModal(true)}
                  className="p-1.5 rounded-full hover:bg-primary/20 text-text-muted hover:text-primary transition-colors"
                  title="View Star Rating Guide"
                >
                  <Icons.Info className="w-4 h-4" />
                </button>
              </div>

              <div className="flex items-center gap-4">
                <input
                  type="range"
                  min="4"
                  max="42"
                  step="1"
                  value={keepStars}
                  onChange={(e) => setKeepStars(parseInt(e.target.value))}
                  className="flex-1 accent-primary h-2 bg-bg-card border border-border-base rounded-lg appearance-none cursor-pointer"
                />
                <span className="w-8 text-center font-bold text-primary text-lg">{keepStars}</span>
              </div>
              <div className="flex justify-between text-xs text-text-muted mt-1 px-1 font-mono">
                <span>4</span>
                <span>42</span>
              </div>
            </div>

            <p className="text-xs text-text-muted text-center pt-2">
              Selling {isGems ? 'gems' : 'items'} with less than <span className="text-text-inverted font-mono font-medium">{keepStars}</span> total stars.
            </p>
          </div>
        );
    }
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={
        <div className="flex w-full mt-2 shrink-0 mb-4 px-1">
          <PillSelector
            value={relicTab}
            onChange={(v) => setRelicTab(v as RelicTab)}
            options={['Non Relic', 'Relic 1.0', 'Relic 2.0']}
            size="sm"
            fullWidth={true}
            variant="primary"
          />
        </div>
      }
      footer={
        <>
          <div className="mr-auto text-xs text-text-muted">
            <span className="text-primary font-bold">1</span> ops-coin per {singularItem}
          </div>
          <Button variant="ghost" onClick={onClose}>Cancel</Button>
          <Button variant="danger" onClick={handleConfirm}>Confirm Sell</Button>
        </>
      }
    >
      <div className="flex flex-col relative">
        {showInfoModal && (
          <div className="absolute z-50 left-1/2 -translate-x-1/2 top-4 w-full bg-bg-card border border-border-base rounded-global shadow-2xl animate-fade-in p-5">
            <div className="flex items-center justify-between border-b border-border-base pb-3 mb-4">
              <h4 className="text-base font-semibold text-text-main flex items-center gap-2">
                <Icons.Info className="w-5 h-5 text-primary" />
                Star Rating Guide
              </h4>
              <button
                onClick={() => setShowInfoModal(false)}
                className="p-1 rounded hover:bg-error/20 text-text-muted hover:text-error transition-colors"
              >
                <Icons.X className="w-5 h-5" />
              </button>
            </div>
            <div className="space-y-4">
              <div className="rounded-global border border-primary/30 bg-primary/5 p-4">
                <div className="flex items-center gap-2 mb-3">
                  <span className="w-6 h-6 rounded flex items-center justify-center text-sm font-bold bg-primary/20 text-primary">1</span>
                  <span className="text-sm font-semibold text-primary">Individual Stats (1-7 Stars)</span>
                </div>
                <div className="bg-bg-app p-3 rounded-xl border border-border-base flex justify-between items-end gap-2 mb-3">
                  <div className="text-center">
                    <img src={img1Star} alt="1 Star" className="h-6 w-auto mx-auto mb-1.5 drop-shadow" />
                    <span className="text-[10px] font-medium text-text-muted uppercase tracking-wider">1 Star</span>
                  </div>
                  <div className="text-center">
                    <img src={img4Star} alt="4 Stars" className="h-6 w-auto mx-auto mb-1.5 drop-shadow" />
                    <span className="text-[10px] font-medium text-text-muted uppercase tracking-wider">4 Stars</span>
                  </div>
                  <div className="text-center">
                    <img src={img7Star} alt="7 Stars" className="h-6 w-auto mx-auto mb-1.5 drop-shadow" />
                    <span className="text-[10px] font-medium text-text-muted uppercase tracking-wider">7 Stars (Max)</span>
                  </div>
                </div>
                <p className="text-xs text-text-muted leading-relaxed">
                  Each stat on a Relic 2.0 item can have between <span className="text-text-main font-medium">1 and 7 stars</span>.
                </p>
              </div>
              <div className="rounded-global border border-warning/30 bg-warning/5 p-4">
                <div className="flex items-center gap-2 mb-3">
                  <span className="w-6 h-6 rounded flex items-center justify-center text-sm font-bold bg-warning/20 text-warning">2</span>
                  <span className="text-sm font-semibold text-warning">Safety Threshold</span>
                </div>
                <div className="bg-bg-app p-2 rounded-xl border border-border-base flex justify-center mb-3">
                  <img src={img12Star} alt="12 Star Example" className="max-w-full h-auto rounded drop-shadow" />
                </div>
                <p className="text-xs text-text-muted leading-relaxed">
                  The <span className="text-text-main font-medium">Default Setting (12)</span> is designed to keep decent items like this one or better (12 from top 3+4+2+3).
                </p>
              </div>
            </div>
          </div>
        )}

        {renderContent()}

        <div className="flex items-start gap-2 text-xs text-text-muted mt-2 border-t border-border-base pt-4">
          <span className="text-primary mt-0.5">💡</span>
          <span>
            <span className="font-medium text-text-main">Tip:</span> If you want to save a {singularItem}, manually {action} before cleaning the storage.
          </span>
        </div>
      </div>
    </Modal>
  );
};

const EquipmentSelection: React.FC<EquipmentSelectionProps> = ({
  equipmentMode,
  setEquipmentMode,
  combatMode,
  setCombatMode,
  selectedIndex,
  setSelectedIndex,
  fullArray,
  selectedItem,
}) => {
  const [showSellModal, setShowSellModal] = useState(false);
  const [showSwapModal, setShowSwapModal] = useState(false);
  const [sellType, setSellType] = useState<SellItemType>('Equipment');
  const [autoSellEquipment, setAutoSellEquipment] = useState(() => {
    try {
      return localStorage.getItem(AUTO_SELL_EQUIPMENT_STORAGE_KEY) === 'true';
    } catch {
      return false;
    }
  });
  const [autoSellIntervalMinutes, setAutoSellIntervalMinutes] = useState(loadAutoSellEquipmentIntervalMinutes);
  const autoSellLastRunRef = React.useRef(0);

  const openSellModal = (type: SellItemType) => {
    setSellType(type);
    setShowSellModal(true);
  };

  const sendAutoSellEquipment = React.useCallback(() => {
    const now = Date.now();
    const intervalMs = autoSellIntervalMinutes * 60_000;
    const cooldownMs = Math.max(0, intervalMs - 5_000);
    if (now - autoSellLastRunRef.current < cooldownMs) return;
    if (FrontendWebsocket.getStatus() !== 'Connected') return;
    const sent = FrontendWebsocket.sendMessage({
      type: 'sellNonRelicEquipment',
      payload: {
        sellLookItems: false,
        sellSpecialPost2026: false,
        silentZero: true,
      }
    });
    if (sent) {
      autoSellLastRunRef.current = now;
    }
  }, [autoSellIntervalMinutes]);

  React.useEffect(() => {
    try {
      localStorage.setItem(AUTO_SELL_EQUIPMENT_INTERVAL_STORAGE_KEY, String(autoSellIntervalMinutes));
    } catch {
      // Ignore storage failures; the live interval still works for this session.
    }
  }, [autoSellIntervalMinutes]);

  React.useEffect(() => {
    try {
      localStorage.setItem(AUTO_SELL_EQUIPMENT_STORAGE_KEY, autoSellEquipment ? 'true' : 'false');
    } catch {
      // Ignore storage failures; the live toggle still works for this session.
    }

    if (!autoSellEquipment) return;
    sendAutoSellEquipment();
    const interval = window.setInterval(sendAutoSellEquipment, autoSellIntervalMinutes * 60_000);
    return () => window.clearInterval(interval);
  }, [autoSellEquipment, autoSellIntervalMinutes, sendAutoSellEquipment]);

  const handleSwapConfirm = (firstIndex: number, secondIndex: number) => {
    FrontendWebsocket.sendMessage({
      type: 'swapEquipmentLoadouts',
      payload: {
        equipmentMode,
        firstIndex,
        secondIndex,
      }
    });
    setShowSwapModal(false);
  };

  const handleSellConfirm = (config: SellConfig) => {
    if (sellType === 'Equipment') {
      if (config.relicType === 'Non Relic') {
        FrontendWebsocket.sendMessage({
          type: 'sellNonRelicEquipment',
          payload: {
            sellLookItems: !!config.sellLookItems,
            sellSpecialPost2026: !!config.sellSpecialPost2026
          }
        });
      } else if (config.relicType === 'Relic 1.0') {
        FrontendWebsocket.sendMessage({
          type: 'sellRelic1Equipment',
          payload: {}
        });
      } else if (config.relicType === 'Relic 2.0') {
        FrontendWebsocket.sendMessage({
          type: 'sellRelic2Equipment',
          payload: {
            keepStars: config.keepStars || 0
          }
        });
      }
    } else {
      if (config.relicType === 'Non Relic') {
        FrontendWebsocket.sendMessage({
          type: 'sellNonRelicGems',
          payload: {
            sellSpecialPost2026: !!config.sellSpecialPost2026
          }
        });
      } else if (config.relicType === 'Relic 1.0') {
        FrontendWebsocket.sendMessage({
          type: 'sellRelic1Gems',
          payload: {}
        });
      } else if (config.relicType === 'Relic 2.0') {
        FrontendWebsocket.sendMessage({
          type: 'sellRelic2Gems',
          payload: {
            keepStars: config.keepStars || 0
          }
        });
      }
    }
    setShowSellModal(false);
  };

  return (
    <Card className="liquid-prominent-header-card h-full flex flex-col min-h-0">
      <CautionModal
        isOpen={showSellModal}
        onClose={() => setShowSellModal(false)}
        onConfirm={handleSellConfirm}
        itemType={sellType}
      />
      <SwapEquipmentModal
        isOpen={showSwapModal}
        onClose={() => setShowSwapModal(false)}
        onConfirm={handleSwapConfirm}
        equipmentMode={equipmentMode}
        fullArray={fullArray}
        selectedIndex={selectedIndex}
      />

      <CardHeader className="liquid-card-header-prominent flex flex-wrap items-center gap-4">
        <PillSelector
          value={equipmentMode}
          onChange={(v) => setEquipmentMode(v as EquipmentMode)}
          options={['Commander', 'Castellan']}
        />
        <PillSelector
          value={combatMode}
          onChange={(v) => setCombatMode(v as CombatMode)}
          options={['PvP', 'PvE']}
        />

        <div className="equipment-actions ml-auto">
          <div
            className="flex items-center gap-2 rounded-global border border-primary/25 bg-primary/5 px-3 py-1.5 text-sm text-text-main"
            title="Automatically sells old non-relic equipment with look items and special post-2026 gear excluded."
          >
            <span className="font-medium">AutoSell</span>
            <Switch
              checked={autoSellEquipment}
              onChange={setAutoSellEquipment}
              size="sm"
            />
            <span className="text-xs text-text-muted">Every</span>
            <div className="w-16">
              <Input
                type="number"
                min={MIN_AUTO_SELL_EQUIPMENT_INTERVAL_MIN}
                max={MAX_AUTO_SELL_EQUIPMENT_INTERVAL_MIN}
                value={autoSellIntervalMinutes}
                onChange={(e) => {
                  setAutoSellIntervalMinutes(clampAutoSellEquipmentIntervalMinutes(Number(e.target.value)));
                }}
                className="h-8 px-2 py-1 text-center"
              />
            </div>
            <span className="text-xs text-text-muted">min</span>
          </div>
          <Button
            size="sm"
            variant="outline"
            onClick={() => setShowSwapModal(true)}
          >
            <Icons.RefreshCw className="w-4 h-4 mr-1.5" />
            Swap Gear
          </Button>
          <Button
            size="sm"
            onClick={() => openSellModal('Gems')}
            className="bg-warning/10 text-warning border border-warning/30 hover:bg-warning/20 hover:border-warning/50"
          >
            Sell Gems
          </Button>
          <Button
            size="sm"
            onClick={() => openSellModal('Equipment')}
            className="bg-warning/10 text-warning border border-warning/30 hover:bg-warning/20 hover:border-warning/50"
          >
            Sell Equipment
          </Button>
        </div>
      </CardHeader>

      <CardContent className="liquid-prominent-header-content liquid-prominent-header-content-flush equipment-selection-body p-0">
        <div className="equipment-loadout-list custom-scrollbar">
          <div className="equipment-loadout-list-items">
            {fullArray.every(item => item === null) ? (
              <div className="px-3 py-4 text-center text-text-muted text-sm font-medium">
                No {equipmentMode.toLowerCase()}s available
              </div>
            ) : (
              fullArray.map((item, index) => {
                if (item === null) return null;
                return (
                  <div
                    key={index}
                    className={`
                      equipment-loadout-item
                      rounded-global px-3 py-2.5 cursor-pointer transition-all duration-200
                      ${selectedIndex === index
                        ? 'bg-primary/10 text-primary border border-primary/30 shadow-[0_0_10px_rgba(52,211,153,0.1)]'
                        : 'text-text-muted hover:bg-bg-card-hover hover:text-text-main border border-transparent'
                      }
                    `}
                    onClick={() => setSelectedIndex(index)}
                  >
                    <div className="flex items-center gap-2">
                      <span className={`
                        rounded-full w-6 h-6 flex items-center justify-center text-xs font-bold
                        ${selectedIndex === index
                          ? 'bg-primary text-bg-app'
                          : 'bg-bg-app border border-border-base text-text-muted'
                        }
                      `}>
                        {index + 1}
                      </span>
                      <span className="text-sm font-medium truncate">
                        {item.name}
                      </span>
                    </div>
                  </div>
                );
              })
            )}
          </div>
        </div>

        <div className="equipment-stats-pane custom-scrollbar">
          <EquipmentStats
            equipmentMode={equipmentMode}
            combatMode={combatMode}
            selectedItem={selectedItem}
            selectedIndex={selectedIndex}
          />
        </div>
      </CardContent>
    </Card>
  );
};

export default EquipmentSelection;
