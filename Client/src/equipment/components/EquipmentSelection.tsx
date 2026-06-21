import React, { useState } from 'react';
import EquipmentStats from './EquipmentStats';
import { FrontendWebsocket } from '../../Websocket';
import { Icons } from '../../components/Icons';
import { Button, Modal, Card, CardHeader, CardContent, ToggleGroup, Switch } from '../../components/ui';

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

interface SellConfig {
  relicType: RelicTab;
  sellLookItems?: boolean;
  sellSpecialPost2026?: boolean;
  keepStars?: number;
}

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
          <ToggleGroup
            value={relicTab}
            onChange={(v) => setRelicTab(v as RelicTab)}
            options={[
              { value: 'Non Relic', label: 'Non Relic' },
              { value: 'Relic 1.0', label: 'Relic 1.0' },
              { value: 'Relic 2.0', label: 'Relic 2.0' },
            ]}
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
  const [sellType, setSellType] = useState<SellItemType>('Equipment');

  const openSellModal = (type: SellItemType) => {
    setSellType(type);
    setShowSellModal(true);
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
    <Card className="h-full flex flex-col min-h-0">
      <CautionModal
        isOpen={showSellModal}
        onClose={() => setShowSellModal(false)}
        onConfirm={handleSellConfirm}
        itemType={sellType}
      />

      <CardHeader className="flex flex-wrap items-center gap-4 border-b border-border-base pb-4">
        <ToggleGroup
          value={equipmentMode}
          onChange={(v) => setEquipmentMode(v as EquipmentMode)}
          options={[
            { value: 'Commander', label: 'Commander' },
            { value: 'Castellan', label: 'Castellan' }
          ]}
        />
        <ToggleGroup
          value={combatMode}
          onChange={(v) => setCombatMode(v as CombatMode)}
          options={[
            { value: 'PvP', label: 'PvP' },
            { value: 'PvE', label: 'PvE' }
          ]}
        />

        <div className="ml-auto flex gap-2">
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

      <CardContent className="flex flex-1 min-h-0 p-0">
        <div className="w-56 border-r border-border-base overflow-y-auto custom-scrollbar p-2">
          <div className="space-y-1">
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

        <div className="flex-1 overflow-y-auto custom-scrollbar">
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
