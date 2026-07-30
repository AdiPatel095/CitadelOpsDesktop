import React, { useEffect, useId, useRef } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { Button } from './Button';

const FOCUSABLE = 'button:not(:disabled), [href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])';
const modalStack: symbol[] = [];
let bodyLockCount = 0;
let previousBodyOverflow = '';

export interface ModalProps {
  isOpen: boolean;
  onClose: () => void;
  title?: React.ReactNode;
  children: React.ReactNode;
  footer?: React.ReactNode;
  maxWidth?: 'sm' | 'md' | 'lg' | 'xl' | '2xl' | '3xl' | '4xl' | '5xl' | '6xl' | 'full';
  hideCloseButton?: boolean;
  ariaLabel?: string;
}

export const Modal: React.FC<ModalProps> = ({
  isOpen,
  onClose,
  title,
  children,
  footer,
  maxWidth = 'lg',
  hideCloseButton = false,
  ariaLabel,
}) => {
  const dialogRef = useRef<HTMLDivElement>(null);
  const instanceRef = useRef(Symbol('modal'));
  const onCloseRef = useRef(onClose);
  const titleID = useId();
  onCloseRef.current = onClose;

  useEffect(() => {
    if (!isOpen) return;
    const instance = instanceRef.current;
    const previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    modalStack.push(instance);
    if (bodyLockCount === 0) previousBodyOverflow = document.body.style.overflow;
    bodyLockCount += 1;
    document.body.style.overflow = 'hidden';

    const focusFrame = window.requestAnimationFrame(() => {
      const dialog = dialogRef.current;
      const firstFocusable = dialog?.querySelector<HTMLElement>(FOCUSABLE);
      (firstFocusable ?? dialog)?.focus();
    });

    const handleKeyDown = (event: KeyboardEvent) => {
      if (modalStack.at(-1) !== instance) return;
      if (event.key === 'Escape') {
        event.preventDefault();
        onCloseRef.current();
        return;
      }
      if (event.key !== 'Tab') return;
      const dialog = dialogRef.current;
      if (!dialog) return;
      const focusable = Array.from(dialog.querySelectorAll<HTMLElement>(FOCUSABLE));
      if (focusable.length === 0) {
        event.preventDefault();
        dialog.focus();
        return;
      }
      const first = focusable[0];
      const last = focusable[focusable.length - 1];
      if (event.shiftKey && (document.activeElement === first || !dialog.contains(document.activeElement))) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener('keydown', handleKeyDown);
    return () => {
      window.cancelAnimationFrame(focusFrame);
      document.removeEventListener('keydown', handleKeyDown);
      const stackIndex = modalStack.lastIndexOf(instance);
      if (stackIndex >= 0) modalStack.splice(stackIndex, 1);
      bodyLockCount = Math.max(0, bodyLockCount - 1);
      if (bodyLockCount === 0) document.body.style.overflow = previousBodyOverflow;
      if (previouslyFocused?.isConnected) previouslyFocused.focus();
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
      <div className="absolute inset-0" onClick={onClose} aria-hidden="true" />
      <div
        ref={dialogRef}
        className={`liquid-modal-surface ${maxWidthClasses[maxWidth]}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? titleID : undefined}
        aria-label={!title ? ariaLabel ?? 'Dialog' : undefined}
        tabIndex={-1}
      >
        {(title || !hideCloseButton) && (
          <div className="liquid-modal-header">
            {title && <h2 id={titleID} className="liquid-modal-title">{title}</h2>}
            {!hideCloseButton && (
              <Button
                variant="ghost"
                size="icon"
                onClick={onClose}
                className="liquid-modal-close"
                aria-label="Close modal"
              >
                <X className="h-5 w-5" />
              </Button>
            )}
          </div>
        )}
        <div className="liquid-modal-body custom-scrollbar">{children}</div>
        {footer && <div className="liquid-modal-footer">{footer}</div>}
      </div>
    </div>,
    document.body,
  );
};
