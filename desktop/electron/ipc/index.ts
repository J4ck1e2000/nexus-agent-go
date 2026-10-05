import type { IpcDeps } from './context';
import { registerAIIpc } from './ai';
import { registerAuthIpc } from './auth';
import { registerConversationsIpc } from './conversations';
import { registerIdleReservationIpc } from './idle-reservations';
import { registerNodesIpc } from './nodes';
import { registerNotificationIpc } from './notifications';
import { registerSettingsIpc } from './settings';
import { registerTerminalIpc } from './terminal';
import { registerUsersIpc } from './users';

/** Registers every IPC handler; returns a dispose fn that cancels in-flight AI streams. */
export function registerIpcHandlers(deps: IpcDeps): () => void {
  registerAuthIpc(deps);
  registerSettingsIpc(deps);
  registerNodesIpc(deps);
  registerIdleReservationIpc(deps);
  registerNotificationIpc(deps);
  registerTerminalIpc(deps);
  registerUsersIpc(deps);
  registerConversationsIpc(deps);
  const disposeAI = registerAIIpc(deps);
  return disposeAI;
}
