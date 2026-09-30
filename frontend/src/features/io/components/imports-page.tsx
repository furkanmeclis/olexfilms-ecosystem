"use client";

import { useMemo } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import {
  EntityPage,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useImportsColumns } from "@/features/io/components/imports-columns";
import { ioKeys } from "@/features/io/hooks/query-keys";
import { importsService } from "@/features/io/services/imports.service";
import type { ExportJobScope } from "@/features/io/types";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type ImportsPageProps = {
  scope?: ExportJobScope;
  slug?: string;
};

export function ImportsPage({ scope = "platform", slug }: ImportsPageProps) {
  const { t } = useLocale();
  const router = useRouter();
  const queryClient = useQueryClient();
  const listState = useServerListState({ initialPageSize: 20 });
  const tenant = scope === "tenant";

  const listQuery = useQuery({
    queryKey: ioKeys.imports.list(listState.params, scope),
    queryFn: () =>
      importsService.list(
        {
          limit: listState.params.limit,
          offset: listState.params.offset,
        },
        scope,
      ),
  });

  const rollback = useAppMutation({
    mutationFn: (uuid: string) => importsService.rollback(uuid, scope),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ioKeys.imports.lists(scope),
      });
      appToast.success(t("imports.toast.rollback_success"));
    },
  });

  const columns = useImportsColumns({
    onRollback: (uuid) => rollback.mutate(uuid),
    rollbackPending: rollback.isPending,
    detailHref: (uuid) =>
      tenant && slug
        ? routes.tenant.imports.detail(slug, uuid)
        : routes.platform.imports.detail(uuid),
  });

  const pageCount = useMemo(() => {
    const total = listQuery.data?.total ?? 0;
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listQuery.data?.total, listState.pagination.pageSize]);

  const homeHref =
    tenant && slug ? routes.tenant.home(slug) : routes.platform.home;
  const persistKey = tenant
    ? `tenant-imports-v1-${slug}`
    : "platform-imports-v1";

  return (
    <EntityPage
      title={t("imports.title")}
      description={
        tenant ? t("imports.tenant_description") : t("imports.description")
      }
      permission={
        tenant ? permissions.imports.tenantRead : permissions.imports.read
      }
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("imports.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: homeHref },
        { label: t("imports.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(job) =>
          router.push(
            tenant && slug
              ? routes.tenant.imports.detail(slug, job.uuid)
              : routes.platform.imports.detail(job.uuid),
          )
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("imports.empty_title")}
        emptyDescription={
          tenant
            ? t("imports.tenant_empty_description")
            : t("imports.empty_description")
        }
        pageCount={pageCount}
        state={listState.tableState}
        features={{
          persistKey,
          rowSelection: false,
          columnFilters: false,
          globalFilter: false,
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
