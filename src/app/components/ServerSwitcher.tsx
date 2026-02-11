import React from 'react';
import { useServer } from '../context/ServerContext';
import { DropdownMenu, DropdownMenuTrigger, DropdownMenuContent, DropdownMenuItem } from './ui/dropdown-menu';
import { ChevronDown, Check } from 'lucide-react';
import clsx from 'clsx';

type Variant = 'sidebar' | 'header';

interface ServerSwitcherProps {
  variant?: Variant;
  className?: string;
}

export const ServerSwitcher = ({ variant = 'sidebar', className }: ServerSwitcherProps) => {
  const { servers, activeServerId, setActiveServerId } = useServer();
  const activeServer = servers.find(s => s.id === activeServerId) || null;
  const hasMultiple = servers.length > 1;

  const statusDot = activeServer
    ? activeServer.status === 'Running'
      ? 'bg-green-500'
      : activeServer.status === 'Crashed' || activeServer.status === 'Error'
        ? 'bg-red-500'
        : activeServer.status === 'Booting' || activeServer.status === 'Installing'
          ? 'bg-yellow-500'
          : 'bg-gray-500'
    : 'bg-gray-500';

  const buttonClasses = clsx(
    "w-full flex items-center justify-between gap-2 rounded-md border border-[#3a3a3a] bg-[#252524] text-gray-200",
    variant === 'sidebar' ? "px-3 py-2 text-sm" : "px-3 py-1.5 text-xs",
    hasMultiple ? "hover:bg-[#2f2f2e] transition-colors" : "cursor-default",
    className,
  );

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild disabled={!hasMultiple}>
        <button className={buttonClasses}>
          <div className="flex items-center gap-2 min-w-0">
            <div className={clsx("w-2 h-2 rounded-full shrink-0", statusDot)} />
            <span className="truncate">
              {activeServer ? activeServer.name : 'Select server'}
            </span>
          </div>
          {hasMultiple && <ChevronDown size={14} className="text-gray-400 shrink-0" />}
        </button>
      </DropdownMenuTrigger>
      {hasMultiple && (
        <DropdownMenuContent
          align={variant === 'header' ? 'start' : 'center'}
          className="bg-[#252524] border border-[#3a3a3a] text-gray-200 w-64"
        >
          {servers.map(server => (
            <DropdownMenuItem
              key={server.id}
              onSelect={() => setActiveServerId(server.id)}
              className="cursor-pointer focus:bg-[#333] focus:text-white"
            >
              <div className={clsx(
                "w-2 h-2 rounded-full shrink-0",
                server.status === 'Running'
                  ? 'bg-green-500'
                  : server.status === 'Crashed' || server.status === 'Error'
                    ? 'bg-red-500'
                    : server.status === 'Booting' || server.status === 'Installing'
                      ? 'bg-yellow-500'
                      : 'bg-gray-500'
              )} />
              <span className="truncate flex-1">{server.name}</span>
              {server.id === activeServerId && <Check size={14} className="text-[#E5B80B]" />}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      )}
    </DropdownMenu>
  );
};
