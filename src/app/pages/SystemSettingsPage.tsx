import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { Loader2, Cpu, HardDrive, ChevronDown, ChevronUp, Settings, Square, X } from 'lucide-react';
import { Area, AreaChart, ResponsiveContainer } from 'recharts';
import { toast } from 'sonner';
import clsx from 'clsx';
import { AnimatePresence, motion } from 'motion/react';
import { Tooltip, TooltipContent, TooltipTrigger } from '../components/ui/tooltip';
import { useServer } from '../context/ServerContext';
import { apiRequest, toErrorMessage } from '../lib/api';

type View = 'servers' | 'management' | 'plugins' | 'backups' | 'logs' | 'cloning' | 'settings';

type UsageHost = {
  logicalCpuCount: number;
  totalRamBytes: number;
};

type UsageProcess = {
  id?: string;
  name: string;
  type?: string;
  status?: string;
  pid: number;
  cpuPercent: number;
  ramBytes: number;
  ramPercent: number;
};

type UsageTotals = {
  cpuPercent: number;
  ramBytes: number;
  ramPercent: number;
};

type SystemUsage = {
  timestamp: string;
  host: UsageHost;
  panel: UsageProcess;
  servers: UsageProcess[];
  total: UsageTotals;
};

type UsageHistoryPoint = {
  cpu: number;
  ram: number;
};

type SettingsSnapshot = {
  loginUser: string;
  loginPassword: string;
  userAgent: string;
  defaultMinRam: string;
  defaultMaxRam: string;
  defaultFlags: string;
  statusPollInterval: string;
  tpsPollInterval: string;
  playerSyncInterval: string;
  pingPollInterval: string;
};

type SystemSettingsPageProps = {
  onViewChange?: (view: View) => void;
};

const DETAILS_STORAGE_KEY = 'orexa.systemSettings.detailsOpen';

const emptyUsage: SystemUsage = {
  timestamp: '',
  host: { logicalCpuCount: 0, totalRamBytes: 0 },
  panel: { name: 'Orexa Panel', pid: 0, cpuPercent: 0, ramBytes: 0, ramPercent: 0 },
  servers: [],
  total: { cpuPercent: 0, ramBytes: 0, ramPercent: 0 },
};

const formatBytes = (bytes: number) => {
  if (!Number.isFinite(bytes) || bytes <= 0) return '0 MB';
  const mb = bytes / 1024 / 1024;
  if (mb < 1024) return `${mb.toFixed(2)} MB`;
  return `${(mb / 1024).toFixed(2)} GB`;
};

const MiniMetricChart = ({
  title,
  value,
  color,
  unit,
  points,
}: {
  title: string;
  value: number;
  color: string;
  unit: string;
  points: { v: number }[];
}) => (
  <div className="bg-[#1a1a1a] border border-[#333] rounded p-3">
    <div className="flex justify-between items-center mb-2">
      <span className="text-xs uppercase tracking-wider text-gray-500 font-bold">{title}</span>
      <span className="font-mono text-white text-sm font-bold">{value.toFixed(2)}{unit}</span>
    </div>
    <div className="h-20">
      <ResponsiveContainer width="100%" height="100%">
        <AreaChart data={points}>
          <defs>
            <linearGradient id={`overall-grad-${title}`} x1="0" y1="0" x2="0" y2="1">
              <stop offset="5%" stopColor={color} stopOpacity={0.35} />
              <stop offset="95%" stopColor={color} stopOpacity={0} />
            </linearGradient>
          </defs>
          <Area
            type="monotone"
            dataKey="v"
            stroke={color}
            strokeWidth={2}
            fill={`url(#overall-grad-${title})`}
            isAnimationActive={false}
          />
        </AreaChart>
      </ResponsiveContainer>
    </div>
  </div>
);

export const SystemSettingsPage = ({ onViewChange }: SystemSettingsPageProps) => {
  const { setActiveServerId, stopServer } = useServer();
  const [passwordMinLength, setPasswordMinLength] = useState(10);
  const [loginUser, setLoginUser] = useState('mcpanel');
  const [loginPassword, setLoginPassword] = useState('');
  const [userAgent, setUserAgent] = useState('');
  const [defaultMinRam, setDefaultMinRam] = useState('0.5');
  const [defaultMaxRam, setDefaultMaxRam] = useState('1');
  const [defaultFlags, setDefaultFlags] = useState('none');
  const [statusPollInterval, setStatusPollInterval] = useState('3');
  const [tpsPollInterval, setTpsPollInterval] = useState('30');
  const [playerSyncInterval, setPlayerSyncInterval] = useState('15');
  const [pingPollInterval, setPingPollInterval] = useState('20');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  const [usageLoading, setUsageLoading] = useState(true);
  const [usageError, setUsageError] = useState<string | null>(null);
  const [usage, setUsage] = useState<SystemUsage>(emptyUsage);
  const [detailsOpen, setDetailsOpen] = useState(false);
  const [stoppingServerIds, setStoppingServerIds] = useState<Set<string>>(new Set());
  const [usageHistory, setUsageHistory] = useState<UsageHistoryPoint[]>(
    Array.from({ length: 25 }, () => ({ cpu: 0, ram: 0 }))
  );
  const [savedSnapshot, setSavedSnapshot] = useState<SettingsSnapshot | null>(null);

  const currentSnapshot = useMemo<SettingsSnapshot>(
    () => ({
      loginUser,
      loginPassword,
      userAgent,
      defaultMinRam,
      defaultMaxRam,
      defaultFlags,
      statusPollInterval,
      tpsPollInterval,
      playerSyncInterval,
      pingPollInterval,
    }),
    [
      defaultFlags,
      defaultMaxRam,
      defaultMinRam,
      loginPassword,
      loginUser,
      pingPollInterval,
      playerSyncInterval,
      statusPollInterval,
      tpsPollInterval,
      userAgent,
    ]
  );

  const hasUnsavedChanges = useMemo(() => {
    if (!savedSnapshot) return false;
    return (
      currentSnapshot.loginUser !== savedSnapshot.loginUser ||
      currentSnapshot.loginPassword !== savedSnapshot.loginPassword ||
      currentSnapshot.userAgent !== savedSnapshot.userAgent ||
      currentSnapshot.defaultMinRam !== savedSnapshot.defaultMinRam ||
      currentSnapshot.defaultMaxRam !== savedSnapshot.defaultMaxRam ||
      currentSnapshot.defaultFlags !== savedSnapshot.defaultFlags ||
      currentSnapshot.statusPollInterval !== savedSnapshot.statusPollInterval ||
      currentSnapshot.tpsPollInterval !== savedSnapshot.tpsPollInterval ||
      currentSnapshot.playerSyncInterval !== savedSnapshot.playerSyncInterval ||
      currentSnapshot.pingPollInterval !== savedSnapshot.pingPollInterval
    );
  }, [currentSnapshot, savedSnapshot]);

  const fetchUsage = useCallback(async () => {
    try {
      const data = await apiRequest<SystemUsage>('/api/system/usage', undefined, 'Failed to load overall usage');
      setUsage(data);
      setUsageError(null);
      setUsageHistory(prev => {
        const next = [...prev, { cpu: data.total.cpuPercent || 0, ram: data.total.ramPercent || 0 }];
        return next.slice(-25);
      });
    } catch (err) {
      setUsageError(toErrorMessage(err, 'Failed to load overall usage'));
    } finally {
      setUsageLoading(false);
    }
  }, []);

  useEffect(() => {
    let isMounted = true;
    const loadSettings = async () => {
      try {
        const data = await apiRequest('/api/settings', undefined, 'Couldn’t load settings.');
        if (isMounted) {
          setLoginUser(data.loginUser || 'mcpanel');
          if (typeof data.passwordMinLength === 'number' && data.passwordMinLength > 0) {
            setPasswordMinLength(data.passwordMinLength);
          }
          setUserAgent(data.userAgent || '');
          setDefaultMinRam(data.defaultMinRam || '0.5');
          setDefaultMaxRam(data.defaultMaxRam || '1');
          setDefaultFlags(data.defaultFlags || 'none');
          setStatusPollInterval(String(data.statusPollInterval || 3));
          setTpsPollInterval(String(data.tpsPollInterval || 30));
          setPlayerSyncInterval(String(data.playerSyncInterval || 15));
          setPingPollInterval(String(data.pingPollInterval || 20));
          setSavedSnapshot({
            loginUser: data.loginUser || 'mcpanel',
            loginPassword: '',
            userAgent: data.userAgent || '',
            defaultMinRam: data.defaultMinRam || '0.5',
            defaultMaxRam: data.defaultMaxRam || '1',
            defaultFlags: data.defaultFlags || 'none',
            statusPollInterval: String(data.statusPollInterval || 3),
            tpsPollInterval: String(data.tpsPollInterval || 30),
            playerSyncInterval: String(data.playerSyncInterval || 15),
            pingPollInterval: String(data.pingPollInterval || 20),
          });
        }
      } catch (err) {
        toast.error(toErrorMessage(err, 'Couldn’t load settings.'));
      } finally {
        if (isMounted) setLoading(false);
      }
    };
    loadSettings();
    return () => {
      isMounted = false;
    };
  }, []);

  useEffect(() => {
    let alive = true;

    const safeFetch = async () => {
      if (!alive) return;
      await fetchUsage();
    };

    safeFetch();
    const interval = setInterval(() => {
      if (!alive) return;
      safeFetch();
    }, 2000);
    return () => {
      alive = false;
      clearInterval(interval);
    };
  }, [fetchUsage]);

  useEffect(() => {
    try {
      const raw = window.localStorage.getItem(DETAILS_STORAGE_KEY);
      if (!raw) return;
      setDetailsOpen(raw === '1');
    } catch {
      // ignore persistence errors
    }
  }, []);

  const cpuSeries = useMemo(() => usageHistory.map(p => ({ v: p.cpu })), [usageHistory]);
  const ramSeries = useMemo(() => usageHistory.map(p => ({ v: p.ram })), [usageHistory]);

  const handleOpenManagement = (serverId: string) => {
    setActiveServerId(serverId);
    onViewChange?.('management');
  };

  const handleStopFromUsage = async (serverId: string, serverName: string) => {
    setStoppingServerIds(prev => {
      const next = new Set(prev);
      next.add(serverId);
      return next;
    });
    try {
      await stopServer(serverId);
      toast.success(`${serverName} stopping...`);
      setUsage(prev => ({
        ...prev,
        servers: prev.servers.filter(server => server.id !== serverId),
      }));
      await fetchUsage();
    } catch (err) {
      toast.error(toErrorMessage(err, 'Failed to stop server'));
    } finally {
      setStoppingServerIds(prev => {
        const next = new Set(prev);
        next.delete(serverId);
        return next;
      });
    }
  };

  const handleSave = async () => {
    const trimmedLoginUser = loginUser.trim();
    if (trimmedLoginUser.length < 4 || trimmedLoginUser.length > 12) {
      toast.error('Username must be between 4 and 12 characters.');
      return;
    }
    if (loginPassword.trim() && loginPassword.length < passwordMinLength) {
      toast.error(`Password must be at least ${passwordMinLength} characters.`);
      return;
    }

    const pollInterval = parseInt(String(statusPollInterval), 10);
    if (isNaN(pollInterval) || pollInterval < 1 || pollInterval > 30) {
      toast.error('Please enter a valid number.');
      return;
    }
    const parsedTpsPoll = parseInt(String(tpsPollInterval), 10);
    if (isNaN(parsedTpsPoll) || parsedTpsPoll < 5 || parsedTpsPoll > 300) {
      toast.error('TPS poll interval must be between 5 and 300 seconds.');
      return;
    }
    const parsedPlayerSync = parseInt(String(playerSyncInterval), 10);
    if (isNaN(parsedPlayerSync) || parsedPlayerSync < 2 || parsedPlayerSync > 300) {
      toast.error('Player sync interval must be between 2 and 300 seconds.');
      return;
    }
    const parsedPingPoll = parseInt(String(pingPollInterval), 10);
    if (isNaN(parsedPingPoll) || parsedPingPoll < 5 || parsedPingPoll > 300) {
      toast.error('Ping poll interval must be between 5 and 300 seconds.');
      return;
    }

    setSaving(true);
    try {
      await apiRequest(
        '/api/settings',
        {
          method: 'PUT',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            loginUser: trimmedLoginUser,
            loginPassword,
            userAgent,
            defaultMinRam,
            defaultMaxRam,
            defaultFlags,
            statusPollInterval: pollInterval,
            tpsPollInterval: parsedTpsPoll,
            playerSyncInterval: parsedPlayerSync,
            pingPollInterval: parsedPingPoll,
          }),
        },
        'Couldn’t save settings. Try again.'
      );
      setLoginPassword('');
      setSavedSnapshot({
        loginUser: trimmedLoginUser,
        loginPassword: '',
        userAgent,
        defaultMinRam,
        defaultMaxRam,
        defaultFlags,
        statusPollInterval: String(pollInterval),
        tpsPollInterval: String(parsedTpsPoll),
        playerSyncInterval: String(parsedPlayerSync),
        pingPollInterval: String(parsedPingPoll),
      });
      toast.success('Applied changes.');
    } catch (err) {
      toast.error(toErrorMessage(err, 'Couldn’t save settings. Try again.'));
    } finally {
      setSaving(false);
    }
  };

  const handleDismissUnsavedChanges = () => {
    if (!savedSnapshot) return;
    setLoginUser(savedSnapshot.loginUser);
    setLoginPassword(savedSnapshot.loginPassword || '');
    setUserAgent(savedSnapshot.userAgent);
    setDefaultMinRam(savedSnapshot.defaultMinRam);
    setDefaultMaxRam(savedSnapshot.defaultMaxRam);
    setDefaultFlags(savedSnapshot.defaultFlags);
    setStatusPollInterval(savedSnapshot.statusPollInterval);
    setTpsPollInterval(savedSnapshot.tpsPollInterval);
    setPlayerSyncInterval(savedSnapshot.playerSyncInterval);
    setPingPollInterval(savedSnapshot.pingPollInterval);
    toast.info('Unsaved changes discarded.');
  };

  return (
    <div className="flex-1 p-4 md:p-8 overflow-y-auto">
      <div className="mb-8">
        <h2 className="text-3xl font-bold text-white mb-2">System Settings</h2>
        <p className="text-gray-400 text-sm">Global configuration for the admin panel.</p>
      </div>

      <div className="grid grid-cols-1 xl:grid-cols-[minmax(0,1.2fr)_minmax(460px,1fr)] gap-6 items-start">
        <div className="bg-[#202020] border border-[#3a3a3a] rounded-lg p-6">
          {loading ? (
            <div className="flex items-center gap-2 text-gray-500">
              <Loader2 size={18} className="animate-spin" />
              Loading settings...
            </div>
          ) : (
            <>
              <label className="block text-sm text-gray-400 mb-3">Login Credentials</label>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4 max-w-[780px]">
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Username</label>
                  <input
                    type="text"
                    value={loginUser}
                    onChange={(e) => setLoginUser(e.target.value)}
                    minLength={4}
                    maxLength={12}
                    className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B]"
                    disabled={saving}
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Password</label>
                  <input
                    type="password"
                    value={loginPassword}
                    onChange={(e) => setLoginPassword(e.target.value)}
                    placeholder="Leave empty to keep current password"
                    className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B]"
                    disabled={saving}
                  />
                </div>
              </div>
              <p className="text-xs text-gray-500 mt-1">Username must be between 4 and 12 characters.</p>
              <p className="text-xs text-gray-500 mt-1">If setting a new password, minimum length is {passwordMinLength} characters.</p>

              <hr className="border-[#3a3a3a] my-6" />
              <label className="block text-sm text-gray-400 mb-3">Default RAM Allocation</label>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Min RAM (GB)</label>
                  <input
                    type="number"
                    value={defaultMinRam}
                    onChange={(e) => setDefaultMinRam(e.target.value)}
                    min={0.5}
                    max={64}
                    step={0.5}
                    className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B]"
                    disabled={saving}
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Max RAM (GB)</label>
                  <input
                    type="number"
                    value={defaultMaxRam}
                    onChange={(e) => setDefaultMaxRam(e.target.value)}
                    min={0.5}
                    max={64}
                    step={0.5}
                    className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B]"
                    disabled={saving}
                  />
                </div>
              </div>
              <p className="text-xs text-gray-500 mt-2">Default RAM allocation for new servers.</p>

              <hr className="border-[#3a3a3a] my-6" />
              <label className="block text-sm text-gray-400 mb-3">Default JVM Flags Preset</label>
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
                    onClick={() => setDefaultFlags(opt.value)}
                    disabled={saving}
                    className={clsx(
                      'flex-1 min-w-[140px] px-4 py-3 rounded border text-left transition-all',
                      defaultFlags === opt.value
                        ? 'border-[#E5B80B] bg-[#E5B80B]/10 text-white'
                        : 'border-[#3a3a3a] bg-[#1a1a1a] text-gray-400 hover:border-[#E5B80B]/40 hover:text-white'
                    )}
                  >
                    <div className="text-sm font-medium">{opt.label}</div>
                    <div className="text-xs text-gray-500 mt-0.5">{opt.desc}</div>
                  </button>
                ))}
              </div>
              <p className="text-xs text-gray-500 mt-2">Pre-selected JVM flag preset when creating servers.</p>

              <hr className="border-[#3a3a3a] my-6" />
              <label className="block text-sm text-gray-400 mb-2">Status Polling Interval (seconds)</label>
              <input
                type="text"
                inputMode="numeric"
                value={statusPollInterval}
                onChange={(e) => setStatusPollInterval(e.target.value)}
                pattern="\d*"
                className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B] max-w-[200px]"
                disabled={saving}
              />
              <p className="text-xs text-gray-500 mt-2">How often the panel polls for server status updates (1-30 seconds). Lower values = more responsive.</p>

              <hr className="border-[#3a3a3a] my-6" />
              <label className="block text-sm text-gray-400 mb-3">Live Data Polling (seconds)</label>
              <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
                <div>
                  <label className="block text-xs text-gray-500 mb-1">TPS Poll</label>
                  <input
                    type="text"
                    inputMode="numeric"
                    value={tpsPollInterval}
                    onChange={(e) => setTpsPollInterval(e.target.value)}
                    pattern="\d*"
                    className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B]"
                    disabled={saving}
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Player Sync</label>
                  <input
                    type="text"
                    inputMode="numeric"
                    value={playerSyncInterval}
                    onChange={(e) => setPlayerSyncInterval(e.target.value)}
                    pattern="\d*"
                    className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B]"
                    disabled={saving}
                  />
                </div>
                <div>
                  <label className="block text-xs text-gray-500 mb-1">Ping Poll</label>
                  <input
                    type="text"
                    inputMode="numeric"
                    value={pingPollInterval}
                    onChange={(e) => setPingPollInterval(e.target.value)}
                    pattern="\d*"
                    className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B]"
                    disabled={saving}
                  />
                </div>
              </div>
              <p className="text-xs text-gray-500 mt-2">How often will these values be updated (seconds).</p>

              <div className="flex justify-end mt-8">
                <button
                  onClick={handleSave}
                  className="px-5 py-2 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded font-bold disabled:opacity-50"
                  disabled={saving}
                >
                  {saving ? 'Saving...' : 'Save'}
                </button>
              </div>
            </>
          )}
        </div>

        <div className="xl:sticky xl:top-8 bg-[#202020] border border-[#3a3a3a] rounded-lg p-4">
          <div className="flex items-center justify-between mb-3">
            <h3 className="text-sm text-gray-300 font-semibold uppercase tracking-wider">Overall Usage</h3>
            <button
              type="button"
              onClick={() =>
                setDetailsOpen((prev) => {
                  const next = !prev;
                  try {
                    window.localStorage.setItem(DETAILS_STORAGE_KEY, next ? '1' : '0');
                  } catch {
                    // ignore persistence errors
                  }
                  return next;
                })
              }
              className="px-3 py-1.5 text-xs bg-[#252524] border border-[#3a3a3a] rounded text-gray-200 hover:border-[#E5B80B] hover:text-white transition-colors inline-flex items-center gap-2"
            >
              Detailed View
              {detailsOpen ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
            </button>
          </div>

          {usageLoading ? (
            <div className="h-[220px] flex items-center justify-center text-gray-500 text-sm">
              <Loader2 size={18} className="animate-spin mr-2" /> Loading usage...
            </div>
          ) : (
            <>
              <div className="grid grid-cols-1 gap-3">
                <MiniMetricChart title="CPU" value={usage.total.cpuPercent || 0} color="#E5B80B" unit="%" points={cpuSeries} />
                <MiniMetricChart title="RAM" value={usage.total.ramPercent || 0} color="#3b82f6" unit="%" points={ramSeries} />
              </div>

              <div className="mt-3 p-3 rounded border border-[#333] bg-[#151515] text-sm">
                <div className="flex items-center justify-between text-gray-300">
                  <span className="inline-flex items-center gap-2"><Cpu size={14} className="text-[#E5B80B]" />Total CPU</span>
                  <span className="font-mono text-white">{(usage.total.cpuPercent || 0).toFixed(2)}%</span>
                </div>
                <div className="flex items-center justify-between mt-2 text-gray-300">
                  <span className="inline-flex items-center gap-2"><HardDrive size={14} className="text-blue-400" />Total RAM</span>
                  <span className="font-mono text-white">{formatBytes(usage.total.ramBytes || 0)} ({(usage.total.ramPercent || 0).toFixed(2)}%)</span>
                </div>
                {usageError && <p className="text-xs text-red-400 mt-2">{usageError}</p>}
              </div>

              <AnimatePresence initial={false}>
                {detailsOpen && (
                  <motion.div
                    initial={{ opacity: 0, height: 0, y: -4 }}
                    animate={{ opacity: 1, height: 'auto', y: 0 }}
                    exit={{ opacity: 0, height: 0, y: -4 }}
                    transition={{ duration: 0.2, ease: 'easeOut' }}
                    className="overflow-hidden mt-3"
                  >
                    <div className="border border-[#333] rounded bg-[#171717]">
                      <div className="px-3 py-2 border-b border-[#333] text-[11px] text-gray-500 uppercase tracking-wider">
                        Host: {usage.host.logicalCpuCount || '-'} CPUs | RAM: {formatBytes(usage.host.totalRamBytes || 0)}
                      </div>

                      <div className="max-h-[380px] overflow-y-auto">
                        <div className="grid grid-cols-[1.2fr_0.8fr_0.9fr_0.7fr_88px] gap-2 px-3 py-2 text-[10px] uppercase tracking-wider text-gray-500 border-b border-[#2f2f2f]">
                          <span>Process</span>
                          <span>CPU</span>
                          <span>RAM</span>
                          <span>PID</span>
                          <span className="text-center">Action</span>
                        </div>

                        <div className="grid grid-cols-[1.2fr_0.8fr_0.9fr_0.7fr_88px] gap-2 px-3 py-2 text-xs border-b border-[#2a2a2a]">
                          <span className="text-white">Orexa Panel</span>
                          <span className="font-mono text-[#E5B80B]">{(usage.panel.cpuPercent || 0).toFixed(2)}%</span>
                          <span className="font-mono text-blue-300">{formatBytes(usage.panel.ramBytes || 0)}</span>
                          <span className="font-mono text-gray-300">{usage.panel.pid || '-'}</span>
                          <span className="text-center text-gray-600">-</span>
                        </div>

                        {usage.servers.map((server) => (
                          <div key={`${server.id}-${server.pid}`} className="grid grid-cols-[1.2fr_0.8fr_0.9fr_0.7fr_88px] gap-2 px-3 py-2 text-xs border-b border-[#2a2a2a] items-center">
                            <div>
                              <div className="text-white truncate">{server.name}</div>
                              <div className="text-[10px] text-gray-500 uppercase truncate">{server.type} {server.status ? `- ${server.status}` : ''}</div>
                            </div>
                            <span className="font-mono text-[#E5B80B]">{(server.cpuPercent || 0).toFixed(2)}%</span>
                            <span className="font-mono text-blue-300">{formatBytes(server.ramBytes || 0)}</span>
                            <span className="font-mono text-gray-300">{server.pid || '-'}</span>
                            <div className="flex justify-center gap-1">
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <button
                                    type="button"
                                    disabled={!server.id || stoppingServerIds.has(server.id)}
                                    onClick={() => server.id && handleStopFromUsage(server.id, server.name)}
                                    className="h-7 w-7 rounded border border-red-700/70 bg-[#1a1a1a] text-red-400 hover:text-red-300 hover:border-red-500 transition-colors inline-flex items-center justify-center disabled:opacity-50 disabled:cursor-not-allowed"
                                  >
                                    <Square size={11} fill="currentColor" />
                                  </button>
                                </TooltipTrigger>
                                <TooltipContent className="bg-[#252524] border border-[#3a3a3a] px-3 py-1.5 text-gray-300">Stop</TooltipContent>
                              </Tooltip>
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <button
                                    type="button"
                                    onClick={() => server.id && handleOpenManagement(server.id)}
                                    className="h-7 w-7 rounded border border-[#3a3a3a] bg-[#1a1a1a] text-gray-300 hover:text-[#E5B80B] hover:border-[#E5B80B] transition-colors inline-flex items-center justify-center"
                                  >
                                    <Settings size={12} />
                                  </button>
                                </TooltipTrigger>
                                <TooltipContent className="bg-[#252524] border border-[#3a3a3a] px-3 py-1.5 text-gray-300">Manage</TooltipContent>
                              </Tooltip>
                            </div>
                          </div>
                        ))}

                        {usage.servers.length === 0 && (
                          <div className="px-3 py-5 text-center text-xs text-gray-500">No managed servers are currently running.</div>
                        )}
                      </div>
                    </div>
                  </motion.div>
                )}
              </AnimatePresence>
            </>
          )}
        </div>
      </div>

      {hasUnsavedChanges && !loading && (
        <div className="fixed bottom-4 right-4 z-[85] min-w-[340px]">
          <div className="relative overflow-hidden rounded-lg border border-[#3a3a3a] bg-[#252524] shadow-2xl">
            <div className="flex items-start justify-between gap-3 px-4 py-3">
              <div className="flex items-start gap-3 text-sm text-gray-200">
                <div className="mt-0.5 inline-flex h-6 w-6 items-center justify-center rounded-full bg-[#E5B80B]/20 text-[#E5B80B]">
                  <Settings size={14} />
                </div>
                <div>
                  <p className="font-semibold text-white leading-none">Unsaved changes</p>
                  <p className="text-sm text-gray-300 mt-1">Careful! You have unsaved changes.</p>
                </div>
              </div>
              <div className="flex items-center gap-2 pt-0.5">
                <button
                  type="button"
                  onClick={handleSave}
                  disabled={saving}
                  className="rounded bg-[#E5B80B] px-3 py-1.5 text-xs font-bold text-black hover:bg-[#d4a90a] disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center gap-2"
                >
                  {saving && <Loader2 size={12} className="animate-spin" />}
                  {saving ? 'Saving...' : 'Save'}
                </button>
                <button
                  type="button"
                  onClick={handleDismissUnsavedChanges}
                  disabled={saving}
                  className="inline-flex h-5 w-5 items-center justify-center rounded-full text-gray-400 hover:text-[#E5B80B] disabled:opacity-50 disabled:cursor-not-allowed"
                  aria-label="Discard unsaved changes"
                >
                  <X size={11} />
                </button>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
};
