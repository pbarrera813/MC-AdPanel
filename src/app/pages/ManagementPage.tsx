import React, { useState, useEffect, useRef } from 'react';
import { useServer } from '../context/ServerContext';
import { Play, Square, AlertOctagon, RotateCw, Terminal, Folder, Users, ShieldAlert, Cpu, HardDrive, Clock, Calendar, AlertTriangle, Settings, Save, ChevronUp, ChevronDown } from 'lucide-react';
import { AreaChart, Area, ResponsiveContainer } from 'recharts';
import clsx from 'clsx';
import { motion, AnimatePresence } from 'motion/react';
import { toast } from 'sonner';
import { Tooltip, TooltipTrigger, TooltipContent } from '../components/ui/tooltip';
import { useEscapeKey } from '../hooks/useEscapeKey';

// Sub-components (could be separate files but kept here for now for speed)
import { ConsoleView } from '../components/management/ConsoleView';
import { FileBrowser } from '../components/management/FileBrowser';
import { PlayerList } from '../components/management/PlayerList';

type Tab = 'console' | 'browse' | 'players';
type RestartOption = 'now' | '5m' | '30m' | '1h' | '3h' | '6h' | 'custom';

export const ManagementPage = () => {
  const { activeServer, startServer, stopServer, refreshServers } = useServer();
  const [activeTab, setActiveTab] = useState<Tab>('console');
  const [isLargeScreen, setIsLargeScreen] = useState(true);
  const playersUnsupportedForType = activeServer?.type === 'Velocity';

  // Modals State
  const [isRestartModalOpen, setIsRestartModalOpen] = useState(false);
  const [isSafeModeModalOpen, setIsSafeModeModalOpen] = useState(false);
  const [isKillModalOpen, setIsKillModalOpen] = useState(false);
  const [isStopToEditModalOpen, setIsStopToEditModalOpen] = useState(false);

  // Settings state
  const [settingsMinRam, setSettingsMinRam] = useState('');
  const [settingsMaxRam, setSettingsMaxRam] = useState('');
  const [settingsMaxPlayers, setSettingsMaxPlayers] = useState('');
  const [settingsPort, setSettingsPort] = useState('');
  const [savingSettings, setSavingSettings] = useState(false);

  // Convert MB string (e.g. "1024M") to GB number for display
  const mbToGb = (mb: string) => String(parseInt(mb?.replace('M', '') || '1024') / 1024);

  // Sync settings from activeServer (display in GB)
  useEffect(() => {
    if (activeServer) {
      setSettingsMinRam(mbToGb(activeServer.minRam));
      setSettingsMaxRam(mbToGb(activeServer.maxRam));
      setSettingsMaxPlayers(String(activeServer.maxPlayers || 20));
      setSettingsPort(String(activeServer.port || 25565));
    }
  }, [activeServer?.id, activeServer?.minRam, activeServer?.maxRam, activeServer?.maxPlayers, activeServer?.port]);
  
  const [restartOption, setRestartOption] = useState<RestartOption>('5m');
  const [customTime, setCustomTime] = useState('');

  useEffect(() => {
    const handleResize = () => setIsLargeScreen(window.innerWidth >= 1280);
    handleResize(); // Init check
    window.addEventListener('resize', handleResize);
    return () => window.removeEventListener('resize', handleResize);
  }, []);

  useEffect(() => {
    if (playersUnsupportedForType && activeTab === 'players') {
      setActiveTab('console');
    }
  }, [playersUnsupportedForType, activeTab]);

  useEscapeKey(isRestartModalOpen, () => setIsRestartModalOpen(false));
  useEscapeKey(isSafeModeModalOpen, () => setIsSafeModeModalOpen(false));
  useEscapeKey(isKillModalOpen, () => setIsKillModalOpen(false));
  useEscapeKey(isStopToEditModalOpen, () => setIsStopToEditModalOpen(false));

  const handleScheduleRestart = async () => {
    if (!activeServer) return;

    let delaySeconds = 0;
    const delayMap: Record<Exclude<RestartOption, 'custom'>, number> = {
      now: 0,
      '5m': 300,
      '30m': 1800,
      '1h': 3600,
      '3h': 10800,
      '6h': 21600,
    };

    if (restartOption === 'custom') {
      if (!customTime) {
        toast.error("Please select a time");
        return;
      }
      const targetTime = new Date(customTime).getTime();
      const now = Date.now();
      delaySeconds = Math.round((targetTime - now) / 1000);
      if (delaySeconds <= 0) {
        toast.error("Selected time must be in the future");
        return;
      }
    } else {
      delaySeconds = delayMap[restartOption];
    }

    setIsRestartModalOpen(false);

    try {
      const res = await fetch(`/api/servers/${activeServer.id}/schedule-restart`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ delaySeconds }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to schedule restart');
      }
      const labelMap: Record<Exclude<RestartOption, 'custom'>, string> = {
        now: 'now',
        '5m': '5 minutes',
        '30m': '30 minutes',
        '1h': '1 hour',
        '3h': '3 hours',
        '6h': '6 hours',
      };
      const message = restartOption === 'custom'
        ? `Server restart scheduled for ${customTime}`
        : restartOption === 'now'
          ? 'Server restart initiated now'
          : `Server restart scheduled in ${labelMap[restartOption]}`;
      toast.success(message);
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to schedule restart');
    }

    setCustomTime('');
    setRestartOption('5m');
  };

  const handleSafeMode = async () => {
    setIsSafeModeModalOpen(false);
    if (activeServer) {
      try {
        const res = await fetch(`/api/servers/${activeServer.id}/start-safe`, { method: 'POST' });
        if (!res.ok) {
          const data = await res.json().catch(() => ({}));
          throw new Error(data.error || 'Failed to start in safe mode');
        }
        toast.info("Starting in Safe Mode (plugins/mods disabled)...");
        await refreshServers();
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to start server');
      }
    }
  };

  const handleKill = async () => {
    setIsKillModalOpen(false);
    if (activeServer) {
      if (activeServer.status === 'Running' || activeServer.status === 'Booting') {
        try {
          await stopServer(activeServer.id);
          toast.error("Server killed forcefully.");
        } catch (err) {
          toast.error(err instanceof Error ? err.message : 'Failed to kill server');
        }
      }
    }
  };

  const isServerRunning = activeServer?.status === 'Running' || activeServer?.status === 'Booting';

  const handleSettingsFieldFocus = () => {
    if (isServerRunning) {
      setIsStopToEditModalOpen(true);
    }
  };

  const handleStopToEdit = async () => {
    setIsStopToEditModalOpen(false);
    if (activeServer) {
      try {
        await stopServer(activeServer.id);
        toast.success('Server stopped. You can now edit settings.');
      } catch (err) {
        toast.error(err instanceof Error ? err.message : 'Failed to stop server');
      }
    }
  };

  const handleSaveSettings = async () => {
    if (!activeServer) return;
    setSavingSettings(true);
    try {
      const res = await fetch(`/api/servers/${activeServer.id}/settings`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          minRam: Math.round((parseFloat(settingsMinRam) || 0.5) * 1024) + 'M',
          maxRam: Math.round((parseFloat(settingsMaxRam) || 1) * 1024) + 'M',
          maxPlayers: parseInt(settingsMaxPlayers) || 20,
          port: parseInt(settingsPort) || 25565,
        }),
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || 'Failed to save settings');
      }
      toast.success('Settings saved successfully');
      await refreshServers();
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to save settings');
    } finally {
      setSavingSettings(false);
    }
  };

  const settingsChanged = activeServer && (
    settingsMinRam !== mbToGb(activeServer.minRam) ||
    settingsMaxRam !== mbToGb(activeServer.maxRam) ||
    settingsMaxPlayers !== String(activeServer.maxPlayers || 20) ||
    settingsPort !== String(activeServer.port || 25565)
  );

  if (!activeServer) {
    return <div className="flex items-center justify-center h-full text-gray-500">No server selected</div>;
  }

  // Derived state for button enabling
  const isServerOff = activeServer.status !== 'Running' && activeServer.status !== 'Booting' && activeServer.status !== 'Installing';

  return (
    <div className="flex flex-col h-full bg-[#1e1e1d]">
      {/* Header */}
      <header className="bg-[#252524] border-b border-[#3a3a3a] px-4 md:px-6 py-4 flex flex-col md:flex-row md:justify-between md:items-center gap-4 shadow-sm z-10">
        <div>
          <h2 className="text-xl font-bold text-white flex items-center gap-3">
            {activeServer.name}
            <span className={`px-2 py-0.5 rounded text-xs uppercase font-bold tracking-wider
              ${activeServer.status === 'Running' ? 'bg-green-900/40 text-green-400 border border-green-800' :
                activeServer.status === 'Crashed' || activeServer.status === 'Error' ? 'bg-red-900/40 text-red-400 border border-red-800' :
                activeServer.status === 'Installing' ? 'bg-blue-900/40 text-blue-400 border border-blue-800' :
                'bg-gray-700/40 text-gray-400 border border-gray-600'}`}>
              {activeServer.status}
            </span>
          </h2>
          <div className="text-xs text-gray-500 mt-1 font-mono">
            {activeServer.type} {activeServer.version} • Port: {activeServer.port}
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-3">
           <button 
             onClick={() => isServerOff && setIsSafeModeModalOpen(true)}
             disabled={!isServerOff}
             className={clsx(
               "flex items-center gap-2 px-3 py-1.5 border rounded text-sm font-medium transition-colors",
               isServerOff 
                 ? "bg-yellow-900/20 text-yellow-500 border-yellow-700/50 hover:bg-yellow-900/40 cursor-pointer" 
                 : "bg-gray-800 text-gray-500 border-gray-700 cursor-not-allowed opacity-50"
             )}
           >
             <ShieldAlert size={16} /> Safe Mode
           </button>
           
           <div className="h-6 w-px bg-[#404040] mx-1"></div>

           {isServerOff ? (
             <button
               onClick={async () => {
                 try { await startServer(activeServer.id); toast.success('Server starting...'); }
                 catch (err) { toast.error(err instanceof Error ? err.message : 'Failed to start'); }
               }}
               className="flex items-center gap-2 px-4 py-2 bg-green-600 hover:bg-green-500 text-white rounded font-bold shadow-lg shadow-green-900/20 transition-all active:scale-95"
             >
               <Play size={18} fill="currentColor" /> Start
             </button>
           ) : (
             <>
               <button
                 onClick={async () => {
                   try { await stopServer(activeServer.id); toast.success('Server stopping...'); }
                   catch (err) { toast.error(err instanceof Error ? err.message : 'Failed to stop'); }
                 }}
                 className="flex items-center gap-2 px-4 py-2 bg-[#2a2a29] border border-[#404040] hover:bg-[#333] text-gray-200 rounded font-medium transition-colors"
               >
                 <Square size={18} fill="currentColor" /> Stop
               </button>
               <button 
                 onClick={() => setIsKillModalOpen(true)}
                 className="flex items-center gap-2 px-3 py-2 bg-red-900/20 border border-red-900/50 hover:bg-red-900/40 text-red-400 rounded font-medium transition-colors"
               >
                 <AlertOctagon size={18} /> Kill
               </button>
             </>
           )}
        </div>
      </header>

      {/* Tabs */}
      <div className="bg-[#202020] border-b border-[#3a3a3a] px-4 md:px-6 pt-2 flex gap-1 flex-wrap">
        <TabButton id="console" label="Console" icon={Terminal} active={activeTab} onClick={setActiveTab} />
        <TabButton id="browse" label="File Browser" icon={Folder} active={activeTab} onClick={setActiveTab} />
        <TabButton
          id="players"
          label="Players"
          icon={Users}
          active={activeTab}
          onClick={setActiveTab}
          disabled={playersUnsupportedForType}
          disabledReason={playersUnsupportedForType ? 'Not supported on this server type' : ''}
        />
      </div>

      {/* Content Area */}
      <div className="flex-1 overflow-hidden flex relative min-h-0">
        <div className="flex-1 overflow-hidden flex flex-col relative z-0 min-h-0">
          <div className={clsx('h-full min-h-0 overflow-hidden', activeTab === 'console' ? 'block' : 'hidden')}>
            <ConsoleView server={activeServer} />
          </div>
          <div className={clsx('h-full min-h-0 overflow-hidden', activeTab === 'browse' ? 'block' : 'hidden')}>
            <FileBrowser server={activeServer} />
          </div>
          <div className={clsx('h-full min-h-0 overflow-hidden', activeTab === 'players' ? 'block' : 'hidden')}>
            <PlayerList server={activeServer} />
          </div>
        </div>
        
        {/* Metrics Panel (Right Side) */}
        {activeTab === 'console' && isLargeScreen && (
           <aside className="w-80 bg-[#1a1a1a] border-l border-[#3a3a3a] flex flex-col p-4 gap-4 z-10 overflow-y-auto">
             <div className="space-y-4">
                {/* TPS Display */}
                {(() => {
                  const showFabricTpsHint = activeServer.type === 'Fabric' && !activeServer.fabricTpsAvailable;
                  const showVanillaTpsHint = activeServer.type === 'Vanilla';
                  const showValue = activeServer.status === 'Running' && activeServer.tps > 0 && !showFabricTpsHint;
                  const tpsCard = (
                    <div className="bg-[#202020] rounded-lg border border-[#333] p-4">
                      <div className="flex justify-between items-center">
                        <h4 className="text-gray-400 text-xs uppercase font-bold tracking-wider">TPS</h4>
                        {showValue ? (
                          <span className={clsx(
                            "font-mono font-bold text-lg",
                            activeServer.tps >= 18 ? "text-green-400" :
                            activeServer.tps >= 15 ? "text-yellow-400" : "text-red-400"
                          )}>
                            {activeServer.tps.toFixed(1)}
                          </span>
                        ) : (
                          <span className="font-mono font-bold text-lg text-gray-500">-</span>
                        )}
                      </div>
                      {showValue && (
                        <div className="mt-2 h-1.5 bg-[#1a1a1a] rounded-full overflow-hidden">
                          <div
                            className={clsx(
                              "h-full rounded-full transition-all duration-500",
                              activeServer.tps >= 18 ? "bg-green-400" :
                              activeServer.tps >= 15 ? "bg-yellow-400" : "bg-red-400"
                            )}
                            style={{ width: `${Math.min(100, (activeServer.tps / 20) * 100)}%` }}
                          />
                        </div>
                      )}
                    </div>
                  );

                  if (!showFabricTpsHint && !showVanillaTpsHint) {
                    return tpsCard;
                  }

                  return (
                    <Tooltip>
                      <TooltipTrigger asChild>
                        {tpsCard}
                      </TooltipTrigger>
                      <TooltipContent className="bg-[#252524] border border-[#3a3a3a] px-3 py-1.5 text-gray-300">
                        {showVanillaTpsHint ? 'Not supported on this server type' : 'To get TPS install Fabric-TPS mod.'}
                      </TooltipContent>
                    </Tooltip>
                  );
                })()}

                <MetricChart title="CPU Usage" value={activeServer.cpu} color="#E5B80B" unit="%" />
                <MetricChart title="RAM Usage" value={activeServer.ram} color="#3b82f6" unit=" MB" />
             </div>

             {/* Server Settings */}
             <div className="bg-[#202020] rounded-lg border border-[#333] p-4 space-y-3">
               <div className="flex items-center gap-2 mb-1">
                 <Settings size={14} className="text-gray-400" />
                 <h4 className="text-gray-400 text-xs uppercase font-bold tracking-wider">Server Settings</h4>
               </div>

               <div>
                 <label className="block text-xs text-gray-500 mb-1">Allocated RAM</label>
                 <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
                   <div>
                     <span className="block text-[10px] text-gray-600 mb-0.5">Min (GB)</span>
                     <div className="relative group/input">
                       <input
                         type="number"
                         value={settingsMinRam}
                         onChange={(e) => setSettingsMinRam(e.target.value)}
                         onFocus={handleSettingsFieldFocus}
                         readOnly={isServerRunning}
                         min={0.5}
                         max={64}
                         step={0.5}
                         className={clsx(
                           "w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded px-2 py-1.5 pr-6 text-sm text-white font-mono focus:outline-none transition-all",
                           isServerRunning ? "cursor-not-allowed opacity-60" : "focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B]"
                         )}
                       />
                       {!isServerRunning && (
                         <div className="absolute right-0 top-0 bottom-0 w-5 flex flex-col border-l border-[#3a3a3a] group-focus-within/input:border-[#E5B80B]/50">
                           <button type="button" onClick={() => setSettingsMinRam(String(Math.min(64, (parseFloat(settingsMinRam) || 0) + 0.5)))} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-tr"><ChevronUp size={10} /></button>
                           <button type="button" onClick={() => setSettingsMinRam(String(Math.max(0.5, (parseFloat(settingsMinRam) || 0) - 0.5)))} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-br"><ChevronDown size={10} /></button>
                         </div>
                       )}
                     </div>
                   </div>
                   <div>
                     <span className="block text-[10px] text-gray-600 mb-0.5">Max (GB)</span>
                     <div className="relative group/input">
                       <input
                         type="number"
                         value={settingsMaxRam}
                         onChange={(e) => setSettingsMaxRam(e.target.value)}
                         onFocus={handleSettingsFieldFocus}
                         readOnly={isServerRunning}
                         min={0.5}
                         max={64}
                         step={0.5}
                         className={clsx(
                           "w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded px-2 py-1.5 pr-6 text-sm text-white font-mono focus:outline-none transition-all",
                           isServerRunning ? "cursor-not-allowed opacity-60" : "focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B]"
                         )}
                       />
                       {!isServerRunning && (
                         <div className="absolute right-0 top-0 bottom-0 w-5 flex flex-col border-l border-[#3a3a3a] group-focus-within/input:border-[#E5B80B]/50">
                           <button type="button" onClick={() => setSettingsMaxRam(String(Math.min(64, (parseFloat(settingsMaxRam) || 0) + 0.5)))} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-tr"><ChevronUp size={10} /></button>
                           <button type="button" onClick={() => setSettingsMaxRam(String(Math.max(0.5, (parseFloat(settingsMaxRam) || 0) - 0.5)))} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-br"><ChevronDown size={10} /></button>
                         </div>
                       )}
                     </div>
                   </div>
                 </div>
               </div>

               <div>
                 <label className="block text-xs text-gray-500 mb-1">Max Players</label>
                 <div className="relative group/input">
                   <input
                     type="number"
                     value={settingsMaxPlayers}
                     onChange={(e) => setSettingsMaxPlayers(e.target.value)}
                     onFocus={handleSettingsFieldFocus}
                     readOnly={isServerRunning}
                     min={1}
                     max={1000}
                     className={clsx(
                       "w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded px-2 py-1.5 pr-6 text-sm text-white font-mono focus:outline-none transition-all",
                       isServerRunning ? "cursor-not-allowed opacity-60" : "focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B]"
                     )}
                   />
                   {!isServerRunning && (
                     <div className="absolute right-0 top-0 bottom-0 w-5 flex flex-col border-l border-[#3a3a3a] group-focus-within/input:border-[#E5B80B]/50">
                       <button type="button" onClick={() => setSettingsMaxPlayers(String(Math.min(1000, (parseInt(settingsMaxPlayers) || 0) + 1)))} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-tr"><ChevronUp size={10} /></button>
                       <button type="button" onClick={() => setSettingsMaxPlayers(String(Math.max(1, (parseInt(settingsMaxPlayers) || 0) - 1)))} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-br"><ChevronDown size={10} /></button>
                     </div>
                   )}
                 </div>
               </div>

               <div>
                 <label className="block text-xs text-gray-500 mb-1">Port</label>
                 <div className="relative group/input">
                   <input
                     type="number"
                     value={settingsPort}
                     onChange={(e) => setSettingsPort(e.target.value)}
                     onFocus={handleSettingsFieldFocus}
                     readOnly={isServerRunning}
                     min={1024}
                     max={65535}
                     className={clsx(
                       "w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded px-2 py-1.5 pr-6 text-sm text-white font-mono focus:outline-none transition-all",
                       isServerRunning ? "cursor-not-allowed opacity-60" : "focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B]"
                     )}
                   />
                   {!isServerRunning && (
                     <div className="absolute right-0 top-0 bottom-0 w-5 flex flex-col border-l border-[#3a3a3a] group-focus-within/input:border-[#E5B80B]/50">
                       <button type="button" onClick={() => setSettingsPort(String(Math.min(65535, (parseInt(settingsPort) || 1024) + 1)))} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-tr"><ChevronUp size={10} /></button>
                       <button type="button" onClick={() => setSettingsPort(String(Math.max(1024, (parseInt(settingsPort) || 1024) - 1)))} className="flex-1 flex items-center justify-center text-[#E5B80B]/60 hover:text-[#E5B80B] hover:bg-[#E5B80B]/10 transition-colors rounded-br"><ChevronDown size={10} /></button>
                     </div>
                   )}
                 </div>
               </div>

               {settingsChanged && !isServerRunning && (
                 <button
                   onClick={handleSaveSettings}
                   disabled={savingSettings}
                   className="w-full py-2 bg-[#E5B80B] text-black rounded font-bold text-sm hover:bg-[#d4a90a] transition-colors flex items-center justify-center gap-2 disabled:opacity-50"
                 >
                   <Save size={14} />
                   {savingSettings ? 'Saving...' : 'Save Changes'}
                 </button>
               )}
             </div>

             <div className="mt-auto">
               <button
                onClick={() => setIsRestartModalOpen(true)}
                className="w-full py-3 border border-[#3a3a3a] bg-[#252524] text-gray-300 rounded hover:bg-[#333] transition-colors flex items-center justify-center gap-2 text-sm font-medium"
               >
                 <RotateCw size={16} /> Schedule Restart
               </button>
             </div>
           </aside>
        )}
      </div>

      {/* Schedule Restart Modal */}
      <AnimatePresence>
        {isRestartModalOpen && (
          <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div 
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-sm bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6"
            >
              <div className="flex items-center gap-3 text-white mb-6">
                <Clock className="text-[#E5B80B]" size={24} />
                <h3 className="text-xl font-bold">Schedule Restart</h3>
              </div>
              
              <div className="space-y-4 mb-6">
                <label className="block text-sm text-gray-400">Restart will happen:</label>
                <select 
                  value={restartOption}
                  onChange={(e) => setRestartOption(e.target.value as RestartOption)}
                  className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B] focus:ring-1 focus:ring-[#E5B80B]"
                >
                  <option value="now">Restart now</option>
                  <option value="5m">In 5 minutes</option>
                  <option value="30m">In 30 minutes</option>
                  <option value="1h">In 1 hour</option>
                  <option value="3h">In 3 hours</option>
                  <option value="6h">In 6 hours</option>
                  <option value="custom">Custom Time</option>
                </select>

                <AnimatePresence initial={false}>
                  {restartOption === 'custom' && (
                    <motion.div
                      initial={{ opacity: 0, height: 0, y: -4 }}
                      animate={{ opacity: 1, height: 'auto', y: 0 }}
                      exit={{ opacity: 0, height: 0, y: -4 }}
                      transition={{ duration: 0.22, ease: 'easeOut' }}
                      className="overflow-hidden"
                    >
                      <label className="block text-sm text-gray-400 mb-2">Select Time:</label>
                      <input 
                        type="datetime-local" 
                        value={customTime}
                        onChange={(e) => setCustomTime(e.target.value)}
                        className="w-full bg-[#1a1a1a] border border-[#3a3a3a] rounded p-3 text-white focus:outline-none focus:border-[#E5B80B]"
                      />
                    </motion.div>
                  )}
                </AnimatePresence>
              </div>

              <div className="flex justify-end gap-3">
                <button 
                  onClick={() => setIsRestartModalOpen(false)} 
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                >
                  Cancel
                </button>
                <button 
                  onClick={handleScheduleRestart} 
                  className="px-4 py-2 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded font-bold shadow-lg shadow-[#E5B80B]/20"
                >
                  Confirm
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Safe Mode Modal */}
      <AnimatePresence>
        {isSafeModeModalOpen && (
          <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div 
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-sm bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6"
            >
              <div className="flex items-center gap-3 text-white mb-6">
                <ShieldAlert className="text-[#E5B80B]" size={24} />
                <h3 className="text-xl font-bold">Start Safe Mode?</h3>
              </div>
              
              <p className="text-gray-300 mb-8">
                Safe mode turns on the server without any mods/plugins active, do you wish to continue?
              </p>

              <div className="flex justify-end gap-3">
                <button 
                  onClick={() => setIsSafeModeModalOpen(false)} 
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                >
                  Cancel
                </button>
                <button 
                  onClick={handleSafeMode} 
                  className="px-4 py-2 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded font-bold shadow-lg shadow-[#E5B80B]/20"
                >
                  Continue
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Kill Server Modal */}
      <AnimatePresence>
        {isKillModalOpen && (
          <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div 
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-sm bg-[#252524] border border-red-900/50 rounded-lg shadow-2xl p-6"
            >
              <div className="flex items-center gap-3 text-white mb-6">
                <AlertTriangle className="text-red-500" size={24} />
                <h3 className="text-xl font-bold">Kill Server?</h3>
              </div>
              
              <p className="text-gray-300 mb-8">
                Killing the server can lead to errors/file corruption on the server. Are you sure you want to continue?
              </p>

              <div className="flex justify-end gap-3">
                <button 
                  onClick={() => setIsKillModalOpen(false)} 
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                >
                  Cancel
                </button>
                <button 
                  onClick={handleKill} 
                  className="px-4 py-2 bg-red-600 hover:bg-red-500 text-white rounded font-bold shadow-lg shadow-red-900/20"
                >
                  Continue
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

      {/* Stop Server to Edit Settings Modal */}
      <AnimatePresence>
        {isStopToEditModalOpen && (
          <div className="absolute inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
            <motion.div
              initial={{ opacity: 0, scale: 0.95 }}
              animate={{ opacity: 1, scale: 1 }}
              exit={{ opacity: 0, scale: 0.95 }}
              className="w-full max-w-sm bg-[#252524] border border-[#404040] rounded-lg shadow-2xl p-6"
            >
              <div className="flex items-center gap-3 text-white mb-6">
                <AlertTriangle className="text-yellow-500" size={24} />
                <h3 className="text-xl font-bold">Server is Running</h3>
              </div>

              <p className="text-gray-300 mb-8">
                Changing these settings while the server is enabled can lead to corruption or fatal errors. Do you want to turn off the server before editing?
              </p>

              <div className="flex justify-end gap-3">
                <button
                  onClick={() => setIsStopToEditModalOpen(false)}
                  className="px-4 py-2 bg-[#333] hover:bg-[#404040] text-gray-200 rounded font-medium"
                >
                  Cancel
                </button>
                <button
                  onClick={handleStopToEdit}
                  className="px-4 py-2 bg-[#E5B80B] hover:bg-[#d4a90a] text-black rounded font-bold shadow-lg shadow-[#E5B80B]/20"
                >
                  Accept
                </button>
              </div>
            </motion.div>
          </div>
        )}
      </AnimatePresence>

    </div>
  );
};

const TabButton = ({ id, label, icon: Icon, active, onClick, disabled, disabledReason }: { id: Tab, label: string, icon: any, active: Tab, onClick: (t: Tab) => void, disabled?: boolean, disabledReason?: string }) => (
  <button
    onClick={() => !disabled && onClick(id)}
    disabled={disabled}
    title={disabledReason || undefined}
    className={clsx(
      "relative flex items-center gap-2 px-4 py-3 border-t-2 transition-colors text-sm font-medium outline-none",
      active === id 
        ? "border-[#E5B80B] text-white" 
        : "border-transparent text-gray-500 hover:text-gray-300 hover:bg-[#252525]",
      disabled && "opacity-50 cursor-not-allowed hover:text-gray-500 hover:bg-transparent"
    )}
  >
    {active === id && !disabled && (
      <motion.span
        layoutId="management-tab-active"
        transition={{ type: 'spring', stiffness: 260, damping: 28, mass: 0.75 }}
        className="absolute inset-x-0 inset-y-0 rounded-t-md bg-[#2C2C2B]"
      />
    )}
    <Icon size={16} className="relative z-10" />
    <span className="relative z-10">{label}</span>
  </button>
);

const MetricChart = ({ title, value, color, unit = '%' }: { title: string, value: number, color: string, unit?: string }) => {
  const [data, setData] = useState<{v: number}[]>(Array(20).fill({v: 0}));

  useEffect(() => {
    const interval = setInterval(() => {
      setData(prev => [...prev.slice(1), { v: value > 0 ? value : 0 }]);
    }, 1000);
    return () => clearInterval(interval);
  }, [value]);

  return (
    <div className="bg-[#202020] rounded-lg border border-[#333] p-4">
      <div className="flex justify-between items-center mb-2">
        <h4 className="text-gray-400 text-xs uppercase font-bold tracking-wider">{title}</h4>
        <span className="text-white font-mono font-bold">{Math.max(0, Math.round(data[data.length-1].v))}{unit}</span>
      </div>
      <div className="h-24 w-full">
        <ResponsiveContainer width="100%" height="100%">
          <AreaChart data={data}>
            <defs>
              <linearGradient id={`grad-${title}`} x1="0" y1="0" x2="0" y2="1">
                <stop offset="5%" stopColor={color} stopOpacity={0.3}/>
                <stop offset="95%" stopColor={color} stopOpacity={0}/>
              </linearGradient>
            </defs>
            <Area 
              type="monotone" 
              dataKey="v" 
              stroke={color} 
              strokeWidth={2}
              fill={`url(#grad-${title})`} 
              isAnimationActive={false}
            />
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
};
