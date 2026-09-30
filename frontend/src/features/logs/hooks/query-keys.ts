import type { ListLogsParams } from "@/features/logs/services/logs.service";

export const logsKeys = {
  all: ["platform", "logs"] as const,
  meta: () => [...logsKeys.all, "meta"] as const,
  stats: () => [...logsKeys.all, "stats"] as const,
  sources: () => [...logsKeys.all, "sources"] as const,
  lists: () => [...logsKeys.all, "list"] as const,
  list: (params: ListLogsParams) => [...logsKeys.lists(), params] as const,
  rules: () => [...logsKeys.all, "rules"] as const,
  rulesMeta: () => [...logsKeys.rules(), "meta"] as const,
};
