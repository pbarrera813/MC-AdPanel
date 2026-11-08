import { useEffect, useState } from 'react';

const formatUptime = (startedAt: string) => {
  const minutes = Math.max(0, Math.floor((Date.now() - new Date(startedAt).getTime()) / 60000));
  if (minutes < 1) return '<1m';
  if (minutes < 60) return `${minutes}m`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return `${hours}h ${minutes % 60}m`;
  return `${Math.floor(hours / 24)}d ${hours % 24}h`;
};

export const Uptime = ({ startedAt }: { startedAt?: string }) => {
  const [, setTick] = useState(0);

  useEffect(() => {
    if (!startedAt) return;
    const timer = setInterval(() => setTick((n) => n + 1), 30000);
    return () => clearInterval(timer);
  }, [startedAt]);

  return <>{startedAt ? formatUptime(startedAt) : '-'}</>;
};
