import React, { useState } from 'react';
import { useServer } from '../context/ServerContext';
import { Copy, Check, X } from 'lucide-react';
import { motion, AnimatePresence } from 'motion/react';
import { toast } from 'sonner';
import clsx from 'clsx';
import { useEscapeKey } from '../hooks/useEscapeKey';

export const CloningPage = () => {
  const { servers, refreshServers } = useServer();
  const [selectedSourceIds, setSelectedSourceIds] = useState<Set<string>>(new Set());
  const [isModalOpen, setIsModalOpen] = useState(false);

  // Form State
  const [newName, setNewName] = useState('');
  const [options, setOptions] = useState({
    plugins: true,
    worlds: true,
    config: true
  });

  const [newPort, setNewPort] = useState(25566);

  useEscapeKey(isModalOpen, () => setIsModalOpen(false));

  const handleToggleSource = (id: string) => {
    setSelectedSourceIds(prev => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const handleClone = async () => {
    const sources = servers.filter(s => selectedSourceIds.has(s.id));
    if (sources.length === 0) return;

    try {
      let currentPort = newPort;
      for (const source of sources) {
        const cloneName = sources.length === 1 ? newName : `${source.name} (Clone)`;
        const res = await fetch('/api/servers/clone', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            sourceId: source.id,
            name: cloneName,
            port: currentPort,
            copyPlugins: options.plugins,
            copyWorlds: options.worlds,
            copyConfig: options.config,
          }),
        });
        if (!res.ok) {
          const data = await res.json().catch(() => ({}));
          throw new Error(data.error || `Failed to clone ${source.name}`);
        }
        currentPort++;
      }
      await refreshServers();
      toast.success(sources.length === 1 ? 'Server cloned successfully' : `${sources.length} servers cloned successfully`);
      setIsModalOpen(false);
      setSelectedSourceIds(new Set());
      setNewName('');
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to clone server');
      await refreshServers();
    }
  };

  const openModal = () => {
    const sources = servers.filter(s => selectedSourceIds.has(s.id));
    if (sources.length === 0) return;
    if (sources.length === 1) {
      setNewName(`${sources[0].name} (Clone)`);
      setNewPort(sources[0].port + 1);
    } else {
      setNewName('');
      const maxPort = Math.max(...servers.map(s => s.port));
      setNewPort(maxPort + 1);
    }
    setIsModalOpen(true);
  };

  return (
    <div className="flex-1 p-4 md:p-8 overflow-y-auto">
      <div className="mb-8">
        <h2 className="text-3xl font-bold text-white mb-2">Clone Server</h2>
        <p className="text-gray-400 text-sm">Select source server(s) to duplicate their configuration and data.</p>
      </div>

      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 mb-8">
        {servers.map((server) => (
          <div
            key={server.id}
            onClick={() => handleToggleSource(server.id)}
            className={clsx(
              "cursor-pointer border rounded-lg p-6 transition-all relative overflow-hidden group",
              selectedSourceIds.has(server.id)
                ? "bg-[#E5B80B]/10 border-[#E5B80B] ring-1 ring-[#E5B80B]"
                : "bg-[#202020] border-[#3a3a3a] hover:border-gray-500"
            )}
          >
            <div className="flex justify-between items-start mb-4">
               <div>
                 <h3 className={clsx("text-lg font-bold mb-1", selectedSourceIds.has(server.id) ? "text-[#E5B80B]" : "text-white")}>{server.name}</h3>
                 <div className="text-sm text-gray-500">{server.type} • {server.version}</div>
               </div>
               {selectedSourceIds.has(server.id) && (
                 <div className="bg-[#E5B80B] rounded-full p-1 text-black">
                   <Check size={12} strokeWidth={4} />
                 </div>
               )}
            </div>

            <div className="text-xs text-gray-600 font-mono">
              Port: {server.port}
            </div>
          </div>
        ))}
      </div>

      <div className="flex justify-end border-t border-[#3a3a3a] pt-8">
        <button
          disabled={selectedSourceIds.size === 0}
          onClick={openModal}
          className={clsx(
            "flex items-center gap-2 px-6 py-3 rounded font-bold transition-all",
            selectedSourceIds.size > 0
              ? "bg-[#E5B80B] text-black hover:bg-[#d4a90a] shadow-lg shadow-yellow-900/20"
              : "bg-[#252524] text-gray-500 cursor-not-allowed"
          )}
        >
          <Copy size={20} />
          {selectedSourceIds.size > 1 ? `Clone ${selectedSourceIds.size} Servers` : 'Clone Server'}
        </button>
      </div>

      <AnimatePresence>
        {isModalOpen && (
          <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-md bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6"
            >
              <h3 className="text-xl font-bold text-white mb-6">
                {selectedSourceIds.size > 1 ? `Clone ${selectedSourceIds.size} Servers` : 'Clone Settings'}
              </h3>

              <div className="space-y-4 mb-6">
                {selectedSourceIds.size === 1 && (
                  <div>
                    <label className="block text-sm text-gray-400 mb-1">New Server Name</label>
                    <input
                      type="text"
                      value={newName}
                      onChange={e => setNewName(e.target.value)}
                      className="w-full bg-[#1a1a1a] border border-[#333] rounded px-3 py-2 text-white focus:outline-none focus:border-[#E5B80B]"
                    />
                  </div>
                )}

                <div>
                  <label className="block text-sm text-gray-400 mb-1">
                    {selectedSourceIds.size > 1 ? 'Starting Port' : 'Server Port'}
                  </label>
                  <input
                    type="number"
                    value={newPort}
                    onChange={e => setNewPort(parseInt(e.target.value) || 25565)}
                    min={1024}
                    max={65535}
                    className="w-full bg-[#1a1a1a] border border-[#333] rounded px-3 py-2 text-white focus:outline-none focus:border-[#E5B80B]"
                  />
                  {selectedSourceIds.size > 1 && (
                    <p className="text-xs text-gray-500 mt-1">Ports will auto-increment: {newPort}, {newPort + 1}, {newPort + 2}...</p>
                  )}
                </div>

                <div className="space-y-2">
                  <label className="flex items-center gap-2 text-gray-300 cursor-pointer select-none">
                    <input type="checkbox" checked={options.plugins} onChange={e => setOptions({...options, plugins: e.target.checked})} className="accent-[#E5B80B]" />
                    Copy Plugins & Mods
                  </label>
                  <label className="flex items-center gap-2 text-gray-300 cursor-pointer select-none">
                    <input type="checkbox" checked={options.worlds} onChange={e => setOptions({...options, worlds: e.target.checked})} className="accent-[#E5B80B]" />
                    Copy Worlds (This may take time)
                  </label>
                  <label className="flex items-center gap-2 text-gray-300 cursor-pointer select-none">
                    <input type="checkbox" checked={options.config} onChange={e => setOptions({...options, config: e.target.checked})} className="accent-[#E5B80B]" />
                    Copy Configuration Files
                  </label>
                </div>

                {selectedSourceIds.size > 1 && (
                  <div className="bg-[#1a1a1a] border border-[#333] rounded p-3">
                    <p className="text-xs text-gray-400 mb-2">Servers to clone:</p>
                    <ul className="text-sm text-gray-300 space-y-1">
                      {servers.filter(s => selectedSourceIds.has(s.id)).map((s, i) => (
                        <li key={s.id} className="flex items-center gap-2">
                          <span className="w-1.5 h-1.5 rounded-full bg-[#E5B80B] flex-shrink-0" />
                          {s.name} <span className="text-gray-500 text-xs">→ Port {newPort + i}</span>
                        </li>
                      ))}
                    </ul>
                  </div>
                )}
              </div>

              <div className="flex justify-end gap-3">
                <button onClick={() => setIsModalOpen(false)} className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium">Cancel</button>
                <button onClick={handleClone} className="px-4 py-2 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded font-bold">Confirm Clone</button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>
    </div>
  );
};
