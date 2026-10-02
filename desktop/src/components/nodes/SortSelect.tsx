import { useEffect, useRef, useState } from 'react';
import { useLanguage } from '../../hooks/useLanguage';
import { SORT_MODES, type SortMode } from '../../lib/node-logic';

interface SortSelectProps {
  value: SortMode;
  onChange: (mode: SortMode) => void;
}

/** Accessible listbox for sort modes (button + popup, keyboard + outside click). */
export default function SortSelect({ value, onChange }: SortSelectProps) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  const [highlight, setHighlight] = useState(0);
  const rootRef = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent): void => {
      if (rootRef.current && !rootRef.current.contains(event.target as Node)) {
        setOpen(false);
      }
    };
    document.addEventListener('pointerdown', onPointerDown);
    return () => document.removeEventListener('pointerdown', onPointerDown);
  }, [open]);

  const select = (mode: SortMode): void => {
    onChange(mode);
    setOpen(false);
  };

  const onKeyDown = (event: React.KeyboardEvent): void => {
    if (!open) {
      if (event.key === 'Enter' || event.key === ' ' || event.key === 'ArrowDown') {
        event.preventDefault();
        setHighlight(SORT_MODES.findIndex((mode) => mode.value === value));
        setOpen(true);
      }
      return;
    }
    switch (event.key) {
      case 'Escape':
        event.preventDefault();
        setOpen(false);
        break;
      case 'ArrowDown':
        event.preventDefault();
        setHighlight((index) => (index + 1) % SORT_MODES.length);
        break;
      case 'ArrowUp':
        event.preventDefault();
        setHighlight((index) => (index - 1 + SORT_MODES.length) % SORT_MODES.length);
        break;
      case 'Enter':
      case ' ':
        event.preventDefault();
        select(SORT_MODES[highlight].value);
        break;
      case 'Tab':
        setOpen(false);
        break;
      default:
        break;
    }
  };

  const current = SORT_MODES.find((mode) => mode.value === value) ?? SORT_MODES[0];

  return (
    <div className="relative" ref={rootRef} onKeyDown={onKeyDown}>
      <button
        type="button"
        className="muted-button w-full justify-between"
        aria-haspopup="listbox"
        aria-expanded={open}
        aria-label={t('panel.sortLabel')}
        onClick={() => setOpen((v) => !v)}
      >
        <span>{t(current.labelKey)}</span>
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
          <path d="m6 9 6 6 6-6" />
        </svg>
      </button>

      {open && (
        <ul
          role="listbox"
          aria-label={t('panel.sortLabel')}
          className="absolute right-0 z-30 mt-1 w-full overflow-hidden rounded-2xl border border-line bg-panel-soft py-1 shadow-soft"
        >
          {SORT_MODES.map((mode, index) => (
            <li key={mode.value} role="option" aria-selected={mode.value === value}>
              <button
                type="button"
                className={`w-full px-4 py-2 text-left text-xs transition ${
                  index === highlight ? 'bg-[#f1ece3]' : ''
                } ${mode.value === value ? 'font-semibold text-ink' : 'text-muted'}`}
                onMouseEnter={() => setHighlight(index)}
                onClick={() => select(mode.value)}
              >
                {t(mode.labelKey)}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
