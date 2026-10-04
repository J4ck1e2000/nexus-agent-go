import { useLanguage } from '../../hooks/useLanguage';
import SortSelect from './SortSelect';
import { SORT_MODES, type SortMode } from '../../lib/node-logic';

interface NodeListToolbarProps {
  query: string;
  onQueryChange: (query: string) => void;
  sortMode: SortMode;
  onSortModeChange: (mode: SortMode) => void;
  onClear: () => void;
}

export default function NodeListToolbar({
  query,
  onQueryChange,
  sortMode,
  onSortModeChange,
  onClear,
}: NodeListToolbarProps) {
  const { t } = useLanguage();

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <svg
            className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-[#a39a8f]"
            width="14"
            height="14"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
            aria-hidden="true"
          >
            <circle cx="11" cy="11" r="8" />
            <path d="m21 21-4.3-4.3" />
          </svg>
          <input
            type="text"
            className="input-field pl-9 pr-8"
            value={query}
            placeholder={t('panel.searchPlaceholder')}
            onChange={(event) => onQueryChange(event.target.value)}
            onKeyDown={(event) => {
              if (event.key === 'Escape' && query) {
                event.preventDefault();
                onClear();
              }
            }}
            aria-label={t('panel.searchPlaceholder')}
          />
          {query && (
            <button
              type="button"
              className="absolute right-2.5 top-1/2 -translate-y-1/2 cursor-pointer rounded-full p-1 text-[#a39a8f] transition hover:bg-[#f1ece3] hover:text-ink"
              onClick={onClear}
              aria-label={t('action.clearFilter')}
              title={t('action.clearFilter')}
            >
              <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                <path d="M18 6 6 18" />
                <path d="m6 6 12 12" />
              </svg>
            </button>
          )}
        </div>
        <SortSelect value={sortMode} onChange={onSortModeChange} />
      </div>

      <div className="flex items-center justify-between text-[11px] text-muted">
        <span>
          {sortMode === 'availability' ? t('panel.sortedByAvailability') : t('panel.bestNodesFirst')} ·{' '}
          {t(SORT_MODES.find((mode) => mode.value === sortMode)?.labelKey ?? 'panel.sortAvailability')}
        </span>
        {query && (
          <button
            type="button"
            className="cursor-pointer font-medium text-accent-strong transition hover:text-ink"
            onClick={onClear}
          >
            {t('action.clearFilter')}
          </button>
        )}
      </div>
    </div>
  );
}
