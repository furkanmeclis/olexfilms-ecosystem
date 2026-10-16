import { create } from 'zustand';

/** Credentials kept in memory between the password and 2FA steps; never persisted. */
type PendingLogin = {
  email: string;
  password: string;
  set: (p: { email?: string; password?: string }) => void;
  clear: () => void;
};

export const usePendingLogin = create<PendingLogin>((set) => ({
  email: '',
  password: '',
  set: (p) => set(p),
  clear: () => set({ email: '', password: '' }),
}));
