import React, { useState } from 'react';
import { Toaster } from 'sonner';
import { ServerProvider } from './context/ServerContext';
import { Sidebar } from './components/Sidebar';
import { ServersPage } from './pages/ServersPage';
import { ManagementPage } from './pages/ManagementPage';
import { PluginsPage } from './pages/PluginsPage';
import { BackupsPage } from './pages/BackupsPage';
import { LogsPage } from './pages/LogsPage';
import { CloningPage } from './pages/CloningPage';
import { SystemSettingsPage } from './pages/SystemSettingsPage';
import { Sheet, SheetTrigger, SheetContent } from './components/ui/sheet';
import { Menu } from 'lucide-react';
import { ServerSwitcher } from './components/ServerSwitcher';

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
        <div className="flex-1 min-h-0 overflow-hidden flex flex-col">
          {renderView()}
        </div>
      </main>
    </div>
  );
}

export default function App() {
  return (
    <ServerProvider>
      <MainLayout />
      <Toaster theme="dark" position="bottom-right" richColors closeButton />
    </ServerProvider>
  );
}
