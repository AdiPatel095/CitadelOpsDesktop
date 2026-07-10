import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Check, Search } from 'lucide-react';
import ToolImage from './ToolImage';
import { useMetadata, type MetadataItem } from '../context/MetadataContext';
import { Badge, Button, Input, Modal } from './ui';

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
    <Modal
      isOpen={isOpen}
      onClose={handleCancel}
      maxWidth="6xl"
      title={
        <div className="picker-modal-title">
          <span className="picker-modal-title-mark" aria-hidden="true" />
          <span className="picker-modal-title-text">
            {title || (mode === 'single' ? 'Select Tool' : 'Select Tools')}
          </span>
          {mode === 'multi' && selectedIds.size > 0 && (
            <Badge variant="primary" className="ml-2">
              {selectedIds.size} selected
            </Badge>
          )}
        </div>
      }
      footer={
        <>
          <Button variant="ghost" onClick={handleCancel} className="px-8">
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleConfirm}
            disabled={selectedIds.size === 0}
            className="px-10"
            leftIcon={<Check className="w-4 h-4" />}
          >
            Confirm Selection
          </Button>
        </>
      }
    >
      <div className="picker-shell">
        <div className="picker-toolbar">
          <div className="picker-toolbar-overview">
            <div className="picker-toolbar-copy">
              <span className="picker-toolbar-kicker">
                {mode === 'single' ? 'Single pick' : 'Multi pick'}
              </span>
              <span className="picker-toolbar-count">
                {filteredTools.length.toLocaleString()} {visibleToolLabel}
              </span>
            </div>
            <div className="picker-selection-summary">
              <Check className="w-3.5 h-3.5" />
              <span>{selectedIds.size.toLocaleString()}</span>
              selected
            </div>
          </div>

          <div className="picker-command-row">
            <div className="picker-search-slot">
              <Input
                type="text"
                placeholder="Search by name, type, or ID..."
                value={searchQuery}
                onChange={(event) => setSearchQuery(event.target.value)}
                leftIcon={<Search className="w-4 h-4" />}
              />
            </div>
          </div>
        </div>

        {filteredTools.length === 0 ? (
          <div className="flex-1 overflow-y-auto p-6">
            <div className="picker-empty-state">
              <p className="text-lg font-medium">
                {isLoading ? 'Loading tools' : restrictToAllowed ? 'No queueable tools found' : 'No tools found'}
              </p>
              <p className="mt-2 text-sm">
                {isLoading ? 'Tool metadata is still loading.' : 'Try adjusting your search.'}
              </p>
            </div>
          </div>
        ) : (
          <div className="picker-results-scroll custom-scrollbar">
            <div className="picker-grid-row grid gap-3 p-4" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(8.5rem, 1fr))' }}>
              {filteredTools.map((tool) => {
                const isSelected = selectedIds.has(tool.id);
                return (
                  <button
                    key={tool.id}
                    type="button"
                    onClick={() => handleToolClick(tool.id)}
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
        )}
      </div>
    </Modal>
  );
};

function toolType(tool: MetadataItem): string {
  const raw = tool.type;
  return typeof raw === 'string' && raw.trim() ? raw.trim() : 'Tool';
}

export default ToolPickerModal;
