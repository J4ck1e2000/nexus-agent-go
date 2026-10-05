import type { NotificationPreferences } from '../../lib/notification-preferences';
import { useState } from 'react';
import { useNotificationPreferences } from '../../context/NotificationPreferencesContext';
import { useLanguage } from '../../hooks/useLanguage';
import { localizedError } from '../../lib/errors';

export default function NotificationPreferencesPanel() {
  const { t } = useLanguage();
  const { preferences, update } = useNotificationPreferences();
  const [testMessage, setTestMessage] = useState('');

  const toggle = (key: keyof NotificationPreferences, value: boolean) => update({ [key]: value });
  const range = (key: 'temperatureThresholdCelsius' | 'idleAfterMinutes' | 'sustainedLoadMinutes' | 'releasedVramGb', value: number) => update({ [key]: value });

  return (
    <section className="soft-panel-subtle p-5">
      <h3 className="text-sm font-semibold text-ink">{t('alerts.settingsTitle')}</h3>
      <p className="mt-1 text-xs text-muted">{t('alerts.settingsLocalHint')}</p>
      <div className="mt-3 flex items-center gap-3">
        <button type="button" className="muted-button" onClick={() => {
          void window.nexus.notifications.show(t('alerts.testTitle'), t('alerts.testBody')).then((result) => {
            setTestMessage(result.ok ? t('alerts.testSent') : localizedError(result.error, t));
          });
        }}>{t('alerts.testNotification')}</button>
        {testMessage && <span className="text-xs text-muted" role="status">{testMessage}</span>}
      </div>
      <div className="mt-4 flex flex-col gap-3">
        <Toggle label={t('alerts.enabled')} checked={preferences.enabled} onChange={(value) => toggle('enabled', value)} />
        {preferences.enabled && <>
          <Toggle label={t('alerts.nodeOffline')} checked={preferences.nodeOffline} onChange={(value) => toggle('nodeOffline', value)} />
          <Toggle label={t('alerts.gpuTemperature')} checked={preferences.gpuTemperature} onChange={(value) => toggle('gpuTemperature', value)} />
          {preferences.gpuTemperature && <Range label={t('alerts.temperatureThreshold')} value={preferences.temperatureThresholdCelsius} min={60} max={100} suffix="°C" onChange={(value) => range('temperatureThresholdCelsius', value)} />}
          <Toggle label={t('alerts.idleGpu')} checked={preferences.idleGpu} onChange={(value) => toggle('idleGpu', value)} />
          {preferences.idleGpu && <Range label={t('alerts.idleAfter')} value={preferences.idleAfterMinutes} min={1} max={240} suffix={t('alerts.minutesSuffix')} onChange={(value) => range('idleAfterMinutes', value)} />}
          <Toggle label={t('alerts.sustainedLoad')} checked={preferences.sustainedGpuLoad} onChange={(value) => toggle('sustainedGpuLoad', value)} />
          {preferences.sustainedGpuLoad && <Range label={t('alerts.sustainedDuration')} value={preferences.sustainedLoadMinutes} min={5} max={240} suffix={t('alerts.minutesSuffix')} onChange={(value) => range('sustainedLoadMinutes', value)} />}
          <Toggle label={t('alerts.memoryReleased')} checked={preferences.memoryReleased} onChange={(value) => toggle('memoryReleased', value)} />
          {preferences.memoryReleased && <Range label={t('alerts.releasedVram')} value={preferences.releasedVramGb} min={1} max={256} suffix="GB" onChange={(value) => range('releasedVramGb', value)} />}
          <Toggle label={t('alerts.processEnded')} checked={preferences.processEnded} onChange={(value) => toggle('processEnded', value)} />
        </>}
      </div>
    </section>
  );
}

function Toggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return <label className="flex items-center justify-between gap-3 text-xs text-muted"><span>{label}</span><input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} /></label>;
}

function Range({ label, value, min, max, suffix, onChange }: { label: string; value: number; min: number; max: number; suffix: string; onChange: (value: number) => void }) {
  return <label className="flex items-center gap-3 text-xs text-muted"><span className="min-w-36">{label}</span><input className="min-w-0 flex-1" type="range" min={min} max={max} value={value} onChange={(event) => onChange(Number(event.target.value))} /><span className="w-14 text-right tabular-nums text-ink">{value} {suffix}</span></label>;
}
