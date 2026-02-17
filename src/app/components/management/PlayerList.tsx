import React, { useState, useEffect, useCallback } from 'react';
import { Server, Player } from '../../context/ServerContext';
import { UserX, Ban, Skull, Search, Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import clsx from 'clsx';
import { Tooltip, TooltipTrigger, TooltipContent } from '../ui/tooltip';

interface PlayerListProps {
  server: Server;
}

export const PlayerList = ({ server }: PlayerListProps) => {
  const [players, setPlayers] = useState<Player[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState('');
  const [pingStatus, setPingStatus] = useState<'supported' | 'missing_pingplayer' | 'missing_pingplayer_mod' | 'unsupported_server_type'>('supported');

  const fetchPlayers = useCallback(async () => {
    try {
      const res = await fetch(`/api/servers/${server.id}/players`);
      if (!res.ok) throw new Error('Failed to fetch players');
      const data = await res.json();
      if (Array.isArray(data)) {
        setPlayers(data);
        setPingStatus('supported');
      } else {
        setPlayers(data.players || []);
        if (data.pingSupported) {
          setPingStatus('supported');
        } else if (data.pingStatus === 'unsupported_server_type') {
          setPingStatus('unsupported_server_type');
        } else if (data.pingStatus === 'missing_pingplayer_mod') {
          setPingStatus('missing_pingplayer_mod');
        } else {
          setPingStatus('missing_pingplayer');
        }
      }
    } catch (err) {
      console.error('Failed to fetch players:', err);
    } finally {
      setLoading(false);
    }
  }, [server.id]);

  useEffect(() => {
    if (server.status !== 'Running') {
      setPlayers([]);
      setLoading(false);
      return;
    }
    fetchPlayers();
    const interval = setInterval(fetchPlayers, 2000);
    return () => clearInterval(interval);
  }, [server.status, fetchPlayers]);

  if (server.status !== 'Running') {
    return (
      <div className="flex flex-col items-center justify-center h-full text-gray-500 gap-4">
        <UserX size={48} className="opacity-20" />
        <p>Server is not running. No players online.</p>
      </div>
    );
  }

  const filteredPlayers = players.filter(p =>
    p.name.toLowerCase().includes(filter.toLowerCase())
  );

  const handleAction = async (playerName: string, action: 'kick' | 'ban' | 'kill') => {
    try {
      const res = await fetch(`/api/servers/${server.id}/players/${encodeURIComponent(playerName)}/${action}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({}),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || `Failed to ${action} player`);
      }
      const labels = { kick: 'Kicked', ban: 'Banned', kill: 'Killed' };
      toast.success(`${labels[action]} ${playerName}`);
      setTimeout(fetchPlayers, 1000);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : `Failed to ${action} player`);
    }
  };

  const getPingColor = (ping: number) => {
    if (pingStatus !== 'supported') return 'text-gray-500';
    if (ping < 0) return 'text-gray-500';
    if (ping <= 50) return 'text-green-400';
    if (ping <= 100) return 'text-yellow-400';
    if (ping <= 200) return 'text-amber-400';
    if (ping <= 300) return 'text-red-400';
    return 'text-red-600';
  };

  const getPingDotColor = (ping: number) => {
    if (pingStatus !== 'supported') return 'bg-gray-500';
    if (ping < 0) return 'bg-gray-500';
    if (ping <= 50) return 'bg-green-400';
    if (ping <= 100) return 'bg-yellow-400';
    if (ping <= 200) return 'bg-amber-400';
    if (ping <= 300) return 'bg-red-400';
    return 'bg-red-600';
  };

  const pingHelpText = pingStatus === 'unsupported_server_type'
    ? 'Not supported on this server type'
    : pingStatus === 'missing_pingplayer_mod'
      ? 'Install the PlayerPing mod for this to work.'
      : 'Unable to show ping, install pingplayer plugin in order to be able to see player\'s ping';

  return (
    <div className="flex flex-col h-full bg-[#1e1e1d] p-4 md:p-6">
      <div className="flex flex-col md:flex-row md:justify-between md:items-center gap-4 mb-6">
        <div className="relative w-full md:w-64">
          <Search className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500" size={16} />
          <input
            type="text"
            placeholder="Search players..."
            value={filter}
            onChange={e => setFilter(e.target.value)}
            className="w-full bg-[#252524] border border-[#3a3a3a] rounded-full py-2 pl-9 pr-4 text-sm text-white focus:outline-none focus:border-[#E5B80B]"
          />
        </div>
        <div className="text-sm text-gray-400">
          Online: <span className="text-white font-bold">{players.length}</span>
          <span className="text-gray-500">/{server.maxPlayers}</span>
        </div>
      </div>

      {loading ? (
        <div className="flex items-center justify-center flex-1">
          <Loader2 size={32} className="animate-spin text-gray-500" />
        </div>
      ) : (
        <div className="flex-1 overflow-auto">
          <table className="w-full text-left text-sm border-separate border-spacing-0">
            <thead className="bg-[#252524] text-gray-400 sticky top-0 z-10">
              <tr>
                <th className="px-4 py-3 font-medium border-b border-[#3a3a3a]">Player</th>
                <th className="px-4 py-3 font-medium border-b border-[#3a3a3a] hidden md:table-cell">IP Address</th>
                <th className="px-4 py-3 font-medium border-b border-[#3a3a3a] hidden lg:table-cell">
                  {pingStatus === 'unsupported_server_type' ? (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <span className="cursor-help">Ping</span>
                      </TooltipTrigger>
                      <TooltipContent className="bg-[#252524] border border-[#3a3a3a] px-3 py-2 text-gray-200">
                        Not supported on this server type
                      </TooltipContent>
                    </Tooltip>
                  ) : (
                    'Ping'
                  )}
                </th>
                <th className="px-4 py-3 font-medium border-b border-[#3a3a3a] hidden sm:table-cell">Online Time</th>
                <th className="px-4 py-3 font-medium border-b border-[#3a3a3a] hidden lg:table-cell">Current World</th>
                <th className="px-4 py-3 font-medium border-b border-[#3a3a3a] text-right">Actions</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-[#2a2a29]">
              {filteredPlayers.map(player => (
                <tr key={player.name} className="hover:bg-[#252524] transition-colors group">
                  <td className="px-4 py-3 text-white flex items-center gap-3">
                    <img
                      src={`https://mc-heads.net/avatar/${player.name}/32`}
                      alt=""
                      className="w-8 h-8 rounded bg-[#333]"
                      onError={(e) => { (e.target as HTMLImageElement).src = `https://api.dicebear.com/7.x/initials/svg?seed=${player.name}&backgroundColor=E5B80B`; }}
                    />
                    <div>
                      <div className="font-bold">{player.name}</div>
                    </div>
                  </td>
                  <td className="px-4 py-3 text-gray-400 hidden md:table-cell font-mono">{player.ip}</td>
                  <td className="px-4 py-3 hidden lg:table-cell">
                    {pingStatus === 'supported' ? (
                      <div className={clsx("flex items-center gap-1.5 font-mono", getPingColor(player.ping))}>
                        <div className={clsx("w-2 h-2 rounded-full", getPingDotColor(player.ping))} />
                        {player.ping >= 0 ? `${player.ping}ms` : '-'}
                      </div>
                    ) : (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <div
                            className={clsx("inline-flex items-center gap-1.5 font-mono cursor-help", getPingColor(player.ping))}
                            tabIndex={0}
                          >
                            <div className={clsx("w-2 h-2 rounded-full", getPingDotColor(player.ping))} />
                            -
                          </div>
                        </TooltipTrigger>
                        <TooltipContent className="bg-[#252524] border border-[#3a3a3a] px-3 py-2 text-gray-200">
                          {pingHelpText}
                        </TooltipContent>
                      </Tooltip>
                    )}
                  </td>
                  <td className="px-4 py-3 text-gray-300 hidden sm:table-cell">{player.onlineTime}</td>
                  <td className="px-4 py-3 text-gray-300 hidden lg:table-cell">{player.world || '-'}</td>
                  <td className="px-4 py-3 text-right">
                    <div className="flex items-center justify-end gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                      <ActionBtn icon={UserX} label="Kick" color="hover:bg-yellow-900/40 hover:text-yellow-500" onClick={() => handleAction(player.name, 'kick')} />
                      <ActionBtn icon={Ban} label="Ban" color="hover:bg-red-900/40 hover:text-red-500" onClick={() => handleAction(player.name, 'ban')} />
                      <ActionBtn icon={Skull} label="Kill" color="hover:bg-gray-700 hover:text-gray-300" onClick={() => handleAction(player.name, 'kill')} />
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>

          {filteredPlayers.length === 0 && players.length > 0 && (
            <div className="text-center py-12 text-gray-500">No players found matching your filter.</div>
          )}
          {players.length === 0 && (
            <div className="text-center py-12 text-gray-500">
              <UserX size={48} className="mx-auto mb-4 opacity-20" />
              <p>No players currently online.</p>
            </div>
          )}
        </div>
      )}
    </div>
  );
};

const ActionBtn = ({ icon: Icon, label, color, onClick }: { icon: any, label: string, color: string, onClick: () => void }) => (
  <button
    onClick={onClick}
    className={`p-1.5 rounded transition-colors text-gray-500 ${color}`}
    title={label}
  >
    <Icon size={16} />
  </button>
);
