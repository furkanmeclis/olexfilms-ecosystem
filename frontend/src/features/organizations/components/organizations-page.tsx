"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityCreateButton,
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createSelectColumnDef } from "@/components/tables";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  resolveBulkActionsWithIcons,
  useBulkSelection,
} from "@/features/bulk-engine";
import { ResourceIOToolbar } from "@/features/io";
import type { ResourceMeta } from "@/features/io/types";
import { useOrganizationsColumns } from "@/features/organizations/components/organizations-columns";
import type { OrganizationRowActionHandlers } from "@/features/organizations/components/organization-row-actions";
import {
  useOrganizationsList,
  useOrganizationsMeta,
} from "@/features/organizations/hooks/use-organizations-query";
import type {
  ListOrganizationsParams,
  Organization,
} from "@/features/organizations/services/organizations.service";
import { useLocale } from "@/providers/locale-provider";

export const ORGANIZATIONS_PERSIST_KEY = "platform-organizations-v1";

export function OrganizationsPage() {
  const { t } = useLocale();
  const router = useRouter();

  const metaQuery = useOrganizationsMeta(true);
  const meta = metaQuery.data as ResourceMeta | undefined;

  const openDetail = useCallback(
    (organization: Organization) => {
      router.push(routes.platform.organizations.detail(organization.uuid));
    },
    [router],
  );

  const openEdit = useCallback(
    (organization: Organization) => {
      router.push(routes.platform.organizations.edit(organization.uuid));
    },
    [router],
  );

  const rowHandlers = useMemo<OrganizationRowActionHandlers>(
    () => ({
      onView: openDetail,
      onEdit: openEdit,
    }),
    [openDetail, openEdit],
  );

  const baseColumns = useOrganizationsColumns({ handlers: rowHandlers });
  const columns = useMemo(
    () => [createSelectColumnDef<Organization>(), ...baseColumns],
    [baseColumns],
  );

  // Column meta drives the params: status/type (CSV), plan_code,
  // access_ends_from/_to and created_from/_to (TEC-365).
  const listState = useServerListState({
    columns,
    initialSort: meta?.default_sort ?? "-created_at",
    initialPageSize: 20,
    persistKey: ORGANIZATIONS_PERSIST_KEY,
  });
  const listParams: ListOrganizationsParams = listState.params;

  const listQuery = useOrganizationsList(listParams);
  const total = listQuery.data?.total ?? 0;

  // Same filters for "all matching" bulk runs and exports.
  const bulkQuery = useMemo(
    () => ({
      ...listState.filterParams,
      q: listParams.q,
      sort: listParams.sort,
    }),
    [listState.filterParams, listParams.q, listParams.sort],
  );

  const bulkSelection = useBulkSelection({
    listQueryKey: listParams,
    bulkQuery,
    total,
  });

  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons(
        "platform.organizations",
        meta?.bulk_actions ?? [],
      ),
    [meta?.bulk_actions],
  );

  const canCreate = meta?.capabilities?.create !== false;

  return (
    <EntityPage
      title={t("organizations.title")}
      description={t("organizations.description")}
      permission={permissions.organizations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("organizations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("organizations.title") },
      ]}
      actions={
        canCreate ? (
          <EntityCreateButton
            onClick={() => router.push(routes.platform.organizations.create)}
            label={t("organizations.actions.create")}
            permission={permissions.organizations.write}
          />
        ) : null
      }
    >
      <SelectionBanner
        selectedCount={bulkSelection.selectedCount}
        total={total}
        showSelectAll={bulkSelection.showSelectAllBanner}
        allMatchingSelected={bulkSelection.scope.mode === "all"}
        onSelectAllMatching={bulkSelection.selectAllMatching}
        onClearSelection={bulkSelection.clearSelection}
      />
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={openDetail}
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("organizations.empty_title")}
        emptyDescription={t("organizations.empty_description")}
        rowCount={total}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: ORGANIZATIONS_PERSIST_KEY,
          rowSelection: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource="platform.organizations"
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => void listQuery.refetch()}
            />
            <ResourceIOToolbar
              resource="platform.organizations"
              query={bulkQuery}
              capabilities={meta?.capabilities}
            />
            <EntityToolbar
              onRefresh={() => void listQuery.refetch()}
              refreshDisabled={listQuery.isFetching}
            />
          </>
        }
      />
    </EntityPage>
  );
}
