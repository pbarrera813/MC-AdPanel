import React, { useState, useEffect, useRef } from 'react';
import { useServer } from '../context/ServerContext';
import { Plus, Cpu, HardDrive, Play, Square, AlertTriangle, ArrowLeft, Check, ChevronDown, ChevronUp, Loader2, RotateCw, Power, Settings2, X, Trash2, FileUp, Upload } from 'lucide-react';
import { AnimatePresence, motion } from 'motion/react';
import { toast } from 'sonner';
import clsx from 'clsx';
import { useEscapeKey } from '../hooks/useEscapeKey';
import { useStagedDeleteUndo } from '../hooks/useStagedDeleteUndo';

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

interface ImportProperties {
  maxPlayers?: number;
  motd?: string;
  whiteList?: boolean;
  onlineMode?: boolean;
}

interface ImportAnalysis {
  analysisId: string;
  serverType: string;
  typeDetected: boolean;
  version: string;
  worlds: string[];
  plugins: string[];
  mods: string[];
  properties: ImportProperties;
  resolvedName: string;
  resolvedPort: number;
}

type ImportBoolState = 'true' | 'false';

interface ImportFormState {
  name: string;
  port: string;
  serverType: string;
  version: string;
  maxPlayers: string;
  motd: string;
  whiteList: ImportBoolState;
  onlineMode: ImportBoolState;
}

type JVMFlagsPreset = 'none' | 'aikars' | 'velocity' | 'modded';

const DEFAULT_CREATE_FORM = {
  name: '',
  flags: 'none' as JVMFlagsPreset,
  alwaysPreTouch: false,
  type: '',
  version: '',
  port: '25565',
  minRam: '0.5',
  maxRam: '1',
  maxPlayers: '20',
};

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

const importInfoContainerVariants = {
  hidden: { opacity: 0 },
  show: {
    opacity: 1,
    transition: { staggerChildren: 0.03, delayChildren: 0.04 },
  },
};

const importInfoItemVariants = {
  hidden: { opacity: 0, y: 6 },
  show: { opacity: 1, y: 0, transition: { duration: 0.18, ease: 'easeOut' } },
};

export const ServersPage = ({ onViewChange }: ServersPageProps) => {
  const { servers, setActiveServerId, startServer, stopServer, addServer, refreshServers, loading } = useServer();
  const [isCreating, setIsCreating] = useState(false);
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [formDefaults, setFormDefaults] = useState({
    minRam: DEFAULT_CREATE_FORM.minRam,
    maxRam: DEFAULT_CREATE_FORM.maxRam,
    flags: DEFAULT_CREATE_FORM.flags,
  });
  const [formData, setFormData] = useState(DEFAULT_CREATE_FORM);
  const [versions, setVersions] = useState<VersionInfo[]>([]);
  const [versionsLoading, setVersionsLoading] = useState(false);
  const [typeVersionCatalog, setTypeVersionCatalog] = useState<Record<string, VersionInfo[]>>({});
  const [updatePopup, setUpdatePopup] = useState<{ serverId: string; serverName: string; currentVersion: string; selectedVersion: string; options: VersionInfo[] } | null>(null);
  const [updatingVersion, setUpdatingVersion] = useState(false);
  const [isImportOpen, setIsImportOpen] = useState(false);
  const [importDragActive, setImportDragActive] = useState(false);
  const [isImportUploading, setIsImportUploading] = useState(false);
  const [isImportAnalyzing, setIsImportAnalyzing] = useState(false);
  const [isImportSubmitting, setIsImportSubmitting] = useState(false);
  const [importUploadProgress, setImportUploadProgress] = useState(0);
  const [importAnalysis, setImportAnalysis] = useState<ImportAnalysis | null>(null);
  const [importArchiveName, setImportArchiveName] = useState('');
  const [importForm, setImportForm] = useState<ImportFormState>({
    name: '',
    port: '',
    serverType: '',
    version: '',
    maxPlayers: '',
    motd: '',
    whiteList: 'false',
    onlineMode: 'true',
  });
  const [importVersionOptions, setImportVersionOptions] = useState<VersionInfo[]>([]);
  const [importVersionsLoading, setImportVersionsLoading] = useState(false);
  const importFileInputRef = useRef<HTMLInputElement>(null);
  const importAnalyzeXhrRef = useRef<XMLHttpRequest | null>(null);

  // Load system defaults for create form
  useEffect(() => {
    fetch('/api/settings')
      .then(res => res.json())
      .then(data => {
        const defaults = {
          minRam: data.defaultMinRam || DEFAULT_CREATE_FORM.minRam,
          maxRam: data.defaultMaxRam || DEFAULT_CREATE_FORM.maxRam,
          flags: (data.defaultFlags || DEFAULT_CREATE_FORM.flags) as JVMFlagsPreset,
        };
        setFormDefaults(defaults);
        setFormData(prev => ({
          ...prev,
          minRam: defaults.minRam,
          maxRam: defaults.maxRam,
          flags: defaults.flags,
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
        toast.error('Couldn’t load versions for ' + formData.type + '.');
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

  const handleDeleteServer = () => {
    if (selectedServerIds.size === 0) return;
    const serverIds = Array.from(selectedServerIds);
    const deletedCount = serverIds.length;

    setDeleteConfirm(false);
    setSelectedServerIds(new Set());
    setPendingDeletedServerIds((prev) => {
      const next = new Set(prev);
      serverIds.forEach((serverId) => next.add(serverId));
      return next;
    });

    stageDelete({
      label: `${deletedCount} server${deletedCount > 1 ? 's' : ''}`,
      successMessage: deletedCount === 1 ? 'Server deleted permanently' : `${deletedCount} servers deleted permanently`,
      errorMessage: 'Failed to delete server',
      onUndo: () => {
        setPendingDeletedServerIds((prev) => {
          const next = new Set(prev);
          serverIds.forEach((serverId) => next.delete(serverId));
          return next;
        });
      },
      onCommit: async () => {
        for (const serverId of serverIds) {
          const res = await fetch(`/api/servers/${serverId}`, { method: 'DELETE' });
          if (!res.ok) {
            const data = await res.json().catch(() => ({}));
            throw new Error(data.error || `Failed to delete server ${serverId}`);
          }
        }
        setPendingDeletedServerIds((prev) => {
          const next = new Set(prev);
          serverIds.forEach((serverId) => next.delete(serverId));
          return next;
        });
        await refreshServers();
      },
    });
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
      setFormData({
        ...DEFAULT_CREATE_FORM,
        minRam: formDefaults.minRam,
        maxRam: formDefaults.maxRam,
        flags: formDefaults.flags,
      });
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
      toast.error(err instanceof Error ? err.message : 'Couldn’t complete that action. Try again.');
    }
  };

  const [selectedServerIds, setSelectedServerIds] = useState<Set<string>>(new Set());
  const [deleteConfirm, setDeleteConfirm] = useState(false);
  const [pendingDeletedServerIds, setPendingDeletedServerIds] = useState<Set<string>>(new Set());
  const [renamingId, setRenamingId] = useState<string | null>(null);
  const [renameValue, setRenameValue] = useState('');
  const renameInputRef = useRef<HTMLInputElement>(null);
  const renameClickedRef = useRef(false);
  const { stageDelete, undoOverlay } = useStagedDeleteUndo();

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

  const cancelImportAnalysis = async (analysisId?: string) => {
    const id = analysisId?.trim();
    if (!id) return;
    try {
      await fetch(`/api/servers/import/analyze/${encodeURIComponent(id)}`, { method: 'DELETE' });
    } catch {
      // Ignore cleanup errors; TTL cleanup on backend will handle leftovers.
    }
  };

  const toImportBoolState = (value?: boolean | null): ImportBoolState => {
    return value ? 'true' : 'false';
  };

  const fromImportBoolState = (value: ImportBoolState): boolean => {
    if (value === 'true') return true;
    return false;
  };

  const applyAnalysisToImportForm = (analysis: ImportAnalysis) => {
    const detectedType = (analysis.serverType || '').trim();
    const initialType = detectedType || SERVER_TYPES[0];
    const detectedVersion = (analysis.version || '').trim();
    setImportForm({
      name: analysis.resolvedName || '',
      port: analysis.resolvedPort ? String(analysis.resolvedPort) : '',
      serverType: initialType,
      version: detectedVersion,
      maxPlayers: typeof analysis.properties.maxPlayers === 'number' ? String(analysis.properties.maxPlayers) : '',
      motd: analysis.properties.motd || '',
      whiteList: toImportBoolState(analysis.properties.whiteList ?? false),
      onlineMode: toImportBoolState(analysis.properties.onlineMode ?? true),
    });
  };

  const abortImportAnalyze = () => {
    if (importAnalyzeXhrRef.current) {
      importAnalyzeXhrRef.current.abort();
      importAnalyzeXhrRef.current = null;
    }
    setIsImportUploading(false);
    setIsImportAnalyzing(false);
    setImportUploadProgress(0);
  };

  const resetImportState = () => {
    abortImportAnalyze();
    setImportDragActive(false);
    setIsImportSubmitting(false);
    setImportAnalysis(null);
    setImportArchiveName('');
    setImportForm({
      name: '',
      port: '',
      serverType: '',
      version: '',
      maxPlayers: '',
      motd: '',
      whiteList: 'false',
      onlineMode: 'true',
    });
    setImportVersionOptions([]);
    setImportVersionsLoading(false);
    if (importFileInputRef.current) {
      importFileInputRef.current.value = '';
    }
  };

  const closeImportModal = () => {
    const analysisId = importAnalysis?.analysisId;
    resetImportState();
    setIsImportOpen(false);
    void cancelImportAnalysis(analysisId);
  };

  const analyzeImportArchive = async (file: File | null) => {
    if (!file) return;
    const previousAnalysisId = importAnalysis?.analysisId;
    resetImportState();
    setImportArchiveName(file.name);
    setIsImportUploading(true);
    setImportUploadProgress(0);

    if (previousAnalysisId) {
      await cancelImportAnalysis(previousAnalysisId);
    }

    try {
      const formData = new FormData();
      formData.append('file', file);
      const parsed = await new Promise<ImportAnalysis>((resolve, reject) => {
        const xhr = new XMLHttpRequest();
        importAnalyzeXhrRef.current = xhr;
        xhr.open('POST', '/api/servers/import/analyze');
        xhr.responseType = 'json';

        xhr.upload.onprogress = (event) => {
          if (!event.lengthComputable || event.total <= 0) return;
          const progress = Math.min(100, Math.round((event.loaded / event.total) * 100));
          setImportUploadProgress(progress);
        };
        xhr.upload.onload = () => {
          setImportUploadProgress(100);
          setIsImportUploading(false);
          setIsImportAnalyzing(true);
        };
        xhr.onload = () => {
          importAnalyzeXhrRef.current = null;
          setIsImportUploading(false);
          setIsImportAnalyzing(false);
          const raw = xhr.response;
          const payload = raw && typeof raw === 'object' ? raw : (() => {
            try {
              return JSON.parse(xhr.responseText || '{}');
            } catch {
              return {};
            }
          })();

          if (xhr.status === 413) {
            reject(new Error('uploaded file exceeds maximum allowed size'));
            return;
          }
          if (xhr.status >= 200 && xhr.status < 300) {
            resolve(payload as ImportAnalysis);
            return;
          }
          const message = typeof (payload as { error?: unknown }).error === 'string'
            ? String((payload as { error?: string }).error)
            : 'Couldn’t read that server file.';
          reject(new Error(message));
        };
        xhr.onerror = () => {
          importAnalyzeXhrRef.current = null;
          setIsImportUploading(false);
          setIsImportAnalyzing(false);
          reject(new Error('Couldn’t read that server file.'));
        };
        xhr.onabort = () => {
          importAnalyzeXhrRef.current = null;
          setIsImportUploading(false);
          setIsImportAnalyzing(false);
          reject(new DOMException('Import analysis cancelled', 'AbortError'));
        };

        xhr.send(formData);
      });
      setImportAnalysis(parsed);
      applyAnalysisToImportForm(parsed);
      toast.success('Server file uploaded successfully.');
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        toast.info('Import analysis cancelled');
        return;
      }
      toast.error(err instanceof Error ? err.message : 'Couldn’t read that server file.');
    }
  };

  const handleImportDrop = async (e: React.DragEvent<HTMLDivElement>) => {
    e.preventDefault();
    e.stopPropagation();
    setImportDragActive(false);
    const file = e.dataTransfer.files?.[0] || null;
    await analyzeImportArchive(file);
  };

  const handleImportPicker = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0] || null;
    await analyzeImportArchive(file);
    e.target.value = '';
  };

  const handleCommitImport = async () => {
    if (!importAnalysis) return;
    if (!importForm.name.trim()) {
      toast.error('Server name is required');
      return;
    }
    const requiresTypeChoice = !importAnalysis.typeDetected;
    if (requiresTypeChoice && !importForm.serverType.trim()) {
      toast.error('Choose a server type to continue');
      return;
    }
    const port = Number.parseInt(importForm.port, 10);
    if (!Number.isFinite(port) || port < 1024 || port > 65535) {
      toast.error('Port must be between 1024 and 65535');
      return;
    }
    let maxPlayers: number | null = null;
    const maxPlayersRaw = importForm.maxPlayers.trim();
    if (maxPlayersRaw !== '') {
      const parsedMaxPlayers = Number.parseInt(maxPlayersRaw, 10);
      if (!Number.isFinite(parsedMaxPlayers) || parsedMaxPlayers <= 0) {
        toast.error('Max players must be a positive number');
        return;
      }
      maxPlayers = parsedMaxPlayers;
    }
    const versionLockedByDetection = !!importAnalysis.version?.trim();
    if (!versionLockedByDetection) {
      if (importVersionsLoading) {
        toast.error('Wait until versions finish loading');
        return;
      }
      const chosenVersion = importForm.version.trim();
      if (!chosenVersion) {
        toast.error('Choose a server version to continue');
        return;
      }
      const isAllowedVersion = importVersionOptions.some((v) => v.version === chosenVersion);
      if (!isAllowedVersion) {
        toast.error('Selected version is not available for this server type');
        return;
      }
    }

    setIsImportSubmitting(true);
    try {
      const versionOverride = versionLockedByDetection ? undefined : importForm.version.trim();
      const res = await fetch('/api/servers/import/commit', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          analysisId: importAnalysis.analysisId,
          name: importForm.name.trim(),
          port,
          typeOverride: requiresTypeChoice ? importForm.serverType.trim() : '',
          version: versionOverride,
          properties: {
            maxPlayers,
            motd: importForm.motd.trim() || null,
            whiteList: fromImportBoolState(importForm.whiteList),
            onlineMode: fromImportBoolState(importForm.onlineMode),
          },
        }),
      });
      const data = await res.json().catch(() => ({} as { error?: string; message?: string; suggestedPort?: number }));
      if (!res.ok) {
        const payload = data as { error?: string; message?: string; suggestedPort?: number };
        if (payload.error === 'port_in_use') {
          const suggestedPort = typeof payload.suggestedPort === 'number' ? payload.suggestedPort : null;
          const hint = suggestedPort ? ` Closest free port: ${suggestedPort}.` : '';
          toast.error(`That port is already in use.${hint}`);
          return;
        }
        if (payload.error === 'invalid_server_version') {
          toast.error(payload.message || 'Selected version is not valid for this server type');
          return;
        }
        throw new Error(data.error || 'Failed to import server');
      }
      await refreshServers();
      toast.success('Server imported successfully');
      resetImportState();
      setIsImportOpen(false);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to import server');
    } finally {
      setIsImportSubmitting(false);
    }
  };

  const renderImportList = (items: string[]) => {
    return items.length > 0 ? items.join(', ') : 'Not present.';
  };

  useEffect(() => {
    if (!isImportOpen || !importAnalysis) return;
    if (importAnalysis.version?.trim()) {
      setImportVersionOptions([]);
      setImportVersionsLoading(false);
      return;
    }
    const serverType = importForm.serverType.trim();
    if (!serverType) {
      setImportVersionOptions([]);
      setImportVersionsLoading(false);
      return;
    }

    let cancelled = false;
    setImportVersionsLoading(true);
    fetch(`/api/versions/${serverType}`)
      .then((res) => {
        if (!res.ok) throw new Error('Failed to fetch versions');
        return res.json() as Promise<VersionInfo[]>;
      })
      .then((items) => {
        if (cancelled) return;
        const list = Array.isArray(items) ? items : [];
        setImportVersionOptions(list);
        const selected = importForm.version.trim();
        const selectedExists = selected !== '' && list.some((entry) => entry.version === selected);
        if (!selectedExists) {
          const latest = list.find((entry) => entry.latest)?.version || list[0]?.version || '';
          setImportForm((prev) => ({ ...prev, version: latest }));
        }
      })
      .catch(() => {
        if (cancelled) return;
        setImportVersionOptions([]);
        setImportForm((prev) => ({ ...prev, version: '' }));
        toast.error(`Couldn’t load versions for ${serverType}.`);
      })
      .finally(() => {
        if (cancelled) return;
        setImportVersionsLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [isImportOpen, importAnalysis?.analysisId, importAnalysis?.version, importForm.serverType]);

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
  useEscapeKey(isImportOpen, () => closeImportModal());

  useEffect(() => {
    return () => {
      if (importAnalyzeXhrRef.current) {
        importAnalyzeXhrRef.current.abort();
      }
      if (importAnalysis?.analysisId) {
        void cancelImportAnalysis(importAnalysis.analysisId);
      }
    };
  }, [importAnalysis?.analysisId]);

  const visibleServers = servers.filter((server) => !pendingDeletedServerIds.has(server.id));
  const showImportDropZone = isImportOpen && !importAnalysis && !isImportUploading && !isImportAnalyzing;
  const showImportProgress = isImportOpen && !importAnalysis && (isImportUploading || isImportAnalyzing);
  const showImportInfo = isImportOpen && !!importAnalysis;

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
            onClick={() => setIsCreating(true)}
            className="flex items-center gap-2 bg-[#E5B80B] text-black px-4 py-2 rounded font-bold hover:bg-[#d4a90a] transition-colors shadow-lg shadow-[#E5B80B]/20"
          >
            <Plus size={20} />
            Create Server
          </button>
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
            onClick={() => setIsImportOpen(true)}
            className="flex items-center gap-2 bg-blue-600 text-white px-4 py-2 rounded font-bold hover:bg-blue-500 transition-colors shadow-lg shadow-blue-900/30"
          >
            <FileUp size={20} />
            Import Server
          </button>
        </div>
      </div>

      {loading && visibleServers.length === 0 ? (
        <div className="flex items-center justify-center h-64 text-gray-500">
          <Loader2 size={32} className="animate-spin mr-3" />
          Loading servers...
        </div>
      ) : visibleServers.length === 0 ? (
        <div className="flex flex-col items-center justify-center h-64 text-gray-500">
          <p className="text-lg mb-2">No servers yet</p>
          <p className="text-sm">Click "Create Server" to get started</p>
        </div>
      ) : (
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
          {visibleServers.map((server) => (
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

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 mt-6">
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
                  <div className="text-lg font-mono text-white">{server.status === 'Running' ? `${Math.round(server.ram)}%` : '-'}</div>
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
                        .catch(() => toast.error('Couldn’t retry installation. Try again.'));
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

      {/* Import Server Modal */}
      <AnimatePresence>
        {isImportOpen && (
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
            onClick={closeImportModal}
          >
            <motion.div
              initial={{ opacity: 0, scale: 0.96, y: 8 }}
              animate={{ opacity: 1, scale: 1, y: 0 }}
              exit={{ opacity: 0, scale: 0.98, y: 6 }}
              transition={{ duration: 0.2, ease: 'easeOut' }}
              className="bg-[#202020] border border-blue-500/40 rounded-lg p-6 max-w-2xl w-full max-h-[90vh] overflow-y-auto shadow-2xl"
              onClick={(e) => e.stopPropagation()}
            >
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center gap-3">
                  <div className="p-2 rounded-full bg-blue-900/40 text-blue-300">
                    <Upload size={18} />
                  </div>
                  <h3 className="text-xl font-bold text-white">Import Server</h3>
                </div>
                <button onClick={closeImportModal} className="text-gray-500 hover:text-white transition-colors">
                  <X size={18} />
                </button>
              </div>

              <input
                ref={importFileInputRef}
                type="file"
                accept=".zip,.tar.gz,.tgz,application/zip,application/gzip,application/x-gzip"
                onChange={handleImportPicker}
                className="hidden"
              />

              <AnimatePresence mode="wait" initial={false}>
                {showImportDropZone && (
                  <motion.div
                    key="import-drop"
                    initial={{ opacity: 0, y: 8 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -8 }}
                    transition={{ duration: 0.2, ease: 'easeOut' }}
                    onDragOver={(e) => { e.preventDefault(); e.stopPropagation(); setImportDragActive(true); }}
                    onDragLeave={(e) => { e.preventDefault(); e.stopPropagation(); setImportDragActive(false); }}
                    onDrop={handleImportDrop}
                    className={clsx(
                      "border-2 border-dashed rounded-lg p-6 text-center transition-colors",
                      importDragActive ? "border-blue-400 bg-blue-900/20" : "border-[#3a3a3a] bg-[#1a1a1a]"
                    )}
                  >
                    <Upload size={28} className="mx-auto mb-2 text-blue-400" />
                    <p className="text-sm text-gray-300 mb-1">Drop a .zip or .tar.gz archive here</p>
                    <p className="text-xs text-gray-500 mb-3">or choose a file manually</p>
                    <button
                      onClick={() => importFileInputRef.current?.click()}
                      className="px-4 py-2 rounded bg-blue-600 text-white font-semibold hover:bg-blue-500 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                      disabled={isImportUploading || isImportAnalyzing || isImportSubmitting}
                    >
                      Choose Archive
                    </button>
                    {importArchiveName && (
                      <p className="text-xs text-gray-400 mt-3 font-mono break-all">{importArchiveName}</p>
                    )}
                  </motion.div>
                )}

                {showImportProgress && (
                  <motion.div
                    key="import-progress"
                    initial={{ opacity: 0, y: 8 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -8 }}
                    transition={{ duration: 0.2, ease: 'easeOut' }}
                    className="border border-blue-500/30 rounded-lg bg-[#1a1a1a] p-4"
                  >
                    <div className="flex items-center justify-between text-sm mb-2">
                      <span className="text-gray-300">
                        {isImportUploading ? 'Uploading archive...' : 'Analyzing archive...'}
                      </span>
                      <span className="text-blue-300 font-semibold">{importUploadProgress}%</span>
                    </div>
                    <div className="h-2 w-full bg-[#111] rounded overflow-hidden">
                      <div
                        className="h-full bg-blue-500 transition-all duration-200"
                        style={{ width: `${Math.max(0, Math.min(100, importUploadProgress))}%` }}
                      />
                    </div>
                    {importArchiveName && (
                      <p className="text-xs text-gray-500 mt-2 break-all">{importArchiveName}</p>
                    )}
                    <div className="flex justify-end mt-3">
                      <button
                        onClick={abortImportAnalyze}
                        className="px-3 py-1.5 rounded text-xs text-gray-300 bg-[#333] hover:bg-[#404040] transition-colors"
                      >
                        Cancel Upload
                      </button>
                    </div>
                  </motion.div>
                )}

                {showImportInfo && importAnalysis && (
                  <motion.div
                    key="import-info"
                    initial={{ opacity: 0, y: 8 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -8 }}
                    transition={{ duration: 0.22, ease: 'easeOut' }}
                    className="space-y-3"
                  >
                    <div className="flex items-center justify-between">
                      <h4 className="text-sm font-semibold text-white uppercase tracking-wide">Import server info</h4>
                      <button
                        onClick={() => importFileInputRef.current?.click()}
                        className="text-xs px-3 py-1.5 rounded border border-blue-500/40 text-blue-300 hover:bg-blue-900/20 transition-colors"
                        disabled={isImportUploading || isImportAnalyzing || isImportSubmitting}
                      >
                        Choose another archive
                      </button>
                    </div>

                    <motion.div
                      variants={importInfoContainerVariants}
                      initial="hidden"
                      animate="show"
                      className="grid grid-cols-1 md:grid-cols-2 gap-3 text-sm"
                    >
                      <motion.div variants={importInfoItemVariants} className="bg-[#1a1a1a] border border-[#333] rounded p-3">
                        <div className="text-gray-500 text-xs mb-1">Server name</div>
                        <input
                          value={importForm.name}
                          onChange={(e) => setImportForm((prev) => ({ ...prev, name: e.target.value }))}
                          className="w-full bg-[#111] border border-[#3a3a3a] rounded p-2 text-white focus:outline-none focus:border-[#E5B80B]"
                        />
                      </motion.div>
                      <motion.div variants={importInfoItemVariants} className="bg-[#1a1a1a] border border-[#333] rounded p-3">
                        <div className="text-gray-500 text-xs mb-1">Port</div>
                        <input
                          type="text"
                          inputMode="numeric"
                          value={importForm.port}
                          onChange={(e) => {
                            const digits = e.target.value.replace(/[^\d]/g, '').slice(0, 5);
                            setImportForm((prev) => ({ ...prev, port: digits }));
                          }}
                          className="w-full bg-[#111] border border-[#3a3a3a] rounded p-2 text-white focus:outline-none focus:border-[#E5B80B]"
                        />
                      </motion.div>
                      <motion.div variants={importInfoItemVariants} className="bg-[#1a1a1a] border border-[#333] rounded p-3">
                        <div className="text-gray-500 text-xs mb-1">Server Type</div>
                        {importAnalysis.typeDetected ? (
                          <div className="w-full bg-[#111] border border-[#3a3a3a] rounded p-2 text-white">
                            {importForm.serverType}
                          </div>
                        ) : (
                          <select
                            value={importForm.serverType}
                            onChange={(e) => setImportForm((prev) => ({ ...prev, serverType: e.target.value, version: '' }))}
                            className="w-full bg-[#111] border border-[#3a3a3a] rounded p-2 text-white focus:outline-none focus:border-[#E5B80B]"
                          >
                            {SERVER_TYPES.map((type) => (
                              <option key={type} value={type}>{type}</option>
                            ))}
                          </select>
                        )}
                      </motion.div>
                      <motion.div variants={importInfoItemVariants} className="bg-[#1a1a1a] border border-[#333] rounded p-3">
                        <div className="text-gray-500 text-xs mb-1">Server Version</div>
                        {importAnalysis.version?.trim() ? (
                          <div className="w-full bg-[#111] border border-[#3a3a3a] rounded p-2 text-white">
                            {importAnalysis.version}
                          </div>
                        ) : (
                          <select
                            value={importForm.version}
                            onChange={(e) => setImportForm((prev) => ({ ...prev, version: e.target.value }))}
                            className="w-full bg-[#111] border border-[#3a3a3a] rounded p-2 text-white focus:outline-none focus:border-[#E5B80B]"
                            disabled={importVersionsLoading || importVersionOptions.length === 0}
                          >
                            {importVersionsLoading && <option value="">Loading versions...</option>}
                            {!importVersionsLoading && importVersionOptions.length === 0 && <option value="">No versions available</option>}
                            {!importVersionsLoading && importVersionOptions.map((versionInfo) => (
                              <option key={versionInfo.version} value={versionInfo.version}>
                                {versionInfo.version}{versionInfo.latest ? ' (Latest)' : ''}
                              </option>
                            ))}
                          </select>
                        )}
                      </motion.div>
                      <motion.div variants={importInfoItemVariants} className="bg-[#1a1a1a] border border-[#333] rounded p-3 md:col-span-2">
                        <div className="text-gray-500 text-xs mb-1">Worlds</div>
                        <div className="text-white break-words">{renderImportList(importAnalysis.worlds)}</div>
                      </motion.div>
                      <motion.div variants={importInfoItemVariants} className="bg-[#1a1a1a] border border-[#333] rounded p-3 md:col-span-2">
                        <div className="text-gray-500 text-xs mb-1">Plugins/Mods</div>
                        <div className="text-white break-words">{renderImportList([...importAnalysis.plugins, ...importAnalysis.mods])}</div>
                      </motion.div>
                      <motion.div variants={importInfoItemVariants} className="bg-[#1a1a1a] border border-[#333] rounded p-3">
                        <div className="text-gray-500 text-xs mb-1">Max Players</div>
                        <input
                          type="text"
                          inputMode="numeric"
                          value={importForm.maxPlayers}
                          onChange={(e) => {
                            const digits = e.target.value.replace(/[^\d]/g, '').slice(0, 5);
                            setImportForm((prev) => ({ ...prev, maxPlayers: digits }));
                          }}
                          placeholder="20"
                          className="w-full bg-[#111] border border-[#3a3a3a] rounded p-2 text-white focus:outline-none focus:border-[#E5B80B]"
                        />
                      </motion.div>
                      <motion.div variants={importInfoItemVariants} className="bg-[#1a1a1a] border border-[#333] rounded p-3">
                        <div className="text-gray-500 text-xs mb-1">MOTD</div>
                        <input
                          value={importForm.motd}
                          onChange={(e) => setImportForm((prev) => ({ ...prev, motd: e.target.value }))}
                          placeholder="Not present."
                          className="w-full bg-[#111] border border-[#3a3a3a] rounded p-2 text-white focus:outline-none focus:border-[#E5B80B]"
                        />
                      </motion.div>
                      <motion.div variants={importInfoItemVariants} className="bg-[#1a1a1a] border border-[#333] rounded p-3">
                        <div className="text-gray-500 text-xs mb-1">Whitelist</div>
                        <select
                          value={importForm.whiteList}
                          onChange={(e) => setImportForm((prev) => ({ ...prev, whiteList: e.target.value as ImportBoolState }))}
                          className="w-full bg-[#111] border border-[#3a3a3a] rounded p-2 text-white focus:outline-none focus:border-[#E5B80B]"
                        >
                          <option value="true">On</option>
                          <option value="false">Off</option>
                        </select>
                      </motion.div>
                      <motion.div variants={importInfoItemVariants} className="bg-[#1a1a1a] border border-[#333] rounded p-3">
                        <div className="text-gray-500 text-xs mb-1">Online Mode</div>
                        <select
                          value={importForm.onlineMode}
                          onChange={(e) => setImportForm((prev) => ({ ...prev, onlineMode: e.target.value as ImportBoolState }))}
                          className="w-full bg-[#111] border border-[#3a3a3a] rounded p-2 text-white focus:outline-none focus:border-[#E5B80B]"
                        >
                          <option value="true">On</option>
                          <option value="false">Off</option>
                        </select>
                      </motion.div>
                    </motion.div>
                  </motion.div>
                )}
              </AnimatePresence>

              <div className="flex justify-end gap-3 mt-6">
                <button
                  onClick={closeImportModal}
                  className="px-4 py-2 rounded text-gray-300 bg-[#333] hover:bg-[#404040] transition-colors"
                  disabled={isImportSubmitting}
                >
                  Cancel
                </button>
                <button
                  onClick={handleCommitImport}
                  disabled={
                    !importAnalysis ||
                    isImportUploading ||
                    isImportAnalyzing ||
                    isImportSubmitting ||
                    (!importAnalysis.typeDetected && !importForm.serverType.trim()) ||
                    (!importAnalysis.version?.trim() && (importVersionsLoading || !importForm.version.trim()))
                  }
                  className="px-4 py-2 rounded font-bold bg-blue-600 text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-2"
                >
                  {isImportSubmitting ? <Loader2 size={16} className="animate-spin" /> : <FileUp size={16} />}
                  {isImportSubmitting ? 'Importing...' : 'Import'}
                </button>
              </div>
            </motion.div>
          </motion.div>
        )}
      </AnimatePresence>

      {/* Delete Confirmation Popup */}
      {deleteConfirm && (() => {
        const selectedServers = visibleServers.filter(s => selectedServerIds.has(s.id));
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
                    ? <>Are you sure you want to delete <span className="font-bold text-white">{selectedServers[0].name}</span>?</>
                    : <>Are you sure you want to delete {selectedServers.length} servers?</>
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
      {undoOverlay}
    </div>
  );
};
