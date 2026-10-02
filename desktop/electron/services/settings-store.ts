import fs from 'node:fs';
import path from 'node:path';

import { normalizeGatewayUrl } from '../lib/validate';

export interface DesktopSettingsFile {
  gatewayUrl: string;
}

export const DEFAULT_GATEWAY_URL = 'http://127.0.0.1:3000';

const DEFAULT_SETTINGS: DesktopSettingsFile = {
  gatewayUrl: DEFAULT_GATEWAY_URL,
};

/** Persists desktop settings as JSON under userData. Renderer never reads it directly. */
export class SettingsStore {
  private readonly filePath: string;

  constructor(filePath: string) {
    this.filePath = filePath;
  }

  load(): DesktopSettingsFile {
    try {
      const raw = fs.readFileSync(this.filePath, 'utf8');
      const parsed: unknown = JSON.parse(raw);
      return this.sanitize(parsed);
    } catch {
      return { ...DEFAULT_SETTINGS };
    }
  }

  save(settings: DesktopSettingsFile): DesktopSettingsFile {
    const normalized = normalizeGatewayUrl(settings.gatewayUrl);
    if (!normalized) {
      throw new Error('invalid gateway url');
    }
    const sanitized: DesktopSettingsFile = { gatewayUrl: normalized };
    fs.mkdirSync(path.dirname(this.filePath), { recursive: true });
    const tmpPath = `${this.filePath}.tmp-${process.pid}`;
    fs.writeFileSync(tmpPath, `${JSON.stringify(sanitized, null, 2)}\n`, 'utf8');
    fs.renameSync(tmpPath, this.filePath);
    return sanitized;
  }

  private sanitize(value: unknown): DesktopSettingsFile {
    if (value === null || typeof value !== 'object') {
      return { ...DEFAULT_SETTINGS };
    }
    const gatewayUrl = normalizeGatewayUrl((value as Record<string, unknown>).gatewayUrl);
    return {
      gatewayUrl: gatewayUrl ?? DEFAULT_GATEWAY_URL,
    };
  }
}
