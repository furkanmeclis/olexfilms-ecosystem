"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { BookOpen } from "lucide-react";
import Link from "next/link";
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
import { routes } from "@/config/routes";
import {
  ModuleNoteDialog,
  ModuleRequestsTable,
  usePendingModuleRequests,
} from "@/features/modules/components/module-requests";
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
  modulePriceKind,
  modulePriceText,
  moduleRequestAction,
  type ModulePriceKind,
} from "@/features/modules/lib/module-meta";
import {
  modulesService,
  type ListDealerModulesParams,
} from "@/features/modules/services/modules.service";
import type {
  DealerModuleRow,
  FeatureListItem,
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

/**
 * "Özellikler" page of the active organization (TEC-86, TEC-509). A
 * distributor manager also gets its dealers' matrix, the dealer standard and
 * the dealers' module request queue.
 */
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
        <UserGuideLink slug={slug} />
      </div>
      {canManage && org ? (
        <DistributorTabs slug={slug} orgUuid={org.uuid} />
      ) : (
        <OwnModules slug={slug} />
      )}
    </div>
  );
}

/**
 * TEC-509: the add-ons user guide is published to the document library
 * (F5-10c), so the link opens the library while it is reachable.
 */
function UserGuideLink({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const { data } = useFeatures(slug);
  if (!can(permissions.library.read)) return null;
  if (!data?.enabled.includes("announcements")) return null;
  return (
    <p>
      <Link
        href={routes.tenant.library.list(slug)}
        className="text-primary inline-flex items-center gap-1 underline-offset-4 hover:underline"
        data-testid="modules-user-guide"
      >
        <BookOpen className="size-4" aria-hidden />
        {t("modules.user_guide")}
      </Link>
    </p>
  );
}

function DistributorTabs({ slug, orgUuid }: { slug: string; orgUuid: string }) {
  const { t } = useLocale();
  const own = useFeatures(slug);
  const pending = usePendingModuleRequests("tenant");
  const moduleKeys = useMemo(
    () =>
      (own.data?.items ?? [])
        .filter((m) => m.level !== "core")
        .map((m) => m.key),
    [own.data],
  );
  return (
    <Tabs defaultValue="mine">
      <TabsList>
        <TabsTrigger value="mine">{t("modules.tabs.mine")}</TabsTrigger>
        <TabsTrigger value="dealers">{t("modules.tabs.dealers")}</TabsTrigger>
        <TabsTrigger value="standard">{t("modules.tabs.standard")}</TabsTrigger>
        <TabsTrigger value="requests" data-testid="modules-requests-tab">
          {t("modules.tabs.requests")}
          {pending.data ? (
            <Badge variant="warning" className="ms-1 tabular-nums">
              {pending.data}
            </Badge>
          ) : null}
        </TabsTrigger>
      </TabsList>
      <TabsContent value="mine">
        <OwnModules slug={slug} />
      </TabsContent>
      <TabsContent value="dealers">
        <DealerModules slug={slug} orgUuid={orgUuid} />
      </TabsContent>
      <TabsContent value="standard">
        <DealerStandard orgUuid={orgUuid} />
      </TabsContent>
      <TabsContent value="requests" className="space-y-3">
        <p className="text-muted-foreground text-sm">
          {t("modules.requests.tenant_hint")}
        </p>
        <ModuleRequestsTable scope="tenant" canDecide moduleKeys={moduleKeys} />
      </TabsContent>
    </Tabs>
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

/**
 * TEC-311: a module switched on by a module bundle subscription
 * (source=service) is labelled "on by subscription".
 */
export function SubscriptionModuleBadge({ source }: { source?: string }) {
  const { t } = useLocale();
  if (source !== "service") return null;
  return (
    <Badge variant="secondary" data-testid="module-via-subscription">
      {t("modules.via_subscription")}
    </Badge>
  );
}

/**
 * TEC-509 source badge: default / dealer standard / admin / distributor /
 * subscription (service) and who set it.
 */
export function ModuleSourceBadge({ item }: { item: ModuleState }) {
  const { t } = useLocale();
  if (item.source === "service") {
    return <SubscriptionModuleBadge source={item.source} />;
  }
  return (
    <span className="inline-flex flex-wrap items-center gap-1">
      <Badge variant="outline" data-testid="module-source">
        {moduleSourceLabel(t, item.source)}
      </Badge>
      {item.set_by ? (
        <span className="text-muted-foreground text-xs">
          {item.set_by.name}
        </span>
      ) : null}
    </span>
  );
}

const PRICE_KINDS: ModulePriceKind[] = [
  "free_default",
  "paid",
  "contact",
  "free",
];

export function ownModulesPersistKey(level: ModuleLevel) {
  return `${OWN_MODULES_PERSIST_KEY}-${level}`;
}

/**
 * Own modules grouped by level (TEC-509): core / standard / add-on headings
 * with counts; each module with its description, price line, status and
 * source, and the request flow (dialog with a note, pending badge,
 * withdraw). A module off at the level above has no request button.
 */
export function OwnModules({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const queryClient = useQueryClient();
  const org = useActiveOrganization(slug);
  const canRequest = can(permissions.modules.read);
  const { data, isLoading, isError, isFetching, refetch } = useFeatures(slug);
  const [requesting, setRequesting] = useState<string | null>(null);
  const items = useMemo<FeatureListItem[]>(
    () => data?.items ?? [],
    [data?.items],
  );

  const refresh = async () => {
    await queryClient.invalidateQueries({
      queryKey: modulesKeys.features(org?.uuid ?? ""),
    });
  };
  const request = useMutation({
    mutationFn: ({ key, note }: { key: string; note: string }) =>
      modulesService.request(key, note),
    onSuccess: async () => {
      setRequesting(null);
      appToast.success(t("modules.requested"));
      await refresh();
    },
    onError: (error) =>
      appToast.error(errorMessage(error, t("modules.request_failed"))),
  });
  const cancel = useMutation({
    mutationFn: (key: string) => modulesService.cancelRequest(key),
    onSuccess: async () => {
      appToast.success(t("modules.requests.cancelled"));
      await refresh();
    },
    onError: (error) =>
      appToast.error(errorMessage(error, t("modules.toast.failed"))),
  });
  const cancelPending = cancel.isPending;
  const cancelRequest = cancel.mutate;

  const sources = useMemo(
    () => [...new Set(items.map((item) => item.source))],
    [items],
  );

  const columns = useMemo<ColumnDef<FeatureListItem, unknown>[]>(
    () => [
      createColumn<FeatureListItem>({
        id: "module",
        accessorFn: (row) => `${moduleName(t, row.key)} ${row.description}`,
        labelKey: "modules.columns.module",
        enableSorting: true,
        sortingFn: (a, b) =>
          moduleName(t, a.original.key).localeCompare(
            moduleName(t, b.original.key),
          ),
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="flex min-w-0 flex-col gap-0.5">
            <span
              className="font-medium"
              data-testid={`module-${row.original.key}`}
            >
              {moduleName(t, row.original.key)}
            </span>
            {row.original.description ? (
              <span className="text-muted-foreground text-xs whitespace-normal">
                {row.original.description}
              </span>
            ) : null}
          </div>
        ),
      }),
      createColumn<FeatureListItem>({
        accessorKey: "enabled",
        labelKey: "modules.columns.status",
        enableSorting: true,
        filterVariant: "boolean",
        gridSecondary: true,
        cell: ({ row }) => <ModuleStatusCell item={row.original} />,
      }),
      createColumn<FeatureListItem>({
        id: "price",
        accessorFn: (row) => modulePriceKind(row),
        labelKey: "modules.columns.price",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: PRICE_KINDS.map((value) => ({
          value,
          labelKey: `modules.price.filter.${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <span
            className="text-sm whitespace-normal"
            data-testid={`module-price-${row.original.key}`}
          >
            {modulePriceText(
              t,
              (amount, code) => format.currency(amount, code),
              row.original,
            )}
          </span>
        ),
      }),
      createColumn<FeatureListItem>({
        accessorKey: "source",
        labelKey: "modules.columns.source",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: sources.map((value) => ({
          value,
          label:
            value === "service"
              ? t("modules.via_subscription")
              : moduleSourceLabel(t, value),
        })),
        cell: ({ row }) => <ModuleSourceBadge item={row.original} />,
      }),
      createColumn<FeatureListItem>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          if (!canRequest) return null;
          const action = moduleRequestAction(row.original);
          if (action === "request") {
            return (
              <div className="text-end">
                <Button
                  size="sm"
                  variant="outline"
                  data-testid={`module-request-${row.original.key}`}
                  onClick={() => setRequesting(row.original.key)}
                >
                  {t("modules.request")}
                </Button>
              </div>
            );
          }
          if (action === "pending") {
            return (
              <div className="text-end">
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={cancelPending}
                  data-testid={`module-request-cancel-${row.original.key}`}
                  onClick={() => cancelRequest(row.original.key)}
                >
                  {t("modules.requests.withdraw")}
                </Button>
              </div>
            );
          }
          return null;
        },
      }),
    ],
    [canRequest, cancelPending, cancelRequest, format, sources, t],
  );

  if (isLoading) return <Loading label={t("common.loading")} />;

  if (isError) {
    return (
      <EntityTable
        columns={columns}
        data={[]}
        manual={CLIENT_SIDE_MANUAL}
        isError
        errorTitle={t("modules.error.title")}
        errorDescription={t("modules.error.description")}
        onRetry={() => void refetch()}
      />
    );
  }

  return (
    <div className="space-y-8">
      {MODULE_LEVELS.map((level) => {
        const rows = items.filter((item) => item.level === level);
        if (!rows.length) return null;
        return (
          <section
            key={level}
            className="space-y-3"
            data-testid={`module-level-${level}`}
          >
            <h2 className="flex items-center gap-2 text-base font-semibold">
              {moduleLevelLabel(t, level)}
              <Badge variant="secondary" className="tabular-nums">
                {rows.length}
              </Badge>
            </h2>
            <EntityTable
              columns={columns}
              data={rows}
              getRowId={(row) => row.key}
              manual={CLIENT_SIDE_MANUAL}
              emptyTitle={t("modules.empty")}
              emptyDescription=""
              initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
              pageSizeOptions={[20, 50, 100]}
              features={{
                persistKey: ownModulesPersistKey(level),
                rowSelection: false,
              }}
              toolbarExtra={
                <EntityToolbar
                  onRefresh={() => void refetch()}
                  refreshDisabled={isFetching}
                />
              }
            />
          </section>
        );
      })}
      <ModuleNoteDialog
        open={Boolean(requesting)}
        title={t("modules.requests.dialog_title", {
          module: requesting ? moduleName(t, requesting) : "",
        })}
        description={t("modules.requests.dialog_description")}
        submitLabel={t("modules.request")}
        pending={request.isPending}
        onOpenChange={(open) => !open && setRequesting(null)}
        onSubmit={(note) =>
          requesting && request.mutate({ key: requesting, note })
        }
      />
    </div>
  );
}

/** On / off, "off at the level above", and the last request's state. */
function ModuleStatusCell({ item }: { item: FeatureListItem }) {
  const { t } = useLocale();
  const action = moduleRequestAction(item);
  return (
    <span className="inline-flex flex-wrap items-center gap-1">
      {action === "upstream_closed" ? (
        <Badge variant="outline" data-testid="module-upstream-closed">
          {t("modules.upstream_closed")}
        </Badge>
      ) : (
        <StateBadge on={item.enabled} />
      )}
      {action === "pending" ? (
        <Badge variant="warning" data-testid="module-request-pending">
          {t("modules.requests.pending_badge")}
        </Badge>
      ) : null}
      {action === "request" && item.request?.status === "rejected" ? (
        <Badge
          variant="danger"
          title={item.request.decision_note || undefined}
          data-testid="module-request-rejected"
        >
          {t("modules.requests.rejected_badge")}
        </Badge>
      ) : null}
    </span>
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
          <span className="inline-flex flex-wrap items-center gap-2">
            <StateBadge on={Boolean(entry(row.original)?.enabled)} />
            <SubscriptionModuleBadge source={entry(row.original)?.source} />
          </span>
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
