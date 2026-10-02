/** Formatting helpers ported from the web dashboard. */

export function formatPercent(value: number | null | undefined): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return '--';
  return value.toFixed(1);
}

/** Network speeds in MB/s, rendered like the web app. */
export function formatSpeed(megabytesPerSecond: number | null | undefined): string {
  const value = Number(megabytesPerSecond);
  if (!Number.isFinite(value) || value <= 0) return '0 KB/s';
  if (value < 1) return `${(value * 1024).toFixed(0)} KB/s`;
  return `${value.toFixed(2)} MB/s`;
}

export function formatTemperature(celsius: number | null | undefined): string {
  if (celsius === null || celsius === undefined || !Number.isFinite(celsius)) return '--';
  return `${celsius.toFixed(0)}C`;
}

export function formatPower(watts: number | null | undefined): string {
  if (watts === null || watts === undefined || !Number.isFinite(watts)) return '--';
  return `${watts.toFixed(0)}W`;
}

export function formatFanSpeed(percent: number | null | undefined): string {
  if (percent === null || percent === undefined || !Number.isFinite(percent)) return '--';
  return `${percent.toFixed(0)}%`;
}

export function formatGbs(gigabytes: number | null | undefined, digits = 1): string {
  if (gigabytes === null || gigabytes === undefined || !Number.isFinite(gigabytes)) return '--';
  return `${gigabytes.toFixed(digits)} GB`;
}

export function formatMegabytesToGb(megabytes: number | null | undefined, digits = 2): string {
  if (megabytes === null || megabytes === undefined || !Number.isFinite(megabytes)) return '--';
  return `${(megabytes / 1024).toFixed(digits)} GB`;
}

function pad2(value: number): string {
  return String(value).padStart(2, '0');
}

export function formatTime(date: Date): string {
  return `${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())}`;
}

export function formatDateTime(value: string | number | Date | null | undefined): string {
  if (value === null || value === undefined || value === '') return '--';
  const date = value instanceof Date ? value : new Date(value);
  if (Number.isNaN(date.getTime())) return '--';
  return `${date.getFullYear()}-${pad2(date.getMonth() + 1)}-${pad2(date.getDate())} ${formatTime(date)}`;
}

/** Short relative data age, e.g. "<1s", "42s", "3m 12s", "2h 05m". */
export function formatAgeShort(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || !Number.isFinite(seconds)) return 'N/A';
  const total = Math.max(0, Math.floor(seconds));
  if (total < 1) return '<1s';
  if (total < 60) return `${total}s`;
  if (total < 3600) return `${Math.floor(total / 60)}m ${total % 60}s`;
  return `${Math.floor(total / 3600)}h ${pad2(Math.floor((total % 3600) / 60))}m`;
}

const AVATAR_TONES = [
  'bg-[#dcefe5] text-[#2f655a]',
  'bg-[#e5e9f2] text-[#41527a]',
  'bg-[#f0e9dd] text-[#6c5a3f]',
  'bg-[#f2e1de] text-[#7a4740]',
  'bg-[#e9e4f0] text-[#574a75]',
  'bg-[#e2ecec] text-[#38605c]',
];

/** Deterministic avatar background for a user name. */
export function avatarTone(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i += 1) {
    hash = (hash * 31 + name.charCodeAt(i)) >>> 0;
  }
  return AVATAR_TONES[hash % AVATAR_TONES.length];
}

export function initialsOf(name: string): string {
  const trimmed = name.trim();
  if (!trimmed) return '?';
  return trimmed.slice(0, 2).toUpperCase();
}
