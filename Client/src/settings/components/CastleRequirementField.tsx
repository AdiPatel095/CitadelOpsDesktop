import React, { useMemo } from 'react';
import { Castle } from 'lucide-react';
import type { GameStateV2 } from '../../api/Contracts';
import { Select, type SelectOption } from '../../components/ui/Select';
import { useLocale } from '../../i18n/LocaleContext';
import { castleOptionsFor, evaluateCastleReference, type CastlePurpose } from '../requirements/castleRequirements';
import type { ObservationContext } from '../requirements/observationFreshness';
import { ReadinessCheckLine } from './ReadinessPanel';

export interface CastleRequirementFieldProps {
  id: string;
  label: React.ReactNode;
  value: number;
  onChange: (castleId: number) => void;
  state: GameStateV2 | null;
  purpose: CastlePurpose;
  /** When set, the castle's unit counts must be current (CIT-15 D1). */
  requireObservedUnits?: ObservationContext;
  /** The options this module offers today; defaults to every castle matching the purpose. */
  options?: SelectOption[];
  placeholder?: React.ReactNode;
  disabled?: boolean;
}

/**
 * Castle picker plus its requirement line. A saved castle that is not in the
 * selected account/world stays selected and is reported as "reselect"; it is
 * never cleared automatically.
 */
export const CastleRequirementField: React.FC<CastleRequirementFieldProps> = ({
  id,
  label,
  value,
  onChange,
  state,
  purpose,
  requireObservedUnits,
  options,
  placeholder,
  disabled,
}) => {
  const { t: localizeStatic } = useLocale();
  const offered = useMemo(() => options ?? castleOptionsFor(state, purpose).map((castle) => ({
    value: String(castle.id),
    label: `${castle.name} · ${castle.x}:${castle.y}`,
  })), [options, purpose, state]);
  const check = evaluateCastleReference({ castleId: value, state, purpose, requireObservedUnits });
  const castleDataObserved = state != null && Object.keys(state.castles ?? {}).length > 0;
  const savedNotListed = value > 0 && state?.castles[String(value)] == null;
  // Without castle data the saved castle is kept as is ("not observed yet"), never "not in this world".
  const selectOptions = savedNotListed
    ? [...offered, {
      value: String(value),
      label: localizeStatic(castleDataObserved ? 'castleRequirement.missingOption' : 'castleRequirement.unobservedOption', { id: value }),
    }]
    : offered;
  return (
    <div id={id} className="block">
      <span className="mb-1.5 flex items-center gap-2 text-caption font-semibold text-text-muted">
        <Castle className="h-3.5 w-3.5" aria-hidden="true" /> {label}
      </span>
      <Select
        value={value > 0 ? String(value) : ''}
        onChange={(next) => onChange(Number(next) || 0)}
        options={selectOptions}
        placeholder={placeholder}
        disabled={disabled}
        menuGrowToViewport
      />
      {check.state !== 'valid' ? (
        <ul className="mt-1.5">
          <ReadinessCheckLine check={check} />
        </ul>
      ) : null}
    </div>
  );
};
