"use client";

import { useQuery } from "@tanstack/react-query";
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
import { roleDisplayName } from "@/features/roles/lib/role-display";
import { rolesService } from "@/features/roles/services/roles.service";
import { ResourceIOToolbar } from "@/features/io";
import type { ResourceMeta } from "@/features/io/types";
import type {
  ListUsersParams,
  PublicUser,
} from "@/features/users/services/users.service";
import { useLocale } from "@/providers/locale-provider";
import { useAuth } from "@/providers/auth-provider";
import { usePermission } from "@/providers/permission-provider";

export const USERS_PERSIST_KEY = "platform-users-v1";

export function UsersPage() {
  const { t } = useLocale();
  const router = useRouter();
  const { user: currentUser } = useAuth();
  const { can } = usePermission();

  const [passwordTarget, setPasswordTarget] = useState<PublicUser | null>(null);

  const metaQuery = useUsersMeta(true);
  const meta = metaQuery.data as ResourceMeta | undefined;
  // Same query key as the role picker (role-multi-select) → shared cache.
  const rolesQuery = useQuery({
    queryKey: ["platform", "roles", "picker"],
    queryFn: () => rolesService.list({ limit: 100, offset: 0 }),
    enabled: can(permissions.roles.read),
  });
  const roleOptions = useMemo(
    () =>
      (rolesQuery.data?.items ?? []).map((role) => ({
        value: role.slug,
        label: roleDisplayName(role, t),
      })),
    [rolesQuery.data?.items, t],
  );

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
    roleOptions,
    currentUserUuid: currentUser?.uuid,
    canImpersonateSuperAdmin: Boolean(currentUser?.isSuperAdmin),
    isImpersonating: Boolean(currentUser?.impersonation),
  });
  const columns = useMemo(
    () => [createSelectColumnDef<PublicUser>(), ...baseColumns],
    [baseColumns],
  );

  // Column meta drives the params: status (faceted → CSV), role (slug).
  const listState = useServerListState({
    columns,
    initialSort: meta?.default_sort ?? "-created_at",
    initialPageSize: 20,
    persistKey: USERS_PERSIST_KEY,
  });
  const listParams: ListUsersParams = listState.params;

  const listQuery = useUsersList(listParams);
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
      resolveBulkActionsWithIcons("platform.users", meta?.bulk_actions ?? []),
    [meta?.bulk_actions],
  );

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
        rowCount={listQuery.data?.total ?? 0}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: USERS_PERSIST_KEY,
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
