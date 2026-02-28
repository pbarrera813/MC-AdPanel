import React, { useState, useEffect } from 'react';
import { Toaster, toast } from 'sonner';
import { ServerProvider } from './context/ServerContext';
import { Sidebar } from './components/Sidebar';
import { ServersPage } from './pages/ServersPage';
import { ManagementPage } from './pages/ManagementPage';
import { PluginsPage } from './pages/PluginsPage';
import { BackupsPage } from './pages/BackupsPage';
import { LogsPage } from './pages/LogsPage';
import { CloningPage } from './pages/CloningPage';
import { SystemSettingsPage } from './pages/SystemSettingsPage';
import { LoginPage } from './pages/LoginPage';
import { Sheet, SheetTrigger, SheetContent } from './components/ui/sheet';
import { Menu } from 'lucide-react';
import { ServerSwitcher } from './components/ServerSwitcher';
import ErrorBoundary from './components/ErrorBoundary';
import { AnimatePresence, motion } from 'motion/react';

type View = 'servers' | 'management' | 'plugins' | 'backups' | 'logs' | 'cloning' | 'settings';

function MainLayout() {
  const [currentView, setCurrentView] = useState<View>('servers');
  const [mobileNavOpen, setMobileNavOpen] = useState(false);

  const viewLabels: Record<View, string> = {
    servers: 'Servers',
    management: 'Management',
    plugins: 'Plugins & Mods',
    backups: 'Backups',
    logs: 'Logs',
    cloning: 'Cloning',
    settings: 'System Settings',
  };

  const renderView = () => {
    switch (currentView) {
      case 'servers': return <ServersPage onViewChange={setCurrentView} />;
      case 'management': return <ManagementPage />;
      case 'plugins': return <PluginsPage />;
      case 'backups': return <BackupsPage />;
      case 'logs': return <LogsPage />;
      case 'cloning': return <CloningPage />;
      case 'settings': return <SystemSettingsPage />;
      default: return <ManagementPage />;
    }
  };

  return (
    <div className="flex w-full h-screen bg-[#2C2C2B] text-gray-200 overflow-hidden font-sans">
      <div className="hidden md:flex">
        <Sidebar currentView={currentView} setCurrentView={setCurrentView} />
      </div>
      <main className="flex-1 h-screen overflow-hidden flex flex-col relative bg-gradient-to-br from-[#3a3a39] via-[#2C2C2B] to-[#1e1e1d]">
        <div className="md:hidden flex items-center justify-between px-4 py-3 border-b border-[#3a3a3a] bg-[#202020]">
          <Sheet open={mobileNavOpen} onOpenChange={setMobileNavOpen}>
            <SheetTrigger asChild>
              <button className="p-2 rounded-md border border-[#3a3a3a] text-gray-200 hover:bg-[#2a2a29] transition-colors">
                <Menu size={18} />
              </button>
            </SheetTrigger>
            <SheetContent side="left" className="p-0 bg-[#202020] border-r border-[#3a3a3a]">
              <Sidebar
                currentView={currentView}
                setCurrentView={(view) => {
                  setCurrentView(view);
                  setMobileNavOpen(false);
                }}
                className="w-full border-r-0"
              />
            </SheetContent>
          </Sheet>
          <div className="flex flex-col items-center gap-1 min-w-0">
            <ServerSwitcher variant="header" className="max-w-[180px]" />
            <div className="text-[11px] text-gray-500">{viewLabels[currentView]}</div>
          </div>
          <div className="w-8" />
        </div>
        <div className="flex-1 min-h-0 overflow-hidden flex flex-col relative">
          <AnimatePresence mode="wait" initial={false}>
            <motion.div
              key={currentView}
              initial={{ opacity: 0, y: 8 }}
              animate={{ opacity: 1, y: 0 }}
              exit={{ opacity: 0, y: -8 }}
              transition={{ duration: 0.22, ease: 'easeOut' }}
              className="flex-1 min-h-0 overflow-hidden flex flex-col"
            >
              {renderView()}
            </motion.div>
          </AnimatePresence>
        </div>
      </main>
    </div>
  );
}

export default function App() {
  const [authChecked, setAuthChecked] = useState(false);
  const [authenticated, setAuthenticated] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const checkSession = async () => {
      try {
        const res = await fetch('/api/auth/session');
        if (!res.ok) throw new Error('Failed to verify session');
        const data = await res.json();
        if (!cancelled) setAuthenticated(Boolean(data.authenticated));
      } catch {
        if (!cancelled) setAuthenticated(false);
      } finally {
        if (!cancelled) setAuthChecked(true);
      }
    };
    checkSession();
    return () => { cancelled = true; };
  }, []);

  React.useEffect(() => {
    const handler = (ev: any) => {
      // surface global errors in console and optional toast so developer can copy stack
      // eslint-disable-next-line no-console
      console.error('Global error:', ev?.error || ev?.reason || ev);
      try { toast.error('An unexpected error occurred — check console for details'); } catch {}
    };

    window.addEventListener('error', handler);
    window.addEventListener('unhandledrejection', handler as any);
    return () => {
      window.removeEventListener('error', handler);
      window.removeEventListener('unhandledrejection', handler as any);
    };
  }, []);

  if (!authChecked) {
    return (
      <>
        <div className="w-full h-screen bg-gradient-to-br from-[#3a3a39] via-[#2C2C2B] to-[#1e1e1d] flex items-center justify-center text-gray-300">
          Loading...
        </div>
        <Toaster theme="dark" position="bottom-right" richColors closeButton />
      </>
    );
  }

  if (!authenticated) {
    return (
      <>
        <LoginPage onLoginSuccess={() => setAuthenticated(true)} />
        <Toaster theme="dark" position="bottom-right" richColors closeButton />
      </>
    );
  }

  return (
    <ServerProvider>
      <ErrorBoundary>
        <MainLayout />
      </ErrorBoundary>
      <Toaster theme="dark" position="bottom-right" richColors closeButton />
    </ServerProvider>
  );
}
