import { create } from 'zustand';
import { UserProfile } from '../types/growthos';
import { mockUser } from '../mocks/growthOsMockData';

interface AppState {
  user: UserProfile;
  theme: 'light' | 'dark';
  sidebarCollapsed: boolean;
  activeRole: 'user' | 'admin' | 'mcp' | 'agent';
  useMockApi: boolean;
  toggleTheme: () => void;
  toggleSidebar: () => void;
  setActiveRole: (role: 'user' | 'admin' | 'mcp' | 'agent') => void;
  setUseMockApi: (useMock: boolean) => void;
}

export const useAppStore = create<AppState>((set) => ({
  user: mockUser,
  theme: 'light',
  sidebarCollapsed: false,
  activeRole: 'admin',
  useMockApi: true,
  toggleTheme: () =>
    set((state) => {
      const nextTheme = state.theme === 'light' ? 'dark' : 'light';
      if (nextTheme === 'dark') {
        document.documentElement.classList.add('dark');
      } else {
        document.documentElement.classList.remove('dark');
      }
      return { theme: nextTheme };
    }),
  toggleSidebar: () => set((state) => ({ sidebarCollapsed: !state.sidebarCollapsed })),
  setActiveRole: (role) => set({ activeRole: role }),
  setUseMockApi: (useMock) => set({ useMockApi: useMock }),
}));
