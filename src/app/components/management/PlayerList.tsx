import React, { useState, useEffect, useCallback } from 'react';
import { Server, Player } from '../../context/ServerContext';
import { UserX, Ban, Skull, Search, Loader2, ChevronDown, ChevronUp } from 'lucide-react';
import { toast } from 'sonner';
import clsx from 'clsx';
import { AnimatePresence, motion } from 'motion/react';
import { Tooltip, TooltipTrigger, TooltipContent } from '../ui/tooltip';
import { apiRequest, toErrorMessage } from '../../lib/api';

interface PlayerListProps {
  server: Server;
}

export const PlayerList = ({ server }: PlayerListProps) => {
  const [players, setPlayers] = useState<Player[]>([]);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState('');
  const [pingStatus, setPingStatus] = useState<'supported' | 'missing_pingplayer' | 'missing_pingplayer_mod' | 'unsupported_server_type'>('supported');
  const [pollMs, setPollMs] = useState(4000);
  const [dataStale, setDataStale] = useState(false);
  const [openPlayerName, setOpenPlayerName] = useState<string | null>(null);
  const [mobileMenuView, setMobileMenuView] = useState<'details' | 'actions'>('details');

  useEffect(() => {
    let mounted = true;
    const loadPollSettings = async () => {
      try {
        const data = await apiRequest<{ playerSyncInterval?: number; statusPollInterval?: number }>(
          '/api/settings',
          undefined,
          'Failed to load settings'
        );
        const uiPollSeconds = Math.max(
          2,
          Number(data.playerSyncInterval || 0) || Number(data.statusPollInterval || 0) || 3
        );
        if (mounted) {
          setPollMs(uiPollSeconds * 1000);
        }
      } catch {
        // Keep default interval when settings cannot be loaded.
      }
    };
    loadPollSettings();
    return () => {
      mounted = false;
    };
  }, []);

  const fetchPlayers = useCallback(async () => {
    try {
      const data = await apiRequest<{ players?: Player[]; pingSupported?: boolean; pingStatus?: string; dataStale?: boolean } | Player[]>(
        `/api/servers/${server.id}/players`,
        undefined,
        'Failed to fetch players'
      );
      if (Array.isArray(data)) {
        setPlayers(data);
        setPingStatus('supported');
        setDataStale(false);
      } else {
        setPlayers(data.players || []);
        setDataStale(Boolean(data.dataStale));
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
      console.error(toErrorMessage(err, 'Failed to fetch players'));
    } finally {
      setLoading(false);
    }
  }, [server.id]);

  useEffect(() => {
    if (server.type === 'Velocity') {
      setPlayers([]);
      setLoading(false);
      return;
    }
    if (server.status !== 'Running') {
      setPlayers([]);
      setLoading(false);
      return;
    }
    fetchPlayers();
    const interval = setInterval(fetchPlayers, pollMs);
    return () => clearInterval(interval);
  }, [server.type, server.status, fetchPlayers, pollMs]);

  if (server.type === 'Velocity') {
    return (
      <div className="flex flex-col items-center justify-center h-full text-gray-500 gap-4">
        <UserX size={48} className="opacity-20" />
        <p>Not supported on this server type.</p>
      </div>
    );
  }

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
  const animateRows = filteredPlayers.length <= 40;

  useEffect(() => {
    if (!openPlayerName) return;
    const stillVisible = filteredPlayers.some((player) => player.name === openPlayerName);
    if (!stillVisible) {
      setOpenPlayerName(null);
      setMobileMenuView('details');
    }
  }, [filteredPlayers, openPlayerName]);

  const handleAction = async (playerName: string, action: 'kick' | 'ban' | 'kill') => {
    try {
      await apiRequest(
        `/api/servers/${server.id}/players/${encodeURIComponent(playerName)}/${action}`,
        {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({}),
        },
        `Failed to ${action} player`
      );
      const labels = { kick: 'Kicked', ban: 'Banned', kill: 'Killed' };
      toast.success(`${labels[action]} ${playerName}`);
      setTimeout(fetchPlayers, 1000);
    } catch (err) {
      toast.error(toErrorMessage(err, `Failed to ${action} player`));
    }
  };

  const handleMobileToggle = (playerName: string) => {
    if (openPlayerName === playerName) {
      setOpenPlayerName(null);
      setMobileMenuView('details');
      return;
    }
    setOpenPlayerName(playerName);
    setMobileMenuView('details');
  };

  const handleMobileAction = (playerName: string, action: 'kick' | 'ban' | 'kill') => {
    handleAction(playerName, action);
    setOpenPlayerName(null);
    setMobileMenuView('details');
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

  const getUuidAvatarUrl = (playerUuid: string) => `https://mc-heads.net/avatar/${encodeURIComponent(playerUuid)}/32`;
  const getNameAvatarUrl = (playerName: string) => `https://mc-heads.net/avatar/${encodeURIComponent(playerName)}/32`;
  const getInitialsAvatarUrl = (playerName: string) => `https://api.dicebear.com/7.x/initials/svg?seed=${encodeURIComponent(playerName)}&backgroundColor=E5B80B`;
  const getPlayerAvatarUrl = (player: Player) => {
    const playerUuid = (player.uuid || '').trim();
    if (playerUuid) {
      return getUuidAvatarUrl(playerUuid);
    }
    return getNameAvatarUrl(player.name);
  };
  const handleAvatarError = (event: React.SyntheticEvent<HTMLImageElement>, player: Player) => {
    const img = event.currentTarget;
    const stage = img.dataset.fallbackStage || 'primary';
    const hasUUID = Boolean((player.uuid || '').trim());

    if (stage === 'primary' && hasUUID) {
      img.dataset.fallbackStage = 'name';
      img.src = getNameAvatarUrl(player.name);
      return;
    }

    img.dataset.fallbackStage = 'initials';
    img.src = getInitialsAvatarUrl(player.name);
  };

  return (
    <motion.div
      initial={{ opacity: 0, y: 8 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.2, ease: 'easeOut' }}
      className="flex flex-col h-full bg-[#1e1e1d] p-4 md:p-6"
    >
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
          {dataStale && <span className="ml-2 text-amber-400 text-xs">Syncing...</span>}
        </div>
      </div>

      {loading ? (
        <div className="flex items-center justify-center flex-1">
          <Loader2 size={32} className="animate-spin text-gray-500" />
        </div>
      ) : (
        <div className="flex-1 overflow-auto">
          <div className="hidden md:block">
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
                {filteredPlayers.map((player, index) => (
                  <motion.tr
                    key={player.name}
                    initial={animateRows ? { opacity: 0, y: 3 } : false}
                    animate={animateRows ? { opacity: 1, y: 0 } : false}
                    transition={animateRows ? { duration: 0.16, delay: Math.min(index, 12) * 0.012, ease: 'easeOut' } : undefined}
                    className="hover:bg-[#252524] transition-colors group"
                  >
                    <td className="px-4 py-3 text-white flex items-center gap-3">
                      <img
                        src={getPlayerAvatarUrl(player)}
                        alt=""
                        data-fallback-stage="primary"
                        className="w-8 h-8 rounded bg-[#333]"
                        onError={(event) => handleAvatarError(event, player)}
                      />
                      <div>
                        <div className="font-bold">{player.name}</div>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-gray-400 hidden md:table-cell font-mono">{player.ip || '-'}</td>
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
                  </motion.tr>
                ))}
              </tbody>
            </table>
          </div>

          <div className="md:hidden space-y-2">
            {filteredPlayers.map((player, index) => {
              const isOpen = openPlayerName === player.name;
              const showActions = isOpen && mobileMenuView === 'actions';
              return (
                <motion.div
                  key={player.name}
                  initial={animateRows ? { opacity: 0, y: 3 } : false}
                  animate={animateRows ? { opacity: 1, y: 0 } : false}
                  transition={animateRows ? { duration: 0.16, delay: Math.min(index, 12) * 0.012, ease: 'easeOut' } : undefined}
                  className="overflow-hidden rounded border border-[#3a3a3a] bg-[#252524]"
                >
                  <button
                    type="button"
                    onClick={() => handleMobileToggle(player.name)}
                    className="w-full px-3 py-2.5 flex items-center justify-between text-left"
                  >
                    <div className="flex items-center gap-3 min-w-0">
                      <div className="flex items-center gap-2">
                        <div className={clsx("w-2 h-2 rounded-full", getPingDotColor(player.ping))} />
                        <img
                          src={getPlayerAvatarUrl(player)}
                          alt=""
                          data-fallback-stage="primary"
                          className="w-8 h-8 rounded bg-[#333]"
                          onError={(event) => handleAvatarError(event, player)}
                        />
                      </div>
                      <span className="text-white font-bold truncate">{player.name}</span>
                    </div>
                    {isOpen ? <ChevronUp size={17} className="text-gray-400 shrink-0" /> : <ChevronDown size={17} className="text-gray-400 shrink-0" />}
                  </button>

                  <AnimatePresence initial={false}>
                    {isOpen && (
                      <motion.div
                        initial={{ opacity: 0, height: 0, y: -4 }}
                        animate={{ opacity: 1, height: 'auto', y: 0 }}
                        exit={{ opacity: 0, height: 0, y: -4 }}
                        transition={{ duration: 0.2, ease: 'easeOut' }}
                        className="overflow-hidden border-t border-[#333]"
                      >
                        {!showActions ? (
                          <div className="px-3 py-2 text-sm">
                            <MobileInfoRow label="IP Address" value={player.ip || '-'} mono />
                            <MobileInfoRow
                              label="Ping"
                              value={
                                pingStatus === 'supported'
                                  ? (player.ping >= 0 ? `${player.ping}ms` : '-')
                                  : '-'
                              }
                              valueClassName={clsx("inline-flex items-center gap-1.5", getPingColor(player.ping))}
                              extra={pingStatus !== 'supported' ? <span className="text-[11px] text-gray-500">{pingHelpText}</span> : null}
                            />
                            <MobileInfoRow label="Online Time" value={player.onlineTime || '-'} />
                            <MobileInfoRow label="Current World" value={player.world || '-'} />
                            <button
                              type="button"
                              onClick={() => setMobileMenuView('actions')}
                              className="w-full mt-2 px-2 py-2 rounded border border-[#3a3a3a] bg-[#1f1f1f] text-left text-gray-200 hover:border-[#E5B80B]/60 hover:text-white transition-colors"
                            >
                              Actions
                            </button>
                          </div>
                        ) : (
                          <div className="px-3 py-2.5 space-y-2">
                            <button
                              type="button"
                              onClick={() => handleMobileAction(player.name, 'kick')}
                              className="w-full px-3 py-2 rounded border border-yellow-700/40 bg-yellow-900/15 text-yellow-300 text-left hover:bg-yellow-900/30 transition-colors"
                            >
                              Kick
                            </button>
                            <button
                              type="button"
                              onClick={() => handleMobileAction(player.name, 'ban')}
                              className="w-full px-3 py-2 rounded border border-red-700/40 bg-red-900/15 text-red-300 text-left hover:bg-red-900/30 transition-colors"
                            >
                              Ban
                            </button>
                            <button
                              type="button"
                              onClick={() => handleMobileAction(player.name, 'kill')}
                              className="w-full px-3 py-2 rounded border border-gray-600 bg-[#1f1f1f] text-gray-200 text-left hover:bg-[#2b2b2b] transition-colors"
                            >
                              Kill
                            </button>
                          </div>
                        )}
                      </motion.div>
                    )}
                  </AnimatePresence>
                </motion.div>
              );
            })}
          </div>

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
    </motion.div>
  );
};

const MobileInfoRow = ({
  label,
  value,
  mono,
  valueClassName,
  extra,
}: {
  label: string;
  value: string;
  mono?: boolean;
  valueClassName?: string;
  extra?: React.ReactNode;
}) => (
  <div className="py-1.5 border-b border-[#2f2f2f] last:border-b-0">
    <div className="text-[11px] uppercase tracking-wider text-gray-500">{label}</div>
    <div className={clsx("mt-0.5 text-sm text-gray-200", mono && "font-mono", valueClassName)}>{value}</div>
    {extra}
  </div>
);

const ActionBtn = ({ icon: Icon, label, color, onClick }: { icon: any, label: string, color: string, onClick: () => void }) => (
  <button
    onClick={onClick}
    className={`p-1.5 rounded transition-colors text-gray-500 ${color}`}
    title={label}
  >
    <Icon size={16} />
  </button>
);
