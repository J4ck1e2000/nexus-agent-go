import { gatewayError } from '../services/errors';
import type {
  CreateIdleReservationPayload,
  IdleReservation,
  IdleReservationEvaluation,
  IdleReservationFilters,
} from '../types/ipc';
import { asIdField, asObject, asStringField, handleEnvelope, type IpcDeps } from './context';

export function registerIdleReservationIpc(deps: IpcDeps): void {
  handleEnvelope('idle-reservations:list', async () => sanitizeReservationList(
    await deps.gateway.request<unknown>('GET', '/api/idle-reservations'),
  ));

  handleEnvelope('idle-reservations:create', async (payload) => {
    const body = validateCreatePayload(payload);
    return sanitizeReservation(await deps.gateway.request<unknown>('POST', '/api/idle-reservations', { body }));
  });

  handleEnvelope('idle-reservations:set-status', async (payload) => {
    const obj = asObject(payload, 'idle reservation status');
    const id = asIdField(obj, 'id');
    const status = obj.status;
    if (status !== 'active' && status !== 'paused') {
      throw gatewayError({ code: 'invalid_input', message: 'status must be active or paused' });
    }
    return sanitizeReservation(await deps.gateway.request<unknown>('PATCH', `/api/idle-reservations/${id}`, { body: { status } }));
  });

  handleEnvelope('idle-reservations:evaluate', async (payload) => {
    const obj = asObject(payload, 'idle reservation evaluation');
    const id = asIdField(obj, 'id');
    const rawKeys = obj.matchingKeys;
    if (!Array.isArray(rawKeys) || rawKeys.length > 4096 || rawKeys.some((key) => typeof key !== 'string' || key.length > 64)) {
      throw gatewayError({ code: 'invalid_input', message: 'matchingKeys is invalid' });
    }
    const response = await deps.gateway.request<unknown>('POST', `/api/idle-reservations/${id}/evaluate`, {
      body: { matchingKeys: rawKeys as string[] },
    });
    const result = asObject(response, 'idle reservation evaluation response');
    const newMatchKeys = result.newMatchKeys;
    if (!Array.isArray(newMatchKeys) || newMatchKeys.some((key) => typeof key !== 'string')) {
      throw gatewayError({ code: 'invalid_response', message: 'Gateway returned invalid reservation matches' });
    }
    return {
      reservation: sanitizeReservation(result.reservation),
      newMatchKeys: newMatchKeys as string[],
    } satisfies IdleReservationEvaluation;
  });

  handleEnvelope('idle-reservations:remove', async (payload) => {
    const obj = asObject(payload, 'idle reservation id');
    const id = asIdField(obj, 'id');
    await deps.gateway.request<{ deleted: boolean }>('DELETE', `/api/idle-reservations/${id}`);
    return null;
  });
}

function validateCreatePayload(payload: unknown): CreateIdleReservationPayload {
  const obj = asObject(payload, 'idle reservation');
  const name = asStringField(obj, 'name', { max: 64 }).trim();
  if (!name) throw gatewayError({ code: 'invalid_input', message: 'Reservation name is required' });
  const filtersObj = asObject(obj.filters, 'reservation filters');
  const minFreeVramGb = nonNegativeNumber(filtersObj.minFreeVramGb, 'minFreeVramGb', 1024);
  const minFreeSystemMemoryGb = nonNegativeNumber(filtersObj.minFreeSystemMemoryGb, 'minFreeSystemMemoryGb', 16384);
  const gpuModel = asStringField(filtersObj, 'gpuModel', { max: 128, optional: true }).trim();
  const cpuModel = asStringField(filtersObj, 'cpuModel', { max: 256, optional: true }).trim();
  const maxGpuUtilization = nonNegativeNumber(filtersObj.maxGpuUtilization ?? 10, 'maxGpuUtilization', 100);
  const processPolicy = filtersObj.processPolicy;
  if (processPolicy !== 'any' && processPolicy !== 'emptyOnly') {
    throw gatewayError({ code: 'invalid_input', message: 'processPolicy is invalid' });
  }
  const duration = filtersObj.idleDurationMinutes;
  if (duration !== 0 && duration !== 5 && duration !== 10 && duration !== 30 && duration !== 60) {
    throw gatewayError({ code: 'invalid_input', message: 'idleDurationMinutes is invalid' });
  }
  const rawNodeIds = filtersObj.nodeIds;
  if (!Array.isArray(rawNodeIds) || rawNodeIds.length > 100 || rawNodeIds.some((id) => typeof id !== 'number' || !Number.isInteger(id) || id <= 0)) {
    throw gatewayError({ code: 'invalid_input', message: 'nodeIds is invalid' });
  }
  const filters: IdleReservationFilters = {
    minFreeVramGb,
    minFreeSystemMemoryGb,
    gpuModel,
    cpuModel,
    maxGpuUtilization,
    processPolicy,
    nodeIds: [...new Set(rawNodeIds as number[])],
    idleDurationMinutes: duration,
  };
  const notifyMode = obj.notifyMode;
  if (notifyMode !== 'once' && notifyMode !== 'continuous') {
    throw gatewayError({ code: 'invalid_input', message: 'notifyMode is invalid' });
  }
  const expiresInHours = obj.expiresInHours;
  if (expiresInHours !== 0 && expiresInHours !== 1 && expiresInHours !== 4 && expiresInHours !== 8 && expiresInHours !== 24 && expiresInHours !== 72) {
    throw gatewayError({ code: 'invalid_input', message: 'expiresInHours is invalid' });
  }
  return { name, filters, notifyMode, expiresInHours };
}

function nonNegativeNumber(value: unknown, field: string, maximum: number): number {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0 || value > maximum) {
    throw gatewayError({ code: 'invalid_input', message: `${field} is invalid` });
  }
  return value;
}

function sanitizeReservation(value: unknown): IdleReservation {
  const obj = asObject(value, 'idle reservation response');
  if (typeof obj.id !== 'number' || !Number.isInteger(obj.id) || obj.id <= 0) {
    throw gatewayError({ code: 'invalid_response', message: 'Gateway returned an invalid reservation id' });
  }
  const filtersObj = asObject(obj.filters, 'reservation filters response');
  const filters: IdleReservationFilters = {
    minFreeVramGb: finiteOrZero(filtersObj.minFreeVramGb),
    minFreeSystemMemoryGb: finiteOrZero(filtersObj.minFreeSystemMemoryGb),
    gpuModel: typeof filtersObj.gpuModel === 'string' ? filtersObj.gpuModel : '',
    cpuModel: typeof filtersObj.cpuModel === 'string' ? filtersObj.cpuModel : '',
    maxGpuUtilization: typeof filtersObj.maxGpuUtilization === 'number' ? Math.min(100, Math.max(0, filtersObj.maxGpuUtilization)) : 10,
    processPolicy: filtersObj.processPolicy === 'emptyOnly' ? 'emptyOnly' : 'any',
    nodeIds: Array.isArray(filtersObj.nodeIds) ? filtersObj.nodeIds.filter((id): id is number => typeof id === 'number' && Number.isInteger(id) && id > 0) : [],
    idleDurationMinutes: filtersObj.idleDurationMinutes === 5 || filtersObj.idleDurationMinutes === 10 || filtersObj.idleDurationMinutes === 30 || filtersObj.idleDurationMinutes === 60
      ? filtersObj.idleDurationMinutes
      : 0,
  };
  const status = obj.status;
  if (status !== 'active' && status !== 'paused' && status !== 'completed' && status !== 'expired') {
    throw gatewayError({ code: 'invalid_response', message: 'Gateway returned an invalid reservation status' });
  }
  return {
    id: obj.id,
    name: typeof obj.name === 'string' ? obj.name : 'Reservation',
    filters,
    status,
    notifyMode: obj.notifyMode === 'continuous' ? 'continuous' : 'once',
    expiresAtUnix: typeof obj.expiresAtUnix === 'number' ? obj.expiresAtUnix : null,
    currentMatchKeys: Array.isArray(obj.currentMatchKeys) ? obj.currentMatchKeys.filter((key): key is string => typeof key === 'string') : [],
    createdAtUnix: typeof obj.createdAtUnix === 'number' ? obj.createdAtUnix : 0,
  };
}

function sanitizeReservationList(value: unknown): IdleReservation[] {
  if (!Array.isArray(value)) throw gatewayError({ code: 'invalid_response', message: 'Expected an idle reservation list' });
  return value.map(sanitizeReservation);
}

function finiteOrZero(value: unknown): number {
  return typeof value === 'number' && Number.isFinite(value) ? value : 0;
}
