import React, { useState, useEffect, useCallback } from 'react';
import { useServer, Backup } from '../context/ServerContext';
import { Archive, Clock, Download, Upload, Trash2, Plus, Loader2, AlertTriangle, CalendarClock, X } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import { format } from 'date-fns';
import { toast } from 'sonner';
import clsx from 'clsx';
import { useEscapeKey } from '../hooks/useEscapeKey';

export const BackupsPage = () => {
  const { activeServer } = useServer();
  const [backups, setBackups] = useState<Backup[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [restoreTarget, setRestoreTarget] = useState<string | null>(null);
  const [restoring, setRestoring] = useState(false);
  const [schedulePopup, setSchedulePopup] = useState(false);
  const [currentSchedule, setCurrentSchedule] = useState('');
  const [selectedSchedule, setSelectedSchedule] = useState('');
  const [nextBackup, setNextBackup] = useState<string | null>(null);
  const [selectedBackups, setSelectedBackups] = useState<Set<string>>(new Set());
  const [batchDeleteConfirm, setBatchDeleteConfirm] = useState(false);

  const fetchBackups = useCallback(async () => {
    if (!activeServer) return;
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/backups`);
      if (!res.ok) throw new Error('Failed to fetch backups');
      const data: Backup[] = await res.json();
      setBackups(data);
    } catch (err) {
      console.error('Failed to fetch backups:', err);
    } finally {
      setLoading(false);
    }
  }, [activeServer]);

  useEffect(() => {
    fetchBackups();
  }, [fetchBackups]);

  const handleCreateBackup = async () => {
    if (!activeServer) return;
    setCreating(true);
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/backups`, { method: 'POST' });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to create backup');
      }
      toast.success('Backup created successfully');
      fetchBackups();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Backup failed');
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (name: string) => {
    if (!activeServer) return;
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/backups/${encodeURIComponent(name)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error('Failed to delete backup');
      toast.success('Backup deleted');
      setDeleteTarget(null);
      setSelectedBackups(new Set());
      fetchBackups();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete');
    }
  };

  const handleRestore = async (name: string) => {
    if (!activeServer) return;
    setRestoring(true);
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/backups/${encodeURIComponent(name)}/restore`, {
        method: 'POST',
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to restore backup');
      }
      toast.success('Backup restored successfully');
      setRestoreTarget(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to restore');
    } finally {
      setRestoring(false);
    }
  };

  const handleDownload = (name: string) => {
    if (!activeServer) return;
    window.open(`/api/servers/${activeServer.id}/backups/${encodeURIComponent(name)}/download`, '_blank');
  };

  const fetchSchedule = useCallback(async () => {
    if (!activeServer) return;
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/backup-schedule`);
      if (!res.ok) return;
      const data = await res.json();
      setCurrentSchedule(data.schedule || '');
      setNextBackup(data.nextBackup || null);
    } catch { /* ignore */ }
  }, [activeServer?.id]);

  useEffect(() => {
    fetchSchedule();
  }, [fetchSchedule]);

  useEffect(() => {
    setSelectedBackups(new Set());
  }, [activeServer?.id]);

  useEscapeKey(!!deleteTarget, () => setDeleteTarget(null));
  useEscapeKey(!!restoreTarget, () => setRestoreTarget(null));
  useEscapeKey(schedulePopup, () => setSchedulePopup(false));
  useEscapeKey(batchDeleteConfirm, () => setBatchDeleteConfirm(false));

  const handleSaveSchedule = async () => {
    if (!activeServer) return;
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/backup-schedule`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ schedule: selectedSchedule }),
      });
      if (!res.ok) throw new Error('Failed to update schedule');
      const data = await res.json();
      setCurrentSchedule(selectedSchedule);
      setNextBackup(data.nextBackup || null);
      setSchedulePopup(false);
      if (selectedSchedule) {
        const labels: Record<string, string> = { daily: 'daily', weekly: 'weekly', monthly: 'monthly', sixmonths: 'every 6 months', yearly: 'yearly' };
        toast.success(`Backups scheduled ${labels[selectedSchedule]}`);
      } else {
        toast.success('Scheduled backups disabled');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update schedule');
    }
  };

  const handleToggleBackupSelect = (name: string) => {
    setSelectedBackups(prev => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  const handleBatchDelete = async () => {
    if (!activeServer || selectedBackups.size === 0) return;
    try {
      for (const name of selectedBackups) {
        const res = await fetch(`/api/servers/${activeServer.id}/backups/${encodeURIComponent(name)}`, { method: 'DELETE' });
        if (!res.ok) throw new Error(`Failed to delete ${name}`);
      }
      toast.success(`${selectedBackups.size} backup${selectedBackups.size > 1 ? 's' : ''} deleted`);
      setBatchDeleteConfirm(false);
      setSelectedBackups(new Set());
      fetchBackups();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete backups');
    }
  };

  if (!activeServer) {
    return <div className="flex items-center justify-center h-full text-gray-500">No server selected</div>;
  }

  return (
    <div className="flex-1 p-4 md:p-8 overflow-y-auto">
      <div className="flex flex-col md:flex-row md:justify-between md:items-start gap-4 mb-8">
        <div>
          <h2 className="text-3xl font-bold text-white mb-2">Backups</h2>
          <div className="flex items-center gap-4 text-sm text-gray-400">
            <span className="flex items-center gap-1.5">
              <Clock size={14} /> Last Backup: {backups.length > 0 ? format(new Date(backups[0].date), 'MMM d, HH:mm') : 'Never'}
            </span>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-3">
          {selectedBackups.size > 0 && (
            <button
              onClick={() => setBatchDeleteConfirm(true)}
              className="flex items-center gap-2 px-4 py-2 rounded font-bold border border-red-500 text-red-400 hover:bg-red-900/20 transition-colors"
            >
              <Trash2 size={18} />
              Delete Selected ({selectedBackups.size})
            </button>
          )}
          <button
            onClick={() => { setSelectedSchedule(currentSchedule); setSchedulePopup(true); }}
            className={clsx(
              "flex items-center gap-2 px-4 py-2 rounded font-bold border transition-colors",
              currentSchedule
                ? "border-[#E5B80B] text-[#E5B80B] hover:bg-[#E5B80B]/10"
                : "border-[#3a3a3a] text-gray-400 hover:border-gray-500 hover:text-white"
            )}
          >
            <CalendarClock size={18} />
            Schedule Backups
          </button>
          <button
            onClick={handleCreateBackup}
            disabled={creating}
            className="flex items-center gap-2 px-4 py-2 bg-[#E5B80B] text-black rounded font-bold hover:bg-[#d4a90a] disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {creating ? <Loader2 size={18} className="animate-spin" /> : <Plus size={18} />}
            {creating ? 'Creating...' : 'Create Backup'}
          </button>
        </div>
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-20">
          <Loader2 size={32} className="animate-spin text-[#E5B80B]" />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4">
          {backups.map((backup) => (
            <motion.div
              key={backup.name}
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              onClick={() => handleToggleBackupSelect(backup.name)}
              className={clsx(
                "bg-[#202020] border rounded-lg p-4 flex flex-col md:flex-row md:items-center md:justify-between gap-4 group cursor-pointer transition-colors",
                selectedBackups.has(backup.name)
                  ? "border-[#E5B80B] ring-1 ring-[#E5B80B] bg-[#E5B80B]/5"
                  : "border-[#3a3a3a] hover:border-gray-500"
              )}
            >
              <div className="flex items-center gap-4">
                <input
                  type="checkbox"
                  checked={selectedBackups.has(backup.name)}
                  onChange={() => handleToggleBackupSelect(backup.name)}
                  onClick={(e) => e.stopPropagation()}
                  className="accent-[#E5B80B] w-4 h-4 cursor-pointer flex-shrink-0"
                />
                <div className="w-12 h-12 bg-[#2a2a29] rounded flex items-center justify-center text-[#E5B80B]">
                  <Archive size={24} />
                </div>
                <div>
                  <div className="font-bold text-white text-lg">{format(new Date(backup.date), 'MMM d, yyyy')}</div>
                  <div className="text-sm text-gray-500 font-mono">{format(new Date(backup.date), 'HH:mm:ss')} &bull; {backup.size}</div>
                  <div className="text-xs text-gray-600 font-mono mt-0.5">{backup.name}</div>
                </div>
              </div>

              <div className="flex items-center gap-2 opacity-60 group-hover:opacity-100 transition-opacity md:self-auto self-end">
                <button
                  onClick={() => setRestoreTarget(backup.name)}
                  className="p-2 hover:bg-blue-900/30 text-blue-400 rounded"
                  title="Restore"
                >
                  <Upload size={20} />
                </button>
                <button
                  onClick={() => handleDownload(backup.name)}
                  className="p-2 hover:bg-[#333] text-gray-300 rounded"
                  title="Download"
                >
                  <Download size={20} />
                </button>
                <button
                  onClick={() => setDeleteTarget(backup.name)}
                  className="p-2 hover:bg-red-900/30 text-red-400 rounded"
                  title="Delete"
                >
                  <Trash2 size={20} />
                </button>
              </div>
            </motion.div>
          ))}
          {backups.length === 0 && (
            <div className="p-12 text-center border-2 border-dashed border-[#3a3a3a] rounded-lg text-gray-500">
              <Archive size={48} className="mx-auto mb-4 opacity-20" />
              <p>No backups found.</p>
            </div>
          )}
        </div>
      )}

      {/* Delete Confirmation Modal */}
      <AnimatePresence>
        {deleteTarget && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-md bg-[#252524] border border-red-900/50 rounded-lg shadow-2xl p-6"
            >
              <div className="flex items-center gap-3 text-red-500 mb-4">
                <AlertTriangle size={24} />
                <h3 className="text-xl font-bold">Delete Backup?</h3>
              </div>
              <p className="text-gray-300 mb-6">Are you sure you want to delete <span className="font-mono text-white">{deleteTarget}</span>? This action cannot be undone.</p>
              <div className="flex justify-end gap-3">
                <button onClick={() => setDeleteTarget(null)} className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium">Cancel</button>
                <button onClick={() => handleDelete(deleteTarget)} className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white rounded font-bold">Delete</button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Restore Confirmation Modal */}
      <AnimatePresence>
        {restoreTarget && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-md bg-[#252524] border border-blue-900/50 rounded-lg shadow-2xl p-6"
            >
              <div className="flex items-center gap-3 text-blue-400 mb-4">
                <Upload size={24} />
                <h3 className="text-xl font-bold">Restore Backup?</h3>
              </div>
              <p className="text-gray-300 mb-2">
                Are you sure you want to restore <span className="font-mono text-white">{restoreTarget}</span>?
              </p>
              <p className="text-yellow-400 text-sm mb-6">
                This will replace all current server files with the backup contents. The server must be stopped.
              </p>
              <div className="flex justify-end gap-3">
                <button
                  onClick={() => setRestoreTarget(null)}
                  disabled={restoring}
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                >
                  Cancel
                </button>
                <button
                  onClick={() => handleRestore(restoreTarget)}
                  disabled={restoring}
                  className="px-4 py-2 bg-blue-600 hover:bg-blue-500 text-white rounded font-bold flex items-center gap-2 disabled:opacity-50"
                >
                  {restoring && <Loader2 size={16} className="animate-spin" />}
                  {restoring ? 'Restoring...' : 'Restore'}
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Batch Delete Confirmation Modal */}
      <AnimatePresence>
        {batchDeleteConfirm && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={() => setBatchDeleteConfirm(false)}>
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-md bg-[#252524] border border-red-900/50 rounded-lg shadow-2xl p-6"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="flex items-center gap-3 text-red-500 mb-4">
                <AlertTriangle size={24} />
                <h3 className="text-xl font-bold">Delete {selectedBackups.size} Backup{selectedBackups.size > 1 ? 's' : ''}?</h3>
              </div>
              <p className="text-gray-300 mb-6">
                Are you sure you want to delete {selectedBackups.size} selected backup{selectedBackups.size > 1 ? 's' : ''}? This action cannot be undone.
              </p>
              <div className="flex justify-end gap-3">
                <button onClick={() => setBatchDeleteConfirm(false)} className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium">Cancel</button>
                <button onClick={handleBatchDelete} className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white rounded font-bold">Delete</button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Schedule Backups Popup */}
      <AnimatePresence>
        {schedulePopup && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={() => setSchedulePopup(false)}>
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-md bg-[#252524] border border-[#E5B80B]/30 rounded-lg shadow-2xl p-6"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="flex items-center justify-between mb-6">
                <div className="flex items-center gap-3 text-[#E5B80B]">
                  <CalendarClock size={24} />
                  <h3 className="text-xl font-bold text-white">Schedule Backups</h3>
                </div>
                <button onClick={() => setSchedulePopup(false)} className="text-gray-500 hover:text-white transition-colors">
                  <X size={20} />
                </button>
              </div>

              <div className="flex flex-col gap-2 mb-6">
                {([
                  { value: '', label: 'Disabled', desc: 'No automatic backups' },
                  { value: 'daily', label: 'Daily', desc: 'Every 24 hours' },
                  { value: 'weekly', label: 'Weekly', desc: 'Every 7 days' },
                  { value: 'monthly', label: 'Monthly', desc: 'Once a month' },
                  { value: 'sixmonths', label: 'Every 6 Months', desc: 'Twice a year' },
                  { value: 'yearly', label: 'Yearly', desc: 'Once a year' },
                ] as const).map(opt => (
                  <button
                    key={opt.value}
                    onClick={() => setSelectedSchedule(opt.value)}
                    className={clsx(
                      "px-4 py-3 rounded border text-left transition-all",
                      selectedSchedule === opt.value
                        ? "border-[#E5B80B] bg-[#E5B80B]/10 text-white"
                        : "border-[#3a3a3a] bg-[#1a1a1a] text-gray-400 hover:border-[#E5B80B]/40 hover:text-white"
                    )}
                  >
                    <div className="text-sm font-medium">{opt.label}</div>
                    <div className="text-xs text-gray-500 mt-0.5">{opt.desc}</div>
                  </button>
                ))}
              </div>

              {nextBackup && currentSchedule && (
                <p className="text-xs text-gray-500 mb-4">
                  Next backup: {format(new Date(nextBackup), 'MMM d, yyyy HH:mm')}
                </p>
              )}

              <div className="flex justify-end gap-3 pt-4 border-t border-[#3a3a3a]">
                <button
                  onClick={() => setSchedulePopup(false)}
                  className="px-4 py-2 rounded text-gray-400 hover:text-white hover:bg-[#3a3a3a] transition-colors"
                >
                  Cancel
                </button>
                <button
                  onClick={handleSaveSchedule}
                  className="px-4 py-2 rounded font-bold bg-[#E5B80B] text-black hover:bg-[#d4a90a] transition-colors"
                >
                  Accept
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>
    </div>
  );
};
