import { useLanguage } from '../../hooks/useLanguage';
import { useAuth } from '../../hooks/useAuth';
import LanguageToggle from '../common/LanguageToggle';

export type AppView = 'dashboard' | 'settings';

interface AppShellProps {
  view: AppView;
  onNavigate: (view: AppView) => void;
  children: React.ReactNode;
}

/**
 * Logged-in chrome around the main content. The dashboard KPI tiles and
 * admin actions are rendered by the pages themselves in later phases.
 */
export default function AppShell({ view, onNavigate, children }: AppShellProps) {
  const { t } = useLanguage();
  const { user, logout } = useAuth();

  return (
    <div className="flex min-h-screen flex-col">
      <header className="mx-auto w-full max-w-[1600px] px-6 pt-6">
        <div className="soft-panel flex flex-wrap items-center gap-4 p-5">
          <div className="min-w-0 flex-1">
            <p className="eyebrow">{t('app.eyebrow')}</p>
            <h1 className="mt-1 truncate text-2xl font-semibold tracking-tight text-ink">
              {t('app.title')}
              <span className="ml-2 text-base font-normal text-muted">
                / {t('app.fleetOverviewHeading')}
              </span>
            </h1>
            <p className="mt-1 text-sm text-muted">{t('app.dashboardSubtitle')}</p>
          </div>

          <div className="flex flex-wrap items-center gap-2">
            <span className="rounded-full border border-line bg-panel-soft px-3 py-1.5 text-xs font-medium text-muted">
              {user?.role ?? ''}
            </span>
            <button
              type="button"
              className={`muted-button ${view === 'settings' ? 'border-accent text-accent-strong' : ''}`}
              onClick={() => onNavigate(view === 'settings' ? 'dashboard' : 'settings')}
            >
              {t('desktop.settings.title')}
            </button>
            <LanguageToggle />
            <button type="button" className="primary-button" onClick={() => void logout()}>
              {t('action.logout')}
            </button>
          </div>
        </div>
      </header>

      <main className="mx-auto w-full max-w-[1600px] flex-1 px-6 py-6">{children}</main>
    </div>
  );
}
