"use client";

import { useMemo, useState } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { routes } from "@/config/routes";
import {
  useUsageColumns,
  type FilterOption,
} from "@/features/ai-admin/components/usage-columns";
import { useAIUsage } from "@/features/ai-admin/hooks/use-ai-admin";
import { mergeOptions } from "@/features/ai-admin/lib/options";
import { aiUserName } from "@/features/ai-admin/lib/quota";
import {
  AI_USAGE_EXPORT_PATHS,
  type AIUsageListParams,
  type AIUsageRow,
  type AIUsageScope,
} from "@/features/ai-admin/services/ai-admin.service";
import { ExportMenu } from "@/features/io/components/export-menu";
import { useLocale } from "@/providers/locale-provider";

/** Backend default sort of the usage report (no /meta endpoint). */
export const AI_USAGE_DEFAULT_SORT = "-created_at";

export const AI_USAGE_PERSIST_KEYS: Record<AIUsageScope, string> = {
  platform: "platform-ai-usage-v1",
  tenant: "tenant-ai-usage-v1",
};

type UsageTableProps = {
  scope: AIUsageScope;
  /** Tenant slug, for the export jobs link. */
  slug?: string;
  /** Known users (e.g. the monthly summary), merged with the rows seen. */
  userOptions?: FilterOption[];
  /** Organization directory of the platform report. */
  organizationOptions?: FilterOption[];
  /** Model allow list, merged with the rows seen. */
  modelOptions?: FilterOption[];
};

/**
 * AI usage ledger (TEC-391): platform report of every organization or the
 * active organization's report; CSV / XLSX export with the same filters.
 */
export function UsageTable({
  scope,
  slug,
  userOptions = [],
  organizationOptions = [],
  modelOptions = [],
}: UsageTableProps) {
  const { t } = useLocale();
  const [seen, setSeen] = useState<{
    users: FilterOption[];
    models: FilterOption[];
    organizations: FilterOption[];
  }>({ users: [], models: [], organizations: [] });

  const users = useMemo(
    () => mergeOptions(userOptions, seen.users),
    [seen.users, userOptions],
  );
  const models = useMemo(
    () => mergeOptions(modelOptions, seen.models),
    [modelOptions, seen.models],
  );
  const organizations = useMemo(
    () => mergeOptions(organizationOptions, seen.organizations),
    [organizationOptions, seen.organizations],
  );

  const columns = useUsageColumns({
    scope,
    userOptions: users,
    organizationOptions: organizations,
    modelOptions: models,
  });
  const persistKey = AI_USAGE_PERSIST_KEYS[scope];
  const listState = useServerListState({
    columns,
    initialSort: AI_USAGE_DEFAULT_SORT,
    initialPageSize: 20,
    persistKey,
  });
  // The usage report has no `q` search.
  const params: AIUsageListParams = useMemo(() => {
    const { q: _q, ...rest } = listState.params;
    return rest;
  }, [listState.params]);
  const listQuery = useAIUsage(scope, params);
  const items = listQuery.data?.items;

  // Filter options stay available after the rows that brought them leave
  // the page (state adjusted while rendering, not in an effect).
  const [seenItems, setSeenItems] = useState<AIUsageRow[] | undefined>();
  if (items !== seenItems) {
    setSeenItems(items);
    if (items?.length) setSeen((prev) => rememberOptions(prev, items));
  }

  const exportQuery = useMemo(
    () => ({ ...listState.filterParams, sort: params.sort }),
    [listState.filterParams, params.sort],
  );

  return (
    <EntityTable
      columns={columns}
      data={items ?? []}
      getRowId={(row) => String(row.id)}
      isLoading={listQuery.isLoading}
      isError={listQuery.isError}
      onRetry={() => void listQuery.refetch()}
      emptyTitle={t("ai_admin.usage.empty_title")}
      emptyDescription={t("ai_admin.usage.empty_description")}
      rowCount={listQuery.data?.total ?? 0}
      state={listState.tableState}
      features={{
        persistKey,
        globalFilter: false,
        rowSelection: false,
        columnOrdering: true,
        columnPinning: true,
        viewMode: true,
        mobileAutoCards: true,
      }}
      toolbarExtra={
        <>
          <ExportMenu
            exportPath={AI_USAGE_EXPORT_PATHS[scope]}
            query={exportQuery}
            formats={["xlsx", "csv"]}
            jobsHref={
              scope === "tenant" && slug
                ? routes.tenant.exports.root(slug)
                : routes.platform.exports.root
            }
          />
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        </>
      }
    />
  );
}

type Seen = {
  users: FilterOption[];
  models: FilterOption[];
  organizations: FilterOption[];
};

function rememberOptions(prev: Seen, rows: AIUsageRow[]): Seen {
  const users = rows.flatMap((row) =>
    row.user ? [{ value: row.user.uuid, label: aiUserName(row.user) }] : [],
  );
  const models = rows.map((row) => ({ value: row.model, label: row.model }));
  const organizations = rows.map((row) => ({
    value: row.organization.uuid,
    label: row.organization.name,
  }));
  const next = {
    users: mergeOptions(prev.users, users),
    models: mergeOptions(prev.models, models),
    organizations: mergeOptions(prev.organizations, organizations),
  };
  const same =
    next.users.length === prev.users.length &&
    next.models.length === prev.models.length &&
    next.organizations.length === prev.organizations.length;
  return same ? prev : next;
}
