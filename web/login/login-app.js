const { useEffect, useState } = React;

const TOKEN_STORAGE_KEY = 'nexus_auth_token';

const LoginApp = () => {
    const [username, setUsername] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
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
        };

        checkSession();
    }, []);

    const handleSubmit = async (event) => {
        event.preventDefault();
        if (isSubmitting) return;

        const normalizedUsername = username.trim();
        if (!normalizedUsername || !password) {
            setError('Username and password are required.');
            return;
        }

        setError('');
        setIsSubmitting(true);

        try {
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
                if (response.status === 401) {
                    setError('Invalid username or password.');
                } else if (payload && payload.error) {
                    setError(`Login failed: ${payload.error}`);
                } else {
                    setError('Login failed. Please try again.');
                }
                return;
            }

            if (!payload || !payload.token) {
                setError('Login failed: empty token.');
                return;
            }

            localStorage.setItem(TOKEN_STORAGE_KEY, payload.token);
            window.location.href = '/dashboard';
        } catch (requestError) {
            setError('Unable to connect to server. Please retry.');
        } finally {
            setIsSubmitting(false);
        }
    };

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
                    <h1 className="text-2xl font-semibold tracking-tight text-[#1d1b18]">Welcome back</h1>
                    <p className="text-sm text-[#6d665d] mt-1.5">Sign in to access your cluster dashboard</p>
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
                                if (error) setError('');
                            }}
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
                            autoComplete="current-password"
                            value={password}
                            onChange={(event) => {
                                setPassword(event.target.value);
                                if (error) setError('');
                            }}
                            placeholder="Enter your password"
                            className="auth-input"
                        />
                    </div>

                    {error ? (
                        <div className="soft-panel-subtle px-3 py-2 text-xs error-text">{error}</div>
                    ) : null}

                    <button type="submit" disabled={isSubmitting} className="primary-button w-full py-2.5 text-sm font-medium">
                        {isSubmitting ? 'Signing in...' : 'Sign in'}
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
