"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";

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
import { useRolesColumns } from "@/features/roles/components/roles-columns";
import type { RoleRowActionHandlers } from "@/features/roles/components/role-row-actions";
import { ResourceIOToolbar } from "@/features/io";
import type { ResourceMeta } from "@/features/io/types";
import { rolesKeys } from "@/features/roles/hooks/query-keys";
import { useDeleteRole } from "@/features/roles/hooks/use-role-mutations";
import {
  rolesService,
  type ListRolesParams,
  type RoleSummary,
} from "@/features/roles/services/roles.service";
import { useLocale } from "@/providers/locale-provider";
import { useDialogs } from "@/providers/dialog-provider";

export const ROLES_PERSIST_KEY = "platform-roles-v1";

export function RolesPage() {
  const { t } = useLocale();
  const router = useRouter();
  const { confirmDelete } = useDialogs();
  const deleteRole = useDeleteRole();

  const metaQuery = useQuery({
    queryKey: rolesKeys.meta(),
    queryFn: () => rolesService.meta(),
    staleTime: 5 * 60_000,
  });
  const meta = metaQuery.data as ResourceMeta | undefined;

  const openDetail = useCallback(
    (role: RoleSummary) => {
      router.push(routes.platform.roles.detail(role.uuid));
    },
    [router],
  );

  const openEdit = useCallback(
    (role: RoleSummary) => {
      router.push(routes.platform.roles.edit(role.uuid));
    },
    [router],
  );

  const handleDelete = useCallback(
    async (role: RoleSummary) => {
      const confirmed = await confirmDelete({
        title: t("roles.delete_title"),
        description: t("roles.delete_description", { name: role.name }),
      });
      if (!confirmed) return;
      await deleteRole.mutateAsync(role.uuid);
    },
    [confirmDelete, deleteRole, t],
  );

  const rowHandlers = useMemo<RoleRowActionHandlers>(
    () => ({
      onView: openDetail,
      onEdit: openEdit,
      onDelete: (role) => void handleDelete(role),
    }),
    [handleDelete, openDetail, openEdit],
  );

  const baseColumns = useRolesColumns(rowHandlers);
  const columns = useMemo(
    () => [createSelectColumnDef<RoleSummary>(), ...baseColumns],
    [baseColumns],
  );

  // Column meta drives the params: `is_system` (select → true|false).
  const listState = useServerListState({
    columns,
    initialSort: meta?.default_sort ?? "name",
    initialPageSize: 20,
    persistKey: ROLES_PERSIST_KEY,
  });
  const listParams: ListRolesParams = listState.params;

  const listQuery = useQuery({
    queryKey: rolesKeys.list(listParams),
    queryFn: () => rolesService.list(listParams),
    placeholderData: (previous) => previous,
  });

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
    total: listQuery.data?.total ?? 0,
  });

  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons("platform.roles", meta?.bulk_actions ?? []),
    [meta?.bulk_actions],
  );

  return (
    <EntityPage
      title={t("roles.title")}
      description={t("roles.subtitle")}
      permission={permissions.roles.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("roles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("roles.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.platform.roles.create)}
          label={t("roles.actions.create")}
          permission={permissions.roles.write}
        />
      }
    >
      <SelectionBanner
        selectedCount={bulkSelection.selectedCount}
        total={listQuery.data?.total ?? 0}
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
        emptyTitle={t("roles.empty_title")}
        emptyDescription={t("roles.empty_description")}
        rowCount={listQuery.data?.total ?? 0}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: ROLES_PERSIST_KEY,
          rowSelection: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource="platform.roles"
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => void listQuery.refetch()}
            />
            <ResourceIOToolbar
              resource="platform.roles"
              query={bulkQuery}
              capabilities={meta?.capabilities}
              onImportComplete={() => void listQuery.refetch()}
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
