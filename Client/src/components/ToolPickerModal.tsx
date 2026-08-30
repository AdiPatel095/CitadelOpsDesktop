import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { Check } from 'lucide-react';
import { useVirtualizer } from '@tanstack/react-virtual';
import ToolImage from './ToolImage';
import { useMetadata, type MetadataItem } from '../context/MetadataContext';
import { CatalogPickerModal, EmptyState } from './ui';

export type ToolPickerSelectionMode = 'single' | 'multi';

export interface ToolPickerOptions {
  mode: ToolPickerSelectionMode;
  title?: string;
  preselected?: number[];
  allowedToolIds?: number[];
  /** Optional available stock counts shown on each tool card. */
  stockQuantities?: Record<number, number>;
}

export type ToolPickerResult = number | number[] | null;

interface ToolPickerModalProps {
  isOpen: boolean;
  options: ToolPickerOptions;
  onClose: (result: ToolPickerResult) => void;
}

interface VirtualizedToolGridProps {
  tools: MetadataItem[];
  selectedIds: Set<number>;
  stockQuantities?: Record<number, number>;
  onToolClick: (toolId: number) => void;
}

const TOOL_GRID_GAP_PX = 12;
const TOOL_GRID_ROW_ESTIMATE = 208;

const VirtualizedToolGrid: React.FC<VirtualizedToolGridProps> = ({
  tools,
  selectedIds,
  stockQuantities,
  onToolClick,
}) => {
  const parentRef = useRef<HTMLDivElement>(null);
  const [columns, setColumns] = useState(7);
  const [rowEstimate, setRowEstimate] = useState(TOOL_GRID_ROW_ESTIMATE);

  useEffect(() => {
    const updateGrid = () => {
      if (!parentRef.current) return;
      const styles = window.getComputedStyle(parentRef.current);
      const horizontalPadding = (parseFloat(styles.paddingLeft) || 0) + (parseFloat(styles.paddingRight) || 0);
      const width = Math.max(1, parentRef.current.clientWidth - horizontalPadding);
      let nextColumns = 2;
      if (width >= 1040) nextColumns = 7;
      else if (width >= 880) nextColumns = 6;
      else if (width >= 720) nextColumns = 5;
      else if (width >= 560) nextColumns = 4;
      else if (width >= 400) nextColumns = 3;
      const cardWidth = (width - TOOL_GRID_GAP_PX * Math.max(0, nextColumns - 1)) / nextColumns;
      setColumns(nextColumns);
      setRowEstimate(Math.round(cardWidth * 4 / 3 + TOOL_GRID_GAP_PX));
    };

    updateGrid();
    const observer = new ResizeObserver(updateGrid);
    if (parentRef.current) observer.observe(parentRef.current);
    return () => observer.disconnect();
  }, []);

  const rows = useMemo(() => {
    const result: MetadataItem[][] = [];
    for (let index = 0; index < tools.length; index += columns) {
      result.push(tools.slice(index, index + columns));
    }
    return result;
  }, [columns, tools]);

  const rowVirtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => rowEstimate,
    overscan: 3,
    measureElement: (element) => element?.getBoundingClientRect().height ?? rowEstimate,
  });

  return (
    <div ref={parentRef} className="picker-results-scroll custom-scrollbar">
      <div
        style={{
          height: `${rowVirtualizer.getTotalSize()}px`,
          width: '100%',
          position: 'relative',
        }}
      >
        {rowVirtualizer.getVirtualItems().map((virtualRow) => (
          <div
            key={virtualRow.index}
            ref={rowVirtualizer.measureElement}
            data-index={virtualRow.index}
            style={{
              position: 'absolute',
              top: 0,
              left: 0,
              width: '100%',
              transform: `translateY(${virtualRow.start}px)`,
            }}
          >
            <div className="picker-grid-row grid gap-3" style={{ gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))` }}>
              {rows[virtualRow.index].map((tool) => {
                const isSelected = selectedIds.has(tool.id);
                return (
                  <button
                    key={tool.id}
                    type="button"
                    onClick={() => onToolClick(tool.id)}
                    className={`picker-grid-card text-left ${isSelected ? 'picker-grid-card-selected' : ''}`}
                  >
                    <div className="picker-card-topline">
                      <span className="picker-card-id">#{tool.id}</span>
                      <span className="picker-stock-pill">
                        {stockQuantities?.[tool.id] != null
                          ? stockQuantities[tool.id].toLocaleString()
                          : toolType(tool)}
                      </span>
                    </div>

                    {isSelected && (
                      <div className="picker-grid-actions">
                        <div className="picker-selection-indicator">
                          <Check className="w-4 h-4 text-primary stroke-[3]" />
                        </div>
                      </div>
                    )}

                    <div className="picker-image-stage">
                      <ToolImage toolId={tool.id} size={92} showLevel={true} className="!bg-transparent drop-shadow-md" />
                    </div>

                    <div className="picker-card-body">
                      <span className={`picker-unit-name line-clamp-2 ${isSelected ? 'picker-unit-name-selected' : ''}`}>
                        {tool.name}
                      </span>
                      {stockQuantities?.[tool.id] != null ? (
                        <span className="mt-1 block truncate text-[10px] font-semibold text-text-muted">
                          {toolType(tool)} · available
                        </span>
                      ) : null}
                    </div>
                  </button>
                );
              })}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
};

let resolvePickerPromise: ((value: ToolPickerResult) => void) | null = null;
let setPickerState: React.Dispatch<React.SetStateAction<{ isOpen: boolean; options: ToolPickerOptions | null }>> | null = null;

export function showToolPicker(options: ToolPickerOptions): Promise<ToolPickerResult> {
  return new Promise((resolve) => {
    resolvePickerPromise = resolve;
    if (setPickerState) {
      setPickerState({ isOpen: true, options });
    }
  });
}

export const ToolPickerProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [state, setState] = useState<{ isOpen: boolean; options: ToolPickerOptions | null }>({
    isOpen: false,
    options: null,
  });

  useEffect(() => {
    setPickerState = setState;
    return () => {
      setPickerState = null;
    };
  }, []);

  const handleClose = useCallback((result: ToolPickerResult) => {
    setState({ isOpen: false, options: null });
    if (resolvePickerPromise) {
      resolvePickerPromise(result);
      resolvePickerPromise = null;
    }
  }, []);

  return (
    <>
      {children}
      {state.isOpen && state.options && (
        <ToolPickerModal
          isOpen={state.isOpen}
          options={state.options}
          onClose={handleClose}
        />
      )}
    </>
  );
};

const ToolPickerModal: React.FC<ToolPickerModalProps> = ({ isOpen, options, onClose }) => {
  const { tools, isLoading } = useMetadata();
  const { mode, title, preselected = [], allowedToolIds, stockQuantities } = options;
  const [selectedIds, setSelectedIds] = useState<Set<number>>(new Set(preselected));
  const [searchQuery, setSearchQuery] = useState('');
  const restrictToAllowed = allowedToolIds !== undefined;

  useEffect(() => {
    if (!isOpen) return;
    setSelectedIds(new Set(preselected));
    setSearchQuery('');
  }, [isOpen, preselected]);

  const filteredTools = useMemo(() => {
    let entries = Object.values(tools)
      .filter((tool) => tool.id > 0)
      .filter((tool) => tool.name && tool.name.toLowerCase() !== 'unknown');

    if (restrictToAllowed) {
      const allowed = new Set(allowedToolIds ?? []);
      entries = entries.filter((tool) => allowed.has(tool.id));
    }

    const query = searchQuery.trim().toLowerCase();
    if (query) {
      entries = entries.filter((tool) => {
        const type = toolType(tool).toLowerCase();
        return tool.name.toLowerCase().includes(query) || String(tool.id).includes(query) || type.includes(query);
      });
    }

    return entries.sort((a, b) =>
      toolType(a).localeCompare(toolType(b)) ||
      a.name.localeCompare(b.name) ||
      a.id - b.id
    );
  }, [allowedToolIds, restrictToAllowed, searchQuery, tools]);

  const handleToolClick = (toolId: number) => {
    if (mode === 'single') {
      setSelectedIds(new Set([toolId]));
      return;
    }

    setSelectedIds((current) => {
      const next = new Set(current);
      if (next.has(toolId)) {
        next.delete(toolId);
      } else {
        next.add(toolId);
      }
      return next;
    });
  };

  const handleConfirm = () => {
    const selected = Array.from(selectedIds);
    if (mode === 'single') {
      onClose(selected[0] ?? null);
      return;
    }
    onClose(selected.length > 0 ? selected : null);
  };

  const handleCancel = () => {
    onClose(null);
  };

  if (!isOpen) return null;

  const visibleToolLabel = filteredTools.length === 1 ? 'tool' : 'tools';

  return (
    <CatalogPickerModal
      isOpen={isOpen}
      onClose={handleCancel}
      onConfirm={handleConfirm}
      title={title || (mode === 'single' ? 'Select Tool' : 'Select Tools')}
      modeLabel={mode === 'single' ? 'Single pick' : 'Multi pick'}
      selectedCount={selectedIds.size}
      resultCount={filteredTools.length}
      resultLabel={visibleToolLabel}
      searchValue={searchQuery}
      onSearchChange={setSearchQuery}
      searchPlaceholder="Search by name, type, or ID..."
    >
      {filteredTools.length === 0 ? (
        <div className="flex-1 overflow-y-auto p-6">
          <EmptyState
            surface="plain"
            title={isLoading ? 'Loading tools' : restrictToAllowed ? 'No queueable tools found' : 'No tools found'}
            description={isLoading ? 'Tool metadata is still loading.' : 'Try adjusting your search.'}
            className="picker-empty-state"
          />
        </div>
      ) : (
        <VirtualizedToolGrid
          tools={filteredTools}
          selectedIds={selectedIds}
          stockQuantities={stockQuantities}
          onToolClick={handleToolClick}
        />
      )}
    </CatalogPickerModal>
  );
};

function toolType(tool: MetadataItem): string {
  const raw = tool.type;
  return typeof raw === 'string' && raw.trim() ? raw.trim() : 'Tool';
}

export default ToolPickerModal;
