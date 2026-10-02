import type { CreateUserPayload, UserRole } from '../types/ipc';
import { asIdField, asObject, asStringField, handleEnvelope, sanitizeUser, sanitizeUserList, type IpcDeps } from './context';
import { gatewayError } from '../services/errors';

function asRole(value: unknown, allowEmpty: boolean): UserRole {
  if (allowEmpty && (value === undefined || value === null || value === '')) {
    return 'user';
  }
  if (value === 'admin' || value === 'user') {
    return value;
  }
  throw gatewayError({ code: 'invalid_input', message: 'role must be "admin" or "user"' });
}

export function registerUsersIpc(deps: IpcDeps): void {
  handleEnvelope('users:list', (payload) => {
    const obj = asObject(payload);
    const q = asStringField(obj, 'q', { max: 128, optional: true }).trim();
    const path = q ? `/api/admin/users?q=${encodeURIComponent(q)}` : '/api/admin/users';
    return deps.gateway.request<unknown>('GET', path).then(sanitizeUserList);
  });

  handleEnvelope('users:create', async (payload) => {
    const obj = asObject(payload);
    const username = asStringField(obj, 'username', { max: 128 });
    const password = asStringField(obj, 'password', { max: 256 });
    const role = asRole(obj.role, true);
    if (!username || !password) {
      throw gatewayError({ code: 'invalid_input', message: 'username and password are required' });
    }
    const body: CreateUserPayload = { username, password, role };
    const created = await deps.gateway.request<unknown>('POST', '/api/admin/users', { body });
    return sanitizeUser(created);
  });

  handleEnvelope('users:remove', (payload) => {
    const obj = asObject(payload);
    const id = asIdField(obj, 'id');
    return deps.gateway.request<unknown>('DELETE', `/api/admin/users/${id}`).then(() => null);
  });

  handleEnvelope('users:setRole', (payload) => {
    const obj = asObject(payload);
    const id = asIdField(obj, 'id');
    const role = asRole(obj.role, false);
    return deps.gateway
      .request<unknown>('PATCH', `/api/admin/users/${id}/role`, { body: { role } })
      .then(sanitizeUser);
  });

  handleEnvelope('users:resetPassword', (payload) => {
    const obj = asObject(payload);
    const id = asIdField(obj, 'id');
    const password = asStringField(obj, 'password', { max: 256 });
    if (password.length < 6) {
      throw gatewayError({ code: 'invalid_input', message: 'Password must be at least 6 characters' });
    }
    return deps.gateway
      .request<unknown>('PATCH', `/api/admin/users/${id}/password`, { body: { password } })
      .then(() => null);
  });
}
