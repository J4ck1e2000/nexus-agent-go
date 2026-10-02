import { contextBridge, ipcRenderer, type IpcRendererEvent } from 'electron';
import type { AIStreamEvent, NexusAPI, Unsubscribe } from './types/ipc';

function subscribe<T>(channel: string, callback: (payload: T) => void): Unsubscribe {
  const listener = (_event: IpcRendererEvent, payload: T): void => callback(payload);
  ipcRenderer.on(channel, listener);
  return () => {
    ipcRenderer.off(channel, listener);
  };
}

const api: NexusAPI = {
  auth: {
    login: (payload) => ipcRenderer.invoke('auth:login', payload),
    register: (payload) => ipcRenderer.invoke('auth:register', payload),
    logout: () => ipcRenderer.invoke('auth:logout'),
    me: () => ipcRenderer.invoke('auth:me'),
    onAuthExpired: (cb) => subscribe('auth:expired', cb),
  },
  nodes: {
    overview: () => ipcRenderer.invoke('nodes:overview'),
    list: () => ipcRenderer.invoke('nodes:list'),
    add: (payload) => ipcRenderer.invoke('nodes:add', payload),
    remove: (id) => ipcRenderer.invoke('nodes:remove', { id }),
  },
  users: {
    list: (q) => ipcRenderer.invoke('users:list', { q: q ?? '' }),
    create: (payload) => ipcRenderer.invoke('users:create', payload),
    remove: (id) => ipcRenderer.invoke('users:remove', { id }),
    setRole: (id, role) => ipcRenderer.invoke('users:setRole', { id, role }),
    resetPassword: (id, password) => ipcRenderer.invoke('users:resetPassword', { id, password }),
  },
  ai: {
    start: (requestId, query) => ipcRenderer.invoke('ai:start', { requestId, query }),
    cancel: (requestId) => ipcRenderer.invoke('ai:cancel', { requestId }),
    onEvent: (cb) => subscribe<AIStreamEvent>('ai:event', cb),
  },
  settings: {
    get: () => ipcRenderer.invoke('settings:get'),
    update: (settings) => ipcRenderer.invoke('settings:update', settings),
    testConnection: (url) => ipcRenderer.invoke('settings:testConnection', { url }),
  },
};

contextBridge.exposeInMainWorld('nexus', api);
