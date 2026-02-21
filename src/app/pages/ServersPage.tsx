import React, { useState, useEffect, useRef } from 'react';
import { useServer } from '../context/ServerContext';
import { Plus, Cpu, HardDrive, Play, Square, AlertTriangle, ArrowLeft, Check, ChevronDown, ChevronUp, Loader2, RotateCw, Power, Settings2, X, Trash2 } from 'lucide-react';
import { motion } from 'motion/react';
import { toast } from 'sonner';
import clsx from 'clsx';
import { useEscapeKey } from '../hooks/useEscapeKey';

interface ServersPageProps {
  onViewChange: (view: 'servers' | 'management' | 'plugins' | 'backups' | 'logs' | 'cloning') => void;
}

const SERVER_TYPES = [
  'Vanilla', 'Spigot', 'Paper', 'Folia', 'Purpur', 'Velocity', 'Forge', 'Fabric', 'NeoForge'
] as const;

interface VersionInfo {
  version: string;
  latest: boolean;
}

const compareVersionStrings = (a: string, b: string) => {
  const parse = (v: string) => v.split(/[^\d]+/).filter(Boolean).map(n => Number.parseInt(n, 10) || 0);
  const ap = parse(a);
  const bp = parse(b);
  const maxLen = Math.max(ap.length, bp.length);
  for (let i = 0; i < maxLen; i += 1) {
    const av = ap[i] ?? 0;
    const bv = bp[i] ?? 0;
    if (av !== bv) return av > bv ? 1 : -1;
  }
  return 0;
};

export const ServersPage = ({ onViewChange }: ServersPageProps) => {
  const { servers, setActiveServerId, startServer, stopServer, addServer, refreshServers, loading } = useServer();
  const [isCreating, setIsCreating] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [formData, setFormData] = useState({
    name: '',
    flags: 'none' as 'none' | 'aikars' | 'velocity' | 'modded',
    alwaysPreTouch: false,
    type: '',
    version: '',
    port: '25565',
    minRam: '0.5',
    maxRam: '1',
    maxPlayers: '20',
  });
  const [versions, setVersions] = useState<VersionInfo[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [typeVersionCatalog, setTypeVersionCatalog] = useState<Record<string, VersionInfo[]>>({});
  const [updatePopup, setUpdatePopup] = useState<{ serverId: string; serverName: string; currentVersion: string; selectedVersion: string; options: VersionInfo[] } | null>(null);
  const [updatingVersion, setUpdatingVersion] = useState(false);

  // Load system defaults for create form
  useEffect(() => {
    fetch('/api/settings')
      .then(res => res.json())
      .then(data => {
        setFormData(prev => ({
          ...prev,
          minRam: data.defaultMinRam || prev.minRam,
          maxRam: data.defaultMaxRam || prev.maxRam,
          flags: data.defaultFlags || prev.flags,
        }));
      })
      .catch(() => {});
  }, []);

  // Fetch versions dynamically when server type changes
  useEffect(() => {
    if (!formData.type) {
      setVersions([]);
      setFormData(prev => ({ ...prev, version: '' }));
      return;
    }
    setVersionsLoading(true);
    setFormData(prev => ({ ...prev, version: '' }));

    fetch(`/api/versions/${formData.type}`)
      .then(res => {
        if (!res.ok) throw new Error('Failed to fetch versions');
        return res.json();
      })
      .then((data: VersionInfo[]) => {
        setVersions(data);
        const latest = data.find(v => v.latest);
        if (latest) {
          setFormData(prev => ({ ...prev, version: latest.version }));
        } else if (data.length > 0) {
          setFormData(prev => ({ ...prev, version: data[0].version }));
        }
      })
      .catch(err => {
        console.error('Failed to fetch versions:', err);
        toast.error('Failed to load versions for ' + formData.type);
      })
      .finally(() => setVersionsLoading(false));
  }, [formData.type]);

  useEffect(() => {
    const serverTypes = Array.from(new Set(servers.map(s => s.type)));
    serverTypes.forEach((serverType) => {
      if (typeVersionCatalog[serverType]) return;
      fetch(`/api/versions/${serverType}`)
        .then(res => {
          if (!res.ok) throw new Error('Failed to fetch versions');
          return res.json();
        })
        .then((data: VersionInfo[]) => {
          setTypeVersionCatalog(prev => ({ ...prev, [serverType]: data }));
        })
        .catch(() => {
          setTypeVersionCatalog(prev => ({ ...prev, [serverType]: [] }));
        });
    });
  }, [servers, typeVersionCatalog]);

  const latestVersionForServerType = (serverType: string) => {
    const list = typeVersionCatalog[serverType];
    if (!list || list.length === 0) return '';
    return list.find(v => v.latest)?.version || list[0].version;
  };

  const serverHasNewerVersion = (server: typeof servers[number]) => {
    const latest = latestVersionForServerType(server.type);
    if (!latest) return false;
    return compareVersionStrings(latest, server.version) > 0;
  };

  const versionsAboveCurrent = (server: typeof servers[number]) => {
    const list = typeVersionCatalog[server.type] || [];
    return list.filter(v => compareVersionStrings(v.version, server.version) > 0);
  };

  const handleSelectServer = (id: string) => {
    // If rename handler consumed this click, skip toggling
    if (renameClickedRef.current) {
      renameClickedRef.current = false;
      return;
    }
    setSelectedServerIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleOpenServer = (id: string) => {
    setActiveServerId(id);
    onViewChange('management');
  };

  const handleDeleteServer = async () => {
    if (selectedServerIds.size === 0) return;
    try {
      for (const serverId of selectedServerIds) {
        const res = await fetch(`/api/servers/${serverId}`, { method: 'DELETE' });
        if (!res.ok) {
          const data = await res.json().catch(() => ({}));
          throw new Error(data.error || `Failed to delete server ${serverId}`);
        }
      }
      toast.success(selectedServerIds.size === 1 ? 'Server deleted permanently' : `${selectedServerIds.size} servers deleted permanently`);
      setSelectedServerIds(new Set());
      setDeleteConfirm(false);
      await refreshServers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete server');
      await refreshServers();
    }
  };

  const handleCreateSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!formData.type) return;

    setIsSubmitting(true);
    try {
      await addServer({
        name: formData.name || `My ${formData.type} Server`,
        type: formData.type as any,
        version: formData.version,
        port: parseInt(formData.port) || 25565,
        minRam: Math.round((parseFloat(formData.minRam) || 0.5) * 1024) + 'M',
        maxRam: Math.round((parseFloat(formData.maxRam) || 1) * 1024) + 'M',
        maxPlayers: parseInt(formData.maxPlayers) || 20,
        flags: formData.flags,
        alwaysPreTouch: formData.alwaysPreTouch,
      });
      toast.success('Server created! Installing server jar...');
      setIsCreating(false);
      setFormData({ name: '', flags: 'none', alwaysPreTouch: false, type: '', version: '', port: '25565', minRam: '0.5', maxRam: '1', maxPlayers: '20' });
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to create server');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleToggleStatus = async (e: React.MouseEvent, serverId: string, status: string) => {
    e.stopPropagation();
    try {
      if (status === 'Running') {
        await stopServer(serverId);
        toast.success('Server stopping...');
      } else {
        await startServer(serverId);
        toast.success('Server starting...');
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Action failed');
    }
  };

  const [selectedServerIds, setSelectedServerIds] = useState<Set<string>>(new Set());
  const [deleteConfirm, setDeleteConfirm] = useState(false);
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const renameInputRef = useRef<HTMLInputElement>(null);
  const renameClickedRef = useRef(false);

  const handleStartRename = (_e: React.MouseEvent, server: typeof servers[0]) => {
    if (selectedServerIds.has(server.id)) {
      // Signal the card handler to skip toggling selection
      renameClickedRef.current = true;
      setRenamingId(server.id);
      setRenameValue(server.name);
    }
  };

  useEffect(() => {
    if (renamingId && renameInputRef.current) {
      renameInputRef.current.focus();
      renameInputRef.current.select();
    }
  }, [renamingId]);

  const handleRenameSubmit = async () => {
    if (!renamingId) return;
    const trimmed = renameValue.trim();
    if (!trimmed) {
      setRenamingId(null);
      return;
    }
    try {
      const res = await fetch(`/api/servers/${renamingId}/name`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: trimmed }),
      });
      if (!res.ok) throw new Error('Failed to rename server');
      await refreshServers();
      toast.success('Server renamed');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to rename server');
    }
    setRenamingId(null);
  };

  const handleRenameKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      handleRenameSubmit();
    } else if (e.key === 'Escape') {
      setRenamingId(null);
    }
  };

  const [flagsPopup, setFlagsPopup] = useState<{ serverId: string; flags: string; alwaysPreTouch: boolean } | null>(null);

  const handleOpenFlagsPopup = (e: React.MouseEvent, server: typeof servers[0]) => {
    e.stopPropagation();
    setFlagsPopup({ serverId: server.id, flags: server.flags || 'none', alwaysPreTouch: server.alwaysPreTouch });
  };

  const handleSaveFlags = async () => {
    if (!flagsPopup) return;
    try {
      const res = await fetch(`/api/servers/${flagsPopup.serverId}/flags`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ flags: flagsPopup.flags, alwaysPreTouch: flagsPopup.alwaysPreTouch }),
      });
      if (!res.ok) throw new Error('Failed to update flags');
      await refreshServers();
      toast.success('JVM flags updated — changes will apply on next server restart');
      setFlagsPopup(null);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update flags');
    }
  };

  const handleOpenVersionPopup = async (e: React.MouseEvent, server: typeof servers[number]) => {
    e.stopPropagation();

    if (server.status === 'Running') {
      toast.error("Can't update while server is running.");
      return;
    }

    let list = typeVersionCatalog[server.type];
    if (!list) {
      try {
        const res = await fetch(`/api/versions/${server.type}`);
        if (!res.ok) throw new Error('Failed to fetch versions');
        list = await res.json();
        setTypeVersionCatalog(prev => ({ ...prev, [server.type]: list || [] }));
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to load versions');
        return;
      }
    }

    const options = (list || []).filter(v => compareVersionStrings(v.version, server.version) > 0);
    if (options.length === 0) {
      toast.info('Server is already on the latest version.');
      return;
    }

    setUpdatePopup({
      serverId: server.id,
      serverName: server.name,
      currentVersion: server.version,
      selectedVersion: options[0].version,
      options,
    });
  };

  const handleApplyVersionUpdate = async () => {
    if (!updatePopup) return;
    setUpdatingVersion(true);
    try {
      const res = await fetch(`/api/servers/${updatePopup.serverId}/version`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ version: updatePopup.selectedVersion }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to update server version');
      }
      toast.info(`Updating to ${updatePopup.selectedVersion}...`);
      setUpdatePopup(null);
      await refreshServers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update server version');
    } finally {
      setUpdatingVersion(false);
    }
  };

  const handleToggleAutoStart = async (e: React.MouseEvent, serverId: string, currentValue: boolean) => {
    e.stopPropagation();
    try {
      const res = await fetch(`/api/servers/${serverId}/auto-start`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ autoStart: !currentValue }),
      });
      if (!res.ok) throw new Error('Failed to update auto-start');
      await refreshServers();
      toast.success(!currentValue ? 'Auto-start enabled' : 'Auto-start disabled');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update auto-start');
    }
  };

  useEscapeKey(deleteConfirm, () => setDeleteConfirm(false));
  useEscapeKey(!!flagsPopup, () => setFlagsPopup(null));
  useEscapeKey(!!updatePopup, () => setUpdatePopup(null));

  if (isCreating) {
    return (
      <div className="flex-1 p-4 md:p-8 overflow-y-auto">
        <div className="max-w-4xl mx-auto">
          <div className="flex items-center gap-4 mb-8">
            <button
              onClick={() => setIsCreating(false)}
              className="p-2 rounded hover:bg-[#3a3a3a] text-gray-400 hover:text-white transition-colors"
            >
              <ArrowLeft size={24} />
            </button>
            <h2 className="text-3xl font-bold text-white">Create a Server</h2>
          </div>

          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            className="bg-[#202020] border border-[#E5B80B]/30 rounded-lg p-4 md:p-8 shadow-xl relative overflow-hidden"
          >
            {/* Metallic shine effect top border */}
            <div className="absolute top-0 left-0 right-0 h-1 bg-gradient-to-r from-[#E5B80B] via-[#FCE38A] to-[#C49B09]" />

            <form onSubmit={handleCreateSubmit} className="space-y-8">
              <div>
                <label className="block text-sm font-medium text-gray-400 mb-2">Server Name</label>
                <input
                  type="text"
                  value={formData.name}
                  onChange={(e) => setFormData({...formData, name: e.target.value})}
                  placeholder="Enter server name..."
                  className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B] transition-all"
                />
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-400 mb-2">JVM Flags</label>
                <div className="flex flex-wrap gap-3">
                  {([
                    { value: 'none', label: 'None', desc: 'Default JVM flags.' },
                    { value: 'aikars', label: "Aikar's Flags", desc: 'Optimized GC for game servers.' },
                    { value: 'velocity', label: 'Velocity Proxy', desc: 'Optimized for proxy servers.' },
                    { value: 'modded', label: 'Modded', desc: 'Recommended for modded servers.' },
                  ] as const).map(opt => (
                    <button
                      key={opt.value}
                      type="button"
                      onClick={() => setFormData({...formData, flags: opt.value, alwaysPreTouch: opt.value !== 'none' ? formData.alwaysPreTouch : false })}
                      className={clsx(
                        "flex-1 min-w-[140px] px-4 py-3 rounded border text-left transition-all",
                        formData.flags === opt.value
                          ? "border-[#E5B80B] bg-[#E5B80B]/10 text-white"
                          : "border-[#3a3a3a] bg-[#1a1a1a] text-gray-400 hover:border-[#E5B80B]/40 hover:text-white"
                      )}
                    >
                      <div className="text-sm font-medium">{opt.label}</div>
                      <div className="text-xs text-gray-500 mt-0.5">{opt.desc}</div>
                    </button>
                  ))}
                </div>
                {formData.flags !== 'none' && (
                  <div className="mt-3 flex items-center gap-3">
                    <button
                      type="button"
                      onClick={() => setFormData({...formData, alwaysPreTouch: !formData.alwaysPreTouch})}
                      className="flex items-center gap-2 group/apt"
                    >
                      <div className={clsx(
                        "relative w-8 h-4 rounded-full transition-colors",
                        formData.alwaysPreTouch ? "bg-[#E5B80B]" : "bg-[#3a3a3a]"
                      )}>
                        <div className={clsx(
                          "absolute top-0.5 w-3 h-3 rounded-full transition-all",
                          formData.alwaysPreTouch ? "left-4.5 bg-black" : "left-0.5 bg-gray-500"
                        )} />
                      </div>
                      <span className={clsx(
                        "text-sm transition-colors",
                        formData.alwaysPreTouch ? "text-[#E5B80B]" : "text-gray-500 group-hover/apt:text-gray-400"
                      )}>
                        AlwaysPreTouch
                      </span>
                    </button>
                    <span className="text-xs text-gray-600">Pre-allocates memory at startup — may cause issues on low-RAM systems</span>
                  </div>
                )}
              </div>

              <div className="grid grid-cols-1 md:grid-cols-3 gap-6">
                <div>
                  <label className="block text-sm font-medium text-gray-400 mb-2">Server Type</label>
                  <div className="relative">
                    <select
                      value={formData.type}
                      onChange={(e) => setFormData({...formData, type: e.target.value})}
                      className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white appearance-none cursor-pointer focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B] transition-all"
                    >
                      <option value="" disabled>Select Type</option>
                      {SERVER_TYPES.map(type => (
                        <option key={type} value={type}>{type}</option>
                      ))}
                    </select>
                    <ChevronDown className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 pointer-events-none" size={18} />
                  </div>
                </div>

                <div className={clsx("transition-opacity duration-200", !formData.type && "opacity-50 grayscale")}>
                  <label className="block text-sm font-medium text-gray-400 mb-2">Server Version</label>
                  <div className="relative">
                    <select
                      value={formData.version}
                      onChange={(e) => setFormData({...formData, version: e.target.value})}
                      disabled={!formData.type || versionsLoading}
                      className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white appearance-none cursor-pointer focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B] transition-all disabled:cursor-not-allowed"
                    >
                      {versionsLoading ? (
                        <option value="">Loading versions...</option>
                      ) : versions.length === 0 ? (
                        <option value="">Select a type first</option>
                      ) : (
                        versions.map(v => (
                          <option key={v.version} value={v.version}>
                            {v.version}{v.latest ? ' (Latest)' : ''}
                          </option>
                        ))
                      )}
                    </select>
                    {versionsLoading ? (
                      <Loader2 className="absolute right-3 top-1/2 -translate-y-1/2 text-[#E5B80B] animate-spin pointer-events-none" size={18} />
                    ) : (
                      <ChevronDown className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 pointer-events-none" size={18} />
                    )}
                  </div>
                </div>

                <div className={clsx("transition-opacity duration-200", !formData.type && "opacity-50 grayscale")}>
                  <label className="block text-sm font-medium text-gray-400 mb-2">Server Port</label>
                  <div className="relative group/input">
                    <input
                      type="number"
                      value={formData.port}
                      onChange={(e) => setFormData({...formData, port: e.target.value})}
                      disabled={!formData.type}
                      min={1024}
                      max={65535}
                      className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 pr-8 text-white focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B] transition-all disabled:cursor-not-allowed"
                    />
                    {formData.type && (
                      <div className="absolute right-0 top-0 bottom-0 w-7 flex flex-col border-l border-[#3a3a3a] group-focus-within/input:border-[#E5B80B]/50">
                        <button type="button" onClick={() => setFormData({...formData, port: String(Math.min(65535, (parseInt(formData.port) || 1024) + 1))})} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-tr"><ChevronUp size={14} /></button>
                        <button type="button" onClick={() => setFormData({...formData, port: String(Math.max(1024, (parseInt(formData.port) || 1024) - 1))})} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-br"><ChevronDown size={14} /></button>
                      </div>
                    )}
                  </div>
                </div>
              </div>

              <div className={clsx("grid grid-cols-1 md:grid-cols-3 gap-6 transition-opacity duration-200", !formData.type && "opacity-50 grayscale")}>
                <div>
                  <label className="block text-sm font-medium text-gray-400 mb-2">Min RAM (GB)</label>
                  <div className="relative group/input">
                    <input
                      type="number"
                      value={formData.minRam}
                      onChange={(e) => setFormData({...formData, minRam: e.target.value})}
                      disabled={!formData.type}
                      min={0.5}
                      max={64}
                      step={0.5}
                      className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 pr-8 text-white focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B] transition-all disabled:cursor-not-allowed"
                    />
                    {formData.type && (
                      <div className="absolute right-0 top-0 bottom-0 w-7 flex flex-col border-l border-[#3a3a3a] group-focus-within/input:border-[#E5B80B]/50">
                        <button type="button" onClick={() => setFormData({...formData, minRam: String(Math.min(64, (parseFloat(formData.minRam) || 0) + 0.5))})} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-tr"><ChevronUp size={14} /></button>
                        <button type="button" onClick={() => setFormData({...formData, minRam: String(Math.max(0.5, (parseFloat(formData.minRam) || 0) - 0.5))})} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-br"><ChevronDown size={14} /></button>
                      </div>
                    )}
                  </div>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-400 mb-2">Max RAM (GB)</label>
                  <div className="relative group/input">
                    <input
                      type="number"
                      value={formData.maxRam}
                      onChange={(e) => setFormData({...formData, maxRam: e.target.value})}
                      disabled={!formData.type}
                      min={0.5}
                      max={64}
                      step={0.5}
                      className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 pr-8 text-white focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B] transition-all disabled:cursor-not-allowed"
                    />
                    {formData.type && (
                      <div className="absolute right-0 top-0 bottom-0 w-7 flex flex-col border-l border-[#3a3a3a] group-focus-within/input:border-[#E5B80B]/50">
                        <button type="button" onClick={() => setFormData({...formData, maxRam: String(Math.min(64, (parseFloat(formData.maxRam) || 0) + 0.5))})} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-tr"><ChevronUp size={14} /></button>
                        <button type="button" onClick={() => setFormData({...formData, maxRam: String(Math.max(0.5, (parseFloat(formData.maxRam) || 0) - 0.5))})} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-br"><ChevronDown size={14} /></button>
                      </div>
                    )}
                  </div>
                </div>
                <div>
                  <label className="block text-sm font-medium text-gray-400 mb-2">Max Players</label>
                  <div className="relative group/input">
                    <input
                      type="number"
                      value={formData.maxPlayers}
                      onChange={(e) => setFormData({...formData, maxPlayers: e.target.value})}
                      disabled={!formData.type}
                      min={1}
                      max={1000}
                      className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 pr-8 text-white focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B] transition-all disabled:cursor-not-allowed"
                    />
                    {formData.type && (
                      <div className="absolute right-0 top-0 bottom-0 w-7 flex flex-col border-l border-[#3a3a3a] group-focus-within/input:border-[#E5B80B]/50">
                        <button type="button" onClick={() => setFormData({...formData, maxPlayers: String(Math.min(1000, (parseInt(formData.maxPlayers) || 0) + 1))})} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-tr"><ChevronUp size={14} /></button>
                        <button type="button" onClick={() => setFormData({...formData, maxPlayers: String(Math.max(1, (parseInt(formData.maxPlayers) || 0) - 1))})} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-br"><ChevronDown size={14} /></button>
                      </div>
                    )}
                  </div>
                </div>
              </div>

              <div className="pt-6 border-t border-[#3a3a3a] flex justify-end gap-4">
                <button
                  type="button"
                  onClick={() => setIsCreating(false)}
                  className="px-6 py-2 rounded font-medium text-gray-400 hover:text-white hover:bg-[#3a3a3a] transition-colors"
                >
                  Cancel
                </button>
                <button
                  type="submit"
                  disabled={!formData.type || !formData.version || isSubmitting}
                  className="px-6 py-2 rounded font-bold bg-[#E5B80B] text-black hover:bg-[#d4a90a] disabled:opacity-50 disabled:cursor-not-allowed transition-all shadow-lg shadow-[#E5B80B]/20 flex items-center gap-2"
                >
                  {isSubmitting ? <Loader2 size={18} className="animate-spin" /> : <Check size={18} />}
                  {isSubmitting ? 'Creating...' : 'Create Server'}
                </button>
              </div>
            </form>
          </motion.div>
        </div>
      </div>
    );
  }

  return (
    <div className="flex-1 p-4 md:p-8 overflow-y-auto">
      <div className="flex flex-col md:flex-row md:justify-between md:items-center gap-4 mb-8">
        <h2 className="text-3xl font-bold text-white">Servers</h2>
        <div className="flex flex-wrap items-center gap-3">
          <button
            onClick={() => setDeleteConfirm(true)}
            disabled={selectedServerIds.size === 0}
            className={clsx(
              "flex items-center gap-2 px-4 py-2 rounded font-bold border transition-colors",
              selectedServerIds.size > 0
                ? "border-red-500 text-red-400 hover:bg-red-900/20 cursor-pointer"
                : "border-[#3a3a3a] text-gray-600 cursor-not-allowed"
            )}
          >
            <Trash2 size={18} />
            {selectedServerIds.size > 1 ? `Delete Selected (${selectedServerIds.size})` : 'Delete Server'}
          </button>
          <button
            onClick={() => setIsCreating(true)}
            className="flex items-center gap-2 bg-[#E5B80B] text-black px-4 py-2 rounded font-bold hover:bg-[#d4a90a] transition-colors shadow-lg shadow-[#E5B80B]/20"
          >
            <Plus size={20} />
            Create Server
          </button>
        </div>
      </div>

      {loading && servers.length === 0 ? (
        <div className="flex items-center justify-center h-64 text-gray-500">
          <Loader2 size={32} className="animate-spin mr-3" />
          Loading servers...
        </div>
      ) : servers.length === 0 ? (
        <div className="flex flex-col items-center justify-center h-64 text-gray-500">
          <p className="text-lg mb-2">No servers yet</p>
          <p className="text-sm">Click "Create Server" to get started</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {servers.map((server) => (
            <motion.div
              key={server.id}
              initial={{ opacity: 0, y: 10 }}
              animate={{ opacity: 1, y: 0 }}
              className={`
                relative bg-[#202020] border rounded-lg p-6 cursor-pointer transition-all hover:shadow-lg group
                ${selectedServerIds.has(server.id) ? 'border-[#E5B80B] ring-1 ring-[#E5B80B]' : 'border-[#3a3a3a] hover:border-gray-500'}
              `}
              onClick={() => handleSelectServer(server.id)}
              onDoubleClick={() => handleOpenServer(server.id)}
            >
              <div className="flex justify-between items-start mb-4">
                <div>
                  {renamingId === server.id ? (
                    <input
                      ref={renameInputRef}
                      value={renameValue}
                      onChange={(e) => setRenameValue(e.target.value)}
                      onKeyDown={handleRenameKeyDown}
                      onBlur={handleRenameSubmit}
                      onClick={(e) => e.stopPropagation()}
                      onDoubleClick={(e) => e.stopPropagation()}
                      className="text-xl font-bold text-white mb-1 bg-transparent border-b border-[#E5B80B] outline-none w-full"
                    />
                  ) : (
                    <h3
                      className={clsx(
                        "text-xl font-bold text-white mb-1 group-hover:text-[#E5B80B] transition-colors",
                        selectedServerIds.has(server.id) && "cursor-text hover:border-b hover:border-dashed hover:border-[#E5B80B]/50"
                      )}
                      onClick={(e) => handleStartRename(e, server)}
                    >
                      {server.name}
                    </h3>
                  )}
                  <div className="flex items-center gap-2 text-sm text-gray-400">
                    <span className="bg-[#3a3a3a] px-2 py-0.5 rounded text-xs">{server.type}</span>
                    <span>{server.version}</span>
                  </div>
                </div>
                <div className={`
                  flex items-center gap-1 px-2 py-1 rounded text-xs font-bold uppercase
                  ${server.status === 'Running' ? 'bg-green-900/30 text-green-400' :
                    server.status === 'Crashed' || server.status === 'Error' ? 'bg-red-900/30 text-red-400' :
                    server.status === 'Booting' ? 'bg-yellow-900/30 text-yellow-400' :
                    server.status === 'Installing' ? 'bg-blue-900/30 text-blue-400' :
                    'bg-gray-700/30 text-gray-400'}
                `}>
                  {server.status === 'Running' && <Play size={10} fill="currentColor" />}
                  {server.status === 'Stopped' && <Square size={10} fill="currentColor" />}
                  {server.status === 'Crashed' && <AlertTriangle size={10} />}
                  {server.status === 'Error' && <AlertTriangle size={10} />}
                  {server.status === 'Booting' && <Loader2 size={10} className="animate-spin" />}
                  {server.status === 'Installing' && <Loader2 size={10} className="animate-spin" />}
                  {server.status}
                </div>
              </div>

              <div className="grid grid-cols-2 gap-4 mt-6">
                <div className="bg-[#1a1a1a] p-3 rounded border border-[#333]">
                  <div className="flex items-center gap-2 text-gray-400 text-xs mb-1">
                    <Cpu size={14} /> CPU
                  </div>
                  <div className="text-lg font-mono text-white">{server.status === 'Running' ? `${Math.round(server.cpu)}%` : '-'}</div>
                </div>
                <div className="bg-[#1a1a1a] p-3 rounded border border-[#333]">
                  <div className="flex items-center gap-2 text-gray-400 text-xs mb-1">
                    <HardDrive size={14} /> RAM
                  </div>
                  <div className="text-lg font-mono text-white">{server.status === 'Running' ? `${Math.round(server.ram)} MB` : '-'}</div>
                </div>
              </div>

              {server.status === 'Error' && server.installError && (
                <div className="mt-3 text-xs text-red-400 bg-red-900/10 border border-red-900/30 rounded p-2 truncate" title={server.installError}>
                  {server.installError}
                </div>
              )}

              <div className="mt-4 pt-4 border-t border-[#333] flex items-center justify-between">
                 <div className="flex items-center gap-3">
                   <button
                    onClick={(e) => handleToggleAutoStart(e, server.id, server.autoStart)}
                    className="flex items-center gap-1.5 group/auto"
                    title={server.autoStart ? 'Auto-start enabled' : 'Auto-start disabled'}
                   >
                     <div className={clsx(
                       "relative w-8 h-4 rounded-full transition-colors",
                       server.autoStart ? "bg-[#E5B80B]" : "bg-[#3a3a3a]"
                     )}>
                       <div className={clsx(
                         "absolute top-0.5 w-3 h-3 rounded-full transition-all",
                         server.autoStart ? "left-4.5 bg-black" : "left-0.5 bg-gray-500"
                       )} />
                     </div>
                     <span className={clsx(
                       "text-[10px] font-medium transition-colors",
                       server.autoStart ? "text-[#E5B80B]" : "text-gray-500 group-hover/auto:text-gray-400"
                     )}>
                       Auto Start
                     </span>
                   </button>

                   <button
                    onClick={(e) => handleOpenFlagsPopup(e, server)}
                    className="flex items-center gap-1 text-[10px] font-medium text-gray-500 hover:text-[#E5B80B] transition-colors"
                    title="Change JVM Flags"
                   >
                     <Settings2 size={12} />
                     <span>JVM Flags</span>
                   </button>
                   {serverHasNewerVersion(server) && (
                     <button
                      onClick={(e) => handleOpenVersionPopup(e, server)}
                      className="flex items-center gap-1 text-[10px] font-medium text-gray-500 hover:text-[#E5B80B] transition-colors"
                      title="Update server version"
                     >
                       <RotateCw size={12} />
                       <span>Update version</span>
                     </button>
                   )}
                 </div>

                 <button
                  onClick={(e) => {
                    e.stopPropagation();
                    if (server.status === 'Error') {
                      fetch(`/api/servers/${server.id}/retry-install`, { method: 'POST' })
                        .then(() => { toast.success('Retrying installation...'); refreshServers(); })
                        .catch(() => toast.error('Retry failed'));
                    } else {
                      handleToggleStatus(e, server.id, server.status);
                    }
                  }}
                  disabled={server.status === 'Booting' || server.status === 'Installing'}
                  className={clsx(
                    "text-xs px-3 py-1.5 rounded font-medium border transition-colors",
                    (server.status === 'Booting' || server.status === 'Installing') && "opacity-50 cursor-not-allowed border-blue-500 text-blue-400",
                    server.status === 'Running' && "border-red-500 text-red-400 hover:bg-red-900/20",
                    server.status === 'Error' && "border-orange-500 text-orange-400 hover:bg-orange-900/20",
                    (server.status === 'Stopped' || server.status === 'Crashed') && "border-green-500 text-green-400 hover:bg-green-900/20",
                  )}
                 >
                   {server.status === 'Running' ? 'Stop' :
                    server.status === 'Booting' ? 'Booting...' :
                    server.status === 'Installing' ? 'Installing...' :
                    server.status === 'Error' ? 'Retry' : 'Start'}
                 </button>
              </div>

              {/* JVM Flags Popup */}
              {flagsPopup?.serverId === server.id && (
                <div
                  className="absolute inset-0 z-10 bg-[#202020]/95 backdrop-blur-sm rounded-lg p-5 flex flex-col"
                  onClick={(e) => e.stopPropagation()}
                >
                  <div className="flex items-center justify-between mb-4">
                    <h4 className="text-sm font-bold text-white">JVM Flags</h4>
                    <button onClick={() => setFlagsPopup(null)} className="text-gray-500 hover:text-white transition-colors">
                      <X size={16} />
                    </button>
                  </div>

                  <div className="flex flex-col gap-2 flex-1 min-h-0 overflow-y-auto pr-1">
                    {([
                      { value: 'none', label: 'None', desc: 'Default JVM flags.' },
                      { value: 'aikars', label: "Aikar's Flags", desc: 'Optimized GC for game servers.' },
                      { value: 'velocity', label: 'Velocity Proxy', desc: 'Optimized for proxy servers.' },
                      { value: 'modded', label: 'Modded', desc: 'Recommended for modded servers.' },
                    ] as const).map(opt => (
                      <button
                        key={opt.value}
                        onClick={() => setFlagsPopup({
                          ...flagsPopup,
                          flags: opt.value,
                          alwaysPreTouch: opt.value === 'none' ? false : flagsPopup.alwaysPreTouch,
                        })}
                        className={clsx(
                          "px-3 py-2 rounded border text-left transition-all",
                          flagsPopup.flags === opt.value
                            ? "border-[#E5B80B] bg-[#E5B80B]/10 text-white"
                            : "border-[#3a3a3a] bg-[#1a1a1a] text-gray-400 hover:border-[#E5B80B]/40"
                        )}
                      >
                        <div className="text-xs font-medium">{opt.label}</div>
                        <div className="text-[10px] text-gray-500">{opt.desc}</div>
                      </button>
                    ))}

                    {flagsPopup.flags !== 'none' && (
                      <button
                        onClick={() => setFlagsPopup({ ...flagsPopup, alwaysPreTouch: !flagsPopup.alwaysPreTouch })}
                        className="flex items-center gap-2 mt-1"
                      >
                        <div className={clsx(
                          "relative w-7 h-3.5 rounded-full transition-colors",
                          flagsPopup.alwaysPreTouch ? "bg-[#E5B80B]" : "bg-[#3a3a3a]"
                        )}>
                          <div className={clsx(
                            "absolute top-0.5 w-2.5 h-2.5 rounded-full transition-all",
                            flagsPopup.alwaysPreTouch ? "left-3.5 bg-black" : "left-0.5 bg-gray-500"
                          )} />
                        </div>
                        <span className={clsx(
                          "text-[10px] transition-colors",
                          flagsPopup.alwaysPreTouch ? "text-[#E5B80B]" : "text-gray-500"
                        )}>
                          AlwaysPreTouch
                        </span>
                      </button>
                    )}
                  </div>

                  <div className="flex justify-end gap-2 mt-4 pt-3 border-t border-[#333]">
                    <button
                      onClick={() => setFlagsPopup(null)}
                      className="text-xs px-3 py-1.5 rounded text-gray-400 hover:text-white hover:bg-[#3a3a3a] transition-colors"
                    >
                      Cancel
                    </button>
                    <button
                      onClick={handleSaveFlags}
                      className="text-xs px-3 py-1.5 rounded font-medium bg-[#E5B80B] text-black hover:bg-[#d4a90a] transition-colors"
                    >
                      Apply
                    </button>
                  </div>
                </div>
              )}

              {/* Version Update Popup */}
              {updatePopup?.serverId === server.id && (
                <div
                  className="absolute inset-0 z-10 bg-[#202020]/95 backdrop-blur-sm rounded-lg p-5 flex flex-col"
                  onClick={(e) => e.stopPropagation()}
                >
                  <div className="flex items-center justify-between mb-4">
                    <h4 className="text-sm font-bold text-white">Update version</h4>
                    <button onClick={() => setUpdatePopup(null)} className="text-gray-500 hover:text-white transition-colors">
                      <X size={16} />
                    </button>
                  </div>

                  <div className="text-xs text-gray-400 mb-3">
                    <div>Current version: <span className="text-white">{updatePopup.currentVersion}</span></div>
                    <div className="mt-1">Choose a newer version for <span className="text-white">{updatePopup.serverName}</span>.</div>
                  </div>

                  <label className="block text-xs text-gray-500 mb-2">Available versions</label>
                  <div className="relative">
                    <select
                      value={updatePopup.selectedVersion}
                      onChange={(e) => setUpdatePopup({ ...updatePopup, selectedVersion: e.target.value })}
                      className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white appearance-none cursor-pointer focus:outline-none focus:border-[#E5B80B]"
                      disabled={updatingVersion}
                    >
                      {updatePopup.options.map((v) => (
                        <option key={v.version} value={v.version}>
                          {v.version}{v.latest ? ' (Latest)' : ''}
                        </option>
                      ))}
                    </select>
                    <ChevronDown className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-500 pointer-events-none" size={16} />
                  </div>

                  <div className="flex justify-end gap-2 mt-4 pt-3 border-t border-[#333]">
                    <button
                      onClick={() => setUpdatePopup(null)}
                      className="text-xs px-3 py-1.5 rounded text-gray-400 hover:text-white hover:bg-[#3a3a3a] transition-colors"
                      disabled={updatingVersion}
                    >
                      Cancel
                    </button>
                    <button
                      onClick={handleApplyVersionUpdate}
                      className="text-xs px-3 py-1.5 rounded font-medium bg-[#E5B80B] text-black hover:bg-[#d4a90a] transition-colors disabled:opacity-50"
                      disabled={updatingVersion}
                    >
                      {updatingVersion ? 'Updating...' : 'Accept'}
                    </button>
                  </div>
                </div>
              )}
            </motion.div>
          ))}
        </div>
      )}

      {/* Delete Confirmation Popup */}
      {deleteConfirm && (() => {
        const selectedServers = servers.filter(s => selectedServerIds.has(s.id));
        const hasActive = selectedServers.some(s => s.status === 'Running' || s.status === 'Booting' || s.status === 'Installing');

        return (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm" onClick={() => setDeleteConfirm(false)}>
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              className={clsx("bg-[#202020] border rounded-lg p-6 max-w-md w-full mx-4 shadow-2xl", hasActive ? "border-yellow-500/50" : "border-red-500/50")}
              onClick={(e) => e.stopPropagation()}
            >
              <div className="flex items-center gap-3 mb-4">
                <div className={clsx("p-2 rounded-full", hasActive ? "bg-yellow-900/30" : "bg-red-900/30")}>
                  {hasActive ? <AlertTriangle size={20} className="text-yellow-400" /> : <Trash2 size={20} className="text-red-400" />}
                </div>
                <h3 className="text-lg font-bold text-white">
                  {hasActive ? 'Server(s) Running' : selectedServers.length === 1 ? 'Delete Server' : `Delete ${selectedServers.length} Servers`}
                </h3>
              </div>
              <p className="text-gray-300 mb-4">
                {hasActive
                  ? "You can't delete servers that are currently running. Stop them first."
                  : selectedServers.length === 1
                    ? <>Are you sure you want to delete <span className="font-bold text-white">{selectedServers[0].name}</span> permanently? <span className="text-red-400 font-medium">(That means forever!)</span></>
                    : <>Are you sure you want to delete {selectedServers.length} servers permanently? <span className="text-red-400 font-medium">(That means forever!)</span></>
                }
              </p>
              {selectedServers.length > 1 && !hasActive && (
                <ul className="text-sm text-gray-400 mb-4 space-y-1 max-h-32 overflow-y-auto">
                  {selectedServers.map(s => (
                    <li key={s.id} className="flex items-center gap-2">
                      <span className="w-1.5 h-1.5 rounded-full bg-red-400 flex-shrink-0" />
                      {s.name}
                    </li>
                  ))}
                </ul>
              )}
              <div className="flex justify-end gap-3">
                <button
                  onClick={() => setDeleteConfirm(false)}
                  className="px-4 py-2 rounded text-gray-400 hover:text-white hover:bg-[#3a3a3a] transition-colors"
                >
                  Cancel
                </button>
                {hasActive ? (
                  <button
                    onClick={() => setDeleteConfirm(false)}
                    className="px-4 py-2 rounded font-bold bg-yellow-600 text-black hover:bg-yellow-700 transition-colors"
                  >
                    Accept
                  </button>
                ) : (
                  <button
                    onClick={handleDeleteServer}
                    className="px-4 py-2 rounded font-bold bg-red-600 text-white hover:bg-red-700 transition-colors"
                  >
                    Accept
                  </button>
                )}
              </div>
            </motion.div>
          </div>
        );
      })()}
    </div>
  );
};
