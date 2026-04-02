import React, { useEffect, useMemo } from 'react';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { FrontendWebsocket } from '../../websocket';
import {
  craftingSnapshotForStrip,
  craftingStripRowsMerged,
  mergedCastleFocusRows,
  productionQueueRows,
  SLOT_LID_DRAGON_BREATH_FORGE,
  SLOT_LID_DRAGON_HOARD,
  SLOT_LID_REFINERY,
  SLOT_LID_RECRUITMENT,
  SLOT_LID_TOOLSMITH,
  SLOT_LID_TOOL_WORKSHOP,
  slotProductionForLid,
  type CraftingManualStripId,
  type SlotStripLayout,
} from '../../types/castleFocusState.ts';
import { visibleCastleQueueIds } from '../castleQueueVisibility';
import BarracksQueueSlot from './BarracksQueueSlot';
import CraftingQueueSlot from './CraftingQueueSlot';

interface CastleQueuesCardProps {
  title?: string;
}

export type CastleQueueStripId =
  | 'recruitment'
  | 'tool'
  | 'refinery'
  | 'toolsmith'
  | 'dragon-hoard'
  | 'dragon-breath-forge';

export interface CastleQueueStripDef {
  id: CastleQueueStripId;
  label: string;
  /** Empire **spl** LID for this strip (see `SLOT_LID_*` in castleFocusState). */
  lid: number;
  layout: SlotStripLayout;
}

function stripCells(d: CastleQueueStripDef): number {
  return d.layout.activeSlots + d.layout.queueSlots;
}

/**
 * Recruit / workshop: 1 active + 5 queued. Refinery, toolsmith, dragon forges (manual crafting): 2 active + 4 queued.
 * LID 2–5 are assumed sequential—confirm with a websocket capture if a strip stays empty.
 */
export const CASTLE_QUEUE_DEFINITIONS: CastleQueueStripDef[] = [
  { id: 'recruitment', label: 'Recruitment queue', lid: SLOT_LID_RECRUITMENT, layout: { activeSlots: 1, queueSlots: 5 } },
  { id: 'tool', label: 'Tool queue', lid: SLOT_LID_TOOL_WORKSHOP, layout: { activeSlots: 1, queueSlots: 5 } },
  { id: 'refinery', label: 'Refinery', lid: SLOT_LID_REFINERY, layout: { activeSlots: 2, queueSlots: 4 } },
  { id: 'toolsmith', label: 'Toolsmith', lid: SLOT_LID_TOOLSMITH, layout: { activeSlots: 2, queueSlots: 4 } },
  { id: 'dragon-hoard', label: 'DragonHoard', lid: SLOT_LID_DRAGON_HOARD, layout: { activeSlots: 2, queueSlots: 4 } },
  {
    id: 'dragon-breath-forge',
    label: 'DragonBreathForge',
    lid: SLOT_LID_DRAGON_BREATH_FORGE,
    layout: { activeSlots: 2, queueSlots: 4 },
  },
];

const MANUAL_CRAFTING_STRIP_IDS = new Set<CastleQueueStripId>([
  'refinery',
  'toolsmith',
  'dragon-hoard',
  'dragon-breath-forge',
]);

function stripIdToCraftingManual(id: CastleQueueStripId): CraftingManualStripId | undefined {
  if (!MANUAL_CRAFTING_STRIP_IDS.has(id)) return undefined;
  return id as CraftingManualStripId;
}

const CastleQueuesCard: React.FC<CastleQueuesCardProps> = ({ title = 'Queues' }) => {
  const { castleFocus } = useCastleFocus();
  const rows = useMemo(() => mergedCastleFocusRows(castleFocus), [castleFocus]);
  const visible = useMemo(() => visibleCastleQueueIds(rows), [rows]);
  const queuesToRender = useMemo(
    () => CASTLE_QUEUE_DEFINITIONS.filter((q) => visible.has(q.id)),
    [visible]
  );

  const focusedAid = castleFocus?.aid && castleFocus.aid > 0 ? castleFocus.aid : 0;

  const lidsToPoll = useMemo(() => {
    if (focusedAid <= 0) return [];
    return [...new Set(queuesToRender.map((q) => q.lid))].sort((a, b) => a - b);
  }, [focusedAid, queuesToRender]);

  useEffect(() => {
    if (lidsToPoll.length === 0) return;
    const timers: number[] = [];
    let delay = 260;
    for (const lid of lidsToPoll) {
      const t = delay;
      timers.push(window.setTimeout(() => FrontendWebsocket.sendRequestSlotProduction(lid), t));
      delay += 170;
    }
    return () => timers.forEach((id) => window.clearTimeout(id));
  }, [lidsToPoll]);

  return (
    <div className="castle-card">
      <h3 className="castle-name">{title}</h3>
      <p className="-mt-2 mb-2 text-xs text-text-muted">
        Queues match JAA buildings on this focus. Recruitment and tool workshop use **spl** (troop/tool WIDs and
        **TUA**). Refinery, toolsmith, and dragon forges prefer **crin**/**crst** (**CRID** + labels from
        craftingRecipes.json); **spl** fills in until crafting messages arrive.
      </p>
      {queuesToRender.length === 0 ? (
        <div className="rounded-global border border-dashed border-border-light bg-bg-app/30 px-4 py-8 text-center">
          <p className="text-sm text-text-muted">
            No matching production buildings in this focus snapshot yet.
          </p>
          <p className="mt-1 text-xs text-text-muted/90">
            Open this castle in-game (JAA) so BG/BD rows refresh, or switch focus from the strip under the header.
          </p>
        </div>
      ) : (
        <div className="grid grid-cols-2 gap-4">
          {queuesToRender.map((q) => {
            const nCells = stripCells(q);
            const manualKey = stripIdToCraftingManual(q.id);
            const craftSnap = manualKey ? craftingSnapshotForStrip(castleFocus, manualKey) : undefined;
            const useCrafting = Boolean(manualKey && craftSnap);
            const craftRows = useCrafting ? craftingStripRowsMerged(craftSnap, q.layout) : null;
            const bp = slotProductionForLid(castleFocus, q.lid);
            const splRows = productionQueueRows(bp, q.layout);
            return (
              <div key={q.id} className="queue min-h-0">
                <h4>{q.label}</h4>
                <div className="queue-items flex-wrap">
                  {[...Array(nCells)].map((_, i) => {
                    if (useCrafting && craftRows) {
                      const row = craftRows[i] ?? null;
                      if (row) {
                        return (
                          <CraftingQueueSlot key={`${q.id}-cr-${i}-${row.crid}-${row.qty}`} row={row} boxSize={52} />
                        );
                      }
                      return <div key={i} className="queue-item-placeholder" />;
                    }
                    const row = splRows[i] ?? null;
                    if (row) {
                      return (
                        <BarracksQueueSlot
                          key={`${q.id}-${i}-${row.pid ?? row.wid}-${row.tua}`}
                          row={row}
                          imageSize={40}
                        />
                      );
                    }
                    return <div key={i} className="queue-item-placeholder" />;
                  })}
                </div>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};

export default CastleQueuesCard;
