import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useServer, Plugin } from '../context/ServerContext';
import { Upload, Trash2, RefreshCw, AlertTriangle, CheckCircle, XCircle, Loader2, ArrowDownCircle, Search, Check, Square } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import { toast } from 'sonner';
import { Tooltip, TooltipTrigger, TooltipContent } from '../components/ui/tooltip';
import clsx from 'clsx';
import { useEscapeKey } from '../hooks/useEscapeKey';

interface PluginWithUpdate extends Plugin {
  latestVersion?: string;
  versionStatus?: 'latest' | 'outdated' | 'incompatible' | 'unknown';
  updateUrl?: string;
}

export const PluginsPage = () => {
  const { activeServer } = useServer();
  const activeServerId = activeServer?.id ?? null;
  const [plugins, setPlugins] = useState<PluginWithUpdate[]>([]);
  const [loading, setLoading] = useState(true);
  const [isUploadModalOpen, setIsUploadModalOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [uploading, setUploading] = useState(false);
  const [checkingUpdates, setCheckingUpdates] = useState(false);
  const [updatingPlugin, setUpdatingPlugin] = useState<string | null>(null);
  const [updatingAll, setUpdatingAll] = useState(false);
  const [updatesChecked, setUpdatesChecked] = useState(false);
  const [lastCheckedAt, setLastCheckedAt] = useState<Date | null>(null);
  const [restartPromptOpen, setRestartPromptOpen] = useState(false);
  const [updateAllPromptOpen, setUpdateAllPromptOpen] = useState(false);
  const [pendingUpdate, setPendingUpdate] = useState<PluginWithUpdate | null>(null);
  const [selectedPlugins, setSelectedPlugins] = useState<Set<string>>(new Set());
  const fileInputRef = useRef<HTMLInputElement>(null);

  const isServerOff = activeServer?.status === 'Stopped' || activeServer?.status === 'Crashed' || activeServer?.status === 'Error';

  const isModded = activeServer?.type === 'Forge' || activeServer?.type === 'Fabric' || activeServer?.type === 'NeoForge';
  const itemLabel = isModded ? 'mod' : 'plugin';
  const itemLabelPlural = isModded ? 'mods' : 'plugins';
  const itemLabelCap = isModded ? 'Mod' : 'Plugin';

  const fetchPlugins = useCallback(async (serverId?: string | null) => {
    const id = serverId ?? activeServerId;
    if (!id) return;
    try {
      const res = await fetch(`/api/servers/${id}/plugins`);
      if (!res.ok) throw new Error('Failed to fetch plugins');
      const data: PluginWithUpdate[] = await res.json();
      setPlugins(data || []);
    } catch (err) {
      console.error('Failed to fetch plugins:', err);
    } finally {
      setLoading(false);
    }
  }, [activeServerId]);

  useEffect(() => {
    fetchPlugins(activeServerId);
  }, [fetchPlugins, activeServerId]);

  useEffect(() => {
    setUpdatesChecked(false);
    setLastCheckedAt(null);
    setSelectedPlugins(new Set());
  }, [activeServerId]);

  useEscapeKey(isUploadModalOpen, () => setIsUploadModalOpen(false));
  useEscapeKey(!!deleteTarget, () => setDeleteTarget(null));
  useEscapeKey(restartPromptOpen, () => { setRestartPromptOpen(false); setPendingUpdate(null); });
  useEscapeKey(updateAllPromptOpen, () => setUpdateAllPromptOpen(false));

  const fetchUpdateResults = useCallback(async () => {
    if (!activeServerId) return [];
    const res = await fetch(`/api/servers/${activeServerId}/plugins/check-updates`);
    if (!res.ok) throw new Error('Failed to check updates');
    const results = await res.json();
    setPlugins(prev => prev.map(plugin => {
      const update = results.find((r: PluginWithUpdate) => r.fileName === plugin.fileName);
      if (update) {
        return {
          ...plugin,
          latestVersion: update.latestVersion,
          versionStatus: update.versionStatus,
          updateUrl: update.updateUrl,
        };
      }
      return plugin;
    }));
    setUpdatesChecked(true);
    setLastCheckedAt(new Date());
    return results as PluginWithUpdate[];
  }, [activeServerId]);

  // Restart handled manually by user after updates.

  const handleDelete = async (fileName: string) => {
    if (!activeServer) return;
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/plugins/${encodeURIComponent(fileName)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error(`Failed to delete ${itemLabel}`);
      toast.success(`${itemLabelCap} deleted`);
      setDeleteTarget(null);
      fetchPlugins();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to delete ${itemLabel}`);
    }
  };

  const handleToggle = async (fileName: string) => {
    if (!activeServer) return;
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/plugins/${encodeURIComponent(fileName)}/toggle`, {
        method: 'PUT',
      });
      if (!res.ok) throw new Error(`Failed to toggle ${itemLabel}`);
      toast.success(`${itemLabelCap} state changed. Restart server to apply.`);
      fetchPlugins();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to toggle ${itemLabel}`);
    }
  };

  const handleUpload = async (fileList: FileList | null) => {
    if (!fileList || fileList.length === 0 || !activeServer) return;
    setUploading(true);
    try {
      for (const file of Array.from(fileList)) {
        if (!file.name.endsWith('.jar')) {
          toast.error(`${file.name} is not a .jar file`);
          continue;
        }
        const formData = new FormData();
        formData.append('file', file);
        const res = await fetch(`/api/servers/${activeServer.id}/plugins`, {
          method: 'POST',
          body: formData,
        });
        if (!res.ok) throw new Error(`Failed to upload ${file.name}`);
      }
      toast.success(`${itemLabelCap}(s) uploaded successfully`);
      setIsUploadModalOpen(false);
      fetchPlugins();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Upload failed');
    } finally {
      setUploading(false);
    }
  };

  const handleCheckUpdates = async () => {
    if (!activeServer) return;
    setCheckingUpdates(true);
    try {
      await fetchUpdateResults();
      toast.success('Update check complete');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to check updates');
    } finally {
      setCheckingUpdates(false);
    }
  };

  const runSingleUpdate = async (plugin: PluginWithUpdate) => {
    if (!activeServer || !plugin.updateUrl) return;
    setUpdatingPlugin(plugin.fileName);
    toast.info(`Updating ${itemLabel}`);
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/plugins/${encodeURIComponent(plugin.fileName)}/update`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url: plugin.updateUrl }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to update plugin');
      }
      await res.json().catch(() => ({}));
      notifyUpdateComplete();
      fetchPlugins();
    } catch (err) {
      if (isServerOff) {
        toast.error('Update failed!');
      } else {
        toast.error(err instanceof Error ? err.message : 'Failed to update plugin');
      }
    } finally {
      setUpdatingPlugin(null);
      setPendingUpdate(null);
    }
  };

  const handleUpdate = (plugin: PluginWithUpdate) => {
    if (!activeServer || !plugin.updateUrl) return;
    if (isServerOff) {
      runSingleUpdate(plugin);
      return;
    }
    setPendingUpdate(plugin);
    setRestartPromptOpen(true);
  };

  const confirmSingleUpdate = async () => {
    if (!activeServer || !pendingUpdate || !pendingUpdate.updateUrl) {
      setRestartPromptOpen(false);
      return;
    }
    setRestartPromptOpen(false);
    await runSingleUpdate(pendingUpdate);
  };

  const handleUpdateAll = () => {
    if (!activeServer) return;
    if (isServerOff) {
      setUpdateAllPromptOpen(false);
      confirmUpdateAll();
      return;
    }
    setUpdateAllPromptOpen(true);
  };

  const confirmUpdateAll = async () => {
    if (!activeServer) {
      setUpdateAllPromptOpen(false);
      return;
    }

    setUpdateAllPromptOpen(false);
    setUpdatingAll(true);
    try {
      const results = await fetchUpdateResults();
      let outdated = results.filter(p => p.versionStatus === 'outdated' && p.updateUrl);

      // If plugins were selected, only update selected ones
      if (selectedPlugins.size > 0) {
        outdated = outdated.filter(p => selectedPlugins.has(p.fileName));
      }

      if (outdated.length === 0) {
        toast.info(`No outdated ${itemLabelPlural} found`);
        return;
      }
      toast.info(selectedPlugins.size > 0 ? `Updating ${outdated.length} selected.` : 'Updating all.');

      const failed: string[] = [];
      let updatedCount = 0;
      for (const plugin of outdated) {
        try {
          const res = await fetch(`/api/servers/${activeServer.id}/plugins/${encodeURIComponent(plugin.fileName)}/update`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ url: plugin.updateUrl }),
          });
          if (!res.ok) {
            const data = await res.json().catch(() => ({}));
            throw new Error(data.error || 'Failed to update plugin');
          }
          updatedCount += 1;
        } catch (err) {
          failed.push(plugin.name || plugin.fileName);
        }
      }

      if (failed.length > 0) {
        if (isServerOff) {
          toast.error('Update failed!');
        } else {
          toast.error(`Failed to update: ${failed.join(', ')}`);
        }
      }
      if (updatedCount > 0 && failed.length === 0) {
        notifyUpdateComplete();
      }

      setSelectedPlugins(new Set());
      fetchPlugins();
    } catch (err) {
      if (isServerOff) {
        toast.error('Update failed!');
      } else {
        toast.error(err instanceof Error ? err.message : `Failed to update ${itemLabelPlural}`);
      }
    } finally {
      setUpdatingAll(false);
    }
  };

  const notifyUpdateComplete = () => {
    if (isServerOff) {
      toast.success('Update successful');
      return;
    }
    toast.success('Update complete. A server restart is required for the updates to take effect.');
  };

  const handleTogglePlugin = (fileName: string) => {
    setSelectedPlugins(prev => {
      const next = new Set(prev);
      if (next.has(fileName)) next.delete(fileName);
      else next.add(fileName);
      return next;
    });
  };

  const handleToggleAllPlugins = () => {
    if (selectedPlugins.size === plugins.length) {
      setSelectedPlugins(new Set());
    } else {
      setSelectedPlugins(new Set(plugins.map(p => p.fileName)));
    }
  };

  const handleUpdateSelected = async () => {
    if (!activeServer || selectedPlugins.size === 0) return;

    // If updates haven't been checked yet, check first
    if (!updatesChecked) {
      setCheckingUpdates(true);
      try {
        await fetchUpdateResults();
        toast.success('Update check complete');
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to check updates');
      } finally {
        setCheckingUpdates(false);
      }
      return;
    }

    const selectedOutdated = plugins.filter(
      p => selectedPlugins.has(p.fileName) && p.versionStatus === 'outdated' && p.updateUrl
    );

    if (selectedOutdated.length === 0) {
      toast.info(`No outdated ${itemLabelPlural} in selection`);
      return;
    }

    if (!isServerOff) {
      setUpdateAllPromptOpen(true);
      return;
    }

    setUpdatingAll(true);
    try {
      toast.info(`Updating ${selectedOutdated.length} ${selectedOutdated.length === 1 ? itemLabel : itemLabelPlural}`);
      const failed: string[] = [];
      let updatedCount = 0;
      for (const plugin of selectedOutdated) {
        try {
          const res = await fetch(`/api/servers/${activeServer.id}/plugins/${encodeURIComponent(plugin.fileName)}/update`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ url: plugin.updateUrl }),
          });
          if (!res.ok) throw new Error('Failed');
          updatedCount += 1;
        } catch {
          failed.push(plugin.name || plugin.fileName);
        }
      }
      if (failed.length > 0) {
        toast.error(isServerOff ? 'Update failed!' : `Failed to update: ${failed.join(', ')}`);
      }
      if (updatedCount > 0 && failed.length === 0) {
        notifyUpdateComplete();
      }
      setSelectedPlugins(new Set());
      fetchPlugins();
    } catch (err) {
      toast.error(isServerOff ? 'Update failed!' : (err instanceof Error ? err.message : `Failed to update ${itemLabelPlural}`));
    } finally {
      setUpdatingAll(false);
    }
  };

  const renderVersionBadge = (plugin: PluginWithUpdate) => {
    if (!plugin.version) {
      return <span className="text-gray-600 text-sm">-</span>;
    }

    const status = plugin.versionStatus;
    const statusLabel = status === 'latest'
      ? { text: 'Latest', color: 'text-green-400' }
      : status === 'outdated'
        ? { text: 'Outdated!', color: 'text-yellow-400' }
        : status === 'incompatible'
          ? { text: 'Incompatible', color: 'text-red-400' }
          : null;

    return (
      <div className="flex items-center gap-2">
        {statusLabel ? (
          <Tooltip>
            <TooltipTrigger asChild>
              <span className="text-gray-300 text-sm font-mono cursor-help">{plugin.version}</span>
            </TooltipTrigger>
            <TooltipContent className={clsx("bg-[#252524] border border-[#3a3a3a] px-3 py-1.5", statusLabel.color)}>
              {statusLabel.text}
            </TooltipContent>
          </Tooltip>
        ) : (
          <span className="text-gray-300 text-sm font-mono">{plugin.version}</span>
        )}
      </div>
    );
  };

  if (!activeServer) {
    return <div className="flex items-center justify-center h-full text-gray-500">No server selected</div>;
  }

  const outdatedPlugins = plugins.filter(p => p.versionStatus === 'outdated' && p.updateUrl);
  const showUpdateAll = updatesChecked && outdatedPlugins.length > 1;
  const hasSelection = selectedPlugins.size > 0;
  const allSelected = hasSelection && selectedPlugins.size === plugins.length;

  // Dynamic button label based on selection
  const mainActionLabel = hasSelection
    ? (allSelected ? 'Update all' : `Update selected`)
    : (showUpdateAll ? 'Update all' : 'Check for updates');

  const mainActionHandler = hasSelection
    ? handleUpdateSelected
    : (showUpdateAll ? handleUpdateAll : handleCheckUpdates);

  return (
    <div className="flex-1 p-4 md:p-8 overflow-y-auto">
      <div className="flex flex-col md:flex-row md:justify-between md:items-center gap-4 mb-8">
        <div>
          <h2 className="text-3xl font-bold text-white mb-1">Plugins & Mods</h2>
          <p className="text-gray-400 text-sm">Manage installed extensions for {activeServer.name}</p>
        </div>
        <div className="flex flex-wrap gap-3 items-center">
          {lastCheckedAt && (
            <span className="text-xs text-gray-500">
              Last checked: {lastCheckedAt.toLocaleString()}
            </span>
          )}
          <button
            onClick={mainActionHandler}
            disabled={checkingUpdates || updatingAll}
            className="flex items-center gap-2 px-4 py-2 bg-[#252524] border border-[#404040] text-gray-200 rounded font-medium hover:bg-[#333] transition-colors disabled:opacity-50"
          >
            {checkingUpdates || updatingAll ? (
              <Loader2 size={18} className="animate-spin" />
            ) : hasSelection || showUpdateAll ? (
              <ArrowDownCircle size={18} />
            ) : (
              <Search size={18} />
            )}
            {checkingUpdates ? 'Checking...' : updatingAll ? 'Updating...' : mainActionLabel}
          </button>
          <button
            onClick={() => fetchPlugins()}
            className="flex items-center gap-2 px-4 py-2 bg-[#252524] border border-[#404040] text-gray-200 rounded font-medium hover:bg-[#333] transition-colors"
          >
            <RefreshCw size={18} /> Refresh
          </button>
          <button
            onClick={() => setIsUploadModalOpen(true)}
            className="flex items-center gap-2 bg-[#E5B80B] text-black px-4 py-2 rounded font-bold hover:bg-[#d4a90a] transition-colors"
          >
            <Upload size={20} />
            Upload {itemLabelCap}
          </button>
        </div>
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-20">
          <Loader2 size={32} className="animate-spin text-[#E5B80B]" />
        </div>
      ) : (
        <div className="bg-[#202020] border border-[#3a3a3a] rounded-lg overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-left min-w-[720px]">
              <thead className="bg-[#252524] text-gray-400 border-b border-[#3a3a3a]">
                <tr>
                  <th className="px-4 py-4 w-12">
                    {plugins.length > 0 && (
                      <span
                        onClick={handleToggleAllPlugins}
                        className={clsx('flex-shrink-0 cursor-pointer', selectedPlugins.size === plugins.length ? 'text-[#E5B80B]' : 'text-gray-600')}
                      >
                        {selectedPlugins.size === plugins.length ? <Check size={16} /> : <Square size={16} />}
                      </span>
                    )}
                  </th>
                  <th className="px-4 py-4 font-medium">Name</th>
                  <th className="px-4 py-4 font-medium">File</th>
                  <th className="px-4 py-4 font-medium">Version</th>
                  <th className="px-4 py-4 font-medium">Size</th>
                  <th className="px-4 py-4 font-medium">State</th>
                  <th className="px-4 py-4 font-medium text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-[#3a3a3a]">
                {plugins.map(plugin => (
                  <tr
                    key={plugin.fileName}
                    onClick={() => handleTogglePlugin(plugin.fileName)}
                    className={clsx(
                      "transition-colors group cursor-pointer",
                      selectedPlugins.has(plugin.fileName)
                        ? "bg-[#E5B80B]/5"
                        : "hover:bg-[#252524]"
                    )}
                  >
                    <td className="px-4 py-4 w-12">
                      <span className={clsx('flex-shrink-0', selectedPlugins.has(plugin.fileName) ? 'text-[#E5B80B]' : 'text-gray-600')}>
                        {selectedPlugins.has(plugin.fileName) ? <Check size={16} /> : <Square size={16} />}
                      </span>
                    </td>
                    <td className="px-4 py-4 font-medium text-white">{plugin.name}</td>
                    <td className="px-4 py-4 text-gray-400 font-mono text-sm">{plugin.fileName}</td>
                    <td className="px-4 py-4">{renderVersionBadge(plugin)}</td>
                    <td className="px-4 py-4 text-gray-400 text-sm">{plugin.size}</td>
                    <td className="px-4 py-4">
                      {plugin.versionStatus === 'incompatible' ? (
                        <span className="flex items-center gap-1.5 text-red-400 text-sm">
                          <AlertTriangle size={14} /> Incompatible!
                        </span>
                      ) : plugin.versionStatus === 'outdated' ? (
                        <span className="flex items-center gap-1.5 text-yellow-400 text-sm">
                          <AlertTriangle size={14} /> Outdated!
                        </span>
                      ) : plugin.enabled ? (
                        <span className="flex items-center gap-1.5 text-green-400 text-sm">
                          <CheckCircle size={14} /> Enabled
                        </span>
                      ) : (
                        <span className="flex items-center gap-1.5 text-gray-500 text-sm">
                          <XCircle size={14} /> Disabled
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-4 text-right">
                      <div className="flex items-center justify-end gap-2 opacity-60 group-hover:opacity-100 transition-opacity">
                        {plugin.versionStatus === 'outdated' && plugin.updateUrl && (
                          <button
                            onClick={(e) => { e.stopPropagation(); handleUpdate(plugin); }}
                            disabled={updatingPlugin === plugin.fileName || updatingAll}
                            className="p-2 hover:bg-yellow-900/30 text-yellow-400 rounded transition-colors disabled:opacity-50"
                            title={`Update to ${plugin.latestVersion || 'latest'}`}
                          >
                            {updatingPlugin === plugin.fileName ? <Loader2 size={18} className="animate-spin" /> : <ArrowDownCircle size={18} />}
                          </button>
                        )}
                        <button
                          onClick={(e) => { e.stopPropagation(); handleToggle(plugin.fileName); }}
                          className="p-2 hover:bg-[#333] text-gray-300 rounded"
                          title={plugin.enabled ? 'Disable' : 'Enable'}
                        >
                          {plugin.enabled ? <XCircle size={18} /> : <CheckCircle size={18} />}
                        </button>
                        <button
                          onClick={(e) => { e.stopPropagation(); setDeleteTarget(plugin.fileName); }}
                          className="p-2 hover:bg-red-900/30 text-red-400 rounded"
                          title="Delete"
                        >
                          <Trash2 size={18} />
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {plugins.length === 0 && <div className="p-8 text-center text-gray-500">No {itemLabelPlural} installed.</div>}
        </div>
      )}

      {/* Upload Modal */}
      <AnimatePresence>
        {isUploadModalOpen && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-lg bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6"
            >
              <h3 className="text-xl font-bold text-white mb-4">Upload {itemLabelCap}</h3>
              {uploading ? (
                <div className="h-48 flex flex-col items-center justify-center text-[#E5B80B]">
                  <Loader2 size={48} className="animate-spin mb-4" />
                  <p className="text-gray-300">Uploading...</p>
                </div>
              ) : (
                <div
                  className="border-2 border-dashed border-[#404040] rounded-lg h-48 flex flex-col items-center justify-center text-gray-400 mb-6 cursor-pointer hover:border-[#E5B80B] hover:text-[#E5B80B] transition-colors bg-[#202020]"
                  onClick={() => fileInputRef.current?.click()}
                  onDragOver={(e) => e.preventDefault()}
                  onDrop={(e) => { e.preventDefault(); handleUpload(e.dataTransfer.files); }}
                >
                  <input
                    ref={fileInputRef}
                    type="file"
                    accept=".jar"
                    multiple
                    className="hidden"
                    onChange={(e) => handleUpload(e.target.files)}
                  />
                  <Upload size={32} className="mb-2" />
                  <p>Drag & drop .jar file here</p>
                  <p className="text-xs text-gray-600 mt-2">or click to browse</p>
                </div>
              )}
              <div className="flex justify-end gap-3">
                <button
                  onClick={() => setIsUploadModalOpen(false)}
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                  disabled={uploading}
                >
                  Cancel
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

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
                <h3 className="text-xl font-bold">Delete {itemLabelCap}?</h3>
              </div>
              <p className="text-gray-300 mb-6">Are you sure you want to delete <span className="font-bold text-white">{deleteTarget}</span>? This action cannot be undone.</p>
              <div className="flex justify-end gap-3">
                <button onClick={() => setDeleteTarget(null)} className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium">Cancel</button>
                <button onClick={() => handleDelete(deleteTarget)} className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white rounded font-bold">Delete</button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Restart Needed (Single Update) */}
      <AnimatePresence>
        {restartPromptOpen && !isServerOff && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-md bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6"
            >
              <h3 className="text-xl font-bold text-white mb-3">Restart Required</h3>
              <p className="text-gray-300 mb-6">A server restart is required for the updates to take effect.</p>
              <div className="flex justify-end gap-3">
                <button
                  onClick={() => { setRestartPromptOpen(false); setPendingUpdate(null); }}
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                  disabled={updatingPlugin !== null}
                >
                  Cancel
                </button>
                <button
                  onClick={confirmSingleUpdate}
                  className="px-4 py-2 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded font-bold"
                  disabled={updatingPlugin !== null}
                >
                  Update
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Restart Needed (Update All) */}
      <AnimatePresence>
        {updateAllPromptOpen && !isServerOff && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-md bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6"
            >
              <h3 className="text-xl font-bold text-white mb-3">Restart Required</h3>
              <p className="text-gray-300 mb-6">A server restart is required for the updates to take effect.</p>
              <div className="flex justify-end gap-3">
                <button
                  onClick={() => setUpdateAllPromptOpen(false)}
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                  disabled={updatingAll}
                >
                  Cancel
                </button>
                <button
                  onClick={confirmUpdateAll}
                  className="px-4 py-2 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded font-bold"
                  disabled={updatingAll}
                >
                  {hasSelection && !allSelected ? 'Update selected' : 'Update all'}
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

    </div>
  );
};
