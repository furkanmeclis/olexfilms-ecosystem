import { create } from 'zustand';

type UpdateRequired = { code: 'UPDATE_REQUIRED' | 'MOBILE_API_VERSION_UNSUPPORTED'; storeUrl?: string | null };

type AppStatus = {
  /** Set when the backend answers 426: the app shows the update screen. */
  updateRequired: UpdateRequired | null;
  setUpdateRequired: (u: UpdateRequired) => void;
};

export const useAppStatus = create<AppStatus>((set) => ({
  updateRequired: null,
  setUpdateRequired: (updateRequired) => set({ updateRequired }),
}));
