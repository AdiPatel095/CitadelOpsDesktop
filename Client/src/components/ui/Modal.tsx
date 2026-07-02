import React, { useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { Button } from './Button';

export interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  children: React.ReactNode;
  footer?: React.ReactNode;
  maxWidth?: 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | '4xl' | '5xl' | '6xl' | 'full';
  hideCloseButton?: boolean;
}

export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  title,
  children,
  footer,
  maxWidth = 'lg',
  hideCloseButton = false,
}) => {
  const overlayRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (isOpen) {
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = '';
    }
    return () => {
      document.body.style.overflow = '';
    };
  }, [isOpen]);

  if (!isOpen) return null;

  const maxWidthClasses = {
    sm: 'max-w-sm',
    md: 'max-w-md',
    lg: 'max-w-lg',
    xl: 'max-w-xl',
    '2xl': 'max-w-2xl',
    '3xl': 'max-w-3xl',
    '4xl': 'max-w-4xl',
    '5xl': 'max-w-5xl',
    '6xl': 'max-w-6xl',
    full: 'max-w-[min(1900px,98vw)] mx-auto',
  };

  return createPortal(
    <div className="liquid-modal-overlay animate-fade-in">
      <div
        ref={overlayRef}
        className="absolute inset-0"
        onClick={onClose}
        aria-hidden="true"
      />
      <div
        className={`liquid-modal-surface ${maxWidthClasses[maxWidth]}`}
        role="dialog"
        aria-modal="true"
      >
        {(title || !hideCloseButton) && (
          <div className="liquid-modal-header">
            {title && (
              <h2 className="liquid-modal-title">
                {title}
              </h2>
            )}
            {!hideCloseButton && (
              <Button
                variant="ghost"
                size="icon"
                onClick={onClose}
                className="liquid-modal-close"
                aria-label="Close modal"
              >
                <X className="w-5 h-5" />
              </Button>
            )}
          </div>
        )}
        <div className="liquid-modal-body custom-scrollbar">
          {children}
        </div>
        {footer && (
          <div className="liquid-modal-footer">
            {footer}
          </div>
        )}
      </div>
    </div>,
    document.body
  );
};
