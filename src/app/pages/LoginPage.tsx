import React, { useEffect, useState } from 'react';
import { Loader2 } from 'lucide-react';
import { toast } from 'sonner';

interface LoginPageProps {
  onLoginSuccess: () => void;
}

export const LoginPage = ({ onLoginSuccess }: LoginPageProps) => {
  const [username, setUsername] = useState('mcpanel');
  const [password, setPassword] = useState('mcpanel');
  const [submitting, setSubmitting] = useState(false);
  const [visible, setVisible] = useState(false);

  useEffect(() => {
    const t = setTimeout(() => setVisible(true), 40);
    return () => clearTimeout(t);
  }, []);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (submitting) return;

    setSubmitting(true);
    try {
      const res = await fetch('/api/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username, password }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to login');
      }

      onLoginSuccess();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to login');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen w-full text-gray-200 flex items-center justify-center p-6 relative overflow-hidden">
      <div
        className="absolute inset-0 bg-center bg-cover bg-no-repeat"
        style={{ backgroundImage: "url('/home-background.png')" }}
      />
      <div className="absolute inset-0 bg-gradient-to-br from-black/60 via-black/45 to-black/65" />
      <div
        className={`relative z-10 w-full max-w-md rounded-2xl border border-white/20 bg-white/10 backdrop-blur-xl shadow-[0_25px_80px_rgba(0,0,0,0.45)] p-8 transition-all duration-700 ${
          visible ? 'opacity-100 translate-y-0' : 'opacity-0 translate-y-6'
        }`}
      >
        <div className="flex flex-col items-center mb-7">
          <img src="/icon.png" alt="MC AdPanel" className="w-16 h-16 mb-3 drop-shadow-[0_0_12px_rgba(229,184,11,0.35)]" />
          <h1 className="text-xl font-bold text-white tracking-wide">Minecraft Admin Panel</h1>
        </div>

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label className="block text-sm text-gray-300 mb-1.5">User</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="Insert user"
              className="w-full bg-[#111111]/50 border border-[#4a4a4a] rounded-lg px-3 py-2.5 text-white focus:outline-none focus:border-[#E5B80B]"
              autoComplete="username"
              disabled={submitting}
            />
          </div>
          <div>
            <label className="block text-sm text-gray-300 mb-1.5">Password</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="Insert password"
              className="w-full bg-[#111111]/50 border border-[#4a4a4a] rounded-lg px-3 py-2.5 text-white focus:outline-none focus:border-[#E5B80B]"
              autoComplete="current-password"
              disabled={submitting}
            />
          </div>

          <button
            type="submit"
            disabled={submitting}
            className="w-full mt-1 px-4 py-2.5 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded-lg font-bold disabled:opacity-60 flex items-center justify-center gap-2"
          >
            {submitting && <Loader2 size={16} className="animate-spin" />}
            {submitting ? 'Signing in...' : 'Login'}
          </button>
        </form>
      </div>
    </div>
  );
};
