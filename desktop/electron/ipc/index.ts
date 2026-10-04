import type { IpcDeps } from './context';
import { registerAIIpc } from './ai';
import { registerAuthIpc } from './auth';
import { registerConversationsIpc } from './conversations';
import { registerNodesIpc } from './nodes';
import { registerSettingsIpc } from './settings';
import { registerUsersIpc } from './users';

/** Registers every IPC handler; returns a dispose fn that cancels in-flight AI streams. */
export function registerIpcHandlers(deps: IpcDeps): () => void {
  registerAuthIpc(deps);
  registerSettingsIpc(deps);
  registerNodesIpc(deps);
  registerUsersIpc(deps);
  registerConversationsIpc(deps);
  const disposeAI = registerAIIpc(deps);
  return disposeAI;
}
