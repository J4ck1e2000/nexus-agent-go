const { useState } = React;

const LoginApp = () => {
    const [email, setEmail] = useState('');
    const [password, setPassword] = useState('');
    const [error, setError] = useState('');
    const [isSubmitting, setIsSubmitting] = useState(false);

    const handleSubmit = (event) => {
        event.preventDefault();
        if (isSubmitting) return;
        setError('');
        setIsSubmitting(true);
        window.location.href = '/dashboard';
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
                        <label htmlFor="email" className="block text-[10px] uppercase tracking-[0.13em] font-semibold text-[#847c73] mb-1.5">
                            Email
                        </label>
                        <input
                            id="email"
                            type="email"
                            autoComplete="email"
                            value={email}
                            onChange={(event) => {
                                setEmail(event.target.value);
                                if (error) setError('');
                            }}
                            placeholder="you@company.com"
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

                <div className="mt-4 soft-panel-subtle demo-note px-3 py-2.5 text-xs">
                    Demo only - authentication is not connected yet
                </div>

                <div className="mt-5 text-[11px] text-[#777066] leading-relaxed">
                    Monitor distributed GPU nodes with a calm, real-time dashboard.
                </div>
            </section>
        </main>
    );
};

const root = ReactDOM.createRoot(document.getElementById('root'));
root.render(<LoginApp />);
