import { useLanguage } from '../hooks/useLanguage';

/**
 * Placeholder rendered until the node dashboard migration (Phase 4) lands.
 * Keeps Phase 2/3 (gateway settings + auth) independently verifiable.
 */
export default function DashboardPage() {
  const { t } = useLanguage();

  return (
    <div className="soft-panel flex items-center justify-center p-12">
      <p className="text-sm text-muted">
        {t('app.title')} — {t('app.fleetOverviewHeading')}
      </p>
    </div>
  );
}
