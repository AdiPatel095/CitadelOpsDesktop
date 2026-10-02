import React from 'react';
import { LocalizedText } from '../../i18n/LocalizedText';
import type { FoodCastleRow } from '../requirements/setupReadiness';

/**
 * One castle in the Food Balance table (CIT-20). Every row shows a role (donor, below reserve or not observed) and
 * an honest "last checked": a row that is not current says "Last known" with its reason (and its own time when the
 * game reported one), a current row shows the game's time for that castle, and a current row without a time says
 * "This connection". A castle's food is never shown as freshly checked from the connection alone.
 */
export const FoodCastleTableRow: React.FC<{ row: FoodCastleRow }> = ({ row }) => (
  <tr data-food-castle={row.castleId} data-food-role={row.role}>
    <td className="min-w-[4.5rem] whitespace-normal break-words py-1 pr-2 text-text-main">{row.name}</td>
    <td className="py-1 pr-2 text-right font-mono tabular-nums">{row.food == null ? '—' : Math.floor(row.food).toLocaleString()}</td>
    <td className="min-w-[5.5rem] whitespace-normal py-1 pr-2 text-text-muted"><LocalizedText messageKey="setupReadiness.foodRole" params={{ role: row.role }} /></td>
    <td className="whitespace-normal break-words py-1 text-text-muted" data-food-checked={row.current ? 'current' : 'last-known'}>
      {row.current
        ? row.observedAt
          ? <LocalizedText messageKey="observedAt.castleFoodShort" params={{ observedAt: Date.parse(row.observedAt) }} />
          : <LocalizedText messageKey="observedAt.thisConnection" />
        : (
          <>
            <LocalizedText messageKey="observedAt.lastKnownFood" />
            {row.observedAt ? <> · <LocalizedText messageKey="observedAt.castleFoodShort" params={{ observedAt: Date.parse(row.observedAt) }} /></> : null}
            {row.unavailableReason ? <span className="block text-caption"><LocalizedText messageKey={row.unavailableReason} /></span> : null}
          </>
        )}
    </td>
  </tr>
);
