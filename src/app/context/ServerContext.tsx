import React, { createContext, useContext, useState, useEffect, useCallback, ReactNode } from 'react';

export type ServerStatus = 'Running' | 'Stopped' | 'Crashed' | 'Booting' | 'Installing' | 'Error';
export type ServerType = 'Spigot' | 'Paper' | 'Folia' | 'Purpur' | 'Velocity' | 'Waterfall' | 'Forge' | 'Fabric' | 'NeoForge';

export interface Server {
  id: string;
  name: string;
  type: ServerType;
  version: string;
  status: ServerStatus;
  cpu: number;
  ram: number;
  tps: number;
  port: number;
  maxRam: string;
  minRam: string;
  maxPlayers: number;
  autoStart: boolean;
  flags: string;
  alwaysPreTouch: boolean;
  installError?: string;
  fabricTpsAvailable?: boolean;
}

export interface Player {
  name: string;
  ip: string;
  ping: number;
  world: string;
  onlineTime: string;
}

export interface Plugin {
  name: string;
  fileName: string;
  size: string;
  enabled: boolean;
  version: string;
  latestVersion?: string;
  versionStatus?: 'latest' | 'outdated' | 'incompatible' | 'unknown';
  updateUrl?: string;
}

export interface Backup {
  name: string;
  date: string;
  size: string;
}

export interface FileEntry {
  name: string;
  type: 'file' | 'folder';
  size: string;
  modTime: string;
}

export interface LogEntry {
  id: string;
  timestamp: string;
  level: 'INFO' | 'WARN' | 'ERROR';
  message: string;
}

const API_BASE = '';

interface ServerContextType {
  servers: Server[];
  activeServerId: string | null;
  setActiveServerId: (id: string) => void;
  activeServer: Server | undefined;
  addServer: (server: Omit<Server, 'id' | 'cpu' | 'ram' | 'status' | 'autoStart' | 'installError'>) => Promise<void>;
  startServer: (id: string) => Promise<void>;
  stopServer: (id: string) => Promise<void>;
  refreshServers: () => Promise<void>;
  loading: boolean;
  error: string | null;
}

const ServerContext = createContext<ServerContextType | undefined>(undefined);

export const ServerProvider = ({ children }: { children: ReactNode }) => {
  const [servers, setServers] = useState<Server[]>([]);
  const [activeServerId, setActiveServerId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const [pollInterval, setPollInterval] = useState(3000);

  const activeServer = servers.find(s => s.id === activeServerId);

  // Load poll interval from settings
  useEffect(() => {
    fetch(`${API_BASE}/api/settings`)
      .then(res => res.json())
      .then(data => {
        if (data.statusPollInterval && data.statusPollInterval > 0) {
          setPollInterval(data.statusPollInterval * 1000);
        }
      })
      .catch(() => {});
  }, []);

  // Fetch all servers from API
  const refreshServers = useCallback(async () => {
    try {
      const res = await fetch(`${API_BASE}/api/servers`);
      if (!res.ok) throw new Error('Failed to fetch servers');
      const data: Server[] = await res.json();
      const sorted = [...data].sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true, sensitivity: 'base' }));
      setServers(sorted);
      setError(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Unknown error');
    } finally {
      setLoading(false);
    }
  }, []);

  // Initial fetch + polling at configurable interval
  useEffect(() => {
    refreshServers();
    const interval = setInterval(refreshServers, pollInterval);
    return () => clearInterval(interval);
  }, [refreshServers, pollInterval]);

  // Create a new server via API
  const addServer = async (newServer: Omit<Server, 'id' | 'cpu' | 'ram' | 'status' | 'autoStart' | 'installError'>) => {
    const res = await fetch(`${API_BASE}/api/servers`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(newServer),
    });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      throw new Error(data.error || 'Failed to create server');
    }
    await refreshServers();
  };

  // Start a server via API
  const startServer = async (id: string) => {
    const res = await fetch(`${API_BASE}/api/servers/${id}/start`, { method: 'POST' });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      throw new Error(data.error || 'Failed to start server');
    }
    await refreshServers();
  };

  // Stop a server via API
  const stopServer = async (id: string) => {
    const res = await fetch(`${API_BASE}/api/servers/${id}/stop`, { method: 'POST' });
    if (!res.ok) {
      const data = await res.json().catch(() => ({}));
      throw new Error(data.error || 'Failed to stop server');
    }
    await refreshServers();
  };

  return (
    <ServerContext.Provider value={{
      servers, activeServerId, setActiveServerId, activeServer,
      addServer, startServer, stopServer, refreshServers,
      loading, error,
    }}>
      {children}
    </ServerContext.Provider>
  );
};

export const useServer = () => {
  const context = useContext(ServerContext);
  if (!context) throw new Error('useServer must be used within a ServerProvider');
  return context;
};
