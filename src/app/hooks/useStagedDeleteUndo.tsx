import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { RotateCcw, Trash2 } from 'lucide-react';
import { AnimatePresence, motion } from 'motion/react';
import { toast } from 'sonner';

const UNDO_WINDOW_MS = 3000;

type DeferredDeleteAction = {
  id: string;
  label: string;
  deadline: number;
  progressPercent: number;
  committing: boolean;
  onCommit: () => Promise<void>;
  onUndo: () => void;
  successMessage?: string;
  errorMessage?: string;
  undoMessage?: string;
};

type StageDeleteConfig = {
  label: string;
  onCommit: () => Promise<void>;
  onUndo: () => void;
  successMessage?: string;
  errorMessage?: string;
  undoMessage?: string;
};

const defaultErrorMessage = 'Failed to delete the selected item(s).';

export const useStagedDeleteUndo = () => {
  const [actions, setActions] = useState<DeferredDeleteAction[]>([]);
  const actionsRef = useRef<DeferredDeleteAction[]>([]);

  useEffect(() => {
    actionsRef.current = actions;
  }, [actions]);

  const commitAction = useCallback(async (id: string) => {
    const action = actionsRef.current.find((item) => item.id === id);
    if (!action || action.committing) return;

    setActions((prev) => prev.map((item) => (item.id === id ? { ...item, committing: true } : item)));

    try {
      await action.onCommit();
      if (action.successMessage) {
        toast.success(action.successMessage);
      }
    } catch (err) {
      action.onUndo();
      toast.error(action.errorMessage || defaultErrorMessage);
    } finally {
      setActions((prev) => prev.filter((item) => item.id !== id));
    }
  }, []);

  useEffect(() => {
    if (actions.length === 0) return;

    const timer = window.setInterval(() => {
      const now = Date.now();
      const currentActions = actionsRef.current;

      currentActions
        .filter((item) => !item.committing && now >= item.deadline)
        .forEach((item) => {
          void commitAction(item.id);
        });

      setActions((prev) =>
        prev.map((item) => ({
          ...item,
          progressPercent: item.committing
            ? 0
            : Math.max(0, Math.min(100, ((item.deadline - now) / UNDO_WINDOW_MS) * 100)),
        }))
      );
    }, 100);

    return () => {
      window.clearInterval(timer);
    };
  }, [actions.length, commitAction]);

  const stageDelete = useCallback((config: StageDeleteConfig) => {
    const id = `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;
    const deadline = Date.now() + UNDO_WINDOW_MS;
    setActions((prev) => [
      ...prev,
      {
        id,
        label: config.label,
        deadline,
        progressPercent: 100,
        committing: false,
        onCommit: config.onCommit,
        onUndo: config.onUndo,
        successMessage: config.successMessage,
        errorMessage: config.errorMessage,
        undoMessage: config.undoMessage,
      },
    ]);
  }, []);

  const undoAction = useCallback((id: string) => {
    const action = actionsRef.current.find((item) => item.id === id);
    if (!action || action.committing) return;

    action.onUndo();
    setActions((prev) => prev.filter((item) => item.id !== id));
    toast.info(action.undoMessage || 'Deletion undone.');
  }, []);

  const undoOverlay = useMemo(
    () => (
      <AnimatePresence>
        {actions.length > 0 && (
          <div className="fixed bottom-4 right-4 z-[80] flex flex-col gap-3 pointer-events-none">
            {actions.map((action) => (
              <motion.div
                key={action.id}
                initial={{ opacity: 0, y: 8, scale: 0.98 }}
                animate={{ opacity: 1, y: 0, scale: 1 }}
                exit={{ opacity: 0, y: 8, scale: 0.98 }}
                className="pointer-events-auto rounded-md border border-[#5b4a17] bg-[#1f1b11] px-4 py-3 shadow-xl min-w-[300px]"
              >
                <div className="flex items-start justify-between gap-3">
                  <div className="flex items-start gap-2 text-sm text-gray-200">
                    <Trash2 size={14} className="mt-0.5 text-[#E5B80B]" />
                    <div>
                      <p className="font-medium">{action.label} removed.</p>
                      <p className="text-xs text-gray-400">
                        {action.committing ? 'Deleting permanently...' : 'Undo available'}
                      </p>
                    </div>
                  </div>
                  <button
                    type="button"
                    disabled={action.committing}
                    onClick={() => undoAction(action.id)}
                    className="inline-flex items-center gap-1 rounded border border-[#E5B80B] px-2.5 py-1 text-xs font-bold text-[#E5B80B] hover:bg-[#E5B80B]/10 disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <RotateCcw size={12} />
                    Undo
                  </button>
                </div>
                <div className="mt-2 h-1.5 rounded bg-[#2f2f2f] overflow-hidden">
                  <div
                    className="h-full bg-[#7a7a7a] transition-[width] duration-100 ease-linear"
                    style={{ width: `${action.progressPercent}%` }}
                  />
                </div>
              </motion.div>
            ))}
          </div>
        )}
      </AnimatePresence>
    ),
    [actions, undoAction]
  );

  return { stageDelete, undoOverlay };
};
