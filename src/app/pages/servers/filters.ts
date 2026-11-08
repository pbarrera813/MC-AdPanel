import type { Server } from '../../context/ServerContext';

export type SortKey = 'custom' | 'name' | 'status' | 'type';
export type StatusFilter = 'all' | 'running' | 'stopped' | 'problems';

const statusRank: Record<string, number> = {
  Running: 0,
  Booting: 1,
  Installing: 2,
  Crashed: 3,
  Error: 4,
  Stopped: 5,
};

export const searchServers = (servers: Server[], query: string) => {
  const needle = query.trim().toLowerCase();
  if (!needle) return servers;
  return servers.filter((server) => `${server.name} ${server.type} ${server.version}`.toLowerCase().includes(needle));
};

export const matchesStatusFilter = (server: Server, filter: StatusFilter) => {
  switch (filter) {
    case 'running':
      return server.status === 'Running' || server.status === 'Booting' || server.status === 'Installing';
    case 'stopped':
      return server.status === 'Stopped';
    case 'problems':
      return server.status === 'Crashed' || server.status === 'Error';
    default:
      return true;
  }
};

export const countByStatus = (servers: Server[]): Record<StatusFilter, number> => ({
  all: servers.length,
  running: servers.filter((s) => matchesStatusFilter(s, 'running')).length,
  stopped: servers.filter((s) => matchesStatusFilter(s, 'stopped')).length,
  problems: servers.filter((s) => matchesStatusFilter(s, 'problems')).length,
});

export const sortServers = (servers: Server[], sort: SortKey) => {
  if (sort === 'custom') return servers;
  const byName = (a: Server, b: Server) => a.name.localeCompare(b.name, undefined, { sensitivity: 'base' });
  return [...servers].sort((a, b) => {
    if (sort === 'status') return (statusRank[a.status] ?? 99) - (statusRank[b.status] ?? 99) || byName(a, b);
    if (sort === 'type') return a.type.localeCompare(b.type) || byName(a, b);
    return byName(a, b);
  });
};
