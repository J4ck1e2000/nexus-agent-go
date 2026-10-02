import { useState, type FormEvent } from 'react';
import type { NexusError } from '../../electron/types/ipc';
import { useAuth } from '../hooks/useAuth';
import { useLanguage } from '../hooks/useLanguage';
import { useToastContext } from '../context/ToastContext';
import GatewaySettings from '../components/settings/GatewaySettings';
import LanguageToggle from '../components/common/LanguageToggle';

type Mode = 'login' | 'register';

function mapLoginError(error: NexusError, t: ReturnType<typeof useLanguage>['t']): string {
  if (error.code === 'unauthorized' || error.detail === 'invalid_credentials') {
    return t('login.errors.login.invalidCredentials');
  }
  if (error.detail) {
    return t('login.errors.login.withCode', { errorCode: error.detail });
  }
  return t('login.errors.login.generic');
}

function mapRegisterError(error: NexusError, t: ReturnType<typeof useLanguage>['t']): string {
  switch (error.detail) {
    case 'user_already_exists':
      return t('login.errors.register.userAlreadyExists');
    case 'invalid_username':
      return t('login.errors.register.invalidUsername');
    case 'password_too_short':
      return t('login.errors.register.passwordTooShort');
    default:
      break;
  }
  if (error.code === 'bad_request') {
    return t('login.errors.register.invalidInput');
  }
  if (error.detail) {
    return t('login.errors.register.withCode', { errorCode: error.detail });
  }
  return t('login.errors.register.generic');
}

export default function LoginPage() {
  const { t } = useLanguage();
  const { login, register } = useAuth();
  const { showToast } = useToastContext();

  const [mode, setMode] = useState<Mode>('login');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [success, setSuccess] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const [gatewayOpen, setGatewayOpen] = useState(false);

  const handleSubmit = async (event: FormEvent<HTMLFormElement>): Promise<void> => {
    event.preventDefault();
    setError(null);
    setSuccess(null);

    const name = username.trim();
    if (!name) {
      setError(t('login.validation.usernameRequired'));
      return;
    }
    if (!password) {
      setError(t('login.validation.passwordRequired'));
      return;
    }
    if (mode === 'register') {
      if (!confirmPassword) {
        setError(t('login.validation.confirmPasswordRequired'));
        return;
      }
      if (password.length < 6) {
        setError(t('login.validation.passwordTooShort'));
        return;
      }
      if (password !== confirmPassword) {
        setError(t('login.validation.passwordMismatch'));
        return;
      }
    }

    setSubmitting(true);
    try {
      if (mode === 'login') {
        const result = await login({ username: name, password });
        if (!result.ok) {
          setError(mapLoginError(result.error, t));
        }
        // On success AuthProvider flips to authenticated and the app navigates.
      } else {
        const result = await register({ username: name, password });
        if (!result.ok) {
          setError(mapRegisterError(result.error, t));
        } else {
          setMode('login');
          setPassword('');
          setConfirmPassword('');
          setSuccess(t('login.feedback.registrationSuccess'));
        }
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="relative flex min-h-screen items-center justify-center px-4 py-10">
      <div className="absolute top-4 right-4 flex items-center gap-2">
        <button
          type="button"
          className="muted-button"
          onClick={() => setGatewayOpen((open) => !open)}
          aria-expanded={gatewayOpen}
        >
          {t('desktop.loginGateway.button')}
        </button>
        <LanguageToggle />
      </div>

      {gatewayOpen && (
        <div className="absolute top-16 right-4 z-20 w-[min(92vw,460px)]">
          <GatewaySettings onSaved={() => showToast(t('desktop.loginGateway.saved'), 'success')} />
        </div>
      )}

      <div className="soft-panel w-full max-w-[460px] px-8 py-9">
        <div className="flex items-center gap-3">
          <div className="flex h-11 w-11 items-center justify-center rounded-2xl bg-ink-strong text-sm font-semibold tracking-wide text-[#f8f4ed]">
            NM
          </div>
          <div>
            <p className="eyebrow">{t('login.brand.tagline')}</p>
            <h1 className="mt-1 text-xl font-semibold tracking-tight text-ink">
              {t('login.brand.name')}
            </h1>
          </div>
        </div>

        <div className="mt-6">
          <h2 className="text-lg font-semibold text-ink">
            {mode === 'login' ? t('login.panel.loginTitle') : t('login.panel.registerTitle')}
          </h2>
          <p className="mt-1 text-sm text-muted">
            {mode === 'login'
              ? t('login.panel.loginSubtitle')
              : t('login.panel.registerSubtitle')}
          </p>
        </div>

        <div className="mt-5 flex items-center gap-0.5 self-start rounded-full border border-line bg-panel-soft p-1">
          {(['login', 'register'] as const).map((value) => (
            <button
              key={value}
              type="button"
              onClick={() => {
                setMode(value);
                setError(null);
                setSuccess(null);
              }}
              aria-pressed={mode === value}
              className={`cursor-pointer rounded-full px-4 py-1.5 text-xs font-medium transition ${
                mode === value ? 'bg-ink-strong text-[#f8f4ed]' : 'text-muted hover:text-ink'
              }`}
            >
              {t(`login.mode.${value}`)}
            </button>
          ))}
        </div>

        <form className="mt-5 flex flex-col gap-3" onSubmit={(event) => void handleSubmit(event)}>
          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted">{t('login.form.usernameLabel')}</span>
            <input
              type="text"
              className="input-field"
              value={username}
              autoComplete="username"
              placeholder={t('login.form.usernamePlaceholder')}
              onChange={(event) => setUsername(event.target.value)}
            />
          </label>

          <label className="flex flex-col gap-1.5">
            <span className="text-xs font-medium text-muted">{t('login.form.passwordLabel')}</span>
            <input
              type="password"
              className="input-field"
              value={password}
              autoComplete={mode === 'login' ? 'current-password' : 'new-password'}
              placeholder={
                mode === 'login'
                  ? t('login.form.passwordPlaceholderLogin')
                  : t('login.form.passwordPlaceholderRegister')
              }
              onChange={(event) => setPassword(event.target.value)}
            />
          </label>

          {mode === 'register' && (
            <label className="flex flex-col gap-1.5">
              <span className="text-xs font-medium text-muted">
                {t('login.form.confirmPasswordLabel')}
              </span>
              <input
                type="password"
                className="input-field"
                value={confirmPassword}
                autoComplete="new-password"
                placeholder={t('login.form.confirmPasswordPlaceholder')}
                onChange={(event) => setConfirmPassword(event.target.value)}
              />
            </label>
          )}

          {error && (
            <div className="soft-panel-subtle px-4 py-3 text-sm text-[#8b4b45]" role="alert">
              {error}
            </div>
          )}
          {success && (
            <div className="soft-panel-subtle px-4 py-3 text-sm text-success" role="status">
              {success}
            </div>
          )}

          <button type="submit" className="primary-button mt-1 w-full" disabled={submitting}>
            {submitting
              ? mode === 'login'
                ? t('login.submit.loggingIn')
                : t('login.submit.registering')
              : t(`login.submit.${mode}`)}
          </button>
        </form>

        <p className="mt-6 text-center text-xs text-muted">{t('login.footer.note')}</p>
      </div>
    </div>
  );
}
