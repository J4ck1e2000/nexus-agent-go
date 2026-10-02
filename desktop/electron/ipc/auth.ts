import type { IpcDeps } from './context';
import { asObject, asStringField, handleEnvelope, sanitizeUser } from './context';
import { gatewayError } from '../services/errors';

interface LoginResponse {
  token?: unknown;
  user?: unknown;
}

interface RegisterResponse {
  user?: unknown;
}

export function registerAuthIpc(deps: IpcDeps): void {
  handleEnvelope('auth:login', async (payload) => {
    const obj = asObject(payload);
    const username = asStringField(obj, 'username', { max: 128 });
    const password = asStringField(obj, 'password', { max: 256 });
    if (!username || !password) {
      throw gatewayError({ code: 'invalid_input', message: 'username and password are required' });
    }

    const response = await deps.gateway.request<LoginResponse>('POST', '/api/login', {
      body: { username, password },
      auth: false,
    });
    const token = response.token;
    if (typeof token !== 'string' || token.length === 0 || token.length > 4096) {
      throw gatewayError({
        code: 'invalid_response',
        message: 'Login response did not include a token',
      });
    }
    const user = sanitizeUser(response.user);
    deps.tokens.save(token);
    return user;
  });

  handleEnvelope('auth:register', async (payload) => {
    const obj = asObject(payload);
    const username = asStringField(obj, 'username', { max: 128 });
    const password = asStringField(obj, 'password', { max: 256 });
    if (!username || !password) {
      throw gatewayError({ code: 'invalid_input', message: 'username and password are required' });
    }

    const response = await deps.gateway.request<RegisterResponse>('POST', '/api/register', {
      body: { username, password },
      auth: false,
    });
    return sanitizeUser(response.user);
  });

  handleEnvelope('auth:logout', () => {
    deps.tokens.clear();
    return null;
  });

  handleEnvelope('auth:me', async () => {
    const token = deps.tokens.load();
    if (!token) {
      throw gatewayError({ code: 'unauthorized', message: 'No stored session' });
    }
    const user = await deps.gateway.request<unknown>('GET', '/api/me');
    return sanitizeUser(user);
  });
}
