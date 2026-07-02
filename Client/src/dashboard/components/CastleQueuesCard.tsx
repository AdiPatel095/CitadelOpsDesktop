import React, { useMemo } from 'react';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { Card, CardHeader, CardTitle, CardContent } from '../../components/ui';
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
} from '../../types/CastleFocusState.ts';
import { visibleCastleQueueIds } from '../CastleQueueVisibility';
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
  lid: number;
  layout: SlotStripLayout;
}

function stripCells(d: CastleQueueStripDef): number {
  return d.layout.activeSlots + d.layout.queueSlots;
}

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

  return (
    <Card className="liquid-prominent-header-card flex flex-col min-h-0">
      <CardHeader className="liquid-card-header-prominent">
        <div className="flex flex-col">
          <CardTitle className="text-primary">{title}</CardTitle>
          <p className="text-xs text-text-muted mt-1 uppercase tracking-wider font-bold">Queuing Coming Soon</p>
        </div>
      </CardHeader>

      <CardContent className="liquid-prominent-header-content flex-1 overflow-y-auto custom-scrollbar">
        {queuesToRender.length === 0 ? (
          <div className="rounded-global border border-dashed border-border-light bg-bg-card/35 px-4 py-8 text-center backdrop-blur-xl">
            <p className="text-sm font-medium text-text-main">
              No matching production buildings in this focus snapshot yet.
            </p>
            <p className="mt-2 text-xs text-text-muted max-w-sm mx-auto">
              Open this castle in-game (JAA) so BG/BD rows refresh, or switch focus from the strip under the header.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 xl:grid-cols-2 gap-6 pb-2">
            {queuesToRender.map((q) => {
              const nCells = stripCells(q);
              const manualKey = stripIdToCraftingManual(q.id);
              const craftSnap = manualKey ? craftingSnapshotForStrip(castleFocus, manualKey) : undefined;
              const useCrafting = Boolean(manualKey && craftSnap);
              const craftRows = useCrafting ? craftingStripRowsMerged(craftSnap, q.layout) : null;
              const bp = slotProductionForLid(castleFocus, q.lid);
              const splRows = productionQueueRows(bp, q.layout);
              return (
                <div key={q.id} className="flex flex-col gap-2.5">
                  <h4 className="text-xs font-bold text-text-muted uppercase border-b border-border-base/50 pb-1">{q.label}</h4>
                  <div className="flex flex-wrap gap-2">
                    {[...Array(nCells)].map((_, i) => {
                      if (useCrafting && craftRows) {
                        const row = craftRows[i] ?? null;
                        if (row) {
                          return (
                            <CraftingQueueSlot key={`${q.id}-cr-${i}-${row.crid}-${row.qty}`} row={row} boxSize={48} />
                          );
                        }
                        return <div key={i} className="w-12 h-12 rounded-global bg-bg-card/45 border border-border-light border-dashed backdrop-blur-xl" />;
                      }
                      const row = splRows[i] ?? null;
                      if (row) {
                        const isTool = q.id === 'tool';
                        return (
                          <BarracksQueueSlot
                            key={`${q.id}-${i}-${row.pid ?? row.wid}-${row.tua}`}
                            row={row}
                            imageSize={36}
                            isTool={isTool}
                          />
                        );
                      }
                      return <div key={i} className="w-12 h-12 rounded-global bg-bg-card/45 border border-border-light border-dashed backdrop-blur-xl" />;
                    })}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </CardContent>
    </Card>
  );
};

export default CastleQueuesCard;
