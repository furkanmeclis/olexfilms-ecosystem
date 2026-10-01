"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { modulesKeys } from "@/features/modules/hooks/use-features";
import { moduleLevelLabel, moduleName } from "@/features/modules/lib/labels";
import { modulesService } from "@/features/modules/services/modules.service";
import type { PlatformModulePatch } from "@/features/modules/types";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

/** Platform admin: system switches and defaults of every module. */
export function PlatformModulesPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.modules.platformWrite);
  const queryClient = useQueryClient();
  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: modulesKeys.platform,
    queryFn: () => modulesService.platformList(),
  });
  const patch = useMutation({
    mutationFn: ({ key, body }: { key: string; body: PlatformModulePatch }) =>
      modulesService.platformPatch(key, body),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: modulesKeys.all });
      appToast.success(t("modules.toast.saved"));
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("modules.toast.failed"),
      ),
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("modules.platform.title")}
        description={t("modules.platform.description")}
      />
      {isLoading ? <Loading label={t("common.loading")} /> : null}
      {isError ? (
        <ErrorState
          title={t("modules.error.title")}
          description={t("modules.error.description")}
          retryLabel={t("common.retry")}
          onRetry={() => refetch()}
        />
      ) : null}
      {data ? (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-muted-foreground">
              <tr>
                <th className="px-3 py-2 text-start font-medium">
                  {t("modules.columns.module")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("modules.columns.level")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("modules.platform.system_enabled")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("modules.platform.default_enabled")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("modules.platform.paid")}
                </th>
              </tr>
            </thead>
            <tbody>
              {data.items.map((m) => {
                const core = m.level === "core";
                const disabled = !canWrite || core || patch.isPending;
                const set = (body: PlatformModulePatch) =>
                  patch.mutate({ key: m.key, body });
                return (
                  <tr key={m.key} className="border-t">
                    <td className="px-3 py-2 font-medium">
                      {moduleName(t, m.key)}
                    </td>
                    <td className="px-3 py-2">
                      <Badge variant={core ? "secondary" : "outline"}>
                        {moduleLevelLabel(t, m.level)}
                      </Badge>
                    </td>
                    <td className="px-3 py-2">
                      <Switch
                        aria-label={t("modules.platform.system_enabled")}
                        checked={m.enabled}
                        disabled={disabled}
                        onCheckedChange={(v) => set({ enabled: v === true })}
                      />
                    </td>
                    <td className="px-3 py-2">
                      <Switch
                        aria-label={t("modules.platform.default_enabled")}
                        checked={m.default_enabled}
                        disabled={disabled}
                        onCheckedChange={(v) =>
                          set({ default_enabled: v === true })
                        }
                      />
                    </td>
                    <td className="px-3 py-2">
                      <Switch
                        aria-label={t("modules.platform.paid")}
                        checked={m.paid}
                        disabled={disabled}
                        onCheckedChange={(v) => set({ paid: v === true })}
                      />
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}
