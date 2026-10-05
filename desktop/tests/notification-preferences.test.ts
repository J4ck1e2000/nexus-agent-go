import { beforeEach, describe, expect, it } from 'vitest';
import { DEFAULT_NOTIFICATION_PREFERENCES, loadNotificationPreferences, NOTIFICATION_PREFERENCES_KEY, saveNotificationPreferences } from '../src/lib/notification-preferences';

describe('notification preferences', () => {
  beforeEach(() => Object.defineProperty(globalThis, 'localStorage', { configurable: true, value: memoryStorage() }));

  it('loads defaults and round-trips local settings', () => {
    expect(loadNotificationPreferences()).toEqual(DEFAULT_NOTIFICATION_PREFERENCES);
    const preferences = { ...DEFAULT_NOTIFICATION_PREFERENCES, enabled: false, temperatureThresholdCelsius: 92 };
    saveNotificationPreferences(preferences);
    expect(loadNotificationPreferences()).toEqual(preferences);
  });

  it('clamps corrupt or out-of-range values to safe bounds', () => {
    localStorage.setItem(NOTIFICATION_PREFERENCES_KEY, JSON.stringify({ temperatureThresholdCelsius: 900, idleAfterMinutes: -2, sustainedGpuLoad: false }));
    expect(loadNotificationPreferences()).toMatchObject({ temperatureThresholdCelsius: 100, idleAfterMinutes: 1, sustainedGpuLoad: false });
  });
});

function memoryStorage(): Storage {
  const values = new Map<string, string>();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => { values.set(String(key), String(value)); },
    removeItem: (key) => { values.delete(key); },
    clear: () => values.clear(),
    key: (index) => [...values.keys()][index] ?? null,
    get length() { return values.size; },
  };
}
