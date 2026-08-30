import React, { useState, useEffect, useId, useMemo, useRef } from 'react';
import { createPortal } from 'react-dom';
import { ChevronDown, CheckSquare, Search } from 'lucide-react';

export interface SelectOption {
  value: string;
  label: React.ReactNode;
  searchText?: string;
}

export interface SelectProps {
  value: string;
  options: SelectOption[];
  onChange: (value: string) => void;
  placeholder?: React.ReactNode;
  icon?: React.ReactNode;
  className?: string;
  disabled?: boolean;
  searchable?: boolean;
  searchPlaceholder?: string;
  ariaLabel?: string;
  closeOnScroll?: boolean;
  /**
   * When true, dropdown max-height follows space below the control (viewport),
   * so short lists show a compact panel and long lists use available height before scrolling.
   * When false or omitted, the menu uses a fixed height cap.
   */
  menuGrowToViewport?: boolean;
}

export const Select: React.FC<SelectProps> = ({
  value,
  options,
  onChange,
  placeholder,
  icon,
  className = '',
  disabled = false,
  searchable = false,
  searchPlaceholder = 'Filter options',
  ariaLabel,
  closeOnScroll = true,
  menuGrowToViewport = false,
}) => {
  const [isOpen, setIsOpen] = useState(false);
  const menuID = useId();
  const [searchQuery, setSearchQuery] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const [dropdownPos, setDropdownPos] = useState({ top: 0, left: 0, width: 0, maxHeightPx: 260 });

  const selectedOption = options.find((o) => o.value === value);
  const filteredOptions = useMemo(() => {
    if (!searchable) return options;

    const query = searchQuery.trim().toLowerCase();
    if (!query) return options;

    return options.filter((option) => {
      const searchableText = option.searchText
        ?? (typeof option.label === 'string' ? option.label : option.value);
      return searchableText.toLowerCase().includes(query);
    });
  }, [options, searchQuery, searchable]);

  const toggleDropdown = () => {
    if (disabled) return;
    if (isOpen) {
      setIsOpen(false);
      setSearchQuery('');
      return;
    }
    if (!isOpen && containerRef.current) {
      const rect = containerRef.current.getBoundingClientRect();
      const spaceBelow = window.innerHeight - rect.bottom - 12;
      const maxHeightPx = menuGrowToViewport ? Math.max(120, spaceBelow) : 260;
      setDropdownPos({
        top: rect.bottom + window.scrollY,
        left: rect.left + window.scrollX,
        width: rect.width,
        maxHeightPx,
      });
    }
    setIsOpen(true);
  };

  useEffect(() => {
    if (!isOpen || !searchable) return;
    const animationFrame = window.requestAnimationFrame(() => searchInputRef.current?.focus());
    return () => window.cancelAnimationFrame(animationFrame);
  }, [isOpen, searchable]);

  useEffect(() => {
    const handleDocumentClick = (event: MouseEvent) => {
      const target = event.target as Node;
      const isClickInsideContainer = containerRef.current?.contains(target);
      const isClickInsidePortal = (target as Element).closest?.('.select-portal-content');

      if (!isClickInsideContainer && !isClickInsidePortal) {
        setIsOpen(false);
        setSearchQuery('');
      }
    };

    document.addEventListener('mousedown', handleDocumentClick);
    return () => document.removeEventListener('mousedown', handleDocumentClick);
  }, []);

  useEffect(() => {
    if (!isOpen) return;
    const handleScroll = (e: Event) => {
      if ((e.target as Element)?.closest?.('.select-portal-content')) return;
      setIsOpen(false);
    };
    const handleResize = () => setIsOpen(false);
    if (closeOnScroll) {
      window.addEventListener('scroll', handleScroll, true);
    }
    window.addEventListener('resize', handleResize);
    return () => {
      if (closeOnScroll) {
        window.removeEventListener('scroll', handleScroll, true);
      }
      window.removeEventListener('resize', handleResize);
    };
  }, [closeOnScroll, isOpen]);

  return (
    <div className={`relative ${className}`} ref={containerRef}>
      <button
        type="button"
        aria-label={ariaLabel}
        aria-controls={menuID}
        aria-expanded={isOpen}
        aria-haspopup="listbox"
        role="combobox"
        onClick={toggleDropdown}
        onKeyDown={(event) => {
          if (event.key === 'Escape' && isOpen) {
            event.preventDefault();
            setIsOpen(false);
            setSearchQuery('');
          } else if (event.key === 'ArrowDown' && !isOpen) {
            event.preventDefault();
            toggleDropdown();
          }
        }}
        disabled={disabled}
        className={`m3-select-trigger w-full border px-4 py-2.5 text-sm font-medium text-text-main transition-colors duration-200 flex items-center justify-between group focus:outline-none ${
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
            id={menuID}
            role="listbox"
            className="m3-select-menu select-portal-content fixed z-[140] mt-1 overflow-hidden animate-fade-in custom-scrollbar overflow-y-auto"
            style={{
              top: dropdownPos.top,
              left: dropdownPos.left,
              width: dropdownPos.width,
              maxHeight: `${dropdownPos.maxHeightPx}px`,
            }}
          >
            {searchable && (
              <div className="sticky top-0 z-10 border-b border-border-base bg-bg-card p-2">
                <div className="relative flex items-center">
                  <Search className="pointer-events-none absolute left-3 h-4 w-4 text-text-muted" />
                  <input
                    ref={searchInputRef}
                    type="search"
                    value={searchQuery}
                    onChange={(event) => setSearchQuery(event.target.value)}
                    onKeyDown={(event) => {
                      if (event.key !== 'Escape') return;
                      event.preventDefault();
                      event.stopPropagation();
                      setIsOpen(false);
                      setSearchQuery('');
                    }}
                    placeholder={searchPlaceholder}
                    aria-label={searchPlaceholder}
                    className="m3-input w-full border py-2 pl-9 pr-3 text-sm text-text-main outline-none placeholder:text-text-muted"
                  />
                </div>
              </div>
            )}
            <div className="py-1">
              {filteredOptions.length === 0 ? (
                <div className="px-4 py-3 text-sm text-text-muted italic text-center">
                  {searchQuery.trim() ? 'No matching options' : 'No options available'}
                </div>
              ) : (
                filteredOptions.map((opt) => (
                  <button
                    type="button"
                    role="option"
                    aria-selected={value === opt.value}
                    key={opt.value}
                    onClick={(e) => {
                      e.preventDefault();
                      e.stopPropagation();
                      onChange(opt.value);
                      setIsOpen(false);
                      setSearchQuery('');
                    }}
                    className={`m3-select-option w-full text-left px-4 py-2.5 text-sm transition-colors flex items-center justify-between group ${
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
