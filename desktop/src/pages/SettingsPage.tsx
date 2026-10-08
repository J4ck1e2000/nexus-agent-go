import { useLanguage } from '../hooks/useLanguage';
import GatewaySettings from '../components/settings/GatewaySettings';
import LanguageToggle from '../components/common/LanguageToggle';
import AppModal from '../components/common/AppModal';
import NotificationPreferencesPanel from '../components/settings/NotificationPreferencesPanel';

/**
 * Settings rendered as an overlay above the dashboard so the dashboard (node
 * selection, AI assistant session, polling) stays mounted while it is open.
 */
export default function SettingsPage({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { t } = useLanguage();

  return (
    <AppModal open={open} onClose={onClose} title={t('desktop.settings.title')} subtitle={t('desktop.settings.subtitle')} maxWidth="max-w-2xl">
      <div className="flex flex-col gap-5">
        <GatewaySettings />

        <NotificationPreferencesPanel />

        <div className="soft-panel-subtle p-5">
          <h3 className="text-sm font-semibold text-ink">{t('desktop.settings.languageSection')}</h3>
          <p className="mt-1 text-xs text-muted">{t('desktop.settings.languageHint')}</p>
          <div className="mt-3">
            <LanguageToggle />
          </div>
        </div>
      </div>
    </AppModal>
  );
}
