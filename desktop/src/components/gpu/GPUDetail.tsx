import type { GpuInfo } from '../../../electron/types/ipc';
import { gpuVramPercent } from '../../lib/node-logic';
import {
  formatFanSpeed,
  formatGbs,
  formatPercent,
  formatPower,
  formatTemperature,
} from '../../lib/format';
import { useLanguage } from '../../hooks/useLanguage';

/** Inline GPU detail panel shown next to the matrix (web parity: no modal). */
export default function GPUDetail({
  gpu,
  gpuIndex,
  nodeName,
  onClose,
}: {
  gpu: GpuInfo;
  gpuIndex: number;
  nodeName: string;
  onClose: () => void;
}) {
  const { t } = useLanguage();
  const vramPercent = gpuVramPercent(gpu);

  return (
    <div className="soft-panel-subtle flex h-full flex-col p-5">
      <div className="flex items-start justify-between gap-2">
        <div className="min-w-0">
          <p className="eyebrow">{t('gpu.modal.eyebrow')}</p>
          <h3 className="mt-1 truncate text-base font-semibold text-ink">{gpu.name}</h3>
          <p className="mt-0.5 font-mono text-[11px] text-muted">
            {nodeName} · {t('gpu.modal.index')} {gpuIndex}
          </p>
        </div>
        <button
          type="button"
          className="muted-button px-3 py-1.5 text-xs"
          onClick={onClose}
          aria-label={t('gpu.modal.closeTitle')}
        >
          {t('action.close')}
        </button>
      </div>

      <div className="mt-4 grid grid-cols-2 gap-2.5">
        <Detail label={t('gpu.modal.utilization')} value={`${formatPercent(gpu.utilization)}%`} />
        <Detail label={t('gpu.modal.temperature')} value={formatTemperature(gpu.temperature)} />
        <Detail
          label={t('gpu.modal.vramUsedTotal')}
          value={`${formatGbs(gpu.memory_used)} / ${formatGbs(gpu.memory_total)}`}
        />
        <Detail label={t('gpu.modal.powerDraw')} value={formatPower(gpu.power_draw)} />
      </div>

      <div className="mt-4">
        <div className="flex items-center justify-between text-[11px] text-muted">
          <span>{t('gpu.modal.vramUsage')}</span>
          <span className="font-mono text-ink">{formatPercent(vramPercent)}%</span>
        </div>
        <div className="mt-1 h-1.5 w-full overflow-hidden rounded-full bg-track">
          <div
            className="h-full rounded-full bg-accent transition-[width] duration-500"
            style={{ width: `${Math.min(100, Math.max(0, vramPercent))}%` }}
          />
        </div>
        <p className="mt-2 text-[11px] text-muted">
          {t('gpu.card.fan')} {formatFanSpeed(gpu.fan_speed)}
        </p>
      </div>
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl bg-panel px-3 py-2">
      <p className="truncate text-[10px] uppercase tracking-wide text-muted">{label}</p>
      <p className="mt-0.5 font-mono text-sm font-semibold text-ink">{value}</p>
    </div>
  );
}
