export const aiAdminKeys = {
  all: ["ai-admin"] as const,
  settings: () => [...aiAdminKeys.all, "settings"] as const,
  orgQuotas: () => [...aiAdminKeys.all, "org-quotas"] as const,
  orgQuotaList: (params: Record<string, unknown>) =>
    [...aiAdminKeys.orgQuotas(), params] as const,
  usage: (scope: string) => [...aiAdminKeys.all, "usage", scope] as const,
  usageList: (scope: string, params: Record<string, unknown>) =>
    [...aiAdminKeys.usage(scope), params] as const,
  summary: (period: string) => [...aiAdminKeys.all, "summary", period] as const,
};
