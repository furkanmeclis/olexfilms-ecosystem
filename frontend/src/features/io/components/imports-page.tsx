"use client";

import { useCallback } from "react";
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
import type { ListJobsParams } from "@/features/io/services/exports.service";
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
  const tenant = scope === "tenant";

  const rollback = useAppMutation({
    mutationFn: (uuid: string) => importsService.rollback(uuid, scope),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ioKeys.imports.lists(scope),
      });
      appToast.success(t("imports.toast.rollback_success"));
    },
  });

  const { mutate: rollbackMutate } = rollback;
  const onRollback = useCallback(
    (uuid: string) => rollbackMutate(uuid),
    [rollbackMutate],
  );
  const detailHref = useCallback(
    (uuid: string) =>
      tenant && slug
        ? routes.tenant.imports.detail(slug, uuid)
        : routes.platform.imports.detail(uuid),
    [slug, tenant],
  );
  const columns = useImportsColumns({
    scope,
    onRollback,
    rollbackPending: rollback.isPending,
    detailHref,
  });

  const homeHref =
    tenant && slug ? routes.tenant.home(slug) : routes.platform.home;
  const persistKey = tenant
    ? `tenant-imports-v1-${slug}`
    : "platform-imports-v1";

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
    queryKey: ioKeys.imports.list(listParams, scope),
    queryFn: () => importsService.list(listParams, scope),
    placeholderData: (previous) => previous,
  });

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
