export interface VersionInfo {
  version: string;
  latest: boolean;
}

export interface ImportProperties {
  maxPlayers?: number;
  motd?: string;
  whiteList?: boolean;
  onlineMode?: boolean;
}

export interface ImportAnalysis {
  analysisId: string;
  serverType: string;
  typeDetected: boolean;
  version: string;
  worlds: string[];
  plugins: string[];
  mods: string[];
  properties: ImportProperties;
  resolvedName: string;
  resolvedPort: number;
}

export type ImportBoolState = 'true' | 'false';

export interface ImportFormState {
  name: string;
  port: string;
  serverType: string;
  version: string;
  maxPlayers: string;
  motd: string;
  whiteList: ImportBoolState;
  onlineMode: ImportBoolState;
}

export type JVMFlagsPreset = 'none' | 'aikars' | 'velocity' | 'modded';

export type ContextMenuState = {
  serverId: string;
  x: number;
  y: number;
  showFlagsSubmenu: boolean;
};
