import type { MouseEvent } from 'react';
import clsx from 'clsx';

interface MiniSwitchProps {
  checked: boolean;
  label: string;
  title: string;
  onClick?: (e: MouseEvent<HTMLButtonElement>) => void;
  disabled?: boolean;
}

export const MiniSwitch = ({ checked, label, title, onClick, disabled }: MiniSwitchProps) => (
  <button
    type="button"
    role="switch"
    aria-checked={checked}
    onClick={onClick}
    disabled={disabled}
    title={title}
    className="flex items-center gap-1.5 group/switch"
  >
    <div className={clsx('relative w-8 h-4 rounded-full transition-colors', checked ? 'bg-[#E5B80B]' : 'bg-[#3a3a3a]')}>
      <div className={clsx('absolute top-0.5 w-3 h-3 rounded-full transition-all', checked ? 'left-4.5 bg-black' : 'left-0.5 bg-gray-500')} />
    </div>
    <span className={clsx('text-[10px] font-medium transition-colors', checked ? 'text-[#E5B80B]' : 'text-gray-500 group-hover/switch:text-gray-400')}>
      {label}
    </span>
  </button>
);
