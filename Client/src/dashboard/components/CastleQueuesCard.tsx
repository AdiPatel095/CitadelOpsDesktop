import React, { useMemo } from 'react';
import { SectionCard } from '../../components/ui';
import { useCastleFocus } from '../../context/CastleFocusContext';
import { useMetadata } from '../../context/MetadataContext';
import {
  craftingBuildingForStrip,
  visibleCastleQueueIds,
  type CastleQueueStripId,
} from '../CastleQueueVisibility';
import BarracksQueueSlot, { type ProductionQueueRow } from './BarracksQueueSlot';
import CraftingQueueSlot, { type CraftingQueueRow } from './CraftingQueueSlot';

interface CastleQueuesCardProps {
  title?: string;
}

interface CastleQueueStripDef {
  id: CastleQueueStripId;
  label: string;
  activeSlots: number;
  queueSlots: number;
	productionLineId?: number;
}

const QUEUE_DEFINITIONS: CastleQueueStripDef[] = [
  { id: 'recruitment', label: 'Recruitment queue', activeSlots: 1, queueSlots: 5, productionLineId: 0 },
  { id: 'tool', label: 'Tool queue', activeSlots: 1, queueSlots: 5, productionLineId: 1 },
  { id: 'refinery', label: 'Refinery', activeSlots: 2, queueSlots: 4 },
  { id: 'toolsmith', label: 'Toolsmith', activeSlots: 2, queueSlots: 4 },
  { id: 'dragon-hoard', label: 'Dragon Hoard', activeSlots: 2, queueSlots: 4 },
  { id: 'dragon-breath-forge', label: 'Dragon Breath Forge', activeSlots: 1, queueSlots: 1 },
];

function craftingOutputAmount(baseAmount: number | undefined, outputBoostPercent: number | undefined): number {
  if (typeof baseAmount !== 'number' || !Number.isFinite(baseAmount) || baseAmount <= 0) return 0;
  const boost = typeof outputBoostPercent === 'number' && Number.isFinite(outputBoostPercent)
    ? Math.max(0, outputBoostPercent)
    : 0;
  return baseAmount * (1 + boost / 100);
}

const CastleQueuesCard: React.FC<CastleQueuesCardProps> = ({ title = 'Queues' }) => {
  const { castle } = useCastleFocus();
  const { buildings, getCraftingRecipe } = useMetadata();
  const visible = useMemo(
    () => castle ? visibleCastleQueueIds(castle, buildings) : new Set<CastleQueueStripId>(),
    [buildings, castle],
  );
  const queues = useMemo(() => QUEUE_DEFINITIONS.filter((queue) => visible.has(queue.id)), [visible]);

  return (
    <SectionCard
      variant="solid"
      title={title}
      description="Canonical game queues"
      titleClassName="text-primary"
      descriptionClassName="font-bold uppercase tracking-wider"
      className="flex min-h-0 flex-col"
      contentClassName="custom-scrollbar flex-1 overflow-y-auto"
    >
        {!castle || queues.length === 0 ? (
          <div className="rounded-global border border-dashed border-border-light bg-bg-card/35 px-4 py-8 text-center">
            <p className="text-sm font-medium text-text-main">No production queues observed for this castle.</p>
            <p className="mx-auto mt-2 max-w-sm text-xs text-text-muted">Open the castle in-game to refresh its buildings and queues.</p>
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-6 pb-2 xl:grid-cols-2">
            {queues.map((queue) => {
              const crafting = craftingBuildingForStrip(castle, queue.id, buildings);
              const craftingRows: CraftingQueueRow[] = crafting ? [
                ...(crafting.active ?? []).map((item) => {
                  const recipe = getCraftingRecipe(item.recipeId);
                  return {
                    recipeId: item.recipeId,
                    label: recipe?.name || `Recipe ${item.recipeId}`,
                    imageUrl: recipe?.image,
                    amount: craftingOutputAmount(recipe?.outputAmount, item.batchValue),
                    active: true,
                  };
                }),
                ...(crafting.queued ?? []).map((item) => {
                  const recipe = getCraftingRecipe(item.recipeId);
                  return {
                    recipeId: item.recipeId,
                    label: recipe?.name || `Recipe ${item.recipeId}`,
                    imageUrl: recipe?.image,
                    amount: craftingOutputAmount(recipe?.outputAmount, item.batchValue),
                    active: false,
                  };
                }),
              ] : [];
								const production = queue.productionLineId == null
									? undefined
									: castle.production[String(queue.productionLineId)];
								const productionItems = production
									? [...(production.active ? [production.active] : []), ...(production.queued ?? [])]
									: [];
              const productionRows: ProductionQueueRow[] = productionItems.map((item, index) => ({
                definitionId: item.definition.id,
                amount: item.amount ?? 0,
								active: index === 0 && production?.active != null,
              }));
              const totalSlots = crafting
                ? Math.max(queue.activeSlots + queue.queueSlots, crafting.slotCount ?? 0)
								: Math.max(queue.activeSlots + queue.queueSlots, 1 + (production?.capacity ?? 0));
              return (
                <div key={queue.id} className="flex flex-col gap-2.5">
                  <h4 className="border-b border-border-base/50 pb-1 text-xs font-bold uppercase text-text-muted">{queue.label}</h4>
                  <div className="flex flex-wrap gap-2">
                    {Array.from({ length: totalSlots }, (_, index) => {
                      const craftingRow = craftingRows[index];
                      if (craftingRow) return <CraftingQueueSlot key={`${queue.id}-${index}-${craftingRow.recipeId}`} row={craftingRow} boxSize={48} />;
                      const productionRow = productionRows[index];
                      if (productionRow) return <BarracksQueueSlot key={`${queue.id}-${index}-${productionRow.definitionId}`} row={productionRow} imageSize={36} isTool={queue.id === 'tool'} />;
                      return <div key={`${queue.id}-${index}`} className="h-12 w-12 rounded-global border border-dashed border-border-light bg-bg-card/45" />;
                    })}
                  </div>
                </div>
              );
            })}
          </div>
        )}
    </SectionCard>
  );
};

export default CastleQueuesCard;
