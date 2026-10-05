import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from 'react';
import {
  loadNotificationPreferences,
  saveNotificationPreferences,
  type NotificationPreferences,
} from '../lib/notification-preferences';

interface NotificationPreferencesContextValue {
  preferences: NotificationPreferences;
  update: (patch: Partial<NotificationPreferences>) => void;
}

const NotificationPreferencesContext = createContext<NotificationPreferencesContextValue | null>(null);

export function NotificationPreferencesProvider({ children }: { children: ReactNode }) {
  const [preferences, setPreferences] = useState(loadNotificationPreferences);
  const update = useCallback((patch: Partial<NotificationPreferences>) => {
    setPreferences((current) => {
      const next = { ...current, ...patch };
      saveNotificationPreferences(next);
      return next;
    });
  }, []);
  const value = useMemo(() => ({ preferences, update }), [preferences, update]);
  return <NotificationPreferencesContext.Provider value={value}>{children}</NotificationPreferencesContext.Provider>;
}

export function useNotificationPreferences(): NotificationPreferencesContextValue {
  const value = useContext(NotificationPreferencesContext);
  if (!value) throw new Error('NotificationPreferencesProvider is missing');
  return value;
}
