import { useLocale } from '../../i18n/LocaleContext';
import {LocalizedText} from "../../i18n/LocalizedText";
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
}) => {
  const { t } = useLocale();
  return (
  <label className={className ?? 'block'}>
    <span className="mb-1.5 flex items-center gap-2 text-caption font-semibold text-text-muted">
      <Gauge className="h-3.5 w-3.5" /> <LocalizedText messageKey="ui.settings.components.horseTravelBoostSelect.horse.travel.boost.983830d4" />
    </span>
    <Select
      ariaLabel={t('ui.settings.components.horseTravelBoostSelect.horse.travel.boost.983830d4')}
      value={String(value)}
      onChange={(next) => onChange(parseHorseTravelBoostID(next))}
      options={negativeOneLabel
        ? HORSE_TRAVEL_BOOST_OPTIONS.map((option) => (
            option.value === '-1' ? { ...option, label: negativeOneLabel } : option
          ))
        : HORSE_TRAVEL_BOOST_OPTIONS}
      menuGrowToViewport
    />
    <span className="mt-1.5 block text-caption text-text-muted">
      {description ?? 'The exact HBW ID and speed are resolved from the source castle’s current Stable, Faction Stable, or Harbor level. Ruby tiers are used only when explicitly selected.'}
    </span>
  </label>
  );
};

export default HorseTravelBoostSelect;
