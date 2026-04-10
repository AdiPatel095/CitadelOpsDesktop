import React, { useState, useEffect, useRef } from 'react';
import { createPortal } from 'react-dom';
import { ChevronDown, CheckSquare } from 'lucide-react';

export interface SelectOption {
  value: string;
  label: React.ReactNode;
}

export interface SelectProps {
  value: string;
  options: SelectOption[];
  onChange: (value: string) => void;
  placeholder?: React.ReactNode;
  icon?: React.ReactNode;
  className?: string;
  disabled?: boolean;
}

export const Select: React.FC<SelectProps> = ({
  value,
  options,
  onChange,
  placeholder,
  icon,
  className = '',
  disabled = false,
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const containerRef = useRef<HTMLDivElement>(null);
  const [dropdownPos, setDropdownPos] = useState({ top: 0, left: 0, width: 0 });

  const selectedOption = options.find((o) => o.value === value);

  const toggleDropdown = () => {
    if (disabled) return;
    if (!isOpen && containerRef.current) {
      const rect = containerRef.current.getBoundingClientRect();
      setDropdownPos({
        top: rect.bottom + window.scrollY,
        left: rect.left + window.scrollX,
        width: rect.width,
      });
    }
    setIsOpen(!isOpen);
  };

  useEffect(() => {
    const handleDocumentClick = (event: MouseEvent) => {
      const target = event.target as Node;
      const isClickInsideContainer = containerRef.current?.contains(target);
      const isClickInsidePortal = (target as Element).closest?.('.select-portal-content');

      if (!isClickInsideContainer && !isClickInsidePortal) {
        setIsOpen(false);
      }
    };

    document.addEventListener('mousedown', handleDocumentClick);
    return () => document.removeEventListener('mousedown', handleDocumentClick);
  }, []);

  useEffect(() => {
    if (isOpen) {
      const handleScroll = (e: Event) => {
        if ((e.target as Element)?.closest?.('.select-portal-content')) return;
        setIsOpen(false);
      };
      window.addEventListener('scroll', handleScroll, true);
      window.addEventListener('resize', () => setIsOpen(false));
      return () => {
        window.removeEventListener('scroll', handleScroll, true);
        window.removeEventListener('resize', () => setIsOpen(false));
      };
    }
  }, [isOpen]);

  return (
    <div className={`relative ${className}`} ref={containerRef}>
      <button
        type="button"
        onClick={toggleDropdown}
        disabled={disabled}
        className={`w-full bg-bg-input border px-4 py-2.5 text-sm font-medium text-text-main transition-colors duration-200 rounded-global flex items-center justify-between group focus:outline-none focus:ring-1 focus:ring-primary ${
          disabled
            ? 'opacity-50 cursor-not-allowed border-border-base'
            : 'border-border-base hover:border-primary focus:border-primary cursor-pointer'
        }`}
      >
        <div className="flex items-center gap-2 overflow-hidden">
          {icon && (
            <span className="text-text-muted group-hover:text-primary transition-colors shrink-0">
              {icon}
            </span>
          )}
          <span className="truncate">
            {selectedOption ? selectedOption.label : placeholder}
          </span>
        </div>
        <ChevronDown
          className={`w-4 h-4 text-text-muted transition-transform duration-300 shrink-0 ${
            isOpen ? 'rotate-180 text-primary' : ''
          }`}
        />
      </button>

      {isOpen &&
        createPortal(
          <div
            className="select-portal-content fixed z-[100] mt-1 bg-bg-card border border-border-base rounded-2xl shadow-xl overflow-hidden animate-fade-in custom-scrollbar overflow-y-auto"
            style={{
              top: dropdownPos.top,
              left: dropdownPos.left,
              width: dropdownPos.width,
              maxHeight: '260px',
            }}
          >
            <div className="py-1">
              {options.length === 0 ? (
                <div className="px-4 py-3 text-sm text-text-muted italic text-center">
                  No options available
                </div>
              ) : (
                options.map((opt) => (
                  <button
                    type="button"
                    key={opt.value}
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      onChange(opt.value);
                      setIsOpen(false);
                    }}
                    className={`w-full text-left px-4 py-2.5 text-sm transition-colors flex items-center justify-between group ${
                      value === opt.value
                        ? 'bg-primary/10 text-primary font-bold'
                        : 'text-text-main hover:bg-bg-card-hover hover:text-primary'
                    }`}
                  >
                    <span className="truncate">{opt.label}</span>
                    {value === opt.value && <CheckSquare className="w-4 h-4" />}
                  </button>
                ))
              )}
            </div>
          </div>,
          document.body
        )}
    </div>
  );
};
