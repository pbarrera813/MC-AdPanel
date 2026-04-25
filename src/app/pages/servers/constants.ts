import type { JVMFlagsPreset } from './types';

export const SERVER_TYPES = [
  'Vanilla', 'Spigot', 'Paper', 'Folia', 'Purpur', 'Velocity', 'Forge', 'Fabric', 'NeoForge'
] as const;

export const DEFAULT_CREATE_FORM = {
  name: '',
  flags: 'none' as JVMFlagsPreset,
  alwaysPreTouch: false,
  type: '',
  version: '',
  port: '25565',
  minRam: '0.5',
  maxRam: '1',
  maxPlayers: '20',
};

export const importInfoContainerVariants = {
  hidden: { opacity: 0 },
  show: {
    opacity: 1,
    transition: { staggerChildren: 0.03, delayChildren: 0.04 },
  },
};

export const importInfoItemVariants = {
  hidden: { opacity: 0, y: 6 },
  show: { opacity: 1, y: 0, transition: { duration: 0.18, ease: 'easeOut' } },
};

export const LONG_PRESS_DRAG_MS = 450;
export const DRAG_CLICK_GUARD_MS = 450;

export const compareVersionStrings = (a: string, b: string) => {
  const parse = (v: string) => v.split(/[^\d]+/).filter(Boolean).map(n => Number.parseInt(n, 10) || 0);
  const ap = parse(a);
  const bp = parse(b);
  const maxLen = Math.max(ap.length, bp.length);
  for (let i = 0; i < maxLen; i += 1) {
    const av = ap[i] ?? 0;
    const bv = bp[i] ?? 0;
    if (av !== bv) return av > bv ? 1 : -1;
  }
  return 0;
};
