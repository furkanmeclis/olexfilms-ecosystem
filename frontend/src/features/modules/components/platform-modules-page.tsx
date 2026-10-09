"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { PageHeader } from "@/components/layout/page-header";
import {
  CLIENT_SIDE_MANUAL,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import {
  ModuleRequestsTable,
  usePendingModuleRequests,
} from "@/features/modules/components/module-requests";
import { modulesKeys } from "@/features/modules/hooks/use-features";
import { moduleLevelLabel, moduleName } from "@/features/modules/lib/labels";
import { modulesService } from "@/features/modules/services/modules.service";
import type {
  ModuleLevel,
  PlatformModule,
  PlatformModulePatch,
} from "@/features/modules/types";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const PLATFORM_MODULES_PERSIST_KEY = "platform-modules-v1";

export const MODULE_LEVELS: ModuleLevel[] = ["core", "standard", "addon"];

type SwitchField = keyof PlatformModulePatch;

/**
 * Platform admin: system switches and defaults of every module, and
 * (TEC-509) the module requests the center decides: distributors' and those
 * of dealers without a distributor.
 */
export function PlatformModulesPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.modules.platformWrite);
  const queryClient = useQueryClient();
  const { data, isLoading, isError, isFetching, refetch } = useQuery({
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
  const { mutate, isPending } = patch;
  const pending = usePendingModuleRequests("platform");
  const requestableKeys = useMemo(
    () =>
      (data?.items ?? []).filter((m) => m.level !== "core").map((m) => m.key),
    [data?.items],
  );

  const columns = useMemo<ColumnDef<PlatformModule, unknown>[]>(() => {
    // Inline edit: each switch PATCHes /v1/platform/modules/{key}.
    const switchColumn = (field: SwitchField, labelKey: string) =>
      createColumn<PlatformModule>({
        accessorKey: field,
        labelKey,
        enableSorting: true,
        filterVariant: "boolean",
        cell: ({ row }) => {
          const m = row.original;
          return (
            <Switch
              aria-label={t(labelKey)}
              checked={Boolean(m[field])}
              disabled={!canWrite || m.level === "core" || isPending}
              onCheckedChange={(v) =>
                mutate({ key: m.key, body: { [field]: v === true } })
              }
            />
          );
        },
      });

    return [
      createColumn<PlatformModule>({
        id: "module",
        accessorFn: (row) => moduleName(t, row.key),
        labelKey: "modules.columns.module",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="flex min-w-0 flex-col">
            <span className="font-medium">
              {moduleName(t, row.original.key)}
            </span>
            <span className="text-muted-foreground font-mono text-xs">
              {row.original.key}
            </span>
          </div>
        ),
      }),
      createColumn<PlatformModule>({
        accessorKey: "level",
        labelKey: "modules.columns.level",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: MODULE_LEVELS.map((value) => ({
          value,
          labelKey: `modules.level.${value}`,
          label: value,
        })),
        gridSecondary: true,
        cell: ({ row }) => (
          <Badge
            variant={row.original.level === "core" ? "secondary" : "outline"}
          >
            {moduleLevelLabel(t, row.original.level)}
          </Badge>
        ),
      }),
      switchColumn("enabled", "modules.platform.system_enabled"),
      switchColumn("default_enabled", "modules.platform.default_enabled"),
      switchColumn("paid", "modules.platform.paid"),
    ];
  }, [canWrite, isPending, mutate, t]);

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("modules.platform.title")}
        description={t("modules.platform.description")}
      />
      <Tabs defaultValue="modules">
        <TabsList>
          <TabsTrigger value="modules">
            {t("modules.platform.tab_modules")}
          </TabsTrigger>
          <TabsTrigger value="requests" data-testid="modules-requests-tab">
            {t("modules.tabs.requests")}
            {pending.data ? (
              <Badge variant="warning" className="ms-1 tabular-nums">
                {pending.data}
              </Badge>
            ) : null}
          </TabsTrigger>
        </TabsList>
        <TabsContent value="modules">
          <EntityTable
            columns={columns}
            data={data?.items ?? []}
            getRowId={(row) => row.key}
            manual={CLIENT_SIDE_MANUAL}
            isLoading={isLoading}
            isError={isError}
            errorTitle={t("modules.error.title")}
            errorDescription={t("modules.error.description")}
            onRetry={() => void refetch()}
            initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
            pageSizeOptions={[20, 50, 100]}
            features={{
              persistKey: PLATFORM_MODULES_PERSIST_KEY,
              rowSelection: false,
            }}
            toolbarExtra={
              <EntityToolbar
                onRefresh={() => void refetch()}
                refreshDisabled={isFetching}
              />
            }
          />
        </TabsContent>
        <TabsContent value="requests" className="space-y-3">
          <p className="text-muted-foreground text-sm">
            {t("modules.requests.platform_hint")}
          </p>
          <ModuleRequestsTable
            scope="platform"
            canDecide={canWrite}
            moduleKeys={requestableKeys}
          />
        </TabsContent>
      </Tabs>
    </div>
  );
}
