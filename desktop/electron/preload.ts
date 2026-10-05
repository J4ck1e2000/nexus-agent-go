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
    history: (id, fromUnix, stepSeconds, aggregation) => ipcRenderer.invoke('nodes:history', { id, fromUnix, stepSeconds, aggregation }),
    list: () => ipcRenderer.invoke('nodes:list'),
    add: (payload) => ipcRenderer.invoke('nodes:add', payload),
    remove: (id) => ipcRenderer.invoke('nodes:remove', { id }),
    testSSH: (payload) => ipcRenderer.invoke('nodes:test-ssh', payload),
  },
  users: {
    list: (q) => ipcRenderer.invoke('users:list', { q: q ?? '' }),
    create: (payload) => ipcRenderer.invoke('users:create', payload),
    remove: (id) => ipcRenderer.invoke('users:remove', { id }),
    setRole: (id, role) => ipcRenderer.invoke('users:setRole', { id, role }),
    resetPassword: (id, password) => ipcRenderer.invoke('users:resetPassword', { id, password }),
  },
  ai: {
    start: (requestId, query, conversationId) =>
      ipcRenderer.invoke('ai:start', { requestId, query, conversationId: conversationId ?? null }),
    cancel: (requestId) => ipcRenderer.invoke('ai:cancel', { requestId }),
    onEvent: (cb) => subscribe<AIStreamEvent>('ai:event', cb),
  },
  conversations: {
    list: () => ipcRenderer.invoke('conversations:list'),
    create: () => ipcRenderer.invoke('conversations:create'),
    messages: (id) => ipcRenderer.invoke('conversations:messages', { id }),
    remove: (id) => ipcRenderer.invoke('conversations:remove', { id }),
  },
  settings: {
    get: () => ipcRenderer.invoke('settings:get'),
    update: (settings) => ipcRenderer.invoke('settings:update', settings),
    testConnection: (url) => ipcRenderer.invoke('settings:testConnection', { url }),
  },
  idleReservations: {
    list: () => ipcRenderer.invoke('idle-reservations:list'),
    create: (payload) => ipcRenderer.invoke('idle-reservations:create', payload),
    setStatus: (id, status) => ipcRenderer.invoke('idle-reservations:set-status', { id, status }),
    evaluate: (id, matchingKeys) => ipcRenderer.invoke('idle-reservations:evaluate', { id, matchingKeys }),
    remove: (id) => ipcRenderer.invoke('idle-reservations:remove', { id }),
  },
  notifications: {
    show: (title, body) => ipcRenderer.invoke('notifications:show', { title, body }),
  },
  terminal: {
    start: (payload) => ipcRenderer.invoke('terminal:start', payload),
    attach: (sessionId) => ipcRenderer.invoke('terminal:attach', { sessionId }),
    write: (sessionId, data) => ipcRenderer.invoke('terminal:write', { sessionId, data }),
    resize: (sessionId, cols, rows) => ipcRenderer.invoke('terminal:resize', { sessionId, cols, rows }),
    close: (sessionId) => ipcRenderer.invoke('terminal:close', { sessionId }),
    onOutput: (cb) => subscribe('terminal:output', cb),
    onExit: (cb) => subscribe('terminal:exit', cb),
  },
};

contextBridge.exposeInMainWorld('nexus', api);
