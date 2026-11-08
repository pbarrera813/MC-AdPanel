import { useEffect, useState } from 'react';
import { AlertTriangle, Check, Loader2 } from 'lucide-react';
import clsx from 'clsx';
import { apiRequest } from '../lib/api';

interface JavaPreflight {
  ok: boolean;
  selected?: number;
  message?: string;
}

type HintState = { status: 'idle' | 'checking' | 'ok' | 'problem'; text: string };

const idle: HintState = { status: 'idle', text: '' };

export const JavaRuntimeHint = ({ type, version }: { type: string; version: string }) => {
  const [hint, setHint] = useState<HintState>(idle);

  useEffect(() => {
    if (!type || !version) {
      setHint(idle);
      return;
    }
    const controller = new AbortController();
    setHint({ status: 'checking', text: 'Checking Java...' });
    const timer = setTimeout(async () => {
      try {
        const data = await apiRequest<JavaPreflight>(
          `/api/versions/${encodeURIComponent(type)}/${encodeURIComponent(version)}/java`,
          { signal: controller.signal }
        );
        setHint(
          data.ok
            ? { status: 'ok', text: `Runs on Java ${data.selected}` }
            : { status: 'problem', text: data.message || 'Java is not available for this version.' }
        );
      } catch {
        if (!controller.signal.aborted) setHint(idle);
      }
    }, 250);
    return () => {
      clearTimeout(timer);
      controller.abort();
    };
  }, [type, version]);

  if (hint.status === 'idle') return null;

  return (
    <p
      role="status"
      className={clsx('mt-1.5 flex items-center gap-1.5 text-xs', hint.status === 'problem' ? 'text-red-300' : 'text-gray-400')}
    >
      {hint.status === 'checking' && <Loader2 size={12} className="animate-spin" />}
      {hint.status === 'ok' && <Check size={12} className="text-green-400" />}
      {hint.status === 'problem' && <AlertTriangle size={12} />}
      {hint.text}
    </p>
  );
};
