import { asObject, handleEnvelope, type IpcDeps } from './context';
import { requireGatewayUrl } from '../lib/validate';
import { CONNECTION_TEST_TIMEOUT_MS } from '../services/gateway';

interface VersionResponse {
  version?: unknown;
  versionName?: unknown;
  changelog?: unknown;
}

export function registerSettingsIpc(deps: IpcDeps): void {
  handleEnvelope('settings:get', () => deps.settings.load());

  handleEnvelope('settings:update', (payload) => {
    const obj = asObject(payload);
    const gatewayUrl = requireGatewayUrl(obj.gatewayUrl);
    return deps.settings.save({ gatewayUrl });
  });

  handleEnvelope('settings:testConnection', async (payload) => {
    const obj = asObject(payload);
    const baseURL = requireGatewayUrl(obj.url);
    const info = await deps.gateway.request<VersionResponse>('GET', '/api/version', {
      baseURL,
      auth: false,
      timeoutMs: CONNECTION_TEST_TIMEOUT_MS,
    });
    return {
      versionName: typeof info.versionName === 'string' ? info.versionName : '',
      changelog: typeof info.changelog === 'string' ? info.changelog : '',
    };
  });
}
