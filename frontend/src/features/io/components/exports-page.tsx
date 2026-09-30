"use client";

import { useQuery } from "@tanstack/react-query";
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
import { useExportsColumns } from "@/features/io/components/exports-columns";
import { ioKeys } from "@/features/io/hooks/query-keys";
import { exportsService } from "@/features/io/services/exports.service";
import type { ExportJobScope } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";
import { useMemo } from "react";

type ExportsPageProps = {
  scope?: ExportJobScope;
  slug?: string;
};

export function ExportsPage({ scope = "platform", slug }: ExportsPageProps) {
  const { t } = useLocale();
  const router = useRouter();
  const listState = useServerListState({ initialPageSize: 20 });
  const tenant = scope === "tenant";
  const columns = useExportsColumns({
    scope,
    detailHref: (uuid) =>
      tenant && slug
        ? routes.tenant.exports.detail(slug, uuid)
        : routes.platform.exports.detail(uuid),
  });

  const listQuery = useQuery({
    queryKey: ioKeys.exports.list(listState.params, scope),
    queryFn: () =>
      exportsService.list(
        {
          limit: listState.params.limit,
          offset: listState.params.offset,
        },
        scope,
      ),
  });

  const pageCount = useMemo(() => {
    const total = listQuery.data?.total ?? 0;
    const size = listState.pagination.pageSize || 20;
    return Math.max(1, Math.ceil(total / size));
  }, [listQuery.data?.total, listState.pagination.pageSize]);

  const homeHref =
    tenant && slug ? routes.tenant.home(slug) : routes.platform.home;
  const persistKey = tenant
    ? `tenant-exports-v1-${slug}`
    : "platform-exports-v1";

  return (
    <EntityPage
      title={t("exports.title")}
      description={
        tenant ? t("exports.tenant_description") : t("exports.description")
      }
      permission={
        tenant ? permissions.finance.export : permissions.exports.read
      }
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("exports.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: homeHref },
        { label: t("exports.title") },
      ]}
    >
      <EntityTable
        columns={columns}
        data={listQuery.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(job) =>
          router.push(
            tenant && slug
              ? routes.tenant.exports.detail(slug, job.uuid)
              : routes.platform.exports.detail(job.uuid),
          )
        }
        isLoading={listQuery.isLoading}
        isError={listQuery.isError}
        onRetry={() => void listQuery.refetch()}
        emptyTitle={t("exports.empty_title")}
        emptyDescription={
          tenant
            ? t("exports.tenant_empty_description")
            : t("exports.empty_description")
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
