import React from 'react';
import { Server, Terminal, Box, Database, FileText, Copy, Layers, Sliders } from 'lucide-react';
import { useServer } from '../context/ServerContext';
import clsx from 'clsx';
import { AnimatePresence, motion } from 'motion/react';
import minecraftLogo from '../../assets/logo.png';
import { ServerSwitcher } from './ServerSwitcher';

type View = 'servers' | 'management' | 'plugins' | 'backups' | 'logs' | 'cloning' | 'settings';

interface SidebarProps {
  currentView: View;
  setCurrentView: (view: View) => void;
  className?: string;
}

export const Sidebar = ({ currentView, setCurrentView, className }: SidebarProps) => {
  const { activeServer } = useServer();
  const pluginsUnsupportedForType = activeServer?.type === 'Vanilla';

  const menuItems = [
    { id: 'servers', label: 'Servers', icon: Layers },
    { id: 'management', label: 'Management', icon: Terminal, disabled: !activeServer },
    { id: 'plugins', label: 'Plugins / Mods', icon: Box, disabled: !activeServer || pluginsUnsupportedForType, disabledReason: pluginsUnsupportedForType ? 'Not supported on this server type' : '' },
    { id: 'backups', label: 'Backups', icon: Database, disabled: !activeServer },
    { id: 'logs', label: 'Logs', icon: FileText, disabled: !activeServer },
    { id: 'cloning', label: 'Cloning', icon: Copy, disabled: !activeServer },
    { id: 'settings', label: 'System Settings', icon: Sliders },
  ] as const;

  return (
    <div className={clsx("w-64 h-full bg-[#202020] text-gray-300 flex flex-col border-r border-[#3a3a3a]", className)}>
      <div className="p-6 flex items-center gap-3 border-b border-[#3a3a3a]">
        <img src={minecraftLogo} alt="Minecraft Logo" className="w-8 h-8 rounded object-contain" />
        <h1 className="font-bold text-lg text-white tracking-wide">Orexa Panel</h1>
      </div>

      <div className="flex-1 py-4">
        <div className="px-6 mb-6">
          <div className="text-xs uppercase tracking-wider text-gray-500 mb-2">Active Server</div>
          <ServerSwitcher />
        </div>

        <nav className="space-y-1 px-3">
          {menuItems.map((item) => (
            <button
              key={item.id}
              onClick={() => !item.disabled && setCurrentView(item.id as View)}
              disabled={item.disabled}
              title={item.disabledReason || undefined}
              className={clsx(
                "relative w-full flex items-center gap-3 px-3 py-3 rounded-md text-sm font-medium overflow-hidden transition-colors",
                currentView === item.id 
                  ? "text-black"
                  : "hover:bg-[#3a3a3a] text-gray-300",
                item.disabled && "opacity-50 cursor-not-allowed hover:bg-transparent"
              )}
            >
              <AnimatePresence initial={false}>
                {currentView === item.id && (
                  <motion.span
                    layoutId="sidebar-active-item"
                    initial={{ opacity: 0.72, y: 6, scale: 0.985 }}
                    animate={{ opacity: 1, y: 0, scale: 1 }}
                    exit={{ opacity: 0.72, y: -6, scale: 0.985 }}
                    transition={{
                      layout: { type: 'spring', stiffness: 240, damping: 30, mass: 0.8 },
                      opacity: { duration: 0.24, ease: 'easeOut' },
                      y: { duration: 0.28, ease: 'easeOut' },
                      scale: { duration: 0.28, ease: 'easeOut' },
                    }}
                    className="absolute inset-0 rounded-md border border-[#FCE38A]/50 bg-gradient-to-br from-[#E5B80B] via-[#FCE38A] to-[#C49B09] shadow-inner"
                  />
                )}
              </AnimatePresence>
              <item.icon size={18} className="relative z-10" />
              <span className="relative z-10">{item.label}</span>
            </button>
          ))}
        </nav>
      </div>

      <div className="p-4 border-t border-[#3a3a3a] text-xs text-center text-gray-600">
        v1.0.1 &copy; Pablo Barrera 2026
      </div>
    </div>
  );
};

