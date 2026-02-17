import React, { useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { toast } from 'sonner';
import clsx from 'clsx';
import { Tooltip, TooltipTrigger, TooltipContent } from '../components/ui/tooltip';

export const SystemSettingsPage = () => {
  const [loginUser, setLoginUser] = useState('mcpanel');
  const [loginPassword, setLoginPassword] = useState('');
  const [userAgent, setUserAgent] = useState('');
  const [defaultMinRam, setDefaultMinRam] = useState('0.5');
  const [defaultMaxRam, setDefaultMaxRam] = useState('1');
  const [defaultFlags, setDefaultFlags] = useState('none');
  const [statusPollInterval, setStatusPollInterval] = useState('3');
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let isMounted = true;
    const loadSettings = async () => {
      try {
        const res = await fetch('/api/settings');
        if (!res.ok) throw new Error('Failed to load settings');
        const data = await res.json();
        if (isMounted) {
          setLoginUser(data.loginUser || 'mcpanel');
          setUserAgent(data.userAgent || '');
          setDefaultMinRam(data.defaultMinRam || '0.5');
          setDefaultMaxRam(data.defaultMaxRam || '1');
          setDefaultFlags(data.defaultFlags || 'none');
          setStatusPollInterval(String(data.statusPollInterval || 3));
        }
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to load settings');
      } finally {
        if (isMounted) setLoading(false);
      }
    };
    loadSettings();
    return () => { isMounted = false; };
  }, []);

  const handleSave = async () => {
    if (!loginUser.trim()) {
      toast.error('Insert a valid value.');
      return;
    }
    if (loginPassword.trim() && loginPassword.length < 4) {
      toast.error('Password must be at least 4 characters.');
      return;
    }

    // Validate Status Polling Interval
    const pollInterval = parseInt(String(statusPollInterval), 10);
    if (isNaN(pollInterval) || pollInterval < 1 || pollInterval > 30) {
      toast.error('Insert a valid value.');
      return;
    }

    setSaving(true);
    try {
      const res = await fetch('/api/settings', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          loginUser: loginUser.trim(),
          loginPassword,
          userAgent,
          defaultMinRam,
          defaultMaxRam,
          defaultFlags,
          statusPollInterval: pollInterval,
        }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to save settings');
      }
      setLoginPassword('');
      toast.success('Applied changes.');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save settings');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="flex-1 p-4 md:p-8 overflow-y-auto">
      <div className="mb-8">
        <h2 className="text-3xl font-bold text-white mb-2">System Settings</h2>
        <p className="text-gray-400 text-sm">Global configuration for the admin panel.</p>
      </div>

      <div className="bg-[#202020] border border-[#3a3a3a] rounded-lg p-6 max-w-3xl">
        {loading ? (
          <div className="flex items-center gap-2 text-gray-500">
            <Loader2 size={18} className="animate-spin" />
            Loading settings...
          </div>
        ) : (
          <>
            {/* Login Credentials */}
            <label className="block text-sm text-gray-400 mb-3">Login Credentials</label>
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="block text-xs text-gray-500 mb-1">Username</label>
                <input
                  type="text"
                  value={loginUser}
                  onChange={(e) => setLoginUser(e.target.value)}
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
            <p className="text-xs text-gray-500 mt-2">These credentials are required to access the panel after restart.</p>
            <p className="text-xs text-gray-500 mt-1">If setting a new password, minimum length is 4 characters.</p>

            {/* User-Agent */}
            <hr className="border-[#3a3a3a] my-6" />
            <label className="block text-sm text-gray-400 mb-2">User-Agent for downloads</label>
            <Tooltip>
              <TooltipTrigger asChild>
                <input
                  type="text"
                  value={userAgent}
                  onChange={(e) => setUserAgent(e.target.value)}
                  placeholder="MC-AdPanel/1.0 (+https://github.com/pbarrera813/MC-AdPanel)"
                  className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B]"
                  disabled={saving}
                />
              </TooltipTrigger>
              <TooltipContent className="bg-[#252524] border border-[#3a3a3a] px-3 py-1.5 text-gray-300 text-xs">
                Don't touch if you don't know what this is.
              </TooltipContent>
            </Tooltip>
            <p className="text-xs text-gray-500 mt-2">
              Some upstream APIs require a contact User-Agent. This value is used for all version and jar downloads.
            </p>

            {/* Default RAM */}
            <hr className="border-[#3a3a3a] my-6" />
            <label className="block text-sm text-gray-400 mb-3">Default RAM Allocation</label>
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-xs text-gray-500 mb-1">Min RAM (GB)</label>
                <input
                  type="number"
                  value={defaultMinRam}
                  onChange={(e) => setDefaultMinRam(e.target.value)}
                  min={0.5} max={64} step={0.5}
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
                  min={0.5} max={64} step={0.5}
                  className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B]"
                  disabled={saving}
                />
              </div>
            </div>
            <p className="text-xs text-gray-500 mt-2">Default RAM allocation for new servers. Saves time when creating many servers.</p>

            {/* Default JVM Flags */}
            <hr className="border-[#3a3a3a] my-6" />
            <label className="block text-sm text-gray-400 mb-3">Default JVM Flags Preset</label>
            <div className="flex flex-wrap gap-3">
              {([
                { value: 'none', label: 'None', desc: 'Default JVM flags' },
                { value: 'aikars', label: "Aikar's Flags", desc: 'Optimized GC for game servers' },
                { value: 'velocity', label: 'Velocity & Waterfall', desc: 'Optimized for proxy servers' },
              ] as const).map(opt => (
                <button
                  key={opt.value}
                  type="button"
                  onClick={() => setDefaultFlags(opt.value)}
                  disabled={saving}
                  className={clsx(
                    "flex-1 min-w-[140px] px-4 py-3 rounded border text-left transition-all",
                    defaultFlags === opt.value
                      ? "border-[#E5B80B] bg-[#E5B80B]/10 text-white"
                      : "border-[#3a3a3a] bg-[#1a1a1a] text-gray-400 hover:border-[#E5B80B]/40 hover:text-white"
                  )}
                >
                  <div className="text-sm font-medium">{opt.label}</div>
                  <div className="text-xs text-gray-500 mt-0.5">{opt.desc}</div>
                </button>
              ))}
            </div>
            <p className="text-xs text-gray-500 mt-2">Pre-selected JVM flag preset when creating new servers.</p>

            {/* Status Polling Interval */}
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
            <p className="text-xs text-gray-500 mt-2">How often the panel polls for server status updates (1-30 seconds). Lower values = more responsive, higher values = less network traffic.</p>

            {/* Save Button */}
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
    </div>
  );
};
