import React, { useState, useEffect, useRef, useCallback } from 'react';
import { useServer } from '../context/ServerContext';
import { FileText, AlertTriangle, Download, Copy, Trash2, Search, Filter, Pause, Play, Check, Square } from 'lucide-react';
import { clsx } from 'clsx';
import { toast } from 'sonner';
import { motion, AnimatePresence } from 'motion/react';
import { useEscapeKey } from '../hooks/useEscapeKey';

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
    <div className="flex-1 flex flex-col min-h-0 bg-[#1e1e1d]">
      <div className="bg-[#252524] border-b border-[#3a3a3a] px-4 md:px-6 py-4 flex flex-col md:flex-row md:justify-between md:items-center gap-3">
        <h2 className="text-xl font-bold text-white">Logs</h2>
        <div className="flex bg-[#1a1a1a] rounded p-1 border border-[#333]">
           <button
             onClick={() => setActiveTab('live')}
             className={clsx("px-4 py-1.5 rounded text-sm font-medium transition-colors", activeTab === 'live' ? "bg-[#333] text-white" : "text-gray-500 hover:text-gray-300")}
           >
             Live Logs
           </button>
           <button
             onClick={() => setActiveTab('crash-reports')}
             className={clsx("px-4 py-1.5 rounded text-sm font-medium transition-colors", activeTab === 'crash-reports' ? "bg-[#333] text-white" : "text-gray-500 hover:text-gray-300")}
           >
             Crash Reports
           </button>
        </div>
      </div>

      <div className="flex-1 overflow-hidden min-h-0">
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

// StoredLogs: shows files under /logs and allows one-click select + double-click open
const StoredLogs = ({ serverId }: { serverId: string }) => {
  const [files, setFiles] = useState<ServerFile[]>([]);
  const [loadingFiles, setLoadingFiles] = useState(true);
  const [selectedFiles, setSelectedFiles] = useState<Set<string>>(new Set());
  const [viewer, setViewer] = useState<{ name: string | null; content: string | null }>({ name: null, content: null });
  const [batchDeleteConfirm, setBatchDeleteConfirm] = useState(false);

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

  const deleteFile = async (name: string) => {
    try {
      const path = `logs/${name}`;
      const res = await fetch(`/api/servers/${serverId}/files?path=${encodeURIComponent(path)}`, { method: 'DELETE' });
      if (!res.ok) throw new Error('Failed to delete');
      toast.success('Log deleted');
      setSelectedFiles(prev => { const next = new Set(prev); next.delete(name); return next; });
      fetchFiles();
    } catch (err) {
      toast.error('Failed to delete log file');
    }
  };

  const handleBatchDelete = async () => {
    if (selectedFiles.size === 0) return;
    try {
      await Promise.all(Array.from(selectedFiles).map(name =>
        fetch(`/api/servers/${serverId}/files?path=${encodeURIComponent('logs/' + name)}`, { method: 'DELETE' })
      ));
      toast.success(`${selectedFiles.size} log file${selectedFiles.size > 1 ? 's' : ''} deleted`);
      setSelectedFiles(new Set());
      setBatchDeleteConfirm(false);
      fetchFiles();
    } catch (err) {
      toast.error('Failed to delete selected logs');
    }
  };

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
                {files.length > 0 && (
                  <span
                    onClick={() => setSelectedFiles(prev => prev.size === files.length ? new Set() : new Set(files.map(f => f.name)))}
                    className={clsx('flex-shrink-0 cursor-pointer', selectedFiles.size === files.length ? 'text-[#E5B80B]' : 'text-gray-600')}
                  >
                    {selectedFiles.size === files.length ? <Check size={16} /> : <Square size={16} />}
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
            ) : files.length === 0 ? (
              <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-500">No log files found.</td></tr>
            ) : files.map(file => (
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
                    <button onClick={(e) => { e.stopPropagation(); deleteFile(file.name); }} className="p-2 hover:bg-red-900/20 text-red-400 rounded" title="Delete">
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
                Are you sure you want to delete {selectedFiles.size} selected log file{selectedFiles.size > 1 ? 's' : ''}? This action cannot be undone.
              </p>
              <div className="flex justify-end gap-3">
                <button onClick={() => setBatchDeleteConfirm(false)} className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium">Cancel</button>
                <button onClick={handleBatchDelete} className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white rounded font-bold">Delete</button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>
    </div>
    </>
  );
}

const LiveLogs = () => {
  const { activeServer } = useServer();

  // When server is not running, show stored logs (one-click select, double-click open)
  if (!activeServer) return <div className="flex-1 p-4 text-gray-500">No server selected</div>;
  if (activeServer.status !== 'Running' && activeServer.status !== 'Booting') {
    return <div className="flex-1 overflow-y-auto p-4 md:p-8 min-h-0"><StoredLogs serverId={activeServer.id} /></div>;
  }

  const [filterLevel, setFilterLevel] = useState<string | null>(null);
  const [isPaused, setIsPaused] = useState(false);
  const [search, setSearch] = useState('');
  const [logs, setLogs] = useState<ParsedLog[]>([]);
  const [connected, setConnected] = useState(false);
  const logIdRef = useRef(0);
  const scrollRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);

  useEffect(() => {
    setLogs([]);
    logIdRef.current = 0;
  }, [activeServer?.id]);

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
      if (isPaused) return;

      try {
        const data = JSON.parse(event.data);
        if (data.type === 'log') {
          const line = data.line as string;
          // Strip ANSI escape codes and Minecraft color codes for parsing
          const cleanLine = line.replace(/\x1b\[[0-9;]*m/g, '').replace(/§[0-9a-fk-or]/gi, '');
          // Parse Minecraft log format - supports multiple formats:
          // Format A: [HH:MM:SS] [Thread/LEVEL]: message (modern Paper/Spigot)
          // Format B: [Day HH:MM:SS LEVEL] message or [HH:MM:SS LEVEL Source] message
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
          }

          // Convert server timestamp (UTC) to local time
          let displayTime: string;
          if (match) {
            const [hh, mm, ss] = match[1].split(':').map(Number);
            const now = new Date();
            const utcDate = new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate(), hh, mm, ss));
            displayTime = utcDate.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: true });
          } else {
            displayTime = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit', hour12: true });
          }

          const parsed: ParsedLog = {
            id: logIdRef.current++,
            time: displayTime,
            type: logType,
            msg: match ? match[3] : cleanLine,
          };

          setLogs(prev => {
            const newLogs = [...prev, parsed];
            if (newLogs.length > 1000) return newLogs.slice(200);
            return newLogs;
          });
        }
      } catch {
        // Ignore parse errors
      }
    };

    ws.onclose = () => setConnected(false);
    ws.onerror = () => setConnected(false);

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [activeServer?.id, activeServer?.status, isPaused]);

  // Auto-scroll
  useEffect(() => {
    if (!isPaused && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs, isPaused]);

  const filteredLogs = logs.filter(l =>
    (!filterLevel || l.type === filterLevel) &&
    (l.msg.toLowerCase().includes(search.toLowerCase()))
  );

  return (
    <div className="flex-1 flex flex-col min-h-0">
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
           <button
             onClick={() => setIsPaused(!isPaused)}
             className="flex items-center gap-2 px-3 py-1.5 bg-[#333] text-gray-300 rounded hover:bg-[#444] text-xs font-bold"
           >
             {isPaused ? <Play size={14} /> : <Pause size={14} />}
             {isPaused ? 'Resume' : 'Pause'}
           </button>
         </div>
      </div>

      <div ref={scrollRef} className="flex-1 overflow-y-auto p-4 font-mono text-xs space-y-1 bg-[#121212]">
         {filteredLogs.length === 0 && (
           <div className="text-gray-500 text-center py-8">
             {connected ? 'Waiting for log entries...' : 'No live logs — switch to stored logs below'}
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

         {/* When not connected, show stored log files so user can inspect logs while server is off */}
         {!connected && activeServer && (
           <StoredLogs serverId={activeServer.id} />
         )}
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
  }, [activeServer?.id]);

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
    if (selectedReports.size === reports.length) {
      setSelectedReports(new Set());
    } else {
      setSelectedReports(new Set(reports.map(r => r.name)));
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

  const handleDelete = async (reportName: string) => {
    if (!activeServer) return;
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/crash-reports/${encodeURIComponent(reportName)}`, {
        method: 'DELETE',
      });
      if (!res.ok) throw new Error('Failed to delete crash report');
      toast.success('Crash report deleted');
      setSelectedReports(new Set());
      fetchReports();
    } catch (err) {
      toast.error('Failed to delete crash report');
    }
  };

  const handleBatchDelete = async () => {
    if (!activeServer || selectedReports.size === 0) return;
    try {
      for (const name of selectedReports) {
        const res = await fetch(`/api/servers/${activeServer.id}/crash-reports/${encodeURIComponent(name)}`, { method: 'DELETE' });
        if (!res.ok) throw new Error(`Failed to delete ${name}`);
      }
      toast.success(`${selectedReports.size} crash report${selectedReports.size > 1 ? 's' : ''} deleted`);
      setBatchDeleteConfirm(false);
      setSelectedReports(new Set());
      fetchReports();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete crash reports');
    }
  };

  return (
    <div className="flex-1 flex flex-col min-h-0">
      <div className="flex-1 overflow-auto p-4 md:p-8">
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
        <div className="bg-[#202020] border border-[#3a3a3a] rounded-lg overflow-auto max-h-[calc(100vh-220px)] scrollbar-thin scrollbar-thumb-gray-700">
          <div className="overflow-x-auto">
          <table className="w-full text-left min-w-[720px]">
            <thead className="bg-[#252524] text-gray-400 border-b border-[#3a3a3a]">
               <tr>
                 <th className="px-4 py-4 w-12">
                   {reports.length > 0 && (
                     <span
                       onClick={handleToggleSelectAll}
                       className={clsx('flex-shrink-0 cursor-pointer', selectedReports.size === reports.length ? 'text-[#E5B80B]' : 'text-gray-600')}
                     >
                       {selectedReports.size === reports.length ? <Check size={16} /> : <Square size={16} />}
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
             ) : reports.length === 0 ? (
               <tr><td colSpan={5} className="px-6 py-8 text-center text-gray-500">No crash reports found.</td></tr>
             ) : reports.map(report => (
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
                      <button onClick={(e) => { e.stopPropagation(); handleDelete(report.name); }} className="p-2 hover:bg-red-900/30 text-red-400 rounded" title="Delete">
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
                Are you sure you want to delete {selectedReports.size} selected crash report{selectedReports.size > 1 ? 's' : ''}? This action cannot be undone.
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
    </div>
  </div>
  );
};
