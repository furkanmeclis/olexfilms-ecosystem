import type { OverviewFetchScopes } from "@/features/platform-overview/services/overview.service";

export const overviewKeys = {
  all: ["platform-overview"] as const,
  stats: (scopes: OverviewFetchScopes) =>
    [...overviewKeys.all, "stats", scopes] as const,
};
