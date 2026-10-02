import type { NexusAPI } from '../electron/types/ipc';

declare global {
  interface Window {
    nexus: NexusAPI;
  }
}

export {};
