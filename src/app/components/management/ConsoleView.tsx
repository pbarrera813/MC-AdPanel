import React, { useState, useEffect, useRef } from 'react';
import { Server } from '../../context/ServerContext';
import { Send, ChevronsDown } from 'lucide-react';

interface ConsoleLogEntry {
  seq: number;
  line: string;
}

const consoleLogsCache = new Map<string, ConsoleLogEntry[]>();
const maxConsoleLogs = 2000;
const trimConsoleLogs = 200;
const consoleStoragePrefix = 'orexa:console:';

const getStorageKey = (serverId: string) => `${consoleStoragePrefix}${serverId}`;

const trimLogs = (entries: ConsoleLogEntry[]): ConsoleLogEntry[] => {
  if (entries.length > maxConsoleLogs) {
    return entries.slice(trimConsoleLogs);
  }
  return entries;
};

const normalizeStoredEntry = (entry: unknown, fallbackSeq: number): ConsoleLogEntry | null => {
  if (typeof entry === 'string') {
    return { seq: fallbackSeq, line: entry };
  }
  if (!entry || typeof entry !== 'object') return null;
  const raw = entry as { seq?: unknown; line?: unknown };
  if (typeof raw.line !== 'string') return null;
  const seq = typeof raw.seq === 'number' && Number.isFinite(raw.seq) && raw.seq > 0
    ? raw.seq
    : fallbackSeq;
  return { seq, line: raw.line };
};

const loadPersistedConsoleLogs = (serverId: string): ConsoleLogEntry[] => {
  if (typeof window === 'undefined') return [];
  try {
    const raw = window.sessionStorage.getItem(getStorageKey(serverId));
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    const normalized = parsed
      .map((item, index) => normalizeStoredEntry(item, index + 1))
      .filter((item): item is ConsoleLogEntry => item !== null)
      .sort((a, b) => a.seq - b.seq);
    if (normalized.length > maxConsoleLogs) {
      return normalized.slice(normalized.length - maxConsoleLogs);
    }
    return normalized;
  } catch {
    return [];
  }
};

const persistConsoleLogs = (serverId: string, lines: ConsoleLogEntry[]) => {
  if (typeof window === 'undefined') return;
  try {
    window.sessionStorage.setItem(getStorageKey(serverId), JSON.stringify(lines));
  } catch {
    // Ignore storage quota/availability failures.
  }
};

const mergeLogsBySeq = (prev: ConsoleLogEntry[], incoming: ConsoleLogEntry[]): ConsoleLogEntry[] => {
  if (incoming.length === 0) return prev;
  const existing = new Set(prev.map((entry) => entry.seq));
  const merged = [...prev];
  let maxSeq = prev.length > 0 ? prev[prev.length - 1].seq : 0;
  for (const entry of incoming) {
    const seq = Number.isFinite(entry.seq) && entry.seq > 0 ? entry.seq : maxSeq + 1;
    if (existing.has(seq)) {
      continue;
    }
    existing.add(seq);
    maxSeq = Math.max(maxSeq, seq);
    merged.push({ seq, line: entry.line });
  }
  merged.sort((a, b) => a.seq - b.seq);
  return trimLogs(merged);
};

// ANSI color mapping — standard 16 colors used by Minecraft
const ANSI_COLORS: Record<number, string> = {
  0: '#000000', 1: '#aa0000', 2: '#00aa00', 3: '#aa5500',
  4: '#0000aa', 5: '#aa00aa', 6: '#00aaaa', 7: '#aaaaaa',
  8: '#555555', 9: '#ff5555', 10: '#55ff55', 11: '#ffff55',
  12: '#5555ff', 13: '#ff55ff', 14: '#55ffff', 15: '#ffffff',
};

// 256-color ANSI to hex
function ansi256ToHex(n: number): string {
  if (n < 16) return ANSI_COLORS[n] || '#aaaaaa';
  if (n < 232) {
    const idx = n - 16;
    const r = Math.floor(idx / 36) * 51;
    const g = Math.floor((idx % 36) / 6) * 51;
    const b = (idx % 6) * 51;
    return `#${r.toString(16).padStart(2, '0')}${g.toString(16).padStart(2, '0')}${b.toString(16).padStart(2, '0')}`;
  }
  const gray = (n - 232) * 10 + 8;
  return `#${gray.toString(16).padStart(2, '0').repeat(3)}`;
}

// Standard ANSI foreground codes
const ANSI_FG: Record<number, string> = {
  30: ANSI_COLORS[0], 31: ANSI_COLORS[1], 32: ANSI_COLORS[2], 33: ANSI_COLORS[3],
  34: ANSI_COLORS[4], 35: ANSI_COLORS[5], 36: ANSI_COLORS[6], 37: ANSI_COLORS[7],
  90: ANSI_COLORS[8], 91: ANSI_COLORS[9], 92: ANSI_COLORS[10], 93: ANSI_COLORS[11],
  94: ANSI_COLORS[12], 95: ANSI_COLORS[13], 96: ANSI_COLORS[14], 97: ANSI_COLORS[15],
};

interface AnsiSpan {
  text: string;
  color?: string;
  bold?: boolean;
  italic?: boolean;
}

function parseAnsi(line: string): AnsiSpan[] {
  const spans: AnsiSpan[] = [];
  let color: string | undefined;
  let bold = false;
  let italic = false;
  const regex = /\x1b\[([0-9;]*)m/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = regex.exec(line)) !== null) {
    if (match.index > lastIndex) {
      spans.push({ text: line.slice(lastIndex, match.index), color, bold, italic });
    }
    lastIndex = regex.lastIndex;

    const params = match[1] ? match[1].split(';').map(Number) : [0];
    for (let i = 0; i < params.length; i++) {
      const p = params[i];
      if (p === 0) { color = undefined; bold = false; italic = false; }
      else if (p === 1) bold = true;
      else if (p === 3) italic = true;
      else if (p === 22) bold = false;
      else if (p === 23) italic = false;
      else if (p >= 30 && p <= 37) color = ANSI_FG[p];
      else if (p >= 90 && p <= 97) color = ANSI_FG[p];
      else if (p === 39) color = undefined;
      else if (p === 38 && params[i + 1] === 5 && params[i + 2] !== undefined) {
        color = ansi256ToHex(params[i + 2]);
        i += 2;
      }
    }
  }

  if (lastIndex < line.length) {
    spans.push({ text: line.slice(lastIndex), color, bold, italic });
  }

  return spans;
}

// Check if a line contains any ANSI escape codes
const hasAnsi = (line: string) => /\x1b\[/.test(line);

const normalizeLogTimestamp = (line: string) => {
  // Match [HH:MM:SS] or [HH:MM:SS LEVEL] (both Minecraft log formats)
  const match = line.match(/\[(\d{2}):(\d{2}):(\d{2})([\s\]])/);
  if (!match) return line;
  const [matched, hh, mm, ss, trailing] = match;
  const now = new Date();
  const utcDate = new Date(Date.UTC(
    now.getUTCFullYear(),
    now.getUTCMonth(),
    now.getUTCDate(),
    Number(hh),
    Number(mm),
    Number(ss),
  ));
  const localTime = utcDate.toLocaleTimeString([], {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: true,
  });
  return line.replace(matched, `[${localTime}${trailing}`);
};

// Renders a single log line, with ANSI color support
const LogLine = React.memo(({ line }: { line: string }) => {
  // User-typed commands
  if (line.startsWith('>')) {
    return <div className="text-cyan-400 font-bold break-all whitespace-pre-wrap">{line}</div>;
  }

  // Lines without ANSI codes — use simple class-based coloring
  if (!hasAnsi(line)) {
    let cls = 'text-gray-300';
    if (line.includes('WARN')) cls = 'text-yellow-400';
    else if (line.includes('ERROR')) cls = 'text-red-400';
    return <div className={`${cls} break-all whitespace-pre-wrap`}>{line}</div>;
  }

  // Parse ANSI codes into colored spans
  const spans = parseAnsi(line);

  // Determine base line color from the plain text
  const plain = spans.map(s => s.text).join('');
  let baseCls = 'text-gray-300';
  if (plain.includes('WARN')) baseCls = 'text-yellow-400';
  else if (plain.includes('ERROR')) baseCls = 'text-red-400';

  return (
    <div className={`${baseCls} break-all whitespace-pre-wrap`}>
      {spans.map((span, j) => {
        if (!span.color && !span.bold && !span.italic) {
          return <span key={j}>{span.text}</span>;
        }
        const style: React.CSSProperties = {};
        if (span.color) style.color = span.color;
        if (span.bold) style.fontWeight = 'bold';
        if (span.italic) style.fontStyle = 'italic';
        return <span key={j} style={style}>{span.text}</span>;
      })}
    </div>
  );
});

interface ConsoleViewProps {
  server: Server;
}

export const ConsoleView = ({ server }: ConsoleViewProps) => {
  const [logs, setLogs] = useState<ConsoleLogEntry[]>(() => {
    const cached = consoleLogsCache.get(server.id);
    if (cached) return cached;
    const persisted = loadPersistedConsoleLogs(server.id);
    if (persisted.length > 0) {
      consoleLogsCache.set(server.id, persisted);
    }
    return persisted;
  });
  const [input, setInput] = useState('');
  const [autoScroll, setAutoScroll] = useState(true);
  const [connected, setConnected] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const lastSeqRef = useRef(logs.length > 0 ? logs[logs.length - 1].seq : 0);

  useEffect(() => {
    const cached = consoleLogsCache.get(server.id);
    if (cached) {
      setLogs(cached);
      return;
    }
    const persisted = loadPersistedConsoleLogs(server.id);
    consoleLogsCache.set(server.id, persisted);
    setLogs(persisted);
    setAutoScroll(true);
    setInput('');
  }, [server.id]);

  useEffect(() => {
    lastSeqRef.current = logs.length > 0 ? logs[logs.length - 1].seq : 0;
    consoleLogsCache.set(server.id, logs);
    persistConsoleLogs(server.id, logs);
  }, [server.id, logs]);

  // WebSocket connection for real-time console logs
  useEffect(() => {
    // Only connect when server is Running or Booting
    if (server.status !== 'Running' && server.status !== 'Booting') {
      setConnected(false);
      return;
    }

    const loc = window.location;
    const protocol = loc.protocol === 'https:' ? 'wss:' : 'ws:';
    const lastSeq = lastSeqRef.current;
    const wsUrl = `${protocol}//${loc.host}/api/logs/${server.id}?lastSeq=${lastSeq}`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnected(true);
    };

    ws.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data);
        if (data.type === 'snapshot' && Array.isArray(data.entries)) {
          const incoming = data.entries
            .filter((entry: unknown): entry is { seq: number; line: string } => {
              if (!entry || typeof entry !== 'object') return false;
              const raw = entry as { seq?: unknown; line?: unknown };
              return typeof raw.line === 'string' && typeof raw.seq === 'number';
            })
            .map((entry) => ({
              seq: entry.seq,
              line: normalizeLogTimestamp(entry.line),
            }));
          const isReset = data.reset === true;
          if (isReset) {
            setLogs(() => trimLogs(incoming.sort((a, b) => a.seq - b.seq)));
          } else {
            setLogs((prev) => mergeLogsBySeq(prev, incoming));
          }
          return;
        }
        if (data.type === 'log') {
          const line = typeof data.line === 'string' ? data.line : '';
          if (!line) return;
          const seq = typeof data.seq === 'number' ? data.seq : (lastSeqRef.current + 1);
          const normalized = normalizeLogTimestamp(line);
          setLogs(prev => mergeLogsBySeq(prev, [{ seq, line: normalized }]));
        }
      } catch {
        // Handle non-JSON messages as raw text
        const normalized = normalizeLogTimestamp(String(event.data));
        setLogs(prev => mergeLogsBySeq(prev, [{ seq: lastSeqRef.current + 1, line: normalized }]));
      }
    };

    ws.onclose = () => {
      setConnected(false);
    };

    ws.onerror = () => {
      setConnected(false);
    };

    return () => {
      ws.close();
      wsRef.current = null;
    };
  }, [server.id, server.status]);

  // Auto-scroll effect
  useEffect(() => {
    if (autoScroll && scrollRef.current) {
      scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
    }
  }, [logs, autoScroll]);

  const handleSend = (e: React.FormEvent) => {
    e.preventDefault();
    const command = input.trim();
    if (!command) return;

    // Send command via WebSocket to Go backend -> Java stdin
    if (wsRef.current && wsRef.current.readyState === WebSocket.OPEN) {
      wsRef.current.send(command);
    }

    setInput('');
  };

  return (
    <div className="flex flex-col h-full min-h-0 bg-[#121212] font-mono text-sm">
      {/* Connection status indicator */}
      <div className="px-4 py-1 bg-[#1a1a1a] border-b border-[#333] flex items-center gap-2 text-xs">
        <div className={`w-2 h-2 rounded-full ${connected ? 'bg-green-500' : 'bg-gray-500'}`} />
        <span className="text-gray-500">
          {connected ? 'Connected to console' : server.status === 'Running' || server.status === 'Booting' ? 'Connecting...' : 'Server is not running'}
        </span>
      </div>

      <div
        ref={scrollRef}
        className="flex-1 min-h-0 overflow-y-auto p-4 space-y-1 text-gray-300 scrollbar-thin scrollbar-thumb-gray-700"
        onScroll={(e) => {
          const target = e.target as HTMLDivElement;
          const isAtBottom = target.scrollHeight - target.scrollTop - target.clientHeight < 10;
          if (!isAtBottom && autoScroll) setAutoScroll(false);
          if (isAtBottom && !autoScroll) setAutoScroll(true);
        }}
      >
        <div className="text-gray-500 mb-4">
          Welcome to the console. Server is {server.status.toLowerCase()}.
        </div>
        {logs.map((log) => <LogLine key={log.seq} line={log.line} />)}
      </div>

      {!autoScroll && (
        <button
          onClick={() => setAutoScroll(true)}
          className="absolute bottom-16 right-8 bg-[#E5B80B] text-black p-2 rounded-full shadow-lg hover:bg-[#d4a90a] transition-transform animate-bounce"
        >
          <ChevronsDown size={20} />
        </button>
      )}

      <form onSubmit={handleSend} className="bg-[#1a1a1a] p-2 border-t border-[#333] flex gap-2">
        <div className="flex-1 relative">
            <span className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500">{'>'}</span>
            <input
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder={connected ? "Type a command..." : "Console unavailable"}
              disabled={!connected}
              className="w-full bg-[#252524] border border-[#3a3a3a] rounded py-2 pl-6 pr-4 text-white focus:outline-none focus:border-[#E5B80B] disabled:opacity-50 disabled:cursor-not-allowed"
            />
        </div>
        <button
          type="submit"
          disabled={!connected}
          className="bg-[#333] text-gray-300 hover:text-white px-4 rounded border border-[#3a3a3a] hover:border-gray-500 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
        >
          <Send size={18} />
        </button>
      </form>
    </div>
  );
};
