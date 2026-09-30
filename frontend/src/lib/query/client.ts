import { MutationCache, QueryCache, QueryClient } from "@tanstack/react-query";

import { ApiError, emitApiError } from "@/lib/api";

export function createQueryClient() {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 30_000,
        retry: (failureCount, error) => {
          if (error instanceof ApiError) {
            if ([401, 403, 404, 422].includes(error.status)) return false;
          }
          return failureCount < 2;
        },
        refetchOnWindowFocus: false,
      },
      mutations: {
        retry: false,
      },
    },
    queryCache: new QueryCache({
      onError: (error) => {
        if (error instanceof ApiError) emitApiError(error);
      },
    }),
    mutationCache: new MutationCache({
      onError: (error) => {
        if (error instanceof ApiError) emitApiError(error);
      },
    }),
  });
}
