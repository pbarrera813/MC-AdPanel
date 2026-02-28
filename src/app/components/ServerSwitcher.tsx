import React, { useState } from 'react';
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
  const [isOpen, setIsOpen] = useState(false);

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
    "w-full flex items-center justify-between gap-2 rounded-lg border border-[#3a3a3a] bg-[#252524]/95 text-gray-100 shadow-[0_6px_18px_rgba(0,0,0,0.25)] backdrop-blur-sm",
    variant === 'sidebar' ? "px-3 py-2 text-sm" : "px-3 py-1.5 text-xs",
    hasMultiple ? "hover:bg-[#2f2f2e] transition-colors duration-200" : "cursor-default",
    isOpen && hasMultiple ? "border-[#E5B80B]/60 ring-1 ring-[#E5B80B]/35" : "",
    className,
  );

  return (
    <DropdownMenu open={isOpen} onOpenChange={setIsOpen}>
      <DropdownMenuTrigger asChild disabled={!hasMultiple}>
        <button className={buttonClasses}>
          <div className="flex items-center gap-2 min-w-0">
            <div className={clsx("w-2 h-2 rounded-full shrink-0", statusDot)} />
            <span className="truncate">
              {activeServer ? activeServer.name : 'Select server'}
            </span>
          </div>
          {hasMultiple && (
            <ChevronDown
              size={14}
              className={clsx(
                "shrink-0 text-[#E5B80B] transition-transform duration-300 ease-out",
                isOpen ? "rotate-180" : "rotate-0"
              )}
            />
          )}
        </button>
      </DropdownMenuTrigger>
      {hasMultiple && (
        <DropdownMenuContent
          align={variant === 'header' ? 'start' : 'center'}
          className="bg-[#252524]/98 border border-[#3a3a3a] text-gray-200 w-[var(--radix-dropdown-menu-trigger-width)] max-h-40 rounded-lg p-1 shadow-[0_14px_40px_rgba(0,0,0,0.45)]"
        >
          {servers.map(server => (
            <DropdownMenuItem
              key={server.id}
              onSelect={() => setActiveServerId(server.id)}
              className="cursor-pointer rounded-md focus:bg-[#343433] focus:text-white data-[highlighted]:bg-[#343433] data-[highlighted]:text-white"
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
