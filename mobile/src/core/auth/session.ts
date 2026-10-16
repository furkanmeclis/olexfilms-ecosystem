import { create } from 'zustand';

import { setSessionExpiredHandler } from '@/core/api/client';
import { hasLanguagePreference, isLanguage, setLanguage } from '@/core/i18n';
import { queryClient } from '@/core/query';
import { readJSON, storage, writeJSON } from '@/core/storage';

import { fetchMe, logout, type MobileLoginResult, type MobileMe, switchOrganization } from './api';
import { clearTokens, fromApi, loadTokens, setTokens } from './tokens';

type Status = 'loading' | 'signedOut' | 'signedIn';

type Session = {
  status: Status;
  /** Last known session; cached so the app renders before the network answers. */
  me: MobileMe | null;
  hydrate: () => Promise<void>;
  signIn: (result: MobileLoginResult) => Promise<void>;
  setMe: (me: MobileMe) => void;
  refreshMe: () => Promise<MobileMe>;
  switchOrganization: (slug: string) => Promise<MobileMe>;
  signOut: (opts?: { remote?: boolean }) => Promise<void>;
};

const ME_KEY = 'session.me';

/** The backend's resolved language applies until the user picks one in Settings. */
function adoptBackendLocale(me: MobileMe) {
  if (!hasLanguagePreference() && isLanguage(me.effective_locale)) {
    void setLanguage(me.effective_locale, { persist: false });
  }
}

export const useSession = create<Session>((set, get) => ({
  status: 'loading',
  me: null,

  hydrate: async () => {
    const tokens = await loadTokens();
    const me = tokens ? readJSON<MobileMe>(ME_KEY) : null;
    set({ status: tokens && me ? 'signedIn' : 'signedOut', me });
  },

  signIn: async (result) => {
    await setTokens(fromApi(result));
    get().setMe(result.me);
    set({ status: 'signedIn' });
  },

  setMe: (me) => {
    writeJSON(ME_KEY, me);
    adoptBackendLocale(me);
    set({ me });
  },

  refreshMe: async () => {
    const me = await fetchMe();
    get().setMe(me);
    return me;
  },

  switchOrganization: async (slug) => {
    await setTokens(fromApi(await switchOrganization(slug)));
    // Every cached list belongs to the previous organization.
    queryClient.clear();
    return get().refreshMe();
  },

  signOut: async ({ remote = true } = {}) => {
    if (remote) await logout().catch(() => undefined);
    await clearTokens();
    storage.remove(ME_KEY);
    queryClient.clear();
    set({ status: 'signedOut', me: null });
  },
}));

setSessionExpiredHandler(() => {
  void useSession.getState().signOut({ remote: false });
});
