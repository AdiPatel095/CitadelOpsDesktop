import React, { useMemo } from 'react';
import { RefreshCw } from 'lucide-react';
import StaleSessionBanner from '../../components/StaleSessionBanner';
import { useAuth } from '../../context/AuthContext';
import { Card, CardContent, CardHeader, CardTitle, Button } from '../../components/ui';
import { useMovement } from '../context/MovementContext';
import {
  directionLabel,
  formatTroopSummary,
  labelKingdom,
  labelTargetType,
  type GAMMovement,
} from '../types/MovementState';

function formatProgress(pt: number, tt: number): string {
  if (tt <= 0) return `${pt}s`;
  const pct = Math.min(100, Math.round((pt / tt) * 100));
  return `${pt}s / ${tt}s (${pct}%)`;
}

function sortMovements(rows: GAMMovement[]): GAMMovement[] {
  return [...rows].sort((a, b) => {
    const cmdA = a.commanderID >= 0 ? a.commanderID : 999999;
    const cmdB = b.commanderID >= 0 ? b.commanderID : 999999;
    if (cmdA !== cmdB) return cmdA - cmdB;
    if (a.d !== b.d) return a.d - b.d;
    return a.mid - b.mid;
  });
}

const MovementView: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { movement, refreshMovement } = useMovement();

  const rows = useMemo(
    () => sortMovements(movement?.activeMovements ?? []),
    [movement?.activeMovements]
  );

  return (
    <div className="flex flex-col gap-6">
      <StaleSessionBanner />

      <Card className="liquid-prominent-header-card">
        <CardHeader className="liquid-card-header-prominent">
          <CardTitle className="text-lg text-primary">
            Active movements
            <span className="ml-2 text-sm font-normal text-text-muted">({rows.length})</span>
          </CardTitle>
          <Button
            variant="secondary"
            size="sm"
            disabled={!gameLoggedIn}
            onClick={() => refreshMovement(true)}
            title={gameLoggedIn ? 'Request fresh GAM from the game client' : 'Connect to refresh'}
          >
            <RefreshCw className="w-4 h-4 mr-1.5" />
            Refresh
          </Button>
        </CardHeader>
        <CardContent className="liquid-prominent-header-content">
          {rows.length === 0 ? (
            <p className="text-sm text-text-muted">
              {gameLoggedIn
                ? 'No active movements in the latest GAM snapshot.'
                : 'No movements in the last saved session. Connect and refresh to pull live GAM data.'}
            </p>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-border-base custom-scrollbar">
              <table className="min-w-[58rem] w-full text-sm">
                <thead>
                  <tr className="border-b border-border-base bg-bg-card/50 text-left text-[10px] uppercase tracking-wider text-text-muted">
                    <th className="px-3 py-2 font-semibold">Commander</th>
                    <th className="px-3 py-2 font-semibold">Direction</th>
                    <th className="px-3 py-2 font-semibold">Kingdom</th>
                    <th className="px-3 py-2 font-semibold">Target type</th>
                    <th className="px-3 py-2 font-semibold">Route</th>
                    <th className="px-3 py-2 font-semibold">Progress</th>
                    <th className="px-3 py-2 font-semibold">Troops</th>
                    <th className="px-3 py-2 font-semibold">MID</th>
                  </tr>
                </thead>
                <tbody>
                  {rows.map((row) => (
                    <tr
                      key={`${row.mid}-${row.d}-${row.commanderID}`}
                      className="border-b border-border-base/70 last:border-b-0"
                    >
                      <td className="px-3 py-3 font-mono text-text-main whitespace-nowrap">
                        {row.commanderID >= 0 ? `LID ${row.commanderID}` : '—'}
                      </td>
                      <td className="px-3 py-3 text-text-main whitespace-nowrap">
                        {directionLabel(row.d)}
                      </td>
                      <td className="px-3 py-3 text-text-main whitespace-nowrap">
                        {labelKingdom(row.kid)}
                      </td>
                      <td className="px-3 py-3 text-text-main">
                        {labelTargetType(row.targetType)}
                        {row.targetType > 0 ? (
                          <span className="block text-xs font-mono text-text-muted">
                            type {row.targetType}
                          </span>
                        ) : null}
                      </td>
                      <td className="px-3 py-3 font-mono text-text-muted whitespace-nowrap">
                        ({row.sourceX}, {row.sourceY}) → ({row.targetX}, {row.targetY})
                      </td>
                      <td className="px-3 py-3 text-text-muted whitespace-nowrap">
                        {formatProgress(row.pt, row.tt)}
                      </td>
                      <td className="px-3 py-3 text-text-muted">
                        {formatTroopSummary(row.troopArray)}
                      </td>
                      <td className="px-3 py-3 font-mono text-text-muted whitespace-nowrap">
                        {row.mid}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
};

export default MovementView;
