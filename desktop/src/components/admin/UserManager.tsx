import { useMemo, useState } from 'react';
import type { UserRecord, UserRole } from '../../../electron/types/ipc';
import AppModal from '../common/AppModal';
import ConfirmDialog from '../common/ConfirmDialog';
import PromptDialog from '../common/PromptDialog';
import { useAdminUsers } from '../../hooks/useAdminUsers';
import { useAuth } from '../../hooks/useAuth';
import { useLanguage } from '../../hooks/useLanguage';
import { useToastContext } from '../../context/ToastContext';
import { localizedError } from '../../lib/errors';
import { formatDateTime } from '../../lib/format';
import Spinner from '../common/Spinner';

const MIN_PASSWORD_LENGTH = 6;

/** Admin user management modal: create, search, role changes, password resets. */
export default function UserManager({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { t } = useLanguage();
  const { showToast } = useToastContext();
  const { user, applyRoleChange } = useAuth();
  const { users, loading, reload, createUser, deleteUser, setRole, resetPassword } = useAdminUsers(open);

  const [query, setQuery] = useState('');
  const [actionKey, setActionKey] = useState<string | null>(null);
  const [form, setForm] = useState<{ username: string; password: string; role: UserRole }>({
    username: '',
    password: '',
    role: 'user',
  });
  const [deleteTarget, setDeleteTarget] = useState<UserRecord | null>(null);
  const [passwordTarget, setPasswordTarget] = useState<UserRecord | null>(null);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return users;
    return users.filter((entry) => entry.username.toLowerCase().includes(needle));
  }, [users, query]);

  const handleCreate = async (): Promise<void> => {
    const username = form.username.trim();
    if (!username) {
      showToast(t('dialog.usernameRequired'), 'warning');
      return;
    }
    if (!form.password) {
      showToast(t('dialog.passwordRequired'), 'warning');
      return;
    }
    if (form.password.length < MIN_PASSWORD_LENGTH) {
      showToast(t('dialog.passwordTooShort', { min: MIN_PASSWORD_LENGTH }), 'warning');
      return;
    }
    if (form.role !== 'user' && form.role !== 'admin') {
      showToast(t('dialog.invalidRole'), 'warning');
      return;
    }

    setActionKey('create');
    try {
      const result = await createUser({ username, password: form.password, role: form.role });
      if (result.ok) {
        showToast(t('notify.userCreated', { name: result.data.username }), 'success');
        setForm({ username: '', password: '', role: 'user' });
      } else {
        showToast(t('notify.createUserFailed', { error: localizedError(result.error, t) }), 'error');
      }
    } finally {
      setActionKey(null);
    }
  };

  const handleRoleChange = async (target: UserRecord, role: UserRole): Promise<void> => {
    if (role === target.role) return;
    setActionKey(`role:${target.id}`);
    try {
      const result = await setRole(target.id, role);
      if (result.ok) {
        showToast(t('notify.userRoleUpdated'), 'success');
        if (user && user.id === target.id) {
          applyRoleChange(target.id, result.data.role);
        }
      } else {
        showToast(t('notify.adminUserActionFailed', { error: localizedError(result.error, t) }), 'error');
        await reload();
      }
    } finally {
      setActionKey(null);
    }
  };

  const handleDelete = async (): Promise<void> => {
    if (!deleteTarget) return;
    setActionKey(`delete:${deleteTarget.id}`);
    try {
      const result = await deleteUser(deleteTarget.id);
      if (result.ok) {
        showToast(t('notify.userDeleted', { name: deleteTarget.username }), 'success');
        setDeleteTarget(null);
      } else {
        showToast(t('notify.adminUserActionFailed', { error: localizedError(result.error, t) }), 'error');
        setDeleteTarget(null);
        await reload();
      }
    } finally {
      setActionKey(null);
    }
  };

  const handleResetPassword = async (newPassword: string): Promise<void> => {
    if (!passwordTarget) return;
    setActionKey(`password:${passwordTarget.id}`);
    try {
      const result = await resetPassword(passwordTarget.id, newPassword);
      if (result.ok) {
        showToast(t('notify.userPasswordReset', { name: passwordTarget.username }), 'success');
        setPasswordTarget(null);
      } else {
        showToast(t('notify.adminUserActionFailed', { error: localizedError(result.error, t) }), 'error');
      }
    } finally {
      setActionKey(null);
    }
  };

  return (
    <>
      <AppModal
        open={open}
        onClose={onClose}
        title={t('adminUsers.modalTitle')}
        subtitle={t('adminUsers.modalSubtitle')}
        maxWidth="max-w-5xl"
        disableEscape={actionKey !== null}
      >
        <div className="flex flex-col gap-4">
          <section className="soft-panel-subtle p-4">
            <h3 className="text-sm font-semibold text-ink">{t('adminUsers.addUserTitle')}</h3>
            <div className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-[1fr_1fr_140px_auto]">
              <input
                type="text"
                className="input-field"
                placeholder={t('adminUsers.newUsername')}
                value={form.username}
                disabled={actionKey !== null}
                onChange={(event) => setForm((current) => ({ ...current, username: event.target.value }))}
              />
              <input
                type="password"
                className="input-field"
                placeholder={t('adminUsers.newPassword')}
                value={form.password}
                disabled={actionKey !== null}
                onChange={(event) => setForm((current) => ({ ...current, password: event.target.value }))}
              />
              <select
                className="input-field"
                value={form.role}
                disabled={actionKey !== null}
                onChange={(event) =>
                  setForm((current) => ({ ...current, role: event.target.value as UserRole }))
                }
                aria-label={t('adminUsers.newRole')}
              >
                <option value="user">{t('adminUsers.roleUser')}</option>
                <option value="admin">{t('adminUsers.roleAdmin')}</option>
              </select>
              <button
                type="button"
                className="primary-button"
                disabled={actionKey !== null}
                onClick={() => void handleCreate()}
              >
                {actionKey === 'create' ? t('action.submitting') : t('adminUsers.createButton')}
              </button>
            </div>
          </section>

          <div className="flex items-center gap-2">
            <input
              type="text"
              className="input-field max-w-xs"
              placeholder={t('adminUsers.searchPlaceholder')}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
            />
            <button
              type="button"
              className="muted-button"
              disabled={actionKey !== null || loading}
              onClick={() => void reload()}
            >
              {t('action.refreshUsers')}
            </button>
          </div>

          {loading ? (
            <div className="flex items-center gap-2 p-4 text-sm text-muted">
              <Spinner size={16} /> {t('adminUsers.loading')}
            </div>
          ) : filtered.length === 0 ? (
            <p className="p-4 text-sm text-muted">{t('adminUsers.empty')}</p>
          ) : (
            <div className="custom-scrollbar max-h-[46vh] overflow-auto rounded-2xl border border-line">
              <table className="w-full min-w-[760px] border-collapse text-sm">
                <thead>
                  <tr className="bg-panel-soft text-left text-[11px] uppercase tracking-wide text-muted">
                    <th className="px-4 py-2.5 font-semibold">{t('adminUsers.id')}</th>
                    <th className="px-4 py-2.5 font-semibold">{t('adminUsers.username')}</th>
                    <th className="px-4 py-2.5 font-semibold">{t('adminUsers.role')}</th>
                    <th className="px-4 py-2.5 font-semibold">{t('adminUsers.createdAt')}</th>
                    <th className="px-4 py-2.5 text-right font-semibold">{t('adminUsers.actions')}</th>
                  </tr>
                </thead>
                <tbody>
                  {filtered.map((entry) => (
                    <tr key={entry.id} className="border-t border-line">
                      <td className="px-4 py-2.5 font-mono text-xs text-muted">{entry.id}</td>
                      <td className="px-4 py-2.5 font-medium text-ink">{entry.username}</td>
                      <td className="px-4 py-2.5">
                        <select
                          className="input-field w-28 py-1.5 text-xs"
                          value={entry.role}
                          disabled={actionKey !== null}
                          onChange={(event) =>
                            void handleRoleChange(entry, event.target.value as UserRole)
                          }
                        >
                          <option value="user">{t('adminUsers.roleUser')}</option>
                          <option value="admin">{t('adminUsers.roleAdmin')}</option>
                        </select>
                      </td>
                      <td className="px-4 py-2.5 font-mono text-xs text-muted">
                        {formatDateTime(entry.created_at)}
                      </td>
                      <td className="px-4 py-2.5">
                        <div className="flex justify-end gap-2">
                          <button
                            type="button"
                            className="muted-button px-3 py-1.5 text-xs"
                            disabled={actionKey !== null}
                            onClick={() => setPasswordTarget(entry)}
                          >
                            {t('action.resetPassword')}
                          </button>
                          <button
                            type="button"
                            className="outlined-danger-button px-3 py-1.5 text-xs"
                            disabled={actionKey !== null}
                            onClick={() => setDeleteTarget(entry)}
                          >
                            {t('action.deleteUser')}
                          </button>
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </AppModal>

      <ConfirmDialog
        open={deleteTarget !== null}
        title={t('dialog.deleteUserTitle')}
        description={t('dialog.deleteUserDescription', { name: deleteTarget?.username ?? '' })}
        confirmTone="danger"
        busy={deleteTarget !== null && actionKey === `delete:${deleteTarget.id}`}
        onConfirm={() => void handleDelete()}
        onCancel={() => setDeleteTarget(null)}
      />

      <PromptDialog
        open={passwordTarget !== null}
        title={t('dialog.resetPasswordTitle')}
        description={t('dialog.resetPasswordDescription', { name: passwordTarget?.username ?? '' })}
        inputType="password"
        placeholder="******"
        maxLength={128}
        minLength={MIN_PASSWORD_LENGTH}
        required
        confirmLabel={t('action.resetPassword')}
        busy={passwordTarget !== null && actionKey === `password:${passwordTarget.id}`}
        onSubmit={(value) => void handleResetPassword(value)}
        onCancel={() => setPasswordTarget(null)}
      />
    </>
  );
}
