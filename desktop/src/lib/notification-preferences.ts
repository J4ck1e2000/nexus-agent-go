export interface NotificationPreferences {
  enabled: boolean;
  nodeOffline: boolean;
  gpuTemperature: boolean;
  idleGpu: boolean;
  sustainedGpuLoad: boolean;
  memoryReleased: boolean;
  processEnded: boolean;
  temperatureThresholdCelsius: number;
  idleAfterMinutes: number;
  sustainedLoadMinutes: number;
  releasedVramGb: number;
}

export const NOTIFICATION_PREFERENCES_KEY = 'nexus.notificationPreferences.v1';

export const DEFAULT_NOTIFICATION_PREFERENCES: NotificationPreferences = {
  enabled: true,
  nodeOffline: true,
  gpuTemperature: true,
  idleGpu: true,
  sustainedGpuLoad: true,
  memoryReleased: true,
  processEnded: true,
  temperatureThresholdCelsius: 85,
  idleAfterMinutes: 10,
  sustainedLoadMinutes: 30,
  releasedVramGb: 8,
};

export function loadNotificationPreferences(): NotificationPreferences {
  if (typeof localStorage === 'undefined') return { ...DEFAULT_NOTIFICATION_PREFERENCES };
  try {
    const value: unknown = JSON.parse(localStorage.getItem(NOTIFICATION_PREFERENCES_KEY) ?? '{}');
    if (!value || typeof value !== 'object' || Array.isArray(value)) return { ...DEFAULT_NOTIFICATION_PREFERENCES };
    const raw = value as Partial<NotificationPreferences>;
    return {
      enabled: raw.enabled !== false,
      nodeOffline: raw.nodeOffline !== false,
      gpuTemperature: raw.gpuTemperature !== false,
      idleGpu: raw.idleGpu !== false,
      sustainedGpuLoad: raw.sustainedGpuLoad !== false,
      memoryReleased: raw.memoryReleased !== false,
      processEnded: raw.processEnded !== false,
      temperatureThresholdCelsius: clamp(raw.temperatureThresholdCelsius, 60, 100, 85),
      idleAfterMinutes: clamp(raw.idleAfterMinutes, 1, 240, 10),
      sustainedLoadMinutes: clamp(raw.sustainedLoadMinutes, 5, 240, 30),
      releasedVramGb: clamp(raw.releasedVramGb, 1, 256, 8),
    };
  } catch {
    return { ...DEFAULT_NOTIFICATION_PREFERENCES };
  }
}

export function saveNotificationPreferences(preferences: NotificationPreferences): void {
  try {
    localStorage.setItem(NOTIFICATION_PREFERENCES_KEY, JSON.stringify(preferences));
  } catch {
    // Notification preference persistence is best effort.
  }
}

function clamp(value: unknown, minimum: number, maximum: number, fallback: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value)) return fallback;
  return Math.max(minimum, Math.min(maximum, value));
}
