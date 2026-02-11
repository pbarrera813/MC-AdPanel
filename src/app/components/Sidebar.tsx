import React from 'react';
import { Server, Terminal, Box, Database, FileText, Copy, Layers, Sliders } from 'lucide-react';
import { useServer } from '../context/ServerContext';
import clsx from 'clsx';
import minecraftLogo from '../../assets/4850776a1fc2c6be034672b47470d65273a8949d.png';
import { ServerSwitcher } from './ServerSwitcher';

type View = 'servers' | 'management' | 'plugins' | 'backups' | 'logs' | 'cloning' | 'settings';

interface SidebarProps {
  currentView: View;
  setCurrentView: (view: View) => void;
  className?: string;
}

export const Sidebar = ({ currentView, setCurrentView, className }: SidebarProps) => {
  const { activeServer } = useServer();

  const menuItems = [
    { id: 'servers', label: 'Servers', icon: Layers },
    { id: 'management', label: 'Management', icon: Terminal, disabled: !activeServer },
    { id: 'plugins', label: 'Plugins / Mods', icon: Box, disabled: !activeServer },
    { id: 'backups', label: 'Backups', icon: Database, disabled: !activeServer },
    { id: 'logs', label: 'Logs', icon: FileText, disabled: !activeServer },
    { id: 'cloning', label: 'Cloning', icon: Copy, disabled: !activeServer },
    { id: 'settings', label: 'System Settings', icon: Sliders },
  ] as const;

  return (
    <div className={clsx("w-64 h-full bg-[#202020] text-gray-300 flex flex-col border-r border-[#3a3a3a]", className)}>
      <div className="p-6 flex items-center gap-3 border-b border-[#3a3a3a]">
        <img src={minecraftLogo} alt="Minecraft Logo" className="w-8 h-8 rounded object-contain" />
        <h1 className="font-bold text-lg text-white tracking-wide">MC AdPanel</h1>
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
              className={clsx(
                "w-full flex items-center gap-3 px-3 py-3 rounded-md transition-colors text-sm font-medium",
                currentView === item.id 
                  ? "bg-gradient-to-br from-[#E5B80B] via-[#FCE38A] to-[#C49B09] text-black shadow-inner border border-[#FCE38A]/50" 
                  : "hover:bg-[#3a3a3a] text-gray-300",
                item.disabled && "opacity-50 cursor-not-allowed hover:bg-transparent"
              )}
            >
              <item.icon size={18} />
              {item.label}
            </button>
          ))}
        </nav>
      </div>

      <div className="p-4 border-t border-[#3a3a3a] text-xs text-center text-gray-600">
        v1.0.0 &copy; Pablo Barrera 2026
      </div>
    </div>
  );
};
