import { createAsyncStoragePersister } from '@tanstack/query-async-storage-persister';
import { QueryClient } from '@tanstack/react-query';

import { isApiError } from '@/core/api/errors';
import { APP_VERSION } from '@/core/config';
import { queryStorage } from '@/core/storage';

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // Cached data renders immediately; a refetch runs in the background.
      staleTime: 30_000,
      gcTime: 7 * 24 * 60 * 60 * 1000,
      retry: (count, e) => count < 2 && !(isApiError(e) && e.status >= 400 && e.status < 500),
      networkMode: 'offlineFirst',
    },
    mutations: { networkMode: 'offlineFirst' },
  },
});

/** Persists the query cache in MMKV so the first frame after launch has data. */
export const queryPersister = createAsyncStoragePersister({ storage: queryStorage, key: 'olex.query', throttleTime: 1000 });

export const persistOptions = {
  persister: queryPersister,
  maxAge: 7 * 24 * 60 * 60 * 1000,
  // A new app version starts with an empty cache, so old shapes never render.
  buster: APP_VERSION,
};
