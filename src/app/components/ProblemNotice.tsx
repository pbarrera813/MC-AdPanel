import { useState } from 'react';
import { AlertTriangle, ChevronDown } from 'lucide-react';
import clsx from 'clsx';
import type { ServerProblem } from '../context/ServerContext';

export const ProblemNotice = ({ problem, className }: { problem: ServerProblem; className?: string }) => {
  const [open, setOpen] = useState(false);

  return (
    <div
      role="alert"
      className={clsx('text-xs bg-red-900/10 border border-red-900/30 rounded p-2.5', className)}
      onClick={(e) => e.stopPropagation()}
    >
      <div className="flex items-start gap-2">
        <AlertTriangle size={14} className="mt-0.5 shrink-0 text-red-400" />
        <div className="min-w-0">
          <p className="font-semibold text-red-300">{problem.title}</p>
          <p className="mt-0.5 text-gray-300">{problem.hint}</p>
          {problem.detail && (
            <>
              <button
                type="button"
                aria-expanded={open}
                onClick={() => setOpen((v) => !v)}
                className="mt-1.5 flex items-center gap-1 text-[10px] font-medium text-gray-400 hover:text-[#E5B80B] transition-colors"
              >
                <ChevronDown size={12} className={clsx('transition-transform', open && 'rotate-180')} />
                {open ? 'Hide details' : 'Show details'}
              </button>
              {open && (
                <pre className="mt-1.5 max-h-24 overflow-auto whitespace-pre-wrap break-words rounded bg-[#1a1a1a] p-2 font-mono text-[11px] text-gray-400">
                  {problem.detail}
                </pre>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
};
