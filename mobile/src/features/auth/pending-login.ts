import { create } from 'zustand';

/**
 * Credentials kept in memory between the sign-in steps (password or phone code,
 * then 2FA); never persisted.
 */
type PendingLogin = {
  mode: 'password' | 'phone';
  email: string;
  password: string;
  phone: string;
  code: string;
  set: (p: Partial<Omit<PendingLogin, 'set' | 'clear'>>) => void;
  clear: () => void;
};

export const usePendingLogin = create<PendingLogin>((set) => ({
  mode: 'password',
  email: '',
  password: '',
  phone: '',
  code: '',
  set: (p) => set(p),
  clear: () => set({ mode: 'password', email: '', password: '', phone: '', code: '' }),
}));
