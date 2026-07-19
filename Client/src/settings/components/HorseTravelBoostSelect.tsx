import React from 'react';
import { Gauge } from 'lucide-react';
import { Select } from '../../components/ui';
import {
  HORSE_TRAVEL_BOOST_OPTIONS,
  parseHorseTravelBoostID,
  type HorseTravelBoostID,
} from '../HorseTravelBoost';

interface HorseTravelBoostSelectProps {
  value: HorseTravelBoostID;
  onChange: (value: HorseTravelBoostID) => void;
  className?: string;
}

const HorseTravelBoostSelect: React.FC<HorseTravelBoostSelectProps> = ({ value, onChange, className }) => (
  <label className={className ?? 'block'}>
    <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
      <Gauge className="h-3.5 w-3.5" /> Horse travel boost
    </span>
    <Select
      value={String(value)}
      onChange={(next) => onChange(parseHorseTravelBoostID(next))}
      options={HORSE_TRAVEL_BOOST_OPTIONS}
      menuGrowToViewport
    />
    <span className="mt-1.5 block text-[11px] text-text-muted">
      Applied to HBW for every attack launched by this feature. Ruby horses are used only when explicitly selected.
    </span>
  </label>
);

export default HorseTravelBoostSelect;
