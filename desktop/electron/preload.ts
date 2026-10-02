import { contextBridge } from 'electron';

contextBridge.exposeInMainWorld('nexus', {
  platform: process.platform,
});
