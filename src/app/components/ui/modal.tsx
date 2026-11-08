import { useRef, type ReactNode } from 'react';
import { AnimatePresence, motion } from 'motion/react';
import { cn } from './utils';

const widths = { sm: 'max-w-sm', md: 'max-w-md', lg: 'max-w-lg', xl: 'max-w-4xl' } as const;
const tones = {
  neutral: 'border-[#404040]',
  danger: 'border-red-900/50',
  info: 'border-blue-900/50',
  accent: 'border-[#E5B80B]/30',
} as const;

interface ModalProps {
  open: unknown;
  size?: keyof typeof widths;
  tone?: keyof typeof tones;
  scope?: 'viewport' | 'container';
  stacked?: boolean;
  onBackdropClick?: () => void;
  className?: string;
  children: ReactNode;
}

export const Modal = ({
  open,
  size = 'md',
  tone = 'neutral',
  scope = 'viewport',
  stacked = false,
  onBackdropClick,
  className,
  children,
}: ModalProps) => {
  // Keep the last content while the exit animation plays, after callers have already cleared their state.
  const shown = useRef<ReactNode>(null);
  if (open) shown.current = children;

  return (
    <AnimatePresence>
      {!!open && (
        <div
          className={cn(
            'inset-0 flex items-center justify-center backdrop-blur-sm p-4',
            scope === 'container' ? 'absolute' : 'fixed',
            stacked ? 'z-[60] bg-black/70' : 'z-50 bg-black/60'
          )}
          onClick={onBackdropClick}
        >
          <motion.div
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.95 }}
            className={cn('w-full bg-[#252524] border rounded-lg shadow-2xl p-6', widths[size], tones[tone], className)}
            onClick={onBackdropClick ? (e) => e.stopPropagation() : undefined}
          >
            {shown.current}
          </motion.div>
        </div>
      )}
    </AnimatePresence>
  );
};
