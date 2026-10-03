import { describe, expect, it } from 'vitest';
import { GatewayError, gatewayError } from '../electron/services/errors';
import {
  buildAddNodeBody,
  buildTestSSHBody,
  resolveCollectorType,
  sanitizeAgentConfig,
  sanitizeAgentConfigList,
} from '../electron/lib/node-payload';

describe('resolveCollectorType', () => {
  it('defaults absent values to agent', () => {
    expect(resolveCollectorType(undefined)).toBe('agent');
    expect(resolveCollectorType(null)).toBe('agent');
    expect(resolveCollectorType('')).toBe('agent');
    expect(resolveCollectorType('agent')).toBe('agent');
  });

  it('accepts ssh and rejects unknown types', () => {
    expect(resolveCollectorType('ssh')).toBe('ssh');
    expect(() => resolveCollectorType('snmp')).toThrow(GatewayError);
  });
});

describe('sanitizeAgentConfig', () => {
  it('keeps legacy agent nodes intact', () => {
    const config = sanitizeAgentConfig({ id: 1, name: 'node-a', url: 'http://127.0.0.1:8005' });
    expect(config).toEqual({ id: 1, name: 'node-a', collector_type: 'agent', url: 'http://127.0.0.1:8005' });
  });

  it('accepts ssh nodes without url', () => {
    const config = sanitizeAgentConfig({
      id: 2,
      name: 'A6000-01',
      collector_type: 'ssh',
      ssh_host: '10.0.0.15',
      ssh_port: 22,
      ssh_user: 'renhaokun',
      ssh_auth_type: 'key',
    });
    expect(config.collector_type).toBe('ssh');
    expect(config.ssh_host).toBe('10.0.0.15');
    expect(config.ssh_port).toBe(22);
    expect(config.ssh_user).toBe('renhaokun');
    expect(config.ssh_auth_type).toBe('key');
    expect(config.url).toBeUndefined();
  });

  it('rejects invalid payloads', () => {
    expect(() => sanitizeAgentConfig(null)).toThrow(GatewayError);
    expect(() => sanitizeAgentConfig({ id: 'x', name: 'n', url: 'http://x' })).toThrow(GatewayError);
    expect(() => sanitizeAgentConfig({ id: 1, name: 'n' })).toThrow(GatewayError); // agent node missing url
    expect(() =>
      sanitizeAgentConfig({ id: 1, name: 'n', collector_type: 'ssh' }),
    ).toThrow(GatewayError); // ssh node missing fields
    expect(() =>
      sanitizeAgentConfig({
        id: 1,
        name: 'n',
        collector_type: 'ssh',
        ssh_host: 'h',
        ssh_port: 22,
        ssh_user: 'u',
        ssh_auth_type: 'password',
      }),
    ).toThrow(GatewayError);
  });

  it('sanitizes lists', () => {
    const list = sanitizeAgentConfigList([
      { id: 1, name: 'agent-node', url: 'http://127.0.0.1:8005' },
      { id: 2, name: 'ssh-node', collector_type: 'ssh', ssh_host: 'h', ssh_port: 22, ssh_user: 'u' },
    ]);
    expect(list).toHaveLength(2);
    expect(list[0].collector_type).toBe('agent');
    expect(list[1].collector_type).toBe('ssh');
    expect(() => sanitizeAgentConfigList({})).toThrow(GatewayError);
  });
});

describe('buildAddNodeBody', () => {
  it('builds legacy agent payloads', () => {
    expect(buildAddNodeBody({ name: ' node ', collector_type: 'agent', url: ' http://127.0.0.1:8005 ' })).toEqual({
      name: 'node',
      collector_type: 'agent',
      url: 'http://127.0.0.1:8005',
    });
  });

  it('treats a missing collector type as agent', () => {
    expect(buildAddNodeBody({ name: 'node', url: 'http://127.0.0.1:8005' })).toEqual({
      name: 'node',
      collector_type: 'agent',
      url: 'http://127.0.0.1:8005',
    });
  });

  it('builds ssh payloads with key auth', () => {
    expect(
      buildAddNodeBody({
        name: 'A6000-01',
        collector_type: 'ssh',
        ssh_host: '10.0.0.15',
        ssh_port: 22,
        ssh_user: 'renhaokun',
      }),
    ).toEqual({
      name: 'A6000-01',
      collector_type: 'ssh',
      ssh_host: '10.0.0.15',
      ssh_port: 22,
      ssh_user: 'renhaokun',
      ssh_auth_type: 'key',
    });
  });

  it('rejects invalid ssh inputs', () => {
    expect(() => buildAddNodeBody({ name: 'x', collector_type: 'ssh', ssh_port: 22, ssh_user: 'u' })).toThrow(
      GatewayError,
    );
    expect(() =>
      buildAddNodeBody({ name: 'x', collector_type: 'ssh', ssh_host: 'h', ssh_port: 0, ssh_user: 'u' }),
    ).toThrow(GatewayError);
    expect(() =>
      buildAddNodeBody({ name: 'x', collector_type: 'ssh', ssh_host: 'h', ssh_port: 65536, ssh_user: 'u' }),
    ).toThrow(GatewayError);
    expect(() =>
      buildAddNodeBody({ name: 'x', collector_type: 'ssh', ssh_host: 'h', ssh_port: 22, ssh_user: 'bad user' }),
    ).toThrow(GatewayError);
    expect(() => buildAddNodeBody({ name: '', url: 'http://x' })).toThrow(GatewayError);
    expect(() => buildAddNodeBody({ name: 'x', url: 'ftp://x' })).toThrow(GatewayError);
  });
});

describe('buildTestSSHBody', () => {
  it('passes through a valid target', () => {
    expect(buildTestSSHBody({ ssh_host: ' 10.0.0.15 ', ssh_port: 2222, ssh_user: 'ops' })).toEqual({
      ssh_host: '10.0.0.15',
      ssh_port: 2222,
      ssh_user: 'ops',
    });
  });

  it('rejects invalid targets', () => {
    expect(() => buildTestSSHBody({ ssh_port: 22, ssh_user: 'u' })).toThrow(GatewayError);
    expect(() => buildTestSSHBody({ ssh_host: 'h', ssh_port: 1.5, ssh_user: 'u' })).toThrow(GatewayError);
    expect(() => buildTestSSHBody({ ssh_host: 'h', ssh_port: 22, ssh_user: '' })).toThrow(GatewayError);
  });
});

describe('ssh error detail propagation', () => {
  it('keeps gateway ssh error codes on the payload', () => {
    const err = gatewayError({
      code: 'gateway_error',
      detail: 'ssh_auth_failed',
      message: 'SSH connection test failed',
    });
    expect(err.detail).toBe('ssh_auth_failed');
    expect(err.code).toBe('gateway_error');
  });
});
