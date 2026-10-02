import type { GpuInfo } from '../../../electron/types/ipc';
import { getGpuLoadTier, GPU_TIER_STYLES, type GpuLoadTier } from '../../lib/node-logic';
import { formatPercent, formatTemperature } from '../../lib/format';
import { useLanguage } from '../../hooks/useLanguage';

interface GPUCardProps {
  gpu: GpuInfo;
  selected: boolean;
  onSelect: () => void;
}

/** One GPU tile inside the matrix. */
export default function GPUCard({ gpu, selected, onSelect }: GPUCardProps) {
  const { t } = useLanguage();
  const tier: GpuLoadTier = getGpuLoadTier(gpu);
  const tierStyle = GPU_TIER_STYLES[tier];

  return (
    <button
      type="button"
      aria-label={t('gpu.card.aria', { name: gpu.name })}
      onClick={onSelect}
      className={`rounded-xl border p-3 text-left transition hover:-translate-y-px hover:shadow-soft ${tierStyle} ${
        selected ? 'ring-2 ring-[#5f9189]/50' : ''
      }`}
    >
      <div className="flex items-center justify-between gap-2">
        <span className="truncate font-mono text-[11px] font-semibold">{gpu.name}</span>
        <span className="shrink-0 text-[10px] font-medium opacity-80">
          {t(`gpu.status.${tier}`)}
        </span>
      </div>
      <div className="mt-2 flex items-baseline justify-between font-mono">
        <span className="text-lg font-semibold">{formatPercent(gpu.utilization)}%</span>
        <span className="text-[10px] opacity-80">{formatTemperature(gpu.temperature)}</span>
      </div>
      <p className="mt-1 text-[10px] opacity-80">
        {t('gpu.card.vram')} {gpu.memory_used.toFixed(1)}/{gpu.memory_total.toFixed(1)} GB
      </p>
    </button>
  );
}
