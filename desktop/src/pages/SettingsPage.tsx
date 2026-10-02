import { useLanguage } from '../hooks/useLanguage';
import GatewaySettings from '../components/settings/GatewaySettings';
import LanguageToggle from '../components/common/LanguageToggle';

export default function SettingsPage() {
  const { t } = useLanguage();

  return (
    <div className="mx-auto w-full max-w-3xl px-6 py-8">
      <div className="soft-panel p-6">
        <p className="eyebrow">{t('app.eyebrow')}</p>
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-ink">
          {t('desktop.settings.title')}
        </h1>
        <p className="mt-1 text-sm text-muted">{t('desktop.settings.subtitle')}</p>

        <div className="mt-6 flex flex-col gap-5">
          <GatewaySettings />

          <div className="soft-panel-subtle p-5">
            <h3 className="text-sm font-semibold text-ink">{t('desktop.settings.languageSection')}</h3>
            <p className="mt-1 text-xs text-muted">{t('desktop.settings.languageHint')}</p>
            <div className="mt-3">
              <LanguageToggle />
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
