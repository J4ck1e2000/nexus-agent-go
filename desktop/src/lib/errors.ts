import type { NexusError } from '../../electron/types/ipc';
import type { Translator } from '../i18n';

/**
 * Map a normalized gateway error to a user-facing localized message.
 * Preference: specific gateway detail code, then transport code, then fallback.
 */
export function localizedError(error: NexusError, t: Translator): string {
  if (error.detail) {
    const detailKey = `errors.${error.detail}`;
    const byDetail = t(detailKey);
    if (byDetail !== detailKey) return byDetail;
  }
  const codeKey = `errors.${error.code}`;
  const byCode = t(codeKey);
  if (byCode !== codeKey) return byCode;
  return t('errors.fallback');
}

/** Localize a bare gateway error code (e.g. from an SSE error event). */
export function localizedErrorDetail(detail: string | undefined, fallback: string, t: Translator): string {
  if (detail) {
    const key = `errors.${detail}`;
    const value = t(key);
    if (value !== key) return value;
  }
  return fallback;
}
