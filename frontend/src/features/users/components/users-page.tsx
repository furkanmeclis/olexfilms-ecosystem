"use client";

import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

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
import { useUsersColumns } from "@/features/users/components/users-columns";
import { UserSetPasswordDialog } from "@/features/users/components/user-set-password-dialog";
import type { UserRowActionHandlers } from "@/features/users/components/user-row-actions";
import {
  useDisableUser,
  useEnableUser,
  useImpersonateUser,
} from "@/features/users/hooks/use-user-mutations";
import {
  useUsersList,
  useUsersMeta,
} from "@/features/users/hooks/use-users-query";
import { userFullName } from "@/features/users/lib/user-display";
import { ResourceIOToolbar } from "@/features/io";
import type { ResourceMeta } from "@/features/io/types";
import type {
  ListUsersParams,
  PublicUser,
} from "@/features/users/services/users.service";
import { useLocale } from "@/providers/locale-provider";
import { useAuth } from "@/providers/auth-provider";

function firstString(value: string | string[] | undefined) {
  if (Array.isArray(value)) return value[0];
  return value;
}

export function UsersPage() {
  const { t } = useLocale();
  const router = useRouter();
  const { user: currentUser } = useAuth();

  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });

  const [passwordTarget, setPasswordTarget] = useState<PublicUser | null>(null);

  const listParams = useMemo<ListUsersParams>(() => {
    const columnValue = (id: string) =>
      firstString(
        listState.columnFilters.find((filter) => filter.id === id)?.value as
          string | string[] | undefined,
      );

    const q =
      listState.params.q ||
      columnValue("email")?.trim() ||
      columnValue("name")?.trim() ||
      undefined;

    return {
      ...listState.params,
      q,
      status: columnValue("status"),
      role: columnValue("role")?.trim() || undefined,
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useUsersList(listParams);
  const metaQuery = useUsersMeta(true);
  const enableUser = useEnableUser();
  const disableUser = useDisableUser();
  const impersonateUser = useImpersonateUser();

  const openDetail = useCallback(
    (user: PublicUser) => {
      router.push(routes.platform.users.detail(user.uuid));
    },
    [router],
  );

  const openEdit = useCallback(
    (user: PublicUser) => {
      router.push(routes.platform.users.edit(user.uuid));
    },
    [router],
  );

  const rowHandlers = useMemo<UserRowActionHandlers>(
    () => ({
      onView: openDetail,
      onEdit: openEdit,
      onEnable: (user) => enableUser.mutate(user.uuid),
      onDisable: (user) => disableUser.mutate(user.uuid),
      onSetPassword: (user) => setPasswordTarget(user),
      onImpersonate: (user) => impersonateUser.mutate(user),
    }),
    [disableUser, enableUser, impersonateUser, openDetail, openEdit],
  );

  const baseColumns = useUsersColumns({
    handlers: rowHandlers,
    currentUserUuid: currentUser?.uuid,
    canImpersonateSuperAdmin: Boolean(currentUser?.isSuperAdmin),
    isImpersonating: Boolean(currentUser?.impersonation),
  });
  const columns = useMemo(
    () => [createSelectColumnDef<PublicUser>(), ...baseColumns],
    [baseColumns],
  );

  const bulkQuery = useMemo(
    () => ({
      q: listParams.q,
      status: listParams.status,
      role: listParams.role,
      sort: listParams.sort,
    }),
    [listParams.q, listParams.status, listParams.role, listParams.sort],
  );

  const bulkSelection = useBulkSelection({
    listQueryKey: listParams,
    bulkQuery,
    total: listQuery.data?.total ?? 0,
  });

  const meta = metaQuery.data as ResourceMeta | undefined;

  const bulkActions = useMemo(
    () =>
      resolveBulkActionsWithIcons("platform.users", meta?.bulk_actions ?? []),
    [meta?.bulk_actions],
  );

  const pageCount = useMemo(() => {
    const total = listQuery.data?.total ?? 0;
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listQuery.data?.total, listState.pagination.pageSize]);

  return (
    <EntityPage
      title={t("users.title")}
      description={t("users.description")}
      permission={permissions.users.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("users.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("users.title") },
      ]}
      actions={
        <EntityCreateButton
          onClick={() => router.push(routes.platform.users.create)}
          label={t("users.actions.create")}
          permission={permissions.users.write}
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
        emptyTitle={t("users.empty_title")}
        emptyDescription={t("users.empty_description")}
        pageCount={pageCount}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: "platform-users-v1",
          rowSelection: true,
        }}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource="platform.users"
              actions={bulkActions}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => void listQuery.refetch()}
            />
            <ResourceIOToolbar
              resource="platform.users"
              query={{
                q: listParams.q,
                status: listParams.status,
                role: listParams.role,
                sort: listParams.sort,
              }}
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

      <UserSetPasswordDialog
        open={Boolean(passwordTarget)}
        userUuid={passwordTarget?.uuid ?? null}
        userLabel={passwordTarget ? userFullName(passwordTarget) : undefined}
        onOpenChange={(open) => {
          if (!open) setPasswordTarget(null);
        }}
      />
    </EntityPage>
  );
}
