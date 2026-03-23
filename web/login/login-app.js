const { useEffect, useState } = React;

const TOKEN_STORAGE_KEY = 'nexus_auth_token';
const USER_STORAGE_KEY = 'nexus_auth_user';
const MODE_LOGIN = 'login';
const MODE_REGISTER = 'register';
const MIN_PASSWORD_LENGTH = 6;

const getLoginErrorMessage = (response, payload) => {
    if (response.status === 401 || (payload && payload.error === 'invalid_credentials')) {
        return 'Invalid username or password.';
    }
    if (payload && payload.error) {
        return `Login failed: ${payload.error}`;
    }
    return 'Login failed. Please try again.';
};

const getRegisterErrorMessage = (response, payload) => {
    const errorCode = payload && payload.error;
    if (response.status === 409 || errorCode === 'user_already_exists') {
        return 'Username already exists.';
    }
    if (response.status === 400 && errorCode === 'invalid_username') {
        return 'Username must be 3-64 characters and only contain letters, numbers, dot (.), underscore (_) or hyphen (-).';
    }
    if (response.status === 400 && errorCode === 'password_too_short') {
        return 'Password must be at least 6 characters.';
    }
    if (response.status === 400) {
        return 'Please check your username and password format.';
    }
    if (errorCode) {
        return `Registration failed: ${errorCode}`;
    }
    return 'Registration failed. Please try again.';
};

const LoginApp = () => {
    const [mode, setMode] = useState(MODE_LOGIN);
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [confirmPassword, setConfirmPassword] = useState('');
    const [error, setError] = useState('');
    const [success, setSuccess] = useState('');
    const [isSubmitting, setIsSubmitting] = useState(false);

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
        if (error) setError('');
        if (success) setSuccess('');
    };

    const switchMode = (nextMode) => {
        if (isSubmitting || mode === nextMode) return;
        setMode(nextMode);
        setPassword('');
        setConfirmPassword('');
        setError('');
        setSuccess('');
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
            setError('Login failed: empty token.');
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
            setError('Please confirm your password.');
            return;
        }
        if (password.length < MIN_PASSWORD_LENGTH) {
            setError('Password must be at least 6 characters.');
            return;
        }
        if (password !== confirmPassword) {
            setError('Password and confirmation do not match.');
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

        setSuccess('Registration successful. Please sign in.');
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
            setError('Username is required.');
            return;
        }
        if (!password) {
            setError('Password is required.');
            return;
        }
        if (mode === MODE_REGISTER && !confirmPassword) {
            setError('Please confirm your password.');
            return;
        }

        setUsername(normalizedUsername);
        setError('');
        setSuccess('');
        setIsSubmitting(true);

        try {
            if (mode === MODE_REGISTER) {
                await handleRegister(normalizedUsername);
            } else {
                await handleLogin(normalizedUsername);
            }
        } catch (requestError) {
            if (mode === MODE_REGISTER) {
                setError('Unable to connect to server. Please retry registration.');
            } else {
                setError('Unable to connect to server. Please retry.');
            }
        } finally {
            setIsSubmitting(false);
        }
    };

    const isRegisterMode = mode === MODE_REGISTER;
    const panelTitle = isRegisterMode ? 'Create account' : 'Welcome back';
    const panelSubtitle = isRegisterMode ? 'Register to access your cluster dashboard' : 'Sign in to access your cluster dashboard';
    const submitLabel = isRegisterMode ? 'Create account' : 'Sign in';
    const submittingLabel = isRegisterMode ? 'Creating account...' : 'Signing in...';
    const passwordPlaceholder = isRegisterMode ? 'At least 6 characters' : 'Enter your password';

    return (
        <main className="min-h-screen w-full flex items-center justify-center px-4 py-10">
            <section className="w-full max-w-[460px] soft-panel p-6 sm:p-8">
                <div className="flex items-center gap-3">
                    <div className="logo-chip h-10 px-3 flex items-center justify-center text-sm font-mono">NM</div>
                    <div className="min-w-0">
                        <div className="section-eyebrow mb-1">Nexus Monitor</div>
                        <div className="text-sm font-medium text-[#302c28]">Cluster Access</div>
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
                            Sign in
                        </button>
                        <button
                            type="button"
                            className={`mode-toggle-button ${isRegisterMode ? 'is-active' : ''}`}
                            onClick={() => switchMode(MODE_REGISTER)}
                            disabled={isSubmitting}
                        >
                            Register
                        </button>
                    </div>
                </div>

                <form className="mt-6 space-y-3.5" onSubmit={handleSubmit} noValidate>
                    <div>
                        <label htmlFor="username" className="block text-[10px] uppercase tracking-[0.13em] font-semibold text-[#847c73] mb-1.5">
                            Username
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
                            placeholder="admin"
                            className="auth-input"
                        />
                    </div>

                    <div>
                        <label htmlFor="password" className="block text-[10px] uppercase tracking-[0.13em] font-semibold text-[#847c73] mb-1.5">
                            Password
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
                                Confirm Password
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
                                placeholder="Re-enter your password"
                                className="auth-input"
                            />
                        </div>
                    ) : null}

                    {error ? (
                        <div className="soft-panel-subtle px-3 py-2 text-xs error-text">{error}</div>
                    ) : null}

                    {success ? (
                        <div className="soft-panel-subtle px-3 py-2 text-xs success-text">{success}</div>
                    ) : null}

                    <button type="submit" disabled={isSubmitting} className="primary-button w-full py-2.5 text-sm font-medium">
                        {isSubmitting ? submittingLabel : submitLabel}
                    </button>
                </form>

                <div className="mt-5 text-[11px] text-[#777066] leading-relaxed">
                    Monitor distributed GPU nodes with a calm, real-time dashboard.
                </div>
            </section>
        </main>
    );
};

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(<LoginApp />);
