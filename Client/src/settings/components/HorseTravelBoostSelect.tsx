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
  negativeOneLabel?: string;
  description?: React.ReactNode;
}

const HorseTravelBoostSelect: React.FC<HorseTravelBoostSelectProps> = ({
  value,
  onChange,
  className,
  negativeOneLabel,
  description,
}) => (
  <label className={className ?? 'block'}>
    <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
      <Gauge className="h-3.5 w-3.5" /> Horse travel boost
    </span>
    <Select
      value={String(value)}
      onChange={(next) => onChange(parseHorseTravelBoostID(next))}
      options={negativeOneLabel
        ? HORSE_TRAVEL_BOOST_OPTIONS.map((option) => (
            option.value === '-1' ? { ...option, label: negativeOneLabel } : option
          ))
        : HORSE_TRAVEL_BOOST_OPTIONS}
      menuGrowToViewport
    />
    <span className="mt-1.5 block text-[11px] text-text-muted">
      {description ?? 'The exact HBW ID and speed are resolved from the source castle’s current Stable, Faction Stable, or Harbor level. Ruby tiers are used only when explicitly selected.'}
    </span>
  </label>
);

export default HorseTravelBoostSelect;
