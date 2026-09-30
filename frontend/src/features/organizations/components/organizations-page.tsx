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
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
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

function firstString(value: string | string[] | undefined) {
  if (Array.isArray(value)) return value[0];
  return value;
}

export function OrganizationsPage() {
  const { t } = useLocale();
  const router = useRouter();

  const listState = useServerListState({
    initialSort: "-created_at",
    initialPageSize: 20,
  });

  const listParams = useMemo<ListOrganizationsParams>(() => {
    const columnValue = (id: string) =>
      firstString(
        listState.columnFilters.find((filter) => filter.id === id)?.value as
          string | string[] | undefined,
      );

    const q =
      listState.params.q ||
      columnValue("name")?.trim() ||
      columnValue("slug")?.trim() ||
      columnValue("city")?.trim() ||
      undefined;

    return {
      ...listState.params,
      q,
      status: columnValue("status"),
    };
  }, [listState.columnFilters, listState.params]);

  const listQuery = useOrganizationsList(listParams);
  const metaQuery = useOrganizationsMeta(true);

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

  const columns = useOrganizationsColumns({ handlers: rowHandlers });

  const pageCount = useMemo(() => {
    const total = listQuery.data?.total ?? 0;
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listQuery.data?.total, listState.pagination.pageSize]);

  const canCreate = metaQuery.data?.capabilities?.create !== false;

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
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey: "platform-organizations-v1",
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void listQuery.refetch()}
            refreshDisabled={listQuery.isFetching}
          />
        }
      />
    </EntityPage>
  );
}
