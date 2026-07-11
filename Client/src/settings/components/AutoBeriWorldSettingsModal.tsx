import React, { useEffect, useState } from 'react';
import { CalendarDays, Save, Users } from 'lucide-react';
import { showTroopPicker } from '../../components/TroopPickerModal';
import { Modal, Button, Input } from '../../components/ui';
import {
  DEFAULT_AUTO_BERI_WORLD_SETTINGS,
  parseAutoBeriWorldSettings,
  type AutoBeriWorldSettings,
} from '../AutoBeriWorldClientState';
import { useCitadelAPI } from '../../api/ApiContext';
import { queueConfigurationUpdate } from '../Configuration';

interface AutoBeriWorldSettingsModalProps {
  isOpen: boolean;
  onClose: () => void;
  onOpenFeatureSchedule: (featureID: string, featureLabel: string) => void;
}

export const AutoBeriWorldSettingsModal: React.FC<AutoBeriWorldSettingsModalProps> = ({ isOpen, onClose, onOpenFeatureSchedule }) => {
  const { configuration } = useCitadelAPI();
  const [minTroopsToTransfer, setMinTroopsToTransfer] = useState(
    DEFAULT_AUTO_BERI_WORLD_SETTINGS.minTroopsToTransfer,
  );
  const [beriCastleCID, setBeriCastleCID] = useState(DEFAULT_AUTO_BERI_WORLD_SETTINGS.beriCastleCID);
  const [transferTroopWID, setTransferTroopWID] = useState(DEFAULT_AUTO_BERI_WORLD_SETTINGS.transferTroopWID);
  const [kutSourceCastleSCID, setKutSourceCastleSCID] = useState(
    DEFAULT_AUTO_BERI_WORLD_SETTINGS.kutSourceCastleSCID,
  );
  const [kutCastleCID, setKutCastleCID] = useState(DEFAULT_AUTO_BERI_WORLD_SETTINGS.kutCastleCID);
  const [troopSpaceCheckIntervalSec, setTroopSpaceCheckIntervalSec] = useState(
    DEFAULT_AUTO_BERI_WORLD_SETTINGS.troopSpaceCheckIntervalSec,
  );

  const hydrate = (s: AutoBeriWorldSettings) => {
    setMinTroopsToTransfer(s.minTroopsToTransfer);
    setBeriCastleCID(s.beriCastleCID);
    setTransferTroopWID(s.transferTroopWID);
    setKutSourceCastleSCID(s.kutSourceCastleSCID);
    setKutCastleCID(s.kutCastleCID);
    setTroopSpaceCheckIntervalSec(s.troopSpaceCheckIntervalSec);
  };

  useEffect(() => {
    if (!isOpen) return;
    const settings = parseAutoBeriWorldSettings(
      configuration?.sections['automation.beriWorld'] ?? DEFAULT_AUTO_BERI_WORLD_SETTINGS,
    );
    hydrate(settings);
  }, [configuration?.sections, isOpen]);

  const handlePickTroop = async () => {
    const result = await showTroopPicker({
      mode: 'single',
      title: 'Troop type to send to Beri world',
      preselected: transferTroopWID > 0 ? [transferTroopWID] : [],
    });
    if (typeof result === 'number' && result > 0) {
      setTransferTroopWID(result);
    }
  };

  const handleSave = () => {
    const settings = parseAutoBeriWorldSettings({
      minTroopsToTransfer,
      beriCastleCID,
      transferTroopWID,
      kutSourceCastleSCID,
      kutCastleCID,
      troopSpaceCheckIntervalSec,
    });
    queueConfigurationUpdate('automation.beriWorld', settings);
    onClose();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={
        <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
          <div className="min-w-0">
            <span className="text-primary">Auto Beri World</span>
            <p className="mt-1 text-sm font-normal text-text-muted">
              Configure Berimond troop transfers and the runtime schedule for this feature.
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            className="shrink-0"
            onClick={() => onOpenFeatureSchedule('autoBeriWorld', 'Auto Beri World')}
            leftIcon={<CalendarDays className="h-4 w-4" />}
          >
            Calendar
          </Button>
        </div>
      }
      maxWidth="md"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant="primary" leftIcon={<Save className="w-4 h-4" />} onClick={handleSave}>
            Save
          </Button>
        </>
      }
    >
      <div className="space-y-5">
        <p className="text-sm text-text-muted">
          Checks Beri troop space (fuc), sends troops (kut), then applies a march skip (msk).
          Set Beri castle CID here; it is saved and reused every pass (GCL only fills it when left at 0).
        </p>

        <div className="space-y-1.5">
          <label className="text-xs font-bold uppercase tracking-wider text-text-muted">
            Beri castle CID (fuc)
          </label>
          <Input
            type="number"
            min={1}
            value={beriCastleCID > 0 ? beriCastleCID : ''}
            onChange={(e) => setBeriCastleCID(parseInt(e.target.value, 10) || 0)}
            placeholder="e.g. 402"
          />
          <span className="text-xs text-text-muted">Used in the fuc CID field. Required for transfers.</span>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-bold uppercase tracking-wider text-text-muted">
            Check interval (seconds)
          </label>
          <Input
            type="number"
            min={5}
            max={3600}
            value={troopSpaceCheckIntervalSec}
            onChange={(e) => setTroopSpaceCheckIntervalSec(parseInt(e.target.value, 10) || 0)}
          />
          <span className="text-xs text-text-muted">
            How often the module runs fuc and transfers when FUC meets the minimum below.
          </span>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-bold uppercase tracking-wider text-text-muted">
            Minimum troops (from fuc)
          </label>
          <Input
            type="number"
            min={0}
            value={minTroopsToTransfer}
            onChange={(e) => setMinTroopsToTransfer(parseInt(e.target.value, 10) || 0)}
          />
        </div>

        <div className="space-y-2">
          <label className="text-xs font-bold uppercase tracking-wider text-text-muted">Transfer troop</label>
          <div className="flex gap-2">
            <Input readOnly value={transferTroopWID > 0 ? String(transferTroopWID) : ''} placeholder="Unit wodID" />
            <Button variant="outline" leftIcon={<Users className="w-4 h-4" />} onClick={handlePickTroop}>
              Pick unit
            </Button>
          </div>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-bold uppercase tracking-wider text-text-muted">
            kut source castle SCID (main castle)
          </label>
          <Input
            type="number"
            min={0}
            value={kutSourceCastleSCID || ''}
            onChange={(e) => setKutSourceCastleSCID(parseInt(e.target.value, 10) || 0)}
          />
          <span className="text-xs text-text-muted">
            Starting castle for kut (your main castle). Filled from login GCL when empty; transfers always use the current main castle when known.
          </span>
        </div>

        <div className="space-y-1.5">
          <label className="text-xs font-bold uppercase tracking-wider text-text-muted">kut CID field</label>
          <Input
            type="number"
            value={kutCastleCID}
            onChange={(e) => setKutCastleCID(parseInt(e.target.value, 10) || 0)}
          />
          <span className="text-xs text-text-muted">Wire kut CID (use -1 if unsure).</span>
        </div>
      </div>
    </Modal>
  );
};

export default AutoBeriWorldSettingsModal;
