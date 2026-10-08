import { useMemo, useState } from 'react';
import { ChevronDown, ChevronRight, History, LoaderCircle } from 'lucide-react';
import type { EnrichedNode } from '../../lib/node-logic';
import { useLanguage } from '../../hooks/useLanguage';
import { useNodeHistory } from '../../hooks/useNodeHistory';
import { localizedError } from '../../lib/errors';

const RANGE_DAYS = [1, 7, 30, 90] as const;

export default function NodeHistoryPanel({ node }: { node: EnrichedNode }) {
  const { t } = useLanguage();
  const [open, setOpen] = useState(false);
  const [days, setDays] = useState<(typeof RANGE_DAYS)[number]>(30);
  const { history, loading, error } = useNodeHistory(node.id, days, open);
  const samples = history?.samples ?? [];

  const heatmap = useMemo(() => buildHeatmap(samples, days), [days, samples]);
  const latest = samples.at(-1);
  const cpuPoints = samples.flatMap((sample) => sample.cpuUsage === undefined ? [] : [sample.cpuUsage]);
  const ramPoints = samples.flatMap((sample) => sample.ramPercent === undefined ? [] : [sample.ramPercent]);

  return (
    <section className="soft-panel p-5">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <button type="button" className="flex items-center gap-2 text-left" onClick={() => setOpen((value) => !value)} aria-expanded={open}>
          {open ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
          <History size={16} />
          <span>
            <strong className="block text-sm text-ink">{t('history.title')}</strong>
            <span className="block text-[11px] text-muted">{t('history.retention', { days: history?.retentionDays ?? 90 })}</span>
          </span>
        </button>
        {open && <select aria-label={t('history.range')} className="input-field !w-auto !py-1.5 text-xs" value={days} onChange={(event) => setDays(Number(event.target.value) as (typeof RANGE_DAYS)[number])}>
          {RANGE_DAYS.map((value) => <option key={value} value={value}>{t('history.days', { count: value })}</option>)}
        </select>}
      </div>

      {open && <div className="mt-4">
        {loading && samples.length === 0 ? <div className="flex items-center gap-2 py-8 text-xs text-muted"><LoaderCircle className="animate-spin" size={15} />{t('history.loading')}</div>
            : error ? <p className="py-6 text-xs text-red-700" role="alert">{t('history.error')}: {localizedError(error, t)}</p>
            : samples.length === 0 ? <p className="py-6 text-xs text-muted">{t('history.empty')}</p>
              : <>
                <div className="mb-4 grid gap-3 sm:grid-cols-2">
                  <TrendSummary label={t('history.cpu')} points={cpuPoints} />
                  <TrendSummary label={t('history.memory')} points={ramPoints} />
                </div>
                <div className="overflow-x-auto pb-2">
                  <div className="min-w-[560px]">
                    <div className="mb-2 flex items-center justify-between text-[10px] text-muted">
                      <span>{days === 1 ? t('history.hourBuckets') : t('history.dayBuckets')}</span>
                      <span>{t('history.latestSample', { time: latest ? new Date(latest.timestampUnix * 1000).toLocaleString() : '' })}</span>
                    </div>
                    {heatmap.rows.length === 0 ? <p className="py-6 text-xs text-muted">{t('history.noGpuSamples')}</p> : <div className="flex flex-col gap-2">
                      {heatmap.rows.map((row) => (
                        <div key={row.id} className="grid grid-cols-[92px_1fr] items-center gap-3">
                          <span className="truncate text-[11px] text-muted" title={row.name}>GPU {row.id} · {row.name}</span>
                          <div className="grid gap-1" style={{ gridTemplateColumns: `repeat(${heatmap.buckets.length}, minmax(5px, 1fr))` }}>
                            {heatmap.buckets.map((bucket, index) => {
                              const value = row.values[index];
                              return <span key={bucket.start} className={`h-5 min-w-[5px] rounded-[3px] ${heatColor(value)}`} title={value === null ? bucket.label : `${bucket.label} · GPU ${row.id} · ${t('history.utilization')}: ${value.toFixed(0)}%`} />;
                            })}
                          </div>
                        </div>
                      ))}
                    </div>}
                  </div>
                </div>
                <div className="mt-3 flex items-center justify-between text-[10px] text-muted">
                  <span>{t('history.low')}</span><span className="flex gap-1"><i className={`h-3 w-5 rounded ${heatColor(5)}`} /><i className={`h-3 w-5 rounded ${heatColor(25)}`} /><i className={`h-3 w-5 rounded ${heatColor(55)}`} /><i className={`h-3 w-5 rounded ${heatColor(85)}`} /></span><span>{t('history.high')}</span>
                </div>
              </>}
      </div>}
    </section>
  );
}

function TrendSummary({ label, points }: { label: string; points: number[] }) {
  const latest = points.at(-1);
  const average = points.length > 0 ? points.reduce((sum, value) => sum + value, 0) / points.length : null;
  return <div className="soft-panel-subtle flex items-center justify-between gap-3 px-3 py-2.5">
    <span className="text-xs text-muted">{label}</span>
    <span className="text-right font-mono text-xs text-ink">{latest === undefined ? '—' : `${latest.toFixed(1)}%`}<small className="ml-2 text-muted">{average === null ? '' : `avg ${average.toFixed(1)}%`}</small></span>
  </div>;
}

function buildHeatmap(samples: Array<{ timestampUnix: number; status: string; gpus?: Array<{ id: number; name: string; utilization: number }> }>, days: number) {
  const now = Math.floor(Date.now() / 1000);
  const bucketSeconds = days === 1 ? 3600 : 86_400;
  const bucketCount = days === 1 ? 24 : days;
  const start = Math.floor(now / bucketSeconds) * bucketSeconds - (bucketCount - 1) * bucketSeconds;
  const buckets = Array.from({ length: bucketCount }, (_, index) => {
    const bucketStart = start + index * bucketSeconds;
    return { start: bucketStart, label: days === 1 ? new Date(bucketStart * 1000).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : new Date(bucketStart * 1000).toLocaleDateString() };
  });
  const gpuIds = new Map<number, string>();
  const values = new Map<number, number[][]>();
  for (const sample of samples) {
    const bucketIndex = Math.floor((sample.timestampUnix - start) / bucketSeconds);
    if (bucketIndex < 0 || bucketIndex >= bucketCount || sample.status !== 'online') continue;
    for (const gpu of sample.gpus ?? []) {
      gpuIds.set(gpu.id, gpu.name);
      const row = values.get(gpu.id) ?? Array.from({ length: bucketCount }, () => []);
      row[bucketIndex].push(gpu.utilization);
      values.set(gpu.id, row);
    }
  }
  return {
    buckets,
    rows: [...gpuIds.entries()].sort(([left], [right]) => left - right).map(([id, name]) => ({
      id,
      name,
      values: (values.get(id) ?? []).map((items) => items.length ? items.reduce((sum, value) => sum + value, 0) / items.length : null),
    })),
  };
}

function heatColor(value: number | null): string {
  if (value === null) return 'bg-[#e9e5dd]';
  if (value < 10) return 'bg-[#dbeee5]';
  if (value < 40) return 'bg-[#90c9b5]';
  if (value < 70) return 'bg-[#edc276]';
  return 'bg-[#dd8c78]';
}
