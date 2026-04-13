import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useServer } from '../context/ServerContext';
import { FileText, AlertTriangle, Download, Copy, Trash2, Search, Filter, Pause, Play, Check, Square, ChevronsDown } from 'lucide-react';
import { clsx } from 'clsx';
import { toast } from 'sonner';
import { motion, AnimatePresence } from 'motion/react';
import { useEscapeKey } from '../hooks/useEscapeKey';
import { useStagedDeleteUndo } from '../hooks/useStagedDeleteUndo';

type LogTab = 'live' | 'crash-reports';

// copyToClipboard: prefer Clipboard API, fall back to execCommand/textarea for older browsers
async function copyToClipboard(text: string) {
  if (!text) throw new Error('No text to copy');
  if (navigator.clipboard && navigator.clipboard.writeText) {
    try {
      await navigator.clipboard.writeText(text);
      return;
    } catch (err) {
      // continue to fallback
    }
  }

  const ta = document.createElement('textarea');
  ta.value = text;
  ta.setAttribute('readonly', '');
  ta.style.position = 'absolute';
  ta.style.left = '-9999px';
  document.body.appendChild(ta);

  const sel = document.getSelection();
  const prevRange = sel && sel.rangeCount > 0 ? sel.getRangeAt(0) : null;
  ta.select();

  try {
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    if (prevRange && sel) { sel.removeAllRanges(); sel.addRange(prevRange); }
    if (!ok) throw new Error('execCommand failed');
    return;
  } catch (err) {
    document.body.removeChild(ta);
    if (prevRange && sel) { sel.removeAllRanges(); sel.addRange(prevRange); }
    throw err;
  }
}

export const LogsPage = () => {
  const { activeServer } = useServer();
  const [activeTab, setActiveTab] = useState<LogTab>('live');

  return (
    <div className="flex-1 h-full flex flex-col min-h-0 overflow-hidden bg-[#1e1e1d]">
      <div className="bg-[#252524] border-b border-[#3a3a3a] px-4 md:px-6 py-4 flex flex-col md:flex-row md:justify-between md:items-center gap-3">
        <h2 className="text-xl font-bold text-white">Logs</h2>
        <div className="relative flex bg-[#1a1a1a] rounded p-1 border border-[#333]">
           <button
             onClick={() => setActiveTab('live')}
             className={clsx("relative px-4 py-1.5 rounded text-sm font-medium transition-colors", activeTab === 'live' ? "text-white" : "text-gray-500 hover:text-gray-300")}
           >
             {activeTab === 'live' && (
               <motion.span
                 layoutId="logs-tab-active"
                 transition={{ type: 'spring', stiffness: 260, damping: 28, mass: 0.75 }}
                 className="absolute inset-0 rounded bg-[#333]"
               />
             )}
             <span className="relative z-10">Live Logs</span>
           </button>
           <button
             onClick={() => setActiveTab('crash-reports')}
             className={clsx("relative px-4 py-1.5 rounded text-sm font-medium transition-colors", activeTab === 'crash-reports' ? "text-white" : "text-gray-500 hover:text-gray-300")}
           >
             {activeTab === 'crash-reports' && (
               <motion.span
                 layoutId="logs-tab-active"
                 transition={{ type: 'spring', stiffness: 260, damping: 28, mass: 0.75 }}
                 className="absolute inset-0 rounded bg-[#333]"
               />
             )}
             <span className="relative z-10">Crash Reports</span>
           </button>
        </div>
      </div>

      <div className="flex-1 h-0 overflow-hidden min-h-0">
        {activeTab === 'live' ? <LiveLogs /> : <CrashReports />}
      </div>
    </div>
  );
};

interface ParsedLog {
  id: number;
  time: string;
  type: string;
  msg: string;
}

interface ServerFile {
  name: string;
  type: string;
  size: string;
  modTime: string;
}

const ANSI_COLOR_CODE_REGEX = /\x1b\[[0-9;]*m/g;
const MC_COLOR_CODE_REGEX = /(?:\u00C2)?\u00A7[0-9a-fk-or]/gi;
const CONTINUATION_LINE_REGEX = /^\s*(at\s|Caused by:|Suppressed:|\.{3}\s+\d+\s+more|[a-zA-Z0-9_.$]+(?:Exception|Error))/;

function parseConsoleLine(line: string, id: number, previousType?: string): ParsedLog {
  const cleanLine = line.replace(ANSI_COLOR_CODE_REGEX, '').replace(MC_COLOR_CODE_REGEX, '');
  let match = cleanLine.match(/\[(\d{2}:\d{2}:\d{2})\]\s*\[.*?\/(INFO|WARN(?:ING)?|ERROR|FATAL|SEVERE)\]:?\s*(.*)/i);
  if (!match) {
    match = cleanLine.match(/\[(?:\w+\s+)?(\d{2}:\d{2}:\d{2})\s+(INFO|WARN(?:ING)?|ERROR|FATAL|SEVERE)[^\]]*\]\s*(.*)/i);
  }

  let logType = 'INFO';
  if (match) {
    const raw = match[2].toUpperCase();
    if (raw === 'WARNING') logType = 'WARN';
    else if (raw === 'SEVERE' || raw === 'FATAL') logType = 'ERROR';
    else logType = raw;
  } else {
    if (/\b(ERROR|FATAL|SEVERE)\b/i.test(cleanLine)) {
      logType = 'ERROR';
    } else if (/\bWARN(?:ING)?\b/i.test(cleanLine)) {
      logType = 'WARN';
    }
    if (
      logType === 'INFO' &&
      (previousType === 'WARN' || previousType === 'ERROR') &&
      CONTINUATION_LINE_REGEX.test(cleanLine)
    ) {
      logType = previousType;
    }
  }

  let displayTime: string;
  if (match) {
    const [hh, mm, ss] = match[1].split(':').map(Number);
    const now = new Date();
    const utcDate = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate(), hh, mm, ss));
    displayTime = utcDate.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: true });
  } else {
    displayTime = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: true });
  }

  return {
    id,
    time: displayTime,
    type: logType,
    msg: match ? match[3] : cleanLine,
  };
}

// StoredLogs: shows files under /logs and allows one-click select + double-click open
const StoredLogs = ({ serverId }: { serverId: string }) => {
  const [files, setFiles] = useState<ServerFile[]>([]);
  const [loadingFiles, setLoadingFiles] = useState(true);
  const [selectedFiles, setSelectedFiles] = useState<Set<string>>(new Set());
  const [viewer, setViewer] = useState<{ name: string | null; content: string | null }>({ name: null, content: null });
  const [batchDeleteConfirm, setBatchDeleteConfirm] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [pendingDeletedFiles, setPendingDeletedFiles] = useState<Set<string>>(new Set());
  const { stageDelete, undoOverlay } = useStagedDeleteUndo();

  const fetchFiles = useCallback(async () => {
    setLoadingFiles(true);
    try {
      const res = await fetch(`/api/servers/${serverId}/logs`);
      if (!res.ok) throw new Error('Failed to fetch logs');
      const data: ServerFile[] = await res.json();
      setFiles(data);
    } catch (err) {
      console.error('Failed to fetch logs:', err);
    } finally {
      setLoadingFiles(false);
    }
  }, [serverId]);

  useEffect(() => { fetchFiles(); }, [fetchFiles]);

  useEffect(() => {
    setSelectedFiles(new Set());
    setPendingDeletedFiles(new Set());
    setDeleteTarget(null);
    setBatchDeleteConfirm(false);
  }, [serverId]);
  useEscapeKey(!!deleteTarget, () => setDeleteTarget(null));
  useEscapeKey(batchDeleteConfirm, () => setBatchDeleteConfirm(false));

  const handleToggleFile = (name: string) => {
    setSelectedFiles(prev => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name); else next.add(name);
      return next;
    });
  };

  const openFile = async (name: string) => {
    try {
      const res = await fetch(`/api/servers/${serverId}/logs/${encodeURIComponent(name)}`);
      if (!res.ok) throw new Error('Failed to fetch file');
      const text = await res.text();
      setViewer({ name, content: text });
    } catch (err) {
      toast.error('Failed to open log file');
    }
  };

  const downloadFile = async (name: string) => {
    try {
      const res = await fetch(`/api/servers/${serverId}/logs/${encodeURIComponent(name)}`);
      if (!res.ok) throw new Error('Download failed');
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = name;
      document.body.appendChild(a);
      a.click();
      setTimeout(() => { a.remove(); URL.revokeObjectURL(url); }, 150);
      toast.success('Downloaded log file.');
    } catch {
      toast.error('Download failed');
    }
  };

  const deleteFile = (name: string) => {
    setDeleteTarget(null);
    setSelectedFiles((prev) => {
      const next = new Set(prev);
      next.delete(name);
      return next;
    });
    setPendingDeletedFiles((prev) => {
      const next = new Set(prev);
      next.add(name);
      return next;
    });
    stageDelete({
      label: `Log "${name}"`,
      successMessage: 'Log deleted',
      errorMessage: 'Failed to delete log file',
      onUndo: () => {
        setPendingDeletedFiles((prev) => {
          const next = new Set(prev);
          next.delete(name);
          return next;
        });
      },
      onCommit: async () => {
        const path = `logs/${name}`;
        const res = await fetch(`/api/servers/${serverId}/files?path=${encodeURIComponent(path)}`, { method: 'DELETE' });
        if (!res.ok) throw new Error('Failed to delete');
        setPendingDeletedFiles((prev) => {
          const next = new Set(prev);
          next.delete(name);
          return next;
        });
        await fetchFiles();
      },
    });
  };

  const handleBatchDelete = () => {
    if (selectedFiles.size === 0) return;
    const names = Array.from(selectedFiles);
    setBatchDeleteConfirm(false);
    setSelectedFiles(new Set());
    setPendingDeletedFiles((prev) => {
      const next = new Set(prev);
      names.forEach((name) => next.add(name));
      return next;
    });
    stageDelete({
      label: `${names.length} log file${names.length > 1 ? 's' : ''}`,
      successMessage: `${names.length} log file${names.length > 1 ? 's' : ''} deleted`,
      errorMessage: 'Failed to delete selected logs',
      onUndo: () => {
        setPendingDeletedFiles((prev) => {
          const next = new Set(prev);
          names.forEach((name) => next.delete(name));
          return next;
        });
      },
      onCommit: async () => {
        for (const name of names) {
          const res = await fetch(`/api/servers/${serverId}/files?path=${encodeURIComponent(`logs/${name}`)}`, { method: 'DELETE' });
          if (!res.ok) throw new Error('Failed to delete');
        }
        setPendingDeletedFiles((prev) => {
          const next = new Set(prev);
          names.forEach((name) => next.delete(name));
          return next;
        });
        await fetchFiles();
      },
    });
  };

  const visibleFiles = files.filter((file) => !pendingDeletedFiles.has(file.name));

  return (
    <>
      {selectedFiles.size > 0 && (
        <div className="flex items-center gap-3 mb-4">
          <button
            onClick={() => setBatchDeleteConfirm(true)}
            className="flex items-center gap-2 px-4 py-2 rounded font-bold border border-red-500 text-red-400 hover:bg-red-900/20 transition-colors"
          >
            <Trash2 size={18} />
            Delete Selected ({selectedFiles.size})
          </button>
        </div>
      )}

      <div className="mt-6 bg-[#202020] border border-[#3a3a3a] rounded-lg overflow-auto max-h-[calc(100vh-220px)] scrollbar-thin scrollbar-thumb-gray-700">
        <div className="overflow-x-auto">
          <table className="w-full text-left min-w-[720px]">
          <thead className="bg-[#252524] text-gray-400 border-b border-[#3a3a3a]">
            <tr>
              <th className="px-4 py-4 w-12">
                {visibleFiles.length > 0 && (
                  <span
                    onClick={() =>
                      setSelectedFiles((prev) => (prev.size === visibleFiles.length ? new Set() : new Set(visibleFiles.map((f) => f.name))))
                    }
                    className={clsx('flex-shrink-0 cursor-pointer', selectedFiles.size === visibleFiles.length ? 'text-[#E5B80B]' : 'text-gray-600')}
                  >
                    {selectedFiles.size === visibleFiles.length ? <Check size={16} /> : <Square size={16} />}
                  </span>
                )}
              </th>
              <th className="px-4 py-4 font-medium">Date</th>
              <th className="px-4 py-4 font-medium">File</th>
              <th className="px-4 py-4 font-medium">Size</th>
              <th className="px-4 py-4 font-medium text-right">Actions</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-[#3a3a3a]">
            {loadingFiles ? (
              <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-500">Loading...</td></tr>
            ) : visibleFiles.length === 0 ? (
              <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-500">No log files found.</td></tr>
            ) : visibleFiles.map((file) => (
              <tr key={file.name} onClick={() => handleToggleFile(file.name)} className="transition-colors group cursor-pointer hover:bg-[#252524]">
                <td className="px-4 py-4 w-12">
                  <span className={clsx('flex-shrink-0 cursor-pointer', selectedFiles.has(file.name) ? 'text-[#E5B80B]' : 'text-gray-600')}>
                    {selectedFiles.has(file.name) ? <Check size={16} /> : <Square size={16} />}
                  </span>
                </td>
                <td className="px-4 py-4 font-medium text-white">{file.modTime}</td>
                <td className="px-4 py-4 text-gray-400 font-mono text-sm">{file.name}</td>
                <td className="px-4 py-4 text-gray-400 text-sm">{file.size}</td>
                <td className="px-4 py-4 text-right">
                  <div className="flex items-center justify-end gap-2 opacity-60 group-hover:opacity-100 transition-opacity">
                    <button onClick={(e) => { e.stopPropagation(); openFile(file.name); }} className="p-2 hover:bg-[#333] text-gray-300 rounded" title="Open">
                      <FileText size={18} />
                    </button>
                    <button onClick={(e) => { e.stopPropagation(); downloadFile(file.name); }} className="p-2 hover:bg-[#333] text-gray-300 rounded" title="Download">
                      <Download size={18} />
                    </button>
                    <button onClick={(e) => { e.stopPropagation(); setDeleteTarget(file.name); }} className="p-2 hover:bg-red-900/20 text-red-400 rounded" title="Delete">
                      <Trash2 size={18} />
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {/* Viewer modal */}
      <AnimatePresence>
        {viewer.name && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }} className="w-full max-w-4xl bg-[#0f0f0f] border border-[#404040] rounded-lg shadow-2xl p-4 overflow-auto max-h-[80vh]">
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-lg font-bold">{viewer.name}</h3>
                <div className="flex gap-2">
                  <button
                    onClick={async () => {
                      try {
                        await copyToClipboard(viewer.content || '');
                        toast.info('Log copied to clipboard.');
                      } catch {
                        toast.error('Failed to copy log');
                      }
                    }}
                    className="px-3 py-1 bg-[#333] rounded text-sm"
                  >
                    Copy
                  </button>
                  <button onClick={() => setViewer({ name: null, content: null })} className="px-3 py-1 bg-[#E5B80B] rounded text-sm text-black">Close</button>
                </div>
              </div>
              <pre className="text-xs font-mono text-gray-200 whitespace-pre-wrap">{viewer.content}</pre>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

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
                <h3 className="text-xl font-bold">Delete Log File?</h3>
              </div>
              <p className="text-gray-300 mb-6">
                Are you sure you want to delete the chosen file?
              </p>
              <div className="flex justify-end gap-3">
                <button onClick={() => setDeleteTarget(null)} className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium">Cancel</button>
                <button onClick={() => deleteFile(deleteTarget)} className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white rounded font-bold">Confirm</button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Batch Delete Confirmation Modal for Stored Logs */}
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
                <h3 className="text-xl font-bold">Delete {selectedFiles.size} Log File{selectedFiles.size > 1 ? 's' : ''}?</h3>
              </div>
              <p className="text-gray-300 mb-6">
                Are you sure you want to delete the chosen files?
              </p>
              <div className="flex justify-end gap-3">
                <button onClick={() => setBatchDeleteConfirm(false)} className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium">Cancel</button>
                <button onClick={handleBatchDelete} className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white rounded font-bold">Delete</button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>
      {undoOverlay}
    </div>
    </>
  );
}

const LiveLogs = () => {
  const { activeServer, refreshServers } = useServer();

  // When server is not running, show stored logs (one-click select, double-click open)
  if (!activeServer) return <div className="flex-1 p-4 text-gray-500">No server selected</div>;
  if (activeServer.status !== 'Running' && activeServer.status !== 'Booting') {
    return <div className="flex-1 overflow-y-auto p-4 md:p-8 min-h-0"><StoredLogs serverId={activeServer.id} /></div>;
  }

  const [filterLevel, setFilterLevel] = useState<string | null>(null);
  const [isPaused, setIsPaused] = useState(false);
  const [autoScroll, setAutoScroll] = useState(true);
  const [search, setSearch] = useState('');
  const [logs, setLogs] = useState<ParsedLog[]>([]);
  const [connected, setConnected] = useState(false);
  const logIdRef = useRef(0);
  const isPausedRef = useRef(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    setLogs([]);
    logIdRef.current = 0;
    setAutoScroll(true);
  }, [activeServer?.id]);

  useEffect(() => {
    isPausedRef.current = isPaused;
  }, [isPaused]);

  // WebSocket connection for live logs
  useEffect(() => {
    if (!activeServer || (activeServer.status !== 'Running' && activeServer.status !== 'Booting')) {
      setConnected(false);
      return;
    }

    const loc = window.location;
    const protocol = loc.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${protocol}//${loc.host}/api/logs/${activeServer.id}`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => setConnected(true);

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as {
          type?: string;
          line?: unknown;
          entries?: Array<{ line?: unknown }>;
        };

        if (data.type === 'snapshot') {
          const entries = Array.isArray(data.entries) ? data.entries : [];
          const parsedSnapshot = entries
            .map((entry) => (typeof entry?.line === 'string' ? entry.line : null))
            .filter((line): line is string => line !== null)
            .reduce<ParsedLog[]>((acc, line) => {
              const previousType = acc.length > 0 ? acc[acc.length - 1].type : undefined;
              acc.push(parseConsoleLine(line, logIdRef.current++, previousType));
              return acc;
            }, []);
          setLogs(parsedSnapshot);
          return;
        }

        if (data.type === 'log') {
          if (isPausedRef.current) return;
          if (typeof data.line !== 'string') return;
          setLogs((prev) => {
            const previousType = prev.length > 0 ? prev[prev.length - 1].type : undefined;
            return [...prev, parseConsoleLine(data.line, logIdRef.current++, previousType)];
          });
        }
      } catch {
        // Ignore parse errors
      }
    };

    ws.onclose = () => {
      setConnected(false);
      refreshServers().catch(() => {});
    };
    ws.onerror = () => {
      setConnected(false);
      refreshServers().catch(() => {});
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [activeServer?.id, activeServer?.status, refreshServers]);

  // Auto-scroll
  useEffect(() => {
    if (!isPaused && autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs, isPaused, autoScroll]);

  const filteredLogs = logs.filter(l =>
    (!filterLevel || l.type === filterLevel) &&
    (l.msg.toLowerCase().includes(search.toLowerCase()))
  );

  return (
    <div className="flex-1 min-h-0 overflow-auto p-4 md:p-8">
      <div className="mx-auto flex h-full min-h-[360px] max-h-[calc(100vh-220px)] w-full flex-col rounded-lg border border-[#3a3a3a] bg-[#202020] overflow-hidden">
      <div className="p-4 border-b border-[#3a3a3a] flex flex-wrap gap-4 items-center bg-[#202020]">
         <div className="relative flex-1 max-w-md">
           <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" />
           <input
             type="text"
             placeholder="Search logs..."
             value={search}
             onChange={e => setSearch(e.target.value)}
             className="w-full bg-[#1a1a1a] border border-[#333] rounded py-1.5 pl-9 pr-4 text-sm text-gray-300 focus:outline-none focus:border-[#E5B80B]"
           />
         </div>

         <div className="h-6 w-px bg-[#333] hidden md:block"></div>

         {/* Connection indicator */}
         <div className="flex items-center gap-2 text-xs text-gray-500">
           <div className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-gray-500'}`} />
           {connected ? 'Live' : 'Disconnected'}
         </div>

         <div className="h-6 w-px bg-[#333] hidden md:block"></div>

         <div className="flex gap-2">
           {['INFO', 'WARN', 'ERROR'].map(level => (
             <button
               key={level}
               onClick={() => setFilterLevel(filterLevel === level ? null : level)}
               className={clsx(
                 "px-3 py-1.5 rounded text-xs font-bold border transition-colors",
                 filterLevel === level
                   ? (level === 'INFO' ? "bg-blue-900/30 border-blue-500 text-blue-400" : level === 'WARN' ? "bg-yellow-900/30 border-yellow-500 text-yellow-400" : "bg-red-900/30 border-red-500 text-red-400")
                   : "border-[#333] bg-[#1a1a1a] text-gray-500 hover:border-gray-500"
               )}
             >
               {level}
             </button>
           ))}
         </div>

         <div className="md:ml-auto">
           {!autoScroll && (
             <button
               onClick={() => {
                 setAutoScroll(true);
                 if (scrollRef.current) {
                   scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
                 }
               }}
               className="mr-2 inline-flex items-center gap-1.5 px-3 py-1.5 bg-[#E5B80B] text-black rounded hover:bg-[#d4a90a] text-xs font-bold"
             >
               <ChevronsDown size={14} />
               Jump to latest
             </button>
           )}
           <button
             onClick={() => setIsPaused(!isPaused)}
             className="flex items-center gap-2 px-3 py-1.5 bg-[#333] text-gray-300 rounded hover:bg-[#444] text-xs font-bold"
           >
             {isPaused ? <Play size={14} /> : <Pause size={14} />}
             {isPaused ? 'Resume' : 'Pause'}
           </button>
         </div>
      </div>

      <div
        ref={scrollRef}
        className="flex-1 h-0 min-h-0 overflow-y-auto overscroll-contain p-4 font-mono text-xs space-y-1 bg-[#121212] scrollbar-thin scrollbar-thumb-gray-700"
        onScroll={(event) => {
          const target = event.currentTarget;
          const isAtBottom = target.scrollHeight - target.scrollTop - target.clientHeight < 12;
          if (!isAtBottom && autoScroll) {
            setAutoScroll(false);
          } else if (isAtBottom && !autoScroll) {
            setAutoScroll(true);
          }
        }}
      >
         {filteredLogs.length === 0 && (
           <div className="text-gray-500 text-center py-8">
             {connected ? 'Waiting for log entries...' : 'Waiting for live log stream...'}
           </div>
         )}
         {filteredLogs.map((log) => (
           <div key={log.id} className="flex gap-3 hover:bg-[#1a1a1a] p-0.5 rounded">
             <span className="text-gray-500 select-none">[{log.time}]</span>
             <span className={clsx(
               "font-bold w-12 text-center select-none",
               log.type === 'INFO' ? 'text-blue-400' : log.type === 'WARN' ? 'text-yellow-400' : 'text-red-400'
             )}>{log.type}</span>
             <span className="text-gray-300 break-all">{log.msg}</span>
           </div>
         ))}
      </div>
      </div>
    </div>
  );
};

interface CrashReport {
  name: string;
  date: string;
  size: string;
  cause: string;
}

interface ServerFile {
  name: string;
  type: string;
  size: string;
  modTime: string;
}

const CrashReports = () => {
  const { activeServer } = useServer();
  const [reports, setReports] = useState<CrashReport[]>([]);
  const [loading, setLoading] = useState(true);
  const [selectedReports, setSelectedReports] = useState<Set<string>>(new Set());
  const [batchDeleteConfirm, setBatchDeleteConfirm] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<string | null>(null);
  const [pendingDeletedReports, setPendingDeletedReports] = useState<Set<string>>(new Set());
  const { stageDelete, undoOverlay } = useStagedDeleteUndo();

  const fetchReports = useCallback(async () => {
    if (!activeServer) return;
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/crash-reports`);
      if (!res.ok) throw new Error('Failed to fetch crash reports');
      const data: CrashReport[] = await res.json();
      setReports(data);
    } catch (err) {
      console.error('Failed to fetch crash reports:', err);
    } finally {
      setLoading(false);
    }
  }, [activeServer?.id]);


  useEffect(() => {
    fetchReports();
  }, [fetchReports]);

  useEffect(() => {
    setSelectedReports(new Set());
    setPendingDeletedReports(new Set());
    setDeleteTarget(null);
    setBatchDeleteConfirm(false);
  }, [activeServer?.id]);

  useEscapeKey(!!deleteTarget, () => setDeleteTarget(null));
  useEscapeKey(batchDeleteConfirm, () => setBatchDeleteConfirm(false));

  const handleToggleSelect = (name: string) => {
    setSelectedReports(prev => {
      const next = new Set(prev);
      if (next.has(name)) next.delete(name);
      else next.add(name);
      return next;
    });
  };

  const handleToggleSelectAll = () => {
    if (selectedReports.size === visibleReports.length) {
      setSelectedReports(new Set());
    } else {
      setSelectedReports(new Set(visibleReports.map((r) => r.name)));
    }
  };

  const handleCopy = async (reportName: string) => {
    if (!activeServer) return;
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/crash-reports/${encodeURIComponent(reportName)}/copy`, {
        method: 'POST',
      });
      if (!res.ok) throw new Error('Failed to copy crash report');
      toast.success('Crash report copy created');
      fetchReports();
    } catch {
      toast.error('Failed to copy crash report');
    }
  };

  const handleDownload = async (reportName: string) => {
    if (!activeServer) return;
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/crash-reports/${encodeURIComponent(reportName)}`);
      if (!res.ok) throw new Error('Download failed');
      const blob = await res.blob();
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = reportName;
      document.body.appendChild(a);
      a.click();
      setTimeout(() => {
        a.remove();
        URL.revokeObjectURL(url);
      }, 150);
      toast.success('Downloaded crash report.');
    } catch {
      toast.error('Download failed, try again.');
    }
  };

  const [reportViewer, setReportViewer] = useState<{ name: string | null; content: string | null }>({ name: null, content: null });

  const handleOpenReport = async (reportName: string) => {
    if (!activeServer) return;
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/crash-reports/${encodeURIComponent(reportName)}`);
      if (!res.ok) throw new Error('Failed to open');
      const text = await res.text();
      setReportViewer({ name: reportName, content: text });
    } catch {
      toast.error('Failed to open crash report');
    }
  };

  const handleDelete = (reportName: string) => {
    if (!activeServer) return;
    setDeleteTarget(null);
    setSelectedReports((prev) => {
      const next = new Set(prev);
      next.delete(reportName);
      return next;
    });
    setPendingDeletedReports((prev) => {
      const next = new Set(prev);
      next.add(reportName);
      return next;
    });
    stageDelete({
      label: `Crash report "${reportName}"`,
      successMessage: 'Crash report deleted',
      errorMessage: 'Failed to delete crash report',
      onUndo: () => {
        setPendingDeletedReports((prev) => {
          const next = new Set(prev);
          next.delete(reportName);
          return next;
        });
      },
      onCommit: async () => {
        const res = await fetch(`/api/servers/${activeServer.id}/crash-reports/${encodeURIComponent(reportName)}`, {
          method: 'DELETE',
        });
        if (!res.ok) throw new Error('Failed to delete crash report');
        setPendingDeletedReports((prev) => {
          const next = new Set(prev);
          next.delete(reportName);
          return next;
        });
        await fetchReports();
      },
    });
  };

  const handleBatchDelete = () => {
    if (!activeServer || selectedReports.size === 0) return;
    const names = Array.from(selectedReports);
    setBatchDeleteConfirm(false);
    setSelectedReports(new Set());
    setPendingDeletedReports((prev) => {
      const next = new Set(prev);
      names.forEach((name) => next.add(name));
      return next;
    });
    stageDelete({
      label: `${names.length} crash report${names.length > 1 ? 's' : ''}`,
      successMessage: `${names.length} crash report${names.length > 1 ? 's' : ''} deleted`,
      errorMessage: 'Failed to delete crash reports',
      onUndo: () => {
        setPendingDeletedReports((prev) => {
          const next = new Set(prev);
          names.forEach((name) => next.delete(name));
          return next;
        });
      },
      onCommit: async () => {
        for (const name of names) {
          const res = await fetch(`/api/servers/${activeServer.id}/crash-reports/${encodeURIComponent(name)}`, { method: 'DELETE' });
          if (!res.ok) throw new Error(`Failed to delete ${name}`);
        }
        setPendingDeletedReports((prev) => {
          const next = new Set(prev);
          names.forEach((name) => next.delete(name));
          return next;
        });
        await fetchReports();
      },
    });
  };

  const visibleReports = reports.filter((report) => !pendingDeletedReports.has(report.name));

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <div className="flex-1 min-h-0 overflow-auto p-4 md:p-8">
        {selectedReports.size > 0 && (
          <div className="flex items-center gap-3 mb-4">
            <button
              onClick={() => setBatchDeleteConfirm(true)}
              className="flex items-center gap-2 px-4 py-2 rounded font-bold border border-red-500 text-red-400 hover:bg-red-900/20 transition-colors"
            >
              <Trash2 size={18} />
              Delete Selected ({selectedReports.size})
            </button>
          </div>
        )}
      <div className="bg-[#202020] border border-[#3a3a3a] rounded-lg min-h-0 overflow-auto max-h-[calc(100vh-220px)] scrollbar-thin scrollbar-thumb-gray-700">
          <div className="overflow-x-auto">
          <table className="w-full text-left min-w-[720px]">
            <thead className="bg-[#252524] text-gray-400 border-b border-[#3a3a3a]">
               <tr>
                 <th className="px-4 py-4 w-12">
                  {visibleReports.length > 0 && (
                     <span
                       onClick={handleToggleSelectAll}
                       className={clsx('flex-shrink-0 cursor-pointer', selectedReports.size === visibleReports.length ? 'text-[#E5B80B]' : 'text-gray-600')}
                     >
                       {selectedReports.size === visibleReports.length ? <Check size={16} /> : <Square size={16} />}
                     </span>
                   )}
                 </th>
                 <th className="px-4 py-4 font-medium">Date</th>
                 <th className="px-4 py-4 font-medium">File</th>
                 <th className="px-4 py-4 font-medium">Likely Cause</th>
                 <th className="px-4 py-4 font-medium text-right">Actions</th>
               </tr>
            </thead>
          <tbody className="divide-y divide-[#3a3a3a]">
             {loading ? (
               <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-500">Loading...</td></tr>
             ) : visibleReports.length === 0 ? (
               <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-500">No crash reports found.</td></tr>
             ) : visibleReports.map((report) => (
               <tr
                 key={report.name}
                 onClick={() => handleToggleSelect(report.name)}
                 className={clsx(
                   "transition-colors group cursor-pointer",
                   selectedReports.has(report.name)
                     ? "bg-[#E5B80B]/5"
                     : "hover:bg-[#252524]"
                 )}
               >
                 <td className="px-4 py-4 w-12">
                   <span className={clsx('flex-shrink-0', selectedReports.has(report.name) ? 'text-[#E5B80B]' : 'text-gray-600')}>
                     {selectedReports.has(report.name) ? <Check size={16} /> : <Square size={16} />}
                   </span>
                 </td>
                 <td className="px-4 py-4 font-medium text-white">
                   <div className="flex items-center gap-2">
                     <FileText size={16} className="text-red-400" />
                     {report.date}
                   </div>
                 </td>
                 <td className="px-4 py-4 text-gray-400 font-mono text-sm">{report.name}</td>
                 <td className="px-4 py-4 text-red-300">{report.cause || 'Unknown'}</td>
                 <td className="px-4 py-4 text-right">
                   <div className="flex items-center justify-end gap-2 opacity-60 group-hover:opacity-100 transition-opacity">
                      <button onClick={(e) => { e.stopPropagation(); handleOpenReport(report.name); }} className="p-2 hover:bg-[#333] text-gray-300 rounded" title="Open">
                        <FileText size={18} />
                      </button>
                      <button onClick={(e) => { e.stopPropagation(); handleCopy(report.name); }} className="p-2 hover:bg-[#333] text-gray-300 rounded" title="Copy">
                        <Copy size={18} />
                      </button>
                      <button onClick={(e) => { e.stopPropagation(); handleDownload(report.name); }} className="p-2 hover:bg-[#333] text-gray-300 rounded" title="Download">
                        <Download size={18} />
                      </button>
                      <button onClick={(e) => { e.stopPropagation(); setDeleteTarget(report.name); }} className="p-2 hover:bg-red-900/30 text-red-400 rounded" title="Delete">
                        <Trash2 size={18} />
                      </button>
                   </div>
                 </td>
               </tr>
             ))}
           </tbody>
        </table>
        </div>
      </div>

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
                <h3 className="text-xl font-bold">Delete Crash Report?</h3>
              </div>
              <p className="text-gray-300 mb-6">
                Are you sure you want to delete the chosen file?
              </p>
              <div className="flex justify-end gap-3">
                <button onClick={() => setDeleteTarget(null)} className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium">Cancel</button>
                <button onClick={() => handleDelete(deleteTarget)} className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white rounded font-bold">Confirm</button>
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
                <h3 className="text-xl font-bold">Delete {selectedReports.size} Crash Report{selectedReports.size > 1 ? 's' : ''}?</h3>
              </div>
              <p className="text-gray-300 mb-6">
                Are you sure you want to delete the chosen files?
              </p>
              <div className="flex justify-end gap-3">
                <button onClick={() => setBatchDeleteConfirm(false)} className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium">Cancel</button>
                <button onClick={handleBatchDelete} className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white rounded font-bold">Delete</button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Crash report viewer (double-click to open) */}
      <AnimatePresence>
        {reportViewer.name && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }} exit={{ opacity: 0, scale: 0.95 }} className="w-full max-w-4xl bg-[#0f0f0f] border border-[#404040] rounded-lg shadow-2xl p-4 overflow-auto max-h-[80vh]">
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-lg font-bold">{reportViewer.name}</h3>
                <div className="flex gap-2">
                  <button
                    onClick={async () => {
                      try {
                        await copyToClipboard(reportViewer.content || '');
                        toast.info('Log copied to clipboard.');
                      } catch {
                        toast.error('Failed to copy log');
                      }
                    }}
                    className="px-3 py-1 bg-[#333] rounded text-sm"
                  >
                    Copy
                  </button>
                  <button onClick={() => setReportViewer({ name: null, content: null })} className="px-3 py-1 bg-[#E5B80B] rounded text-sm text-black">Close</button>
                </div>
              </div>
              <pre className="text-xs font-mono text-gray-200 whitespace-pre-wrap">{reportViewer.content}</pre>
            </motion.div>
          </div>
        )}
      </AnimatePresence>
      {undoOverlay}
    </div>
  </div>
  );
};
