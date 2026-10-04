import { useLanguage } from '../../hooks/useLanguage';
import { useAuth } from '../../hooks/useAuth';
import LanguageToggle from '../common/LanguageToggle';

interface AppShellProps {
  onOpenSettings: () => void;
  children: React.ReactNode;
}

/**
 * Logged-in chrome around the main content. Settings opens as an overlay; the
 * dashboard beneath it stays mounted so its state survives.
 */
export default function AppShell({ onOpenSettings, children }: AppShellProps) {
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
              {user ? t(`desktop.role.${user.role}`) : ''}
            </span>
            <button type="button" className="muted-button" onClick={onOpenSettings}>
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
