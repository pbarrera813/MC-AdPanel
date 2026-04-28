import React, { createContext, useContext, useState, useEffect, useCallback, ReactNode } from 'react';
import { apiRequest, toErrorMessage } from '../lib/api';

export type ServerStatus = 'Running' | 'Stopped' | 'Crashed' | 'Booting' | 'Installing' | 'Error';
export type ServerType = 'Vanilla' | 'Spigot' | 'Paper' | 'Folia' | 'Purpur' | 'Velocity' | 'Forge' | 'Fabric' | 'NeoForge';

export interface Server {
  id: string;
  name: string;
  type: ServerType;
  version: string;
  status: ServerStatus;
  cpu: number;
  ram: number;
  cpuExact?: number;
  ramBytes?: number;
  ramMb?: number;
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
  sourceUrl?: string;
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
  killServer: (id: string) => Promise<void>;
  reorderServers: (orderedIds: string[]) => Promise<void>;
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
    apiRequest<{ statusPollInterval?: number }>(`${API_BASE}/api/settings`)
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
      const data = await apiRequest<Server[]>(`${API_BASE}/api/servers`, undefined, 'Failed to fetch servers');
      setServers(data);
      setError(null);
    } catch (err) {
      setError(toErrorMessage(err, 'Unknown error'));
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
    await apiRequest(`${API_BASE}/api/servers`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(newServer),
    }, 'Failed to create server');
    await refreshServers();
  };

  // Start a server via API
  const startServer = async (id: string) => {
    await apiRequest(`${API_BASE}/api/servers/${id}/start`, { method: 'POST' }, 'Failed to start server');
    await refreshServers();
  };

  // Stop a server via API
  const stopServer = async (id: string) => {
    await apiRequest(`${API_BASE}/api/servers/${id}/stop`, { method: 'POST' }, 'Failed to stop server');
    await refreshServers();
  };

  const killServer = async (id: string) => {
    await apiRequest(`${API_BASE}/api/servers/${id}/kill`, { method: 'POST' }, 'Failed to kill server');
    await refreshServers();
  };

  const reorderServers = async (orderedIds: string[]) => {
    const normalized = orderedIds.map((id) => id.trim()).filter(Boolean);
    setServers((prev) => {
      const byId = new Map(prev.map((server) => [server.id, server]));
      const ordered = normalized.map((id) => byId.get(id)).filter((server): server is Server => !!server);
      const missing = prev.filter((server) => !normalized.includes(server.id));
      return [...ordered, ...missing];
    });

    try {
      await apiRequest(`${API_BASE}/api/servers/order`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ orderedIds: normalized }),
      }, 'Failed to save server order');
    } catch (err) {
      await refreshServers();
      throw new Error(toErrorMessage(err, 'Failed to save server order'));
    }
  };

  return (
    <ServerContext.Provider value={{
      servers, activeServerId, setActiveServerId, activeServer,
      addServer, startServer, stopServer, killServer, reorderServers, refreshServers,
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
