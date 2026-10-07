"use client";

import { useMemo, useState } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { OrgQuotaDialog } from "@/features/ai-admin/components/org-quota-dialog";
import {
  useOrgQuotasColumns,
  type OrgQuotaRowHandlers,
} from "@/features/ai-admin/components/org-quotas-columns";
import { PeriodSelect } from "@/features/ai-admin/components/period-select";
import {
  useAIOrgQuotas,
  useUpdateAIOrgQuota,
} from "@/features/ai-admin/hooks/use-ai-admin";
import { currentPeriod } from "@/features/ai-admin/lib/quota";
import type {
  AIOrgQuota,
  AIOrgQuotaListParams,
} from "@/features/ai-admin/services/ai-admin.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const AI_ORG_QUOTAS_PERSIST_KEY = "platform-ai-org-quotas-v1";

/** Backend default sort of GET /v1/platform/ai/orgs (no /meta endpoint). */
export const AI_ORG_QUOTAS_DEFAULT_SORT = "-usage";

/** Organization quota table (TEC-391), platform AI screen. */
export function OrgQuotasTable({ defaultQuota }: { defaultQuota?: number }) {
  const { t } = useLocale();
  const [period, setPeriod] = useState(() => currentPeriod());
  const [editing, setEditing] = useState<AIOrgQuota | null>(null);
  const update = useUpdateAIOrgQuota();

  const handlers = useMemo<OrgQuotaRowHandlers>(
    () => ({ onEditQuota: setEditing }),
    [],
  );
  const columns = useOrgQuotasColumns({ handlers });

  const listState = useServerListState({
    columns,
    initialSort: AI_ORG_QUOTAS_DEFAULT_SORT,
    initialPageSize: 20,
    persistKey: AI_ORG_QUOTAS_PERSIST_KEY,
  });
  const params: AIOrgQuotaListParams = useMemo(
    () => ({ ...listState.params, period }),
    [listState.params, period],
  );
  const listQuery = useAIOrgQuotas(params);

  return (
    <>
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.organization.uuid}
        onRowClick={setEditing}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("ai_admin.orgs.empty_title")}
        emptyDescription={t("ai_admin.orgs.empty_description")}
        rowCount={listQuery.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: AI_ORG_QUOTAS_PERSIST_KEY,
          rowSelection: false,
          columnOrdering: true,
          columnPinning: true,
          viewMode: true,
          mobileAutoCards: true,
        }}
        toolbarExtra={
          <>
            <PeriodSelect value={period} onChange={setPeriod} />
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
      <OrgQuotaDialog
        row={editing}
        defaultQuota={defaultQuota}
        isSaving={update.isPending}
        onOpenChange={(open) => {
          if (!open) setEditing(null);
        }}
        onSubmit={(uuid, body) =>
          update.mutate(
            { uuid, body },
            {
              onSuccess: () => {
                appToast.success(t("ai_admin.toast.quota_saved"));
                setEditing(null);
              },
              onError: (error) =>
                appToast.error(
                  isApiError(error)
                    ? error.message
                    : t("ai_admin.toast.save_failed"),
                ),
            },
          )
        }
      />
    </>
  );
}
