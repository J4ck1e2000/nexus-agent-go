import { useState } from 'react';
import { AuthProvider, useAuthContext } from './context/AuthContext';
import { LanguageProvider, useLanguageContext } from './context/LanguageContext';
import { ToastProvider } from './context/ToastContext';
import AppShell, { type AppView } from './components/layout/AppShell';
import ToastStack from './components/common/Toast';
import Spinner from './components/common/Spinner';
import LoginPage from './pages/LoginPage';
import SettingsPage from './pages/SettingsPage';
import DashboardPage from './pages/DashboardPage';

function Root() {
  const { status } = useAuthContext();
  const { t } = useLanguageContext();
  const [view, setView] = useState<AppView>('dashboard');

  if (status === 'loading') {
    return (
      <div className="flex min-h-screen flex-col items-center justify-center gap-3 text-muted">
        <Spinner size={26} />
        <p className="text-sm">{t('desktop.checkingSession')}</p>
      </div>
    );
  }

  if (status === 'guest') {
    return <LoginPage />;
  }

  return (
    <AppShell view={view} onNavigate={setView}>
      {view === 'settings' ? <SettingsPage /> : <DashboardPage />}
    </AppShell>
  );
}

export default function App() {
  return (
    <LanguageProvider>
      <ToastProvider>
        <AuthProvider>
          <Root />
          <ToastStack />
        </AuthProvider>
      </ToastProvider>
    </LanguageProvider>
  );
}
