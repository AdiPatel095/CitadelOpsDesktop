import React, { useMemo } from 'react';
import { Castle } from 'lucide-react';
import type { GameStateV2 } from '../../api/Contracts';
import { Select, type SelectOption } from '../../components/ui/Select';
import { useLocale } from '../../i18n/LocaleContext';
import { castleOptionsFor, evaluateCastleReference, type CastlePurpose } from '../requirements/castleRequirements';
import { ReadinessCheckLine } from './ReadinessPanel';

export interface CastleRequirementFieldProps {
  id: string;
  label: React.ReactNode;
  value: number;
  onChange: (castleId: number) => void;
  state: GameStateV2 | null;
  purpose: CastlePurpose;
  requireObservedUnits?: boolean;
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
  const savedMissing = value > 0 && state != null && state.castles[String(value)] == null;
  const selectOptions = savedMissing
    ? [...offered, { value: String(value), label: localizeStatic('castleRequirement.missingOption', { id: value }) }]
    : offered;
  return (
    <div id={id} className="block">
      <span className="mb-1.5 flex items-center gap-2 text-[10px] font-black uppercase tracking-wider text-text-muted">
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
