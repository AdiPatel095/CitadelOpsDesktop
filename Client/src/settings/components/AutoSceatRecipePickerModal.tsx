import React, { useEffect, useMemo, useState } from 'react';
import { Clock3, Search } from 'lucide-react';
import { Badge, Button, Input, Modal } from '../../components/ui';
import type {
  AutoSceatBuildingState,
  AutoSceatRecipeCatalogEntry,
  AutoSceatResCatalog,
} from '../AutoSceatResClientState';

interface AutoSceatRecipePickerModalProps {
  isOpen: boolean;
  building: AutoSceatBuildingState | null;
  catalog: AutoSceatResCatalog;
  allowRubyRecipes: boolean;
  onClose: () => void;
  onSelect: (recipe: AutoSceatRecipeCatalogEntry) => void;
}

function formatDuration(seconds: number): string {
  if (seconds >= 3600 && seconds % 3600 === 0) return `${seconds / 3600}h`;
  if (seconds >= 60 && seconds % 60 === 0) return `${seconds / 60}m`;
  return `${seconds}s`;
}

function recipeSearchText(recipe: AutoSceatRecipeCatalogEntry): string {
  return [recipe.output.name, recipe.output.key, recipe.type]
    .join(' ')
    .toLowerCase();
}

interface AutoSceatRecipeLevelGroup {
  key: string;
  recipes: AutoSceatRecipeCatalogEntry[];
}

function recipeLevelGroupKey(recipe: AutoSceatRecipeCatalogEntry): string {
  return [recipe.queueTypeID, recipe.recipeGroupID, recipe.output.key, recipe.type].join(':');
}

function recipeCostDisplay(catalog: AutoSceatResCatalog, key: string): { name: string; iconUrl?: string } {
  switch (key.toLowerCase()) {
    case 'c1':
    case 'coins':
      return { name: 'Coins', iconUrl: '/game-data/resources/images/Coins.webp' };
    case 'sceat':
    case 'sceattoken':
      return { name: 'Sceat', iconUrl: '/game-data/resources/images/Sceat.webp' };
    default: {
      const meta = catalog.resources[key];
      return { name: meta?.name ?? key, iconUrl: meta?.iconUrl };
    }
  }
}

export const AutoSceatRecipePickerModal: React.FC<AutoSceatRecipePickerModalProps> = ({
  isOpen,
  building,
  catalog,
  allowRubyRecipes,
  onClose,
  onSelect,
}) => {
  const [search, setSearch] = useState('');

  useEffect(() => {
    if (!isOpen) return;
    setSearch('');
  }, [isOpen, building?.oid]);

  const recipeGroups = useMemo(() => {
    if (!building) return [];
    const available = new Set(building.availableRecipeIDs);
    const query = search.trim().toLowerCase();
    const grouped = new Map<string, AutoSceatRecipeLevelGroup>();
    catalog.recipes.forEach((recipe) => {
      if (recipe.queueTypeID !== building.queueTypeID || !available.has(recipe.recipeID)) return;
      if ((recipe.costs.rubies ?? 0) > 0 && !allowRubyRecipes) return;
      const key = recipeLevelGroupKey(recipe);
      const group = grouped.get(key) ?? { key, recipes: [] };
      group.recipes.push(recipe);
      grouped.set(key, group);
    });
    return [...grouped.values()]
      .map((group) => ({ ...group, recipes: [...group.recipes].sort((left, right) => left.level - right.level) }))
      .filter((group) => !query || group.recipes.some((recipe) => recipeSearchText(recipe).includes(query)))
      .sort((left, right) => {
        const leftRecipe = left.recipes[left.recipes.length - 1];
        const rightRecipe = right.recipes[right.recipes.length - 1];
        const output = leftRecipe.output.name.localeCompare(rightRecipe.output.name);
        if (output !== 0) return output;
        return leftRecipe.type.localeCompare(rightRecipe.type);
      });
  }, [allowRubyRecipes, building, catalog.recipes, search]);

  return (
    <Modal
      isOpen={isOpen && building != null}
      onClose={onClose}
      maxWidth="5xl"
      title={
        <span className="flex min-w-0 flex-col">
          <span className="text-lg font-black">Choose {building?.name ?? 'Crafting'} Recipe</span>
          <span className="mt-1 text-xs font-semibold text-text-muted">
            Only the highest unlocked level for each recipe type is shown.
          </span>
        </span>
      }
    >
      <div className="flex flex-col gap-4">
        <Input
          value={search}
          onChange={(event) => setSearch(event.target.value)}
          placeholder="Search output or type"
          leftIcon={<Search className="h-4 w-4" />}
        />

        {!catalog.researchLoaded && (
          <div className="rounded-global border border-warning/30 bg-warning/10 px-4 py-3 text-sm font-semibold text-warning">
            Research unlocks have not been loaded yet. Refresh after the game is connected.
          </div>
        )}

        <div className="grid max-h-[62vh] grid-cols-1 gap-3 overflow-y-auto pr-1 custom-scrollbar md:grid-cols-2">
          {recipeGroups.map((group) => {
            const recipe = group.recipes[group.recipes.length - 1];
            const costs = Object.entries(recipe.costs).sort(([left], [right]) => left.localeCompare(right));
            return (
              <div
                key={group.key}
                className="group flex min-w-0 flex-col rounded-global border border-border-base bg-bg-card/60 p-4 text-left transition hover:border-primary/35 hover:bg-bg-card-hover/60"
              >
                <div className="flex min-w-0 items-start gap-4">
                  <span className="flex h-14 w-14 shrink-0 items-center justify-center rounded-xl border border-border-light bg-bg-input/70 shadow-inner">
                    {recipe.output.iconUrl ? (
                      <img src={recipe.output.iconUrl} alt="" className="h-11 w-11 object-contain" />
                    ) : (
                      <span className="text-lg font-black text-primary">{recipe.level}</span>
                    )}
                  </span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-black text-text-main">{recipe.output.name}</span>
                    <span className="mt-2 flex flex-wrap items-center gap-1.5">
                      <Badge className="px-2 py-1 text-[11px]" variant={recipe.type === 'Ruby' ? 'danger' : 'secondary'}>{recipe.type}</Badge>
                      <Badge className="px-2 py-1 text-[11px]" variant="outline">Level {recipe.level}</Badge>
                      <span className="inline-flex items-center gap-1 rounded-full border border-border-base bg-bg-input/55 px-2 py-1 text-[11px] font-bold text-text-muted">
                        <Clock3 className="h-3.5 w-3.5" />
                        {formatDuration(recipe.durationSec)}
                      </span>
                      {costs.map(([key, amount]) => {
                        const meta = recipeCostDisplay(catalog, key);
                        return (
                          <span key={key} className="inline-flex items-center gap-1 rounded-full border border-border-base bg-bg-input/55 px-2 py-1 text-[11px] font-bold text-text-muted">
                            {meta.iconUrl && <img src={meta.iconUrl} alt="" className="h-4 w-auto max-w-7 object-contain" />}
                            {meta.name} {amount.toLocaleString()}
                          </span>
                        );
                      })}
                    </span>
                  </span>
                </div>

                <Button
                  variant="primary"
                  className="mt-3 w-full"
                  onClick={() => {
                    onSelect(recipe);
                    onClose();
                  }}
                >
                  Add to cycle
                </Button>
              </div>
            );
          })}
          {recipeGroups.length === 0 && (
            <div className="col-span-full rounded-global border border-dashed border-border-base bg-bg-card/40 px-5 py-12 text-center text-sm font-semibold text-text-muted">
              No available recipes match this search.
            </div>
          )}
        </div>

        <div className="flex justify-end">
          <Button variant="ghost" onClick={onClose}>Close</Button>
        </div>
      </div>
    </Modal>
  );
};
