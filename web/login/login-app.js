const { useEffect, useState } = React;

const TOKEN_STORAGE_KEY = 'nexus_auth_token';
const USER_STORAGE_KEY = 'nexus_auth_user';
const LANG_STORAGE_KEY = 'nexus_auth_language';
const MODE_LOGIN = 'login';
const MODE_REGISTER = 'register';
const MIN_PASSWORD_LENGTH = 6;

const messages = {
    en: {
        page: {
            titleLogin: 'Nexus Monitor - Sign In',
            titleRegister: 'Nexus Monitor - Register',
        },
        brand: {
            name: 'Nexus Monitor',
            tagline: 'Cluster Access',
        },
        language: {
            label: 'Language',
            switchAriaLabel: 'Switch language',
            options: {
                en: 'English',
                zh: '中文',
            },
        },
        mode: {
            login: 'Sign in',
            register: 'Register',
        },
        panel: {
            loginTitle: 'Welcome back',
            loginSubtitle: 'Sign in to access your cluster dashboard',
            registerTitle: 'Create account',
            registerSubtitle: 'Register to access your cluster dashboard',
        },
        form: {
            usernameLabel: 'Username',
            passwordLabel: 'Password',
            confirmPasswordLabel: 'Confirm Password',
            usernamePlaceholder: 'Enter your username',
            passwordPlaceholderLogin: 'Enter your password',
            passwordPlaceholderRegister: 'At least 6 characters',
            confirmPasswordPlaceholder: 'Re-enter your password',
        },
        submit: {
            login: 'Sign in',
            register: 'Create account',
            loggingIn: 'Signing in...',
            registering: 'Creating account...',
        },
        footer: {
            note: 'Monitor distributed GPU nodes with a calm, real-time dashboard.',
        },
        validation: {
            usernameRequired: 'Username is required.',
            passwordRequired: 'Password is required.',
            confirmPasswordRequired: 'Please confirm your password.',
            passwordTooShort: 'Password must be at least 6 characters.',
            passwordMismatch: 'Password and confirmation do not match.',
        },
        feedback: {
            loginEmptyToken: 'Login failed: empty token.',
            networkLogin: 'Unable to connect to server. Please retry.',
            networkRegister: 'Unable to connect to server. Please retry registration.',
            registrationSuccess: 'Registration successful. Please sign in.',
        },
        errors: {
            login: {
                invalidCredentials: 'Invalid username or password.',
                generic: 'Login failed. Please try again.',
                withCode: ({ errorCode }) => `Login failed: ${errorCode}`,
            },
            register: {
                userAlreadyExists: 'Username already exists.',
                invalidUsername: 'Username must be 3-64 characters and only contain letters, numbers, dot (.), underscore (_) or hyphen (-).',
                passwordTooShort: 'Password must be at least 6 characters.',
                invalidInput: 'Please check your username and password format.',
                generic: 'Registration failed. Please try again.',
                withCode: ({ errorCode }) => `Registration failed: ${errorCode}`,
            },
        },
    },
    zh: {
        page: {
            titleLogin: 'Nexus Monitor - 登录',
            titleRegister: 'Nexus Monitor - 注册',
        },
        brand: {
            name: 'Nexus Monitor',
            tagline: '集群访问',
        },
        language: {
            label: '语言',
            switchAriaLabel: '切换语言',
            options: {
                en: 'English',
                zh: '中文',
            },
        },
        mode: {
            login: '登录',
            register: '注册',
        },
        panel: {
            loginTitle: '欢迎回来',
            loginSubtitle: '登录后即可访问你的集群仪表盘',
            registerTitle: '创建账号',
            registerSubtitle: '注册后即可访问你的集群仪表盘',
        },
        form: {
            usernameLabel: '用户名',
            passwordLabel: '密码',
            confirmPasswordLabel: '确认密码',
            usernamePlaceholder: '请输入用户名',
            passwordPlaceholderLogin: '请输入密码',
            passwordPlaceholderRegister: '至少 6 个字符',
            confirmPasswordPlaceholder: '请再次输入密码',
        },
        submit: {
            login: '登录',
            register: '创建账号',
            loggingIn: '登录中...',
            registering: '创建中...',
        },
        footer: {
            note: '使用沉稳、实时的面板监控分布式 GPU 节点。',
        },
        validation: {
            usernameRequired: '用户名不能为空。',
            passwordRequired: '密码不能为空。',
            confirmPasswordRequired: '请确认密码。',
            passwordTooShort: '密码至少需要 6 个字符。',
            passwordMismatch: '密码与确认密码不一致。',
        },
        feedback: {
            loginEmptyToken: '登录失败：令牌为空。',
            networkLogin: '无法连接服务器，请稍后重试。',
            networkRegister: '无法连接服务器，请稍后重试注册。',
            registrationSuccess: '注册成功，请登录。',
        },
        errors: {
            login: {
                invalidCredentials: '用户名或密码错误。',
                generic: '登录失败，请重试。',
                withCode: ({ errorCode }) => `登录失败：${errorCode}`,
            },
            register: {
                userAlreadyExists: '用户名已存在。',
                invalidUsername: '用户名长度需为 3-64 个字符，且只能包含字母、数字、点(.)、下划线(_)或连字符(-)。',
                passwordTooShort: '密码至少需要 6 个字符。',
                invalidInput: '请检查用户名和密码格式。',
                generic: '注册失败，请重试。',
                withCode: ({ errorCode }) => `注册失败：${errorCode}`,
            },
        },
    },
};

const getMessageValue = (catalog, key) => {
    return key.split('.').reduce((current, segment) => {
        if (current && Object.prototype.hasOwnProperty.call(current, segment)) {
            return current[segment];
        }
        return undefined;
    }, catalog);
};

const translate = (language, key, params = {}) => {
    const primaryCatalog = messages[language] || messages.en;
    const fallbackCatalog = messages.en;
    const value = getMessageValue(primaryCatalog, key) ?? getMessageValue(fallbackCatalog, key);

    if (typeof value === 'function') {
        return value(params);
    }
    if (typeof value === 'string') {
        return value;
    }
    return key;
};

const normalizeLanguage = (candidate) => {
    if (typeof candidate !== 'string') {
        return null;
    }
    const lower = candidate.toLowerCase();
    if (lower.startsWith('zh')) {
        return 'zh';
    }
    if (lower.startsWith('en')) {
        return 'en';
    }
    return null;
};

const getBrowserLanguage = () => {
    if (typeof navigator === 'undefined') {
        return 'en';
    }

    const browserLanguages = [];
    if (Array.isArray(navigator.languages)) {
        browserLanguages.push(...navigator.languages);
    }
    if (navigator.language) {
        browserLanguages.push(navigator.language);
    }

    const hasChinese = browserLanguages.some((candidate) => normalizeLanguage(candidate) === 'zh');
    return hasChinese ? 'zh' : 'en';
};

const getInitialLanguage = () => {
    try {
        const storedLanguage = localStorage.getItem(LANG_STORAGE_KEY);
        const normalizedStored = normalizeLanguage(storedLanguage);
        if (normalizedStored) {
            return normalizedStored;
        }
    } catch (e) {}

    return getBrowserLanguage();
};

const createFeedback = (key, params) => ({ key, params });

const getLoginErrorMessage = (response, payload) => {
    if (response.status === 401 || (payload && payload.error === 'invalid_credentials')) {
        return createFeedback('errors.login.invalidCredentials');
    }
    if (payload && payload.error) {
        return createFeedback('errors.login.withCode', { errorCode: payload.error });
    }
    return createFeedback('errors.login.generic');
};

const getRegisterErrorMessage = (response, payload) => {
    const errorCode = payload && payload.error;
    if (response.status === 409 || errorCode === 'user_already_exists') {
        return createFeedback('errors.register.userAlreadyExists');
    }
    if (response.status === 400 && errorCode === 'invalid_username') {
        return createFeedback('errors.register.invalidUsername');
    }
    if (response.status === 400 && errorCode === 'password_too_short') {
        return createFeedback('errors.register.passwordTooShort');
    }
    if (response.status === 400) {
        return createFeedback('errors.register.invalidInput');
    }
    if (errorCode) {
        return createFeedback('errors.register.withCode', { errorCode });
    }
    return createFeedback('errors.register.generic');
};

const LoginApp = () => {
    const [mode, setMode] = useState(MODE_LOGIN);
    const [language, setLanguage] = useState(getInitialLanguage);
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [confirmPassword, setConfirmPassword] = useState('');
    const [error, setError] = useState(null);
    const [success, setSuccess] = useState(null);
    const [isSubmitting, setIsSubmitting] = useState(false);

    const t = (key, params) => translate(language, key, params);

    useEffect(() => {
        try {
            localStorage.setItem(LANG_STORAGE_KEY, language);
        } catch (e) {}
    }, [language]);

    useEffect(() => {
        document.documentElement.lang = language === 'zh' ? 'zh-CN' : 'en';
        if (mode === MODE_REGISTER) {
            document.title = t('page.titleRegister');
            return;
        }
        document.title = t('page.titleLogin');
    }, [language, mode]);

    useEffect(() => {
        const token = localStorage.getItem(TOKEN_STORAGE_KEY);
        if (!token) return;

        const checkSession = async () => {
            try {
                const res = await fetch('/api/me', {
                    headers: {
                        Authorization: `Bearer ${token}`,
                    },
                });
                if (res.ok) {
                    window.location.href = '/dashboard';
                    return;
                }
            } catch (e) {}

            localStorage.removeItem(TOKEN_STORAGE_KEY);
            localStorage.removeItem(USER_STORAGE_KEY);
        };

        checkSession();
    }, []);

    const resetFeedback = () => {
        if (error) setError(null);
        if (success) setSuccess(null);
    };

    const switchMode = (nextMode) => {
        if (isSubmitting || mode === nextMode) return;
        setMode(nextMode);
        setPassword('');
        setConfirmPassword('');
        setError(null);
        setSuccess(null);
    };

    const switchLanguage = (nextLanguage) => {
        const normalized = normalizeLanguage(nextLanguage);
        if (!normalized || normalized === language) return;
        setLanguage(normalized);
    };

    const handleLogin = async (normalizedUsername) => {
        const response = await fetch('/api/login', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                username: normalizedUsername,
                password,
            }),
        });

        const payload = await response.json().catch(() => null);
        if (!response.ok) {
            setError(getLoginErrorMessage(response, payload));
            return;
        }

        if (!payload || !payload.token) {
            setError(createFeedback('feedback.loginEmptyToken'));
            return;
        }

        localStorage.setItem(TOKEN_STORAGE_KEY, payload.token);
        if (payload.user) {
            localStorage.setItem(USER_STORAGE_KEY, JSON.stringify(payload.user));
        } else {
            localStorage.removeItem(USER_STORAGE_KEY);
        }
        window.location.href = '/dashboard';
    };

    const handleRegister = async (normalizedUsername) => {
        if (!confirmPassword) {
            setError(createFeedback('validation.confirmPasswordRequired'));
            return;
        }
        if (password.length < MIN_PASSWORD_LENGTH) {
            setError(createFeedback('validation.passwordTooShort'));
            return;
        }
        if (password !== confirmPassword) {
            setError(createFeedback('validation.passwordMismatch'));
            return;
        }

        const response = await fetch('/api/register', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
            },
            body: JSON.stringify({
                username: normalizedUsername,
                password,
            }),
        });

        const payload = await response.json().catch(() => null);
        if (!response.ok) {
            setError(getRegisterErrorMessage(response, payload));
            return;
        }

        setSuccess(createFeedback('feedback.registrationSuccess'));
        setMode(MODE_LOGIN);
        setUsername(normalizedUsername);
        setPassword('');
        setConfirmPassword('');
    };

    const handleSubmit = async (event) => {
        event.preventDefault();
        if (isSubmitting) return;

        const normalizedUsername = username.trim();
        if (!normalizedUsername) {
            setError(createFeedback('validation.usernameRequired'));
            return;
        }
        if (!password) {
            setError(createFeedback('validation.passwordRequired'));
            return;
        }
        if (mode === MODE_REGISTER && !confirmPassword) {
            setError(createFeedback('validation.confirmPasswordRequired'));
            return;
        }

        setUsername(normalizedUsername);
        setError(null);
        setSuccess(null);
        setIsSubmitting(true);

        try {
            if (mode === MODE_REGISTER) {
                await handleRegister(normalizedUsername);
            } else {
                await handleLogin(normalizedUsername);
            }
        } catch (requestError) {
            if (mode === MODE_REGISTER) {
                setError(createFeedback('feedback.networkRegister'));
            } else {
                setError(createFeedback('feedback.networkLogin'));
            }
        } finally {
            setIsSubmitting(false);
        }
    };

    const isRegisterMode = mode === MODE_REGISTER;
    const panelTitle = isRegisterMode ? t('panel.registerTitle') : t('panel.loginTitle');
    const panelSubtitle = isRegisterMode ? t('panel.registerSubtitle') : t('panel.loginSubtitle');
    const submitLabel = isRegisterMode ? t('submit.register') : t('submit.login');
    const submittingLabel = isRegisterMode ? t('submit.registering') : t('submit.loggingIn');
    const passwordPlaceholder = isRegisterMode ? t('form.passwordPlaceholderRegister') : t('form.passwordPlaceholderLogin');

    return (
        <main className="min-h-screen w-full flex items-center justify-center px-4 py-10">
            <section className="w-full max-w-[460px] soft-panel p-6 sm:p-8">
                <div className="flex items-start justify-between gap-3">
                    <div className="flex items-center gap-3 min-w-0">
                        <div className="logo-chip h-10 px-3 flex items-center justify-center text-sm font-mono">NM</div>
                        <div className="min-w-0">
                            <div className="section-eyebrow mb-1">{t('brand.name')}</div>
                            <div className="text-sm font-medium text-[#302c28]">{t('brand.tagline')}</div>
                        </div>
                    </div>
                    <div className="shrink-0 text-right">
                        <div className="text-[10px] tracking-[0.12em] font-semibold text-[#847c73] mb-1">{t('language.label')}</div>
                        <div className="mode-toggle inline-flex" role="group" aria-label={t('language.switchAriaLabel')}>
                            <button
                                type="button"
                                className={`mode-toggle-button ${language === 'en' ? 'is-active' : ''}`}
                                onClick={() => switchLanguage('en')}
                            >
                                {t('language.options.en')}
                            </button>
                            <button
                                type="button"
                                className={`mode-toggle-button ${language === 'zh' ? 'is-active' : ''}`}
                                onClick={() => switchLanguage('zh')}
                            >
                                {t('language.options.zh')}
                            </button>
                        </div>
                    </div>
                </div>

                <div className="mt-5">
                    <h1 className="text-2xl font-semibold tracking-tight text-[#1d1b18]">{panelTitle}</h1>
                    <p className="text-sm text-[#6d665d] mt-1.5">{panelSubtitle}</p>
                    <div className="mt-4 mode-toggle inline-flex">
                        <button
                            type="button"
                            className={`mode-toggle-button ${!isRegisterMode ? 'is-active' : ''}`}
                            onClick={() => switchMode(MODE_LOGIN)}
                            disabled={isSubmitting}
                        >
                            {t('mode.login')}
                        </button>
                        <button
                            type="button"
                            className={`mode-toggle-button ${isRegisterMode ? 'is-active' : ''}`}
                            onClick={() => switchMode(MODE_REGISTER)}
                            disabled={isSubmitting}
                        >
                            {t('mode.register')}
                        </button>
                    </div>
                </div>

                <form className="mt-6 space-y-3.5" onSubmit={handleSubmit} noValidate>
                    <div>
                        <label htmlFor="username" className="block text-[10px] uppercase tracking-[0.13em] font-semibold text-[#847c73] mb-1.5">
                            {t('form.usernameLabel')}
                        </label>
                        <input
                            id="username"
                            type="text"
                            autoComplete="username"
                            value={username}
                            onChange={(event) => {
                                setUsername(event.target.value);
                                resetFeedback();
                            }}
                            disabled={isSubmitting}
                            placeholder={t('form.usernamePlaceholder')}
                            className="auth-input"
                        />
                    </div>

                    <div>
                        <label htmlFor="password" className="block text-[10px] uppercase tracking-[0.13em] font-semibold text-[#847c73] mb-1.5">
                            {t('form.passwordLabel')}
                        </label>
                        <input
                            id="password"
                            type="password"
                            autoComplete={isRegisterMode ? 'new-password' : 'current-password'}
                            value={password}
                            onChange={(event) => {
                                setPassword(event.target.value);
                                resetFeedback();
                            }}
                            disabled={isSubmitting}
                            placeholder={passwordPlaceholder}
                            className="auth-input"
                        />
                    </div>

                    {isRegisterMode ? (
                        <div>
                            <label htmlFor="confirmPassword" className="block text-[10px] uppercase tracking-[0.13em] font-semibold text-[#847c73] mb-1.5">
                                {t('form.confirmPasswordLabel')}
                            </label>
                            <input
                                id="confirmPassword"
                                type="password"
                                autoComplete="new-password"
                                value={confirmPassword}
                                onChange={(event) => {
                                    setConfirmPassword(event.target.value);
                                    resetFeedback();
                                }}
                                disabled={isSubmitting}
                                placeholder={t('form.confirmPasswordPlaceholder')}
                                className="auth-input"
                            />
                        </div>
                    ) : null}

                    {error ? (
                        <div className="soft-panel-subtle px-3 py-2 text-xs error-text">{t(error.key, error.params)}</div>
                    ) : null}

                    {success ? (
                        <div className="soft-panel-subtle px-3 py-2 text-xs success-text">{t(success.key, success.params)}</div>
                    ) : null}

                    <button type="submit" disabled={isSubmitting} className="primary-button w-full py-2.5 text-sm font-medium">
                        {isSubmitting ? submittingLabel : submitLabel}
                    </button>
                </form>

                <div className="mt-5 text-[11px] text-[#777066] leading-relaxed">{t('footer.note')}</div>
            </section>
        </main>
    );
};

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(<LoginApp />);
