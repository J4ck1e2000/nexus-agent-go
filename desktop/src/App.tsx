import { useState } from 'react';
import { AuthProvider, useAuthContext } from './context/AuthContext';
import { LanguageProvider, useLanguageContext } from './context/LanguageContext';
import { ToastProvider } from './context/ToastContext';
import AppShell from './components/layout/AppShell';
import ToastStack from './components/common/Toast';
import Spinner from './components/common/Spinner';
import LoginPage from './pages/LoginPage';
import SettingsPage from './pages/SettingsPage';
import DashboardPage from './pages/DashboardPage';

function Root() {
  const { status } = useAuthContext();
  const { t } = useLanguageContext();
  // Settings is an overlay; the dashboard stays mounted underneath so node
  // selection, filters and the AI assistant session all survive it.
  const [settingsOpen, setSettingsOpen] = useState(false);

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
    <>
      <AppShell onOpenSettings={() => setSettingsOpen(true)}>
        <DashboardPage />
      </AppShell>
      <SettingsPage open={settingsOpen} onClose={() => setSettingsOpen(false)} />
      <ToastStack />
    </>
  );
}

export default function App() {
  return (
    <LanguageProvider>
      <ToastProvider>
        <AuthProvider>
          <Root />
        </AuthProvider>
      </ToastProvider>
    </LanguageProvider>
  );
}
