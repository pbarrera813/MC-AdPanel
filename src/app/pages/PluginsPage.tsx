import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useServer, Plugin } from '../context/ServerContext';
import { Upload, Trash2, RefreshCw, AlertTriangle, AlertCircle, CheckCircle, XCircle, Loader2, ArrowDownCircle, Cloud, Check, Square, Save, Pencil } from 'lucide-react';
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

interface StickyUpdateInfo {
  latestVersion?: string;
  versionStatus?: 'latest' | 'outdated' | 'incompatible' | 'unknown';
  updateUrl?: string;
  checkedVersion?: string;
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
  const [stickyUpdates, setStickyUpdates] = useState<Record<string, StickyUpdateInfo>>({});
  const [batchDeleteConfirmPlugins, setBatchDeleteConfirmPlugins] = useState(false);
  const [sourceDrafts, setSourceDrafts] = useState<Record<string, string>>({});
  const [sourceConfirmOpen, setSourceConfirmOpen] = useState(false);
  const [pendingSource, setPendingSource] = useState<{ fileName: string; url: string } | null>(null);
  const [savingSourceFor, setSavingSourceFor] = useState<string | null>(null);
  const [editingSources, setEditingSources] = useState<Set<string>>(new Set());

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
      // Merge sticky update info so "download update" buttons remain visible until user leaves
      const merged = (data || []).map(p => {
        const sticky = stickyUpdates[p.fileName];
        if (!sticky) return p;
        // Apply sticky info only if plugin version is unchanged since the check.
        if (sticky.checkedVersion && sticky.checkedVersion === p.version) {
          return { ...p, ...sticky };
        }
        return p;
      });
      setPlugins(merged);
    } catch (err) {
      console.error('Failed to fetch plugins:', err);
    } finally {
      setLoading(false);
    }
  }, [activeServerId, stickyUpdates]);

  useEffect(() => {
    fetchPlugins(activeServerId);
  }, [fetchPlugins, activeServerId]);

  useEffect(() => {
    setUpdatesChecked(false);
    setLastCheckedAt(null);
    setSelectedPlugins(new Set());
    setStickyUpdates({});
    setSourceDrafts({});
    setSourceConfirmOpen(false);
    setPendingSource(null);
    setEditingSources(new Set());
  }, [activeServerId]);

  useEscapeKey(isUploadModalOpen, () => setIsUploadModalOpen(false));
  useEscapeKey(!!deleteTarget, () => setDeleteTarget(null));
  useEscapeKey(restartPromptOpen, () => { setRestartPromptOpen(false); setPendingUpdate(null); });
  useEscapeKey(updateAllPromptOpen, () => setUpdateAllPromptOpen(false));
  useEscapeKey(sourceConfirmOpen, () => { setSourceConfirmOpen(false); setPendingSource(null); });

  const fetchUpdateResults = useCallback(async () => {
    if (!activeServerId) return [];
    const res = await fetch(`/api/servers/${activeServerId}/plugins/check-updates`);
    if (!res.ok) throw new Error('Failed to check updates');
    const results = await res.json();

    // Merge update info into current list and keep a sticky map so actions remain visible
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

    const sticky: Record<string, StickyUpdateInfo> = {};
    (results as PluginWithUpdate[]).forEach(r => {
      if (r.versionStatus === 'outdated' && r.updateUrl) {
        sticky[r.fileName] = {
          latestVersion: r.latestVersion,
          versionStatus: r.versionStatus,
          updateUrl: r.updateUrl,
          checkedVersion: r.version,
        };
      }
    });
    setStickyUpdates(sticky);

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

  const handleDisableSelected = async () => {
    if (!activeServer || selectedPlugins.size === 0) return;
    setUpdatingAll(true);
    try {
      const toDisable = plugins.filter(p => selectedPlugins.has(p.fileName) && p.enabled);
      if (toDisable.length === 0) {
        toast.info('No enabled items in selection');
        return;
      }
      for (const p of toDisable) {
        const res = await fetch(`/api/servers/${activeServer.id}/plugins/${encodeURIComponent(p.fileName)}/toggle`, { method: 'PUT' });
        if (!res.ok) throw new Error(`Failed to disable ${p.fileName}`);
      }
      toast.success(`Disabled ${toDisable.length} ${toDisable.length === 1 ? itemLabel : itemLabelPlural}`);
      setSelectedPlugins(new Set());
      fetchPlugins();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to disable selected');
    } finally {
      setUpdatingAll(false);
    }
  };

  const handleDeleteSelected = async () => {
    if (!activeServer || selectedPlugins.size === 0) return;
    try {
      for (const name of Array.from(selectedPlugins)) {
        const res = await fetch(`/api/servers/${activeServer.id}/plugins/${encodeURIComponent(name)}`, { method: 'DELETE' });
        if (!res.ok) throw new Error(`Failed to delete ${name}`);
      }
      toast.success(`${selectedPlugins.size} ${itemLabelPlural} deleted`);
      setSelectedPlugins(new Set());
      fetchPlugins();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete selected');
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
      setStickyUpdates(prev => {
        const next = { ...prev };
        delete next[plugin.fileName];
        return next;
      });
      notifyUpdateComplete();
      fetchPlugins();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update plugin');
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
    // Require server to be stopped before applying update to avoid corrupting plugin data
    if (!isServerOff) {
      toast.error('Stop the server before applying updates to avoid data corruption');
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

    // Require server to be stopped before applying updates
    if (!isServerOff) {
      toast.error('Stop the server before applying updates to avoid data corruption');
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
          setStickyUpdates(prev => {
            const next = { ...prev };
            delete next[plugin.fileName];
            return next;
          });
        } catch (err) {
          failed.push(plugin.name || plugin.fileName);
        }
      }

      if (failed.length > 0) {
        toast.error(`Failed to update: ${failed.join(', ')}`);
      }
      if (updatedCount > 0 && failed.length === 0) {
        notifyUpdateComplete();
      }

      setSelectedPlugins(new Set());
      fetchPlugins();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to update ${itemLabelPlural}`);
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
          setStickyUpdates(prev => {
            const next = { ...prev };
            delete next[plugin.fileName];
            return next;
          });
        } catch {
          failed.push(plugin.name || plugin.fileName);
        }
      }
      if (failed.length > 0) {
        toast.error(`Failed to update: ${failed.join(', ')}`);
      }
      if (updatedCount > 0 && failed.length === 0) {
        notifyUpdateComplete();
      }
      setSelectedPlugins(new Set());
      fetchPlugins();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to update ${itemLabelPlural}`);
    } finally {
      setUpdatingAll(false);
    }
  };

  const openSourceConfirmation = (plugin: PluginWithUpdate) => {
    const url = (sourceDrafts[plugin.fileName] ?? plugin.sourceUrl ?? '').trim();
    if (!url) {
      toast.error('Source link is required');
      return;
    }
    setPendingSource({ fileName: plugin.fileName, url });
    setSourceConfirmOpen(true);
  };

  const confirmSaveSource = async () => {
    if (!activeServer || !pendingSource) {
      setSourceConfirmOpen(false);
      setPendingSource(null);
      return;
    }

    setSavingSourceFor(pendingSource.fileName);
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/plugins/${encodeURIComponent(pendingSource.fileName)}/source`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ url: pendingSource.url }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to save source link');
      }

      toast.success('Source link saved');
      setSourceDrafts(prev => {
        const next = { ...prev };
        delete next[pendingSource.fileName];
        return next;
      });
      setEditingSources(prev => {
        const next = new Set(prev);
        next.delete(pendingSource.fileName);
        return next;
      });
      setSourceConfirmOpen(false);
      setPendingSource(null);
      fetchPlugins();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save source link');
    } finally {
      setSavingSourceFor(null);
    }
  };

  const startEditingSource = (plugin: PluginWithUpdate) => {
    setSourceDrafts(prev => ({ ...prev, [plugin.fileName]: prev[plugin.fileName] ?? plugin.sourceUrl ?? '' }));
    setEditingSources(prev => {
      const next = new Set(prev);
      next.add(plugin.fileName);
      return next;
    });
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
          : status === 'unknown'
            ? { text: 'Unknown', color: 'text-sky-400' }
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
  const hasSelection = selectedPlugins.size > 0;
  const allSelected = hasSelection && selectedPlugins.size === plugins.length;

  // Dynamic button label based on selection


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

          {/* Check for updates (cloud) */}
          <button
            onClick={handleCheckUpdates}
            disabled={checkingUpdates}
            className="flex items-center gap-2 px-4 py-2 bg-[#252524] border border-[#404040] text-gray-200 rounded font-medium hover:bg-[#333] transition-colors disabled:opacity-50"
          >
            {checkingUpdates ? <Loader2 size={18} className="animate-spin" /> : <Cloud size={18} />}
            {checkingUpdates ? 'Checking...' : 'Check for updates'}
          </button>

          {/* Update all / Update selected (appears after check or when selection exists) */}
          {(updatesChecked && outdatedPlugins.length > 0) || hasSelection ? (
            <button
              onClick={hasSelection ? handleUpdateSelected : handleUpdateAll}
              disabled={updatingAll}
              className="flex items-center gap-2 px-4 py-2 bg-[#252524] border border-[#404040] text-gray-200 rounded font-medium hover:bg-[#333] transition-colors disabled:opacity-50"
            >
              {updatingAll ? <Loader2 size={18} className="animate-spin" /> : <ArrowDownCircle size={18} />}
              {hasSelection ? (allSelected ? 'Update all' : 'Update selected') : 'Update all'}
            </button>
          ) : null}

          {/* Multi-actions when selection exists */}
          {hasSelection && (
            <>
              <button
                onClick={() => handleDisableSelected()}
                className="flex items-center gap-2 px-4 py-2 bg-[#333] border border-[#404040] text-gray-200 rounded font-medium hover:bg-[#444] transition-colors"
              >
                <XCircle size={16} /> Disable Selected ({selectedPlugins.size})
              </button>
              <button
                onClick={() => setBatchDeleteConfirmPlugins(true)}
                className="flex items-center gap-2 px-4 py-2 rounded font-bold border border-red-500 text-red-400 hover:bg-red-900/20 transition-colors"
              >
                <Trash2 size={16} /> Delete Selected ({selectedPlugins.size})
              </button>
            </>
          )}

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
                  <th className="px-4 py-4 font-medium">Source</th>
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
                    <td className="px-4 py-4">
                      {(plugin.sourceUrl && !editingSources.has(plugin.fileName)) ? (
                        <div className="flex items-center gap-2" onClick={(e) => e.stopPropagation()}>
                          <span className="text-xs text-gray-300">Source added.</span>
                          <Check size={14} className="text-[#E5B80B]" />
                          <button
                            onClick={() => startEditingSource(plugin)}
                            className="p-1.5 rounded border border-[#404040] text-gray-300 hover:bg-[#333]"
                            title="Edit source link"
                          >
                            <Pencil size={14} />
                          </button>
                        </div>
                      ) : (
                        <div className="flex items-center gap-2 min-w-[260px]" onClick={(e) => e.stopPropagation()}>
                          <input
                            type="text"
                            value={sourceDrafts[plugin.fileName] ?? plugin.sourceUrl ?? ''}
                            onChange={(e) => setSourceDrafts(prev => ({ ...prev, [plugin.fileName]: e.target.value }))}
                            placeholder={isModded ? 'Insert permanent link here' : 'Insert plugin link here'}
                            className="w-full bg-[#1a1a1a] border border-[#333] rounded px-2 py-1.5 text-xs text-gray-300 focus:outline-none focus:border-[#E5B80B]"
                          />
                          <button
                            onClick={() => openSourceConfirmation(plugin)}
                            disabled={savingSourceFor === plugin.fileName}
                            className="p-1.5 rounded border border-[#404040] text-gray-300 hover:bg-[#333] disabled:opacity-50"
                            title="Save source link"
                          >
                            {savingSourceFor === plugin.fileName ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
                          </button>
                          {plugin.sourceUrl && (
                            <button
                              onClick={() => startEditingSource(plugin)}
                              className="p-1.5 rounded border border-[#404040] text-gray-300 hover:bg-[#333]"
                              title="Edit source link"
                            >
                              <Pencil size={14} />
                            </button>
                          )}
                        </div>
                      )}
                    </td>
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
                      ) : plugin.versionStatus === 'unknown' ? (
                        <span className="flex items-center gap-1.5 text-sky-400 text-sm">
                          <AlertCircle size={14} /> Unknown
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
                          onClick={(e) => {
                            e.stopPropagation();
                            // If user has a multi-selection and this plugin is part of it, act on the whole selection
                            if (selectedPlugins.size > 0 && selectedPlugins.has(plugin.fileName)) {
                              handleDisableSelected();
                            } else {
                              handleToggle(plugin.fileName);
                            }
                          }}
                          className="p-2 hover:bg-[#333] text-gray-300 rounded"
                          title={plugin.enabled ? 'Disable' : 'Enable'}
                        >
                          {plugin.enabled ? <XCircle size={18} /> : <CheckCircle size={18} />}
                        </button>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            if (selectedPlugins.size > 0 && selectedPlugins.has(plugin.fileName)) {
                              setBatchDeleteConfirmPlugins(true);
                            } else {
                              setDeleteTarget(plugin.fileName);
                            }
                          }}
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

      {/* Batch Delete Selected Plugins */}
      <AnimatePresence>
        {batchDeleteConfirmPlugins && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4" onClick={() => setBatchDeleteConfirmPlugins(false)}>
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-md bg-[#252524] border border-red-900/50 rounded-lg shadow-2xl p-6"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="flex items-center gap-3 text-red-500 mb-4">
                <AlertTriangle size={24} />
                <h3 className="text-xl font-bold">Delete {selectedPlugins.size} {itemLabelCap}{selectedPlugins.size > 1 ? 's' : ''}?</h3>
              </div>
              <p className="text-gray-300 mb-6">Are you sure you want to delete the selected {itemLabelPlural}? This action cannot be undone.</p>
              <div className="flex justify-end gap-3">
                <button onClick={() => setBatchDeleteConfirmPlugins(false)} className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium">Cancel</button>
                <button onClick={() => { setBatchDeleteConfirmPlugins(false); handleDeleteSelected(); }} className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white rounded font-bold">Delete</button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Source Link Confirmation */}
      <AnimatePresence>
        {sourceConfirmOpen && pendingSource && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-md bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6"
            >
              <h3 className="text-xl font-bold text-white mb-3">Confirm source link</h3>
              <p className="text-gray-300">Are you sure this is the correct link?</p>
              <p className="text-xs text-gray-500 mb-3">This can be changed later.</p>
              <p className="text-xs text-[#E5B80B] break-all mb-6">{pendingSource.url}</p>
              <div className="flex justify-end gap-3">
                <button
                  onClick={() => { setSourceConfirmOpen(false); setPendingSource(null); }}
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                  disabled={savingSourceFor !== null}
                >
                  Cancel
                </button>
                <button
                  onClick={confirmSaveSource}
                  className="px-4 py-2 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded font-bold"
                  disabled={savingSourceFor !== null}
                >
                  Accept
                </button>
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
              <h3 className="text-xl font-bold text-white mb-3">Stop server to apply update</h3>
              <p className="text-gray-300 mb-6">Stop the server first to avoid corrupting plugin data. After stopping you can apply updates from this UI.</p>
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
              <h3 className="text-xl font-bold text-white mb-3">Stop server to apply updates</h3>
              <p className="text-gray-300 mb-6">Stop the server first to avoid corrupting plugin data. After stopping you can apply updates from this UI.</p>
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
