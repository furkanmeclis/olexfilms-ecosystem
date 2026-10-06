"use client";

import { useQuery } from "@tanstack/react-query";
import { useRouter } from "next/navigation";
import { useCallback } from "react";

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
import {
  exportsService,
  type ListJobsParams,
} from "@/features/io/services/exports.service";
import type { ExportJobScope } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";

type ExportsPageProps = {
  scope?: ExportJobScope;
  slug?: string;
};

export function ExportsPage({ scope = "platform", slug }: ExportsPageProps) {
  const { t } = useLocale();
  const router = useRouter();
  const tenant = scope === "tenant";
  const persistKey = tenant
    ? `tenant-exports-v1-${slug}`
    : "platform-exports-v1";
  const detailHref = useCallback(
    (uuid: string) =>
      tenant && slug
        ? routes.tenant.exports.detail(slug, uuid)
        : routes.platform.exports.detail(uuid),
    [slug, tenant],
  );
  const columns = useExportsColumns({ scope, detailHref });

  // Column meta drives the params: status/resource/format (CSV) and
  // created_from/_to; sort on created_at/status/resource/format.
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 20,
    persistKey,
  });
  const listParams: ListJobsParams = listState.params;

  const listQuery = useQuery({
    queryKey: ioKeys.exports.list(listParams, scope),
    queryFn: () => exportsService.list(listParams, scope),
    placeholderData: (previous) => previous,
  });

  const homeHref =
    tenant && slug ? routes.tenant.home(slug) : routes.platform.home;
  return (
    <EntityPage
      title={t("exports.title")}
      description={
        tenant ? t("exports.tenant_description") : t("exports.description")
      }
      permission={
        tenant ? permissions.exports.tenantRead : permissions.exports.read
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
        rowCount={listQuery.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey,
          rowSelection: false,
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
