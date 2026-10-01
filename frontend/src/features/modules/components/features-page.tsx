"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import { modulesService } from "@/features/modules/services/modules.service";
import type { ModuleLevel, ModuleState } from "@/features/modules/types";
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

function OwnModules({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRequest = can(permissions.modules.read);
  const { data, isLoading, isError, refetch } = useFeatures(slug);
  const request = useMutation({
    mutationFn: (key: string) => modulesService.request(key),
    onSuccess: () => appToast.success(t("modules.requested")),
    onError: (error) =>
      appToast.error(errorMessage(error, t("modules.request_failed"))),
  });

  if (isLoading) return <Loading label={t("common.loading")} />;
  if (isError || !data) {
    return (
      <ErrorState
        title={t("modules.error.title")}
        description={t("modules.error.description")}
        retryLabel={t("common.retry")}
        onRetry={() => refetch()}
      />
    );
  }
  if (!data.items.length) return <EmptyState title={t("modules.empty")} />;

  return (
    <div className="overflow-x-auto rounded-lg border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50 text-muted-foreground">
          <tr className="text-start">
            <th className="px-3 py-2 text-start font-medium">
              {t("modules.columns.module")}
            </th>
            <th className="px-3 py-2 text-start font-medium">
              {t("modules.columns.level")}
            </th>
            <th className="px-3 py-2 text-start font-medium">
              {t("modules.columns.status")}
            </th>
            <th className="px-3 py-2 text-start font-medium">
              {t("modules.columns.price")}
            </th>
            <th className="px-3 py-2 text-start font-medium">
              {t("modules.columns.source")}
            </th>
            <th className="px-3 py-2" />
          </tr>
        </thead>
        <tbody>
          {data.items.map((item: ModuleState) => (
            <tr
              key={item.key}
              className="border-t"
              data-testid={`module-${item.key}`}
            >
              <td className="px-3 py-2 font-medium">
                {moduleName(t, item.key)}
              </td>
              <td className="px-3 py-2">
                <LevelBadge level={item.level} />
              </td>
              <td className="px-3 py-2">
                <StateBadge on={item.enabled} />
              </td>
              <td className="px-3 py-2">
                {item.paid && !item.default_enabled
                  ? t("modules.price.paid")
                  : t("modules.price.free")}
              </td>
              <td className="text-muted-foreground px-3 py-2">
                {moduleSourceLabel(t, item.source)}
                {item.set_by ? ` · ${item.set_by.name}` : null}
              </td>
              <td className="px-3 py-2 text-end">
                {!item.enabled && canRequest ? (
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={request.isPending}
                    onClick={() => request.mutate(item.key)}
                  >
                    {t("modules.request")}
                  </Button>
                ) : null}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function DealerModules({ slug, orgUuid }: { slug: string; orgUuid: string }) {
  const { t } = useLocale();
  const queryClient = useQueryClient();
  const own = useFeatures(slug);
  const dealers = useQuery({
    queryKey: modulesKeys.dealers(orgUuid),
    queryFn: () => modulesService.dealers(),
  });
  const switchable = useMemo(
    () => (own.data?.items ?? []).filter((m) => m.level !== "core"),
    [own.data],
  );
  const [key, setKey] = useState<string>("");
  const moduleKey = key || switchable[0]?.key || "";
  const ownState = switchable.find((m) => m.key === moduleKey);
  const [selected, setSelected] = useState<Set<string>>(new Set());

  const bulk = useMutation({
    mutationFn: (enabled: boolean | null) =>
      modulesService.bulk([...selected], moduleKey, enabled),
    onSuccess: async () => {
      setSelected(new Set());
      await queryClient.invalidateQueries({
        queryKey: modulesKeys.dealers(orgUuid),
      });
      appToast.success(t("modules.toast.saved"));
    },
    onError: (error) =>
      appToast.error(errorMessage(error, t("modules.toast.failed"))),
  });

  if (dealers.isLoading || own.isLoading) {
    return <Loading label={t("common.loading")} />;
  }
  const rows = dealers.data?.items ?? [];
  if (!rows.length) return <EmptyState title={t("modules.dealers.empty")} />;

  const allSelected = rows.every((d) => selected.has(d.uuid));
  const toggle = (uuid: string, on: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (on) next.add(uuid);
      else next.delete(uuid);
      return next;
    });
  };

  return (
    <div className="space-y-4">
      <p className="text-muted-foreground text-sm">
        {t("modules.dealers.hint")}
      </p>
      <div className="flex flex-wrap items-center gap-3">
        <label className="flex items-center gap-2 text-sm">
          {t("modules.dealers.module")}
          <select
            className="border-input bg-background h-9 rounded-md border px-2 text-sm"
            value={moduleKey}
            onChange={(e) => setKey(e.target.value)}
            data-testid="dealer-module-select"
          >
            {switchable.map((m) => (
              <option key={m.key} value={m.key}>
                {moduleName(t, m.key)}
              </option>
            ))}
          </select>
        </label>
        <span className="text-muted-foreground text-sm">
          {t("modules.dealers.selected", { count: selected.size })}
        </span>
        <Button
          size="sm"
          disabled={!selected.size || bulk.isPending || !ownState?.enabled}
          onClick={() => bulk.mutate(true)}
        >
          {t("modules.dealers.enable")}
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={!selected.size || bulk.isPending}
          onClick={() => bulk.mutate(false)}
        >
          {t("modules.dealers.disable")}
        </Button>
        <Button
          size="sm"
          variant="ghost"
          disabled={!selected.size || bulk.isPending}
          onClick={() => bulk.mutate(null)}
        >
          {t("modules.dealers.reset")}
        </Button>
      </div>
      {ownState && !ownState.enabled ? (
        <p className="text-sm text-amber-700 dark:text-amber-300">
          {t("modules.dealers.upstream_off")}
        </p>
      ) : null}
      <div className="overflow-x-auto rounded-lg border">
        <table className="w-full text-sm">
          <thead className="bg-muted/50 text-muted-foreground">
            <tr>
              <th className="w-10 px-3 py-2">
                <Checkbox
                  aria-label={t("modules.dealers.select_all")}
                  checked={allSelected}
                  onCheckedChange={(v) =>
                    setSelected(
                      v === true ? new Set(rows.map((d) => d.uuid)) : new Set(),
                    )
                  }
                />
              </th>
              <th className="px-3 py-2 text-start font-medium">
                {t("modules.columns.dealer")}
              </th>
              <th className="px-3 py-2 text-start font-medium">
                {t("modules.columns.status")}
              </th>
              <th className="px-3 py-2 text-start font-medium">
                {t("modules.columns.source")}
              </th>
            </tr>
          </thead>
          <tbody>
            {rows.map((d) => {
              const st = d.modules.find((m) => m.key === moduleKey);
              return (
                <tr key={d.uuid} className="border-t">
                  <td className="px-3 py-2">
                    <Checkbox
                      aria-label={d.name}
                      checked={selected.has(d.uuid)}
                      onCheckedChange={(v) => toggle(d.uuid, v === true)}
                    />
                  </td>
                  <td className="px-3 py-2 font-medium">{d.name}</td>
                  <td className="px-3 py-2">
                    <StateBadge on={Boolean(st?.enabled)} />
                  </td>
                  <td className="text-muted-foreground px-3 py-2">
                    {st?.admin_override
                      ? t("modules.dealers.admin_override")
                      : st
                        ? moduleSourceLabel(t, st.source)
                        : null}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </div>
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
