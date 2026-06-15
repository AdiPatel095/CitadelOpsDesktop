import React, { useCallback, useState } from 'react';
import { Pencil, Play, Trash2 } from 'lucide-react';
import { useAuth } from '../../context/AuthContext';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { Card, CardContent, CardHeader, CardTitle, Button, Input } from '../../components/ui';
import { useRiftMap } from '../context/RiftMapContext';
import { formatSavedAt, riftLaunchLabel, type RiftCRALaunchEntry } from '../types/RiftCRALaunch';
import { arriveAtUnixFromOffset, isEarliestOffset } from '../types/RiftArrivalTime';
import RiftArrivalClock from './RiftArrivalClock';

const RiftAttackTemplate: React.FC = () => {
  const { gameLoggedIn } = useAuth();
  const { castleFocus } = useCastleFocus();
  const { riftCRALaunch, replayRiftCRALaunch, renameRiftCRALaunch, deleteRiftCRALaunch } = useRiftMap();
  const [offsetMinutesById, setOffsetMinutesById] = useState<Record<string, number>>({});
  const [editingId, setEditingId] = useState<string | null>(null);
  const [draftName, setDraftName] = useState('');

  const launches = riftCRALaunch?.launches ?? [];

  const setOffsetFor = useCallback((launchId: string, offsetMinutes: number) => {
    setOffsetMinutesById((prev) => ({ ...prev, [launchId]: offsetMinutes }));
  }, []);

  const startRename = useCallback((entry: RiftCRALaunchEntry) => {
    setEditingId(entry.id);
    setDraftName(entry.displayName?.trim() ?? '');
  }, []);

  const cancelRename = useCallback(() => {
    setEditingId(null);
    setDraftName('');
  }, []);

  const commitRename = useCallback(
    (launchId: string) => {
      renameRiftCRALaunch(launchId, draftName);
      setEditingId(null);
      setDraftName('');
    },
    [draftName, renameRiftCRALaunch]
  );

  const handleDelete = useCallback(
    (launchId: string) => {
      if (!window.confirm('Delete this captured Rift attack template?')) return;
      deleteRiftCRALaunch(launchId);
      setOffsetMinutesById((prev) => {
        const next = { ...prev };
        delete next[launchId];
        return next;
      });
      if (editingId === launchId) {
        cancelRename();
      }
    },
    [cancelRename, deleteRiftCRALaunch, editingId]
  );

  const handleAttack = useCallback(
    (entry: RiftCRALaunchEntry) => {
      if (!entry.canResend) return;
      const useFocusCoords =
        castleFocus?.mapPX != null &&
        castleFocus?.mapPY != null &&
        (castleFocus.mapPX !== 0 || castleFocus.mapPY !== 0);
      const offsetMinutes = offsetMinutesById[entry.id] ?? 0;
      const scheduled = !isEarliestOffset(offsetMinutes);
      const arriveAtUnix = scheduled ? arriveAtUnixFromOffset(entry, offsetMinutes) : null;
      replayRiftCRALaunch({
        launchId: entry.id,
        commanderID: entry.commanderID != null && entry.commanderID >= 0 ? entry.commanderID : undefined,
        sourceX: useFocusCoords ? castleFocus!.mapPX : undefined,
        sourceY: useFocusCoords ? castleFocus!.mapPY : undefined,
        ...(scheduled && arriveAtUnix != null ? { arriveAtUnix } : {}),
      });
    },
    [offsetMinutesById, castleFocus, replayRiftCRALaunch]
  );

  return (
    <Card className="border-border-base bg-bg-app/20">
      <CardHeader className="pb-3 border-b border-border-base bg-bg-card-hover/50 rounded-t-[calc(var(--radius-global)-1px)]">
        <div>
          <CardTitle className="text-lg text-primary">Captured Rift attacks</CardTitle>
          <p className="text-xs text-text-muted mt-1">
            Name templates for quick recognition. Feather travel time from the last successful launch sets the earliest
            arrival — use the clock to schedule later, then resend or schedule.
          </p>
        </div>
      </CardHeader>
      <CardContent className="pt-4">
        {launches.length === 0 ? (
          <p className="text-sm text-text-muted">
            {gameLoggedIn
              ? 'No captured attacks yet. Launch a castle attack on the Rift and it will appear here.'
              : 'No captured attacks in the last session. Connect and attack the Rift once to save a template.'}
          </p>
        ) : (
          <div className="overflow-x-auto rounded-lg border border-border-base">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border-base bg-bg-card/50 text-left text-[10px] uppercase tracking-wider text-text-muted">
                  <th className="px-3 py-2 font-semibold">Name</th>
                  <th className="px-3 py-2 font-semibold">Commander</th>
                  <th className="px-3 py-2 font-semibold">Layout</th>
                  <th className="px-3 py-2 font-semibold">Travel</th>
                  <th className="px-3 py-2 font-semibold">Captured</th>
                  <th className="px-3 py-2 font-semibold text-right">Arrive · Attack</th>
                </tr>
              </thead>
              <tbody>
                {launches.map((entry) => {
                  const canAttack = gameLoggedIn && entry.canResend === true;
                  const busy = entry.commanderBusy === true;
                  const offsetMinutes = offsetMinutesById[entry.id] ?? 0;
                  const scheduled = !isEarliestOffset(offsetMinutes);
                  const isEditing = editingId === entry.id;
                  const label = riftLaunchLabel(entry);
                  return (
                    <tr key={entry.id} className="border-b border-border-base/70 last:border-b-0">
                      <td className="px-3 py-3 min-w-[160px]">
                        {isEditing ? (
                          <Input
                            value={draftName}
                            onChange={(e) => setDraftName(e.target.value)}
                            onBlur={() => commitRename(entry.id)}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter') commitRename(entry.id);
                              if (e.key === 'Escape') cancelRename();
                            }}
                            placeholder={riftLaunchLabel(entry)}
                            className="h-8 text-sm"
                            autoFocus
                            maxLength={80}
                          />
                        ) : (
                          <div className="flex items-center gap-1.5 min-w-0">
                            <span
                              className={`truncate font-medium ${entry.displayName?.trim() ? 'text-text-main' : 'text-text-muted'}`}
                              title={label}
                            >
                              {label}
                            </span>
                            <button
                              type="button"
                              onClick={() => startRename(entry)}
                              className="shrink-0 p-1 rounded-md text-text-muted hover:text-primary hover:bg-bg-card-hover"
                              title="Rename template"
                            >
                              <Pencil className="w-3.5 h-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => handleDelete(entry.id)}
                              className="shrink-0 p-1 rounded-md text-text-muted hover:text-error hover:bg-error/10"
                              title="Delete template"
                            >
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          </div>
                        )}
                      </td>
                      <td className="px-3 py-3 font-mono text-text-main">LID {entry.commanderID ?? '—'}</td>
                      <td className="px-3 py-3 text-text-main">
                        {entry.waveCount ?? 0} wave{(entry.waveCount ?? 0) === 1 ? '' : 's'}
                        {entry.useTravelFeather ? (
                          <span className="text-text-muted"> · feather</span>
                        ) : null}
                        <p className="text-xs font-mono text-text-muted mt-0.5">
                          ({entry.sourceX}, {entry.sourceY}) → ({entry.targetX}, {entry.targetY})
                        </p>
                      </td>
                      <td className="px-3 py-3 text-text-muted whitespace-nowrap">
                        {entry.oneWayTTSeconds != null && entry.oneWayTTSeconds > 0 ? (
                          <span className="font-mono">{entry.oneWayTTSeconds}s</span>
                        ) : (
                          <span className="text-xs">pending success</span>
                        )}
                      </td>
                      <td className="px-3 py-3 text-text-muted whitespace-nowrap">
                        {formatSavedAt(entry.savedAtUnix)}
                      </td>
                      <td className="px-3 py-3">
                        <div className="flex items-center justify-end gap-2">
                          <RiftArrivalClock
                            entry={entry}
                            offsetMinutes={offsetMinutes}
                            onOffsetChange={(offset) => setOffsetFor(entry.id, offset)}
                          />
                          <Button
                            variant="primary"
                            size="sm"
                            disabled={!canAttack}
                            onClick={() => handleAttack(entry)}
                            title={
                              !gameLoggedIn
                                ? 'Connect to attack'
                                : busy
                                  ? `Commander LID ${entry.commanderID ?? '—'} is on an attack march`
                                  : scheduled
                                    ? 'Schedule attack for the selected arrival time'
                                    : 'Resend now (earliest feather arrival)'
                            }
                            leftIcon={<Play className="w-3.5 h-3.5" />}
                          >
                            {scheduled ? 'Schedule' : 'Resend'}
                          </Button>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
};

export default RiftAttackTemplate;
