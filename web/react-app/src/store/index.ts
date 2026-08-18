import { create } from 'zustand';
import { User, Node, SystemStats } from '@/types';
import { UserPreferences } from '@/api/settings';
import { Language, translations, TranslationKeys } from '@/i18n';

// Safely parse a JSON value from localStorage, falling back to a default
// when the stored data is corrupt or unparseable.
function safeParse<T>(storedValue: string | null, fallback: T): T {
  if (!storedValue) {
    return fallback;
  }
  try {
    return JSON.parse(storedValue) as T;
  } catch {
    console.warn('Failed to parse stored state, using defaults');
    return fallback;
  }
}

interface AppState {
  // Language state
  language: Language;
  t: TranslationKeys;
  setLanguage: (lang: Language) => void;

  // Auth state
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  setUser: (user: User | null) => void;
  setToken: (token: string | null) => void;
  logout: () => void;

  // User preferences state
  userPreferences: UserPreferences | null;
  setUserPreferences: (preferences: UserPreferences | null) => void;

  // System settings state
  websocketEnabled: boolean;
  setWebsocketEnabled: (enabled: boolean) => void;
  autoRefreshEnabled: boolean;
  setAutoRefreshEnabled: (enabled: boolean) => void;
  refreshInterval: number;
  setRefreshInterval: (interval: number) => void;

  // Nodes state
  nodes: Node[];
  selectedNode: Node | null;
  setNodes: (nodes: Node[]) => void;
  setSelectedNode: (node: Node | null) => void;

  // System stats
  systemStats: SystemStats | null;
  setSystemStats: (stats: SystemStats) => void;

  // UI state
  sidebarCollapsed: boolean;
  toggleSidebar: () => void;
}

export const useStore = create<AppState>((set, get) => ({
  // Language state - Initialize from localStorage, default to 'en'
  language: (localStorage.getItem('language') as Language) || 'en',
  t: translations[(localStorage.getItem('language') as Language) || 'en'],
  setLanguage: (lang: Language) => {
    localStorage.setItem('language', lang);
    set({ language: lang, t: translations[lang] });
  },

  // Auth state - Initialize based on localStorage
  user: safeParse<User | null>(localStorage.getItem('user'), null),
  token: localStorage.getItem('token'),
  // Initialize as authenticated if both user and token exist
  isAuthenticated: !!(localStorage.getItem('token') && localStorage.getItem('user')),
  setUser: (user) => {
    if (user) {
      localStorage.setItem('user', JSON.stringify(user));
    } else {
      localStorage.removeItem('user');
    }
    set({ user, isAuthenticated: !!user && !!get().token });
  },
  setToken: (token) => {
    if (token) {
      localStorage.setItem('token', token);
    } else {
      localStorage.removeItem('token');
    }
    set({ token, isAuthenticated: !!token && !!get().user });
  },
  logout: () => {
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    localStorage.removeItem('userPreferences');
    set({ user: null, token: null, isAuthenticated: false, userPreferences: null });
  },

  // User preferences state
  userPreferences: safeParse<UserPreferences | null>(localStorage.getItem('userPreferences'), null),
  setUserPreferences: (preferences) => {
    if (preferences) {
      localStorage.setItem('userPreferences', JSON.stringify(preferences));
    } else {
      localStorage.removeItem('userPreferences');
    }
    set({ userPreferences: preferences });
  },

  // System settings state - default to true
  websocketEnabled: localStorage.getItem('websocketEnabled') !== 'false',
  setWebsocketEnabled: (enabled) => {
    localStorage.setItem('websocketEnabled', String(enabled));
    set({ websocketEnabled: enabled });
  },
  autoRefreshEnabled: localStorage.getItem('autoRefreshEnabled') !== 'false',
  setAutoRefreshEnabled: (enabled) => {
    localStorage.setItem('autoRefreshEnabled', String(enabled));
    set({ autoRefreshEnabled: enabled });
  },
  refreshInterval: parseInt(localStorage.getItem('refreshInterval') || '30', 10),
  setRefreshInterval: (interval) => {
    localStorage.setItem('refreshInterval', String(interval));
    set({ refreshInterval: interval });
  },

  // Nodes state
  nodes: [],
  selectedNode: null,
  setNodes: (nodes) => set({ nodes }),
  setSelectedNode: (node) => set({ selectedNode: node }),

  // System stats
  systemStats: null,
  setSystemStats: (stats) => set({ systemStats: stats }),

  // UI state
  sidebarCollapsed: false,
  toggleSidebar: () => set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),
}));
