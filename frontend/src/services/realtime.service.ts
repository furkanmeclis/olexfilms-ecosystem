import { apiClient, unwrap } from "@/lib/api";
import type {
  ConnectionTokenResult,
  SubscriptionTokenResult,
} from "@/lib/realtime/types";

/**
 * OpenAPI-backed realtime token service.
 * Browser calls BFF with HttpOnly cookies; never stores API JWTs in JS.
 * Returned `token` values are Centrifugo JWTs (separate from API access tokens).
 *
 * Uses silent unwrap — `503 REALTIME_DISABLED` (and other connect failures)
 * soft-fail in RealtimeProvider without global toast spam.
 */
export const realtimeService = {
  async connectionToken() {
    return unwrap<ConnectionTokenResult>(
      await apiClient.POST("/v1/realtime/connection-token"),
      { silent: true },
    );
  },

  async subscriptionToken(channel: string) {
    return unwrap<SubscriptionTokenResult>(
      await apiClient.POST("/v1/realtime/subscription-token", {
        body: { channel },
      }),
      { silent: true },
    );
  },
};
