"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { useMemo, useState } from "react";

import { Loading } from "@/components/common/loading";
import {
  CLIENT_SIDE_MANUAL,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import {
  modulesKeys,
  useFeatures,
} from "@/features/modules/hooks/use-features";
import {
  moduleLevelLabel,
  moduleName,
  moduleSourceLabel,
} from "@/features/modules/lib/labels";
import {
  modulesService,
  type ListDealerModulesParams,
} from "@/features/modules/services/modules.service";
import type {
  DealerModuleRow,
  ModuleLevel,
  ModuleState,
} from "@/features/modules/types";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

const LEVEL_VARIANT: Record<ModuleLevel, "secondary" | "outline" | "default"> =
  {
    core: "secondary",
    standard: "outline",
    addon: "default",
  };

function LevelBadge({ level }: { level: ModuleLevel }) {
  const { t } = useLocale();
  return (
    <Badge variant={LEVEL_VARIANT[level]}>{moduleLevelLabel(t, level)}</Badge>
  );
}

function StateBadge({ on }: { on: boolean }) {
  const { t } = useLocale();
  return (
    <Badge variant={on ? "success" : "outline"}>
      {on ? t("modules.state.on") : t("modules.state.off")}
    </Badge>
  );
}

function errorMessage(error: unknown, fallback: string) {
  return isApiError(error) ? error.message : fallback;
}

/** "Özellikler" page of the active organization (TEC-86). */
export function FeaturesPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const isDistributor = org?.type === "distributor";
  const canManage = isDistributor && can(permissions.modules.manage);

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("modules.title")}
        description={t("modules.description")}
      />
      <div className="text-muted-foreground space-y-1 text-sm">
        <p>{t("modules.free_note")}</p>
        <p>{t("modules.upstream_note")}</p>
      </div>
      {canManage && org ? (
        <Tabs defaultValue="mine">
          <TabsList>
            <TabsTrigger value="mine">{t("modules.tabs.mine")}</TabsTrigger>
            <TabsTrigger value="dealers">
              {t("modules.tabs.dealers")}
            </TabsTrigger>
            <TabsTrigger value="standard">
              {t("modules.tabs.standard")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="mine">
            <OwnModules slug={slug} />
          </TabsContent>
          <TabsContent value="dealers">
            <DealerModules slug={slug} orgUuid={org.uuid} />
          </TabsContent>
          <TabsContent value="standard">
            <DealerStandard orgUuid={org.uuid} />
          </TabsContent>
        </Tabs>
      ) : (
        <OwnModules slug={slug} />
      )}
    </div>
  );
}

export const OWN_MODULES_PERSIST_KEY = "tenant-own-modules-v1";
export const DEALER_MODULES_PERSIST_KEY = "tenant-dealer-modules-v1";

/** `GET /v1/tenant/modules/dealers` source filter values (TEC-367). */
export const DEALER_MODULE_SOURCES = [
  "core",
  "system",
  "default",
  "upstream",
  "standard",
  "admin",
  "distributor",
  "service",
] as const;
export const DEALER_MODULE_STATES = ["enabled", "disabled"] as const;
const MODULE_LEVELS: ModuleLevel[] = ["core", "standard", "addon"];

function OwnModules({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRequest = can(permissions.modules.read);
  const { data, isLoading, isError, isFetching, refetch } = useFeatures(slug);
  const request = useMutation({
    mutationFn: (key: string) => modulesService.request(key),
    onSuccess: () => appToast.success(t("modules.requested")),
    onError: (error) =>
      appToast.error(errorMessage(error, t("modules.request_failed"))),
  });
  const requestPending = request.isPending;
  const requestModule = request.mutate;

  const sources = useMemo(
    () => [...new Set((data?.items ?? []).map((item) => item.source))],
    [data?.items],
  );

  const columns = useMemo<ColumnDef<ModuleState, unknown>[]>(
    () => [
      createColumn<ModuleState>({
        id: "module",
        accessorFn: (row) => moduleName(t, row.key),
        labelKey: "modules.columns.module",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span
            className="font-medium"
            data-testid={`module-${row.original.key}`}
          >
            {moduleName(t, row.original.key)}
          </span>
        ),
      }),
      createColumn<ModuleState>({
        accessorKey: "level",
        labelKey: "modules.columns.level",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: MODULE_LEVELS.map((value) => ({
          value,
          labelKey: `modules.level.${value}`,
          label: value,
        })),
        cell: ({ row }) => <LevelBadge level={row.original.level} />,
      }),
      createColumn<ModuleState>({
        accessorKey: "enabled",
        labelKey: "modules.columns.status",
        enableSorting: true,
        filterVariant: "boolean",
        gridSecondary: true,
        cell: ({ row }) => <StateBadge on={row.original.enabled} />,
      }),
      createColumn<ModuleState>({
        id: "price",
        accessorFn: (row) =>
          row.paid && !row.default_enabled ? "paid" : "free",
        labelKey: "modules.columns.price",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: ["paid", "free"].map((value) => ({
          value,
          labelKey: `modules.price.${value}`,
          label: value,
        })),
        cell: ({ getValue }) => t(`modules.price.${String(getValue())}`),
      }),
      createColumn<ModuleState>({
        accessorKey: "source",
        labelKey: "modules.columns.source",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: sources.map((value) => ({
          value,
          label: moduleSourceLabel(t, value),
        })),
        cell: ({ row }) => (
          <span className="text-muted-foreground">
            {moduleSourceLabel(t, row.original.source)}
            {row.original.set_by ? ` · ${row.original.set_by.name}` : null}
          </span>
        ),
      }),
      createColumn<ModuleState>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) =>
          !row.original.enabled && canRequest ? (
            <div className="text-end">
              <Button
                size="sm"
                variant="outline"
                disabled={requestPending}
                onClick={() => requestModule(row.original.key)}
              >
                {t("modules.request")}
              </Button>
            </div>
          ) : null,
      }),
    ],
    [canRequest, requestModule, requestPending, sources, t],
  );

  return (
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
      emptyTitle={t("modules.empty")}
      emptyDescription=""
      initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
      pageSizeOptions={[20, 50, 100]}
      features={{
        persistKey: OWN_MODULES_PERSIST_KEY,
        rowSelection: false,
      }}
      toolbarExtra={
        <EntityToolbar
          onRefresh={() => void refetch()}
          refreshDisabled={isFetching}
        />
      }
    />
  );
}

/**
 * Dealer module matrix for one module (server list: q on name/slug, sort
 * name/slug, paging; state/source filters go with `module`). Bulk
 * enable/disable/reset uses `POST /v1/tenant/modules/dealers/bulk`.
 */
export function DealerModules({
  slug,
  orgUuid,
}: {
  slug: string;
  orgUuid: string;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const own = useFeatures(slug);
  const switchable = useMemo(
    () => (own.data?.items ?? []).filter((m) => m.level !== "core"),
    [own.data],
  );
  const [key, setKey] = useState<string>("");
  const moduleKey = key || switchable[0]?.key || "";
  const ownState = switchable.find((m) => m.key === moduleKey);
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const selectedIds = useMemo(
    () => Object.keys(rowSelection).filter((id) => rowSelection[id]),
    [rowSelection],
  );

  const columns = useMemo<ColumnDef<DealerModuleRow, unknown>[]>(() => {
    const entry = (row: DealerModuleRow) =>
      row.modules.find((m) => m.key === moduleKey);
    return [
      createSelectColumnDef<DealerModuleRow>(),
      createColumn<DealerModuleRow>({
        accessorKey: "name",
        labelKey: "modules.columns.dealer",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.name}</span>
        ),
      }),
      createColumn<DealerModuleRow>({
        accessorKey: "slug",
        labelKey: "modules.columns.slug",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => (
          <span className="text-muted-foreground font-mono text-xs">
            {row.original.slug}
          </span>
        ),
      }),
      createColumn<DealerModuleRow>({
        id: "state",
        accessorFn: (row) => (entry(row)?.enabled ? "enabled" : "disabled"),
        labelKey: "modules.columns.status",
        enableSorting: false,
        filterVariant: "faceted",
        param: "state",
        gridSecondary: true,
        filterOptions: DEALER_MODULE_STATES.map((value) => ({
          value,
          labelKey:
            value === "enabled" ? "modules.state.on" : "modules.state.off",
          label: value,
        })),
        cell: ({ row }) => (
          <StateBadge on={Boolean(entry(row.original)?.enabled)} />
        ),
      }),
      createColumn<DealerModuleRow>({
        id: "source",
        accessorFn: (row) => entry(row)?.source ?? "",
        labelKey: "modules.columns.source",
        enableSorting: false,
        filterVariant: "faceted",
        param: "source",
        filterOptions: DEALER_MODULE_SOURCES.map((value) => ({
          value,
          labelKey: `modules.source.${value}`,
          label: value,
        })),
        cell: ({ row }) => {
          const st = entry(row.original);
          return (
            <span className="text-muted-foreground">
              {st?.admin_override
                ? t("modules.dealers.admin_override")
                : st
                  ? moduleSourceLabel(t, st.source)
                  : null}
            </span>
          );
        },
      }),
    ];
  }, [moduleKey, t]);

  const listState = useServerListState({
    columns,
    initialSort: "name",
    initialPageSize: 20,
    persistKey: DEALER_MODULES_PERSIST_KEY,
  });
  const listParams = useMemo<ListDealerModulesParams>(() => {
    const { state, source } = listState.filterParams;
    // state / source only make sense (and are only accepted) with `module`.
    return {
      ...listState.params,
      ...(moduleKey && (state || source) ? { module: moduleKey } : {}),
    };
  }, [listState.filterParams, listState.params, moduleKey]);

  const dealers = useQuery({
    queryKey: [...modulesKeys.dealers(orgUuid), listParams],
    queryFn: () => modulesService.dealers(listParams),
    enabled: Boolean(moduleKey) || !own.isLoading,
    placeholderData: (previous) => previous,
  });

  const bulk = useMutation({
    mutationFn: (enabled: boolean | null) =>
      modulesService.bulk(selectedIds, moduleKey, enabled),
    onSuccess: async () => {
      setRowSelection({});
      await queryClient.invalidateQueries({
        queryKey: modulesKeys.dealers(orgUuid),
      });
      appToast.success(t("modules.toast.saved"));
    },
    onError: (error) =>
      appToast.error(errorMessage(error, t("modules.toast.failed"))),
  });

  const noSelection = !selectedIds.length || bulk.isPending || !moduleKey;

  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        {t("modules.dealers.hint")}
      </p>
      {ownState && !ownState.enabled ? (
        <p className="text-sm text-amber-700 dark:text-amber-300">
          {t("modules.dealers.upstream_off")}
        </p>
      ) : null}
      <EntityTable
        columns={columns}
        data={dealers.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={dealers.isLoading || own.isLoading}
        isError={dealers.isError}
        errorTitle={t("modules.error.title")}
        errorDescription={t("modules.error.description")}
        onRetry={() => void dealers.refetch()}
        emptyTitle={t("modules.dealers.empty")}
        emptyDescription=""
        rowCount={dealers.data?.total ?? 0}
        state={{
          ...listState.tableState,
          rowSelection,
          onRowSelectionChange: setRowSelection,
        }}
        features={{
          persistKey: DEALER_MODULES_PERSIST_KEY,
          rowSelection: true,
        }}
        toolbarExtra={
          <>
            <Select value={moduleKey} onValueChange={setKey}>
              <SelectTrigger
                className="h-8 w-56"
                aria-label={t("modules.dealers.module")}
                data-testid="dealer-module-select"
              >
                <SelectValue placeholder={t("modules.dealers.module")} />
              </SelectTrigger>
              <SelectContent>
                {switchable.map((m) => (
                  <SelectItem key={m.key} value={m.key}>
                    {moduleName(t, m.key)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {selectedIds.length ? (
              <>
                <span className="text-muted-foreground text-sm">
                  {t("modules.dealers.selected", { count: selectedIds.length })}
                </span>
                <Button
                  size="sm"
                  disabled={noSelection || !ownState?.enabled}
                  onClick={() => bulk.mutate(true)}
                >
                  {t("modules.dealers.enable")}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  disabled={noSelection}
                  onClick={() => bulk.mutate(false)}
                >
                  {t("modules.dealers.disable")}
                </Button>
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={noSelection}
                  onClick={() => bulk.mutate(null)}
                >
                  {t("modules.dealers.reset")}
                </Button>
              </>
            ) : null}
            <EntityToolbar
              onRefresh={() => void dealers.refetch()}
              refreshDisabled={dealers.isFetching}
            />
          </>
        }
      />
    </div>
  );
}

function DealerStandard({ orgUuid }: { orgUuid: string }) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const standard = useQuery({
    queryKey: modulesKeys.standard(orgUuid),
    queryFn: () => modulesService.dealerStandard(),
  });
  const save = useMutation({
    mutationFn: ({ key, enabled }: { key: string; enabled: boolean | null }) =>
      modulesService.setDealerStandard(key, enabled),
    onSuccess: async (data) => {
      queryClient.setQueryData(modulesKeys.standard(orgUuid), data);
      await queryClient.invalidateQueries({
        queryKey: modulesKeys.dealers(orgUuid),
      });
      appToast.success(t("modules.toast.saved"));
    },
    onError: (error) =>
      appToast.error(errorMessage(error, t("modules.toast.failed"))),
  });

  if (standard.isLoading) return <Loading label={t("common.loading")} />;
  const items = standard.data?.items ?? [];

  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        {t("modules.standard.hint")}
      </p>
      <div className="divide-y rounded-lg border">
        {items.map((e) => (
          <div
            key={e.key}
            className="flex flex-wrap items-center justify-between gap-3 px-3 py-2"
          >
            <div className="flex items-center gap-2">
              <span className="font-medium">{moduleName(t, e.key)}</span>
              <LevelBadge level={e.level} />
              <span className="text-muted-foreground text-xs">
                {e.explicit
                  ? t("modules.standard.explicit")
                  : t("modules.standard.inherited")}
              </span>
            </div>
            <div className="flex items-center gap-2">
              {e.explicit ? (
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={save.isPending}
                  onClick={() => save.mutate({ key: e.key, enabled: null })}
                >
                  {t("modules.standard.reset")}
                </Button>
              ) : null}
              <Switch
                aria-label={moduleName(t, e.key)}
                checked={e.enabled}
                disabled={
                  save.isPending || (!e.distributor_enabled && !e.enabled)
                }
                onCheckedChange={(v) =>
                  save.mutate({ key: e.key, enabled: v === true })
                }
              />
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
