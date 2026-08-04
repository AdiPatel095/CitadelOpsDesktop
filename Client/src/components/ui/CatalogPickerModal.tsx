import React, { type ReactNode } from 'react';
import { Check, Search } from 'lucide-react';
import { Badge } from './Badge';
import { Button } from './Button';
import { Input } from './Input';
import { Modal, type ModalProps } from './Modal';

export interface CatalogPickerModalProps {
  isOpen: boolean;
  onClose: () => void;
  onConfirm: () => void;
  title: ReactNode;
  modeLabel: ReactNode;
  selectedCount: number;
  resultCount: number;
  resultLabel: ReactNode;
  searchValue: string;
  onSearchChange: (value: string) => void;
  searchPlaceholder: string;
  children: ReactNode;
  commandExtras?: ReactNode;
  filterDock?: ReactNode;
  shellClassName?: string;
  toolbarClassName?: string;
  commandRowClassName?: string;
  confirmLabel?: string;
  maxWidth?: ModalProps['maxWidth'];
}

export const CatalogPickerModal: React.FC<CatalogPickerModalProps> = ({
  isOpen,
  onClose,
  onConfirm,
  title,
  modeLabel,
  selectedCount,
  resultCount,
  resultLabel,
  searchValue,
  onSearchChange,
  searchPlaceholder,
  children,
  commandExtras,
  filterDock,
  shellClassName = '',
  toolbarClassName = '',
  commandRowClassName = 'picker-command-row',
  confirmLabel = 'Confirm Selection',
  maxWidth = '6xl',
}) => (
  <Modal
    isOpen={isOpen}
    onClose={onClose}
    maxWidth={maxWidth}
    title={(
      <div className="picker-modal-title">
        <span className="picker-modal-title-mark" aria-hidden="true" />
        <span className="picker-modal-title-text">{title}</span>
        {selectedCount > 0 && <Badge variant="primary" className="ml-2">{selectedCount} selected</Badge>}
      </div>
    )}
    footer={(
      <>
        <Button variant="ghost" onClick={onClose} className="px-8">Cancel</Button>
        <Button
          variant="primary"
          onClick={onConfirm}
          disabled={selectedCount === 0}
          className="px-10"
          leftIcon={<Check className="h-4 w-4" />}
        >
          {confirmLabel}
        </Button>
      </>
    )}
  >
    <div className={`ui-workspace-surface picker-shell ${shellClassName}`}>
      <div className={`picker-toolbar ${toolbarClassName}`}>
        <div className="picker-toolbar-overview">
          <div className="picker-toolbar-copy">
            <span className="ui-kicker">{modeLabel}</span>
            <span className="picker-toolbar-count">{resultCount.toLocaleString()} {resultLabel}</span>
          </div>
          <div className="picker-selection-summary">
            <Check className="h-3.5 w-3.5" />
            <span>{selectedCount.toLocaleString()}</span>
            selected
          </div>
        </div>
        <div className={commandRowClassName}>
          <div className="picker-search-slot">
            <Input
              type="text"
              placeholder={searchPlaceholder}
              value={searchValue}
              onChange={(event) => onSearchChange(event.target.value)}
              aria-label={searchPlaceholder}
              leftIcon={<Search className="h-4 w-4" />}
            />
          </div>
          {commandExtras}
        </div>
        {filterDock}
      </div>
      {children}
    </div>
  </Modal>
);
