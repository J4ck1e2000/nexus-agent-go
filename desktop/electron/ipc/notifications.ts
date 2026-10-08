import { Notification } from 'electron';
import { gatewayError } from '../services/errors';
import { asObject, asStringField, handleEnvelope, type IpcDeps } from './context';

export function registerNotificationIpc(_deps: IpcDeps): void {
  handleEnvelope('notifications:show', (payload) => {
    const obj = asObject(payload, 'notification');
    const title = asStringField(obj, 'title', { max: 120 }).trim();
    const body = asStringField(obj, 'body', { max: 500 }).trim();
    if (!title || !body) throw gatewayError({ code: 'invalid_input', message: 'Notification title and body are required' });
    if (!Notification.isSupported()) throw gatewayError({ code: 'gateway_error', detail: 'notifications_unavailable', message: 'System notifications are unavailable' });
    new Notification({ title, body }).show();
    return null;
  });
}
