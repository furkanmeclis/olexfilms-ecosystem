"use client";

import { Undo2 } from "lucide-react";
import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useImportJob } from "@/features/io/hooks/use-io-jobs-query";
import { ioKeys } from "@/features/io/hooks/query-keys";
import {
  formatImportFormat,
  formatImportStatus,
  resourceLabelKey,
  rollbackWindowOpen,
} from "@/features/io/lib/display";
import { apiMappingToUi } from "@/features/io/lib/suggest-mapping";
import { IMPORT_SCHEMA, type ExportJobScope } from "@/features/io/types";
import { importsService } from "@/features/io/services/imports.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type ImportDetailPageProps = {
  uuid: string;
  scope?: ExportJobScope;
  slug?: string;
};

function statusVariant(status: string) {
  if (status === "applied") return "default" as const;
  if (status === "failed") return "danger" as const;
  if (status === "applying" || status === "queued") return "secondary" as const;
  return "outline" as const;
}

export function ImportDetailPage({
  uuid,
  scope = "platform",
  slug,
}: ImportDetailPageProps) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const [nowMs] = useState(() => Date.now());
  const jobQuery = useImportJob(uuid, true, scope);
  const job = jobQuery.data;
  const tenant = scope === "tenant";
  const listHref =
    tenant && slug
      ? routes.tenant.imports.root(slug)
      : routes.platform.imports.root;
  const homeHref =
    tenant && slug ? routes.tenant.home(slug) : routes.platform.home;

  const rollback = useAppMutation({
    mutationFn: () => importsService.rollback(uuid, scope),
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: ioKeys.imports.detail(uuid, scope),
      });
      void queryClient.invalidateQueries({
        queryKey: ioKeys.imports.lists(scope),
      });
      appToast.success(t("imports.toast.rollback_success"));
    },
  });

  const resourceName = job
    ? (() => {
        const key = resourceLabelKey(job.resource);
        return key ? t(`imports.${key}`) : job.resource;
      })()
    : "";

  const title = job
    ? t("imports.detail.title_with_resource", {
        resource: resourceName,
        format: formatImportFormat(t, job.format),
      })
    : t("imports.detail.title");

  const canRollback =
    job?.status === "applied" && rollbackWindowOpen(job.rollback_until, nowMs);

  const uiMapping = apiMappingToUi(job?.mapping);
  const schema =
    IMPORT_SCHEMA[job?.resource as keyof typeof IMPORT_SCHEMA] ?? [];
  const preview = job?.preview_summary;

  return (
    <EntityPage
      title={title}
      description={t("imports.detail.description")}
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
        { label: t("imports.title"), href: listHref },
        { label: title },
      ]}
      actions={
        canRollback ? (
          <Button
            size="sm"
            variant="outline"
            disabled={rollback.isPending}
            onClick={() => rollback.mutate()}
          >
            <Undo2 className="size-4" />
            {t("imports.rollback")}
          </Button>
        ) : null
      }
    >
      {jobQuery.isLoading ? <Loading label={t("common.loading")} /> : null}
      {jobQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("imports.detail.not_found")}
          onRetry={() => void jobQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {job ? (
        <div className="space-y-6">
          <EntityHeader
            title={resourceName}
            description={formatImportFormat(t, job.format)}
            badges={
              <Badge variant={statusVariant(job.status)}>
                {formatImportStatus(t, job.status)}
              </Badge>
            }
          />

          <EntitySectionCard title={t("imports.detail.summary")}>
            <EntityDetail
              sections={[
                {
                  id: "summary",
                  fields: [
                    {
                      key: "uuid",
                      label: t("imports.detail.fields.uuid"),
                      value: (
                        <code className="text-xs break-all">{job.uuid}</code>
                      ),
                    },
                    {
                      key: "resource",
                      label: t("imports.columns.resource"),
                      value: resourceName,
                    },
                    {
                      key: "format",
                      label: t("imports.columns.format"),
                      value: formatImportFormat(t, job.format),
                    },
                    {
                      key: "status",
                      label: t("imports.columns.status"),
                      value: formatImportStatus(t, job.status),
                    },
                    {
                      key: "created_at",
                      label: t("imports.columns.created_at"),
                      value: format.dateTime(job.created_at),
                    },
                    {
                      key: "applied_at",
                      label: t("imports.detail.fields.applied_at"),
                      value: job.applied_at
                        ? format.dateTime(job.applied_at)
                        : "—",
                    },
                    {
                      key: "rollback_until",
                      label: t("imports.detail.fields.rollback_until"),
                      value: job.rollback_until
                        ? format.dateTime(job.rollback_until)
                        : "—",
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          {schema.length > 0 ? (
            <EntitySectionCard title={t("imports.detail.mapping_section")}>
              <dl className="divide-y rounded-md border">
                <div className="bg-muted/40 grid grid-cols-2 gap-2 px-3 py-2 text-xs font-medium">
                  <div>{t("imports.detail.mapping_target")}</div>
                  <div>{t("imports.detail.mapping_source")}</div>
                </div>
                {schema.map((field) => (
                  <div
                    key={field.key}
                    className="grid grid-cols-2 gap-2 px-3 py-2 text-sm"
                  >
                    <div>{t(field.labelKey)}</div>
                    <div>
                      {uiMapping[field.key] ?? (
                        <span className="text-muted-foreground">
                          {t("imports.mapping_skip")}
                        </span>
                      )}
                    </div>
                  </div>
                ))}
              </dl>
            </EntitySectionCard>
          ) : null}

          {job.defaults && Object.keys(job.defaults).length > 0 ? (
            <EntitySectionCard title={t("imports.detail.defaults_section")}>
              <EntityDetail
                sections={[
                  {
                    id: "defaults",
                    fields: Object.entries(job.defaults).map(([key, value]) => {
                      const field = schema.find((f) => f.key === key);
                      return {
                        key,
                        label: field ? t(field.labelKey) : key,
                        value,
                      };
                    }),
                  },
                ]}
              />
            </EntitySectionCard>
          ) : null}

          {preview ? (
            <EntitySectionCard title={t("imports.detail.preview_section")}>
              <div className="mb-4 grid grid-cols-3 gap-2 text-sm">
                <div className="rounded-md border p-2 text-center">
                  <div className="font-medium">{preview.total}</div>
                  <div className="text-muted-foreground text-xs">
                    {t("imports.preview_total")}
                  </div>
                </div>
                <div className="rounded-md border p-2 text-center">
                  <div className="font-medium text-green-600">
                    {preview.valid}
                  </div>
                  <div className="text-muted-foreground text-xs">
                    {t("imports.preview_valid")}
                  </div>
                </div>
                <div className="rounded-md border p-2 text-center">
                  <div className="text-destructive font-medium">
                    {preview.invalid}
                  </div>
                  <div className="text-muted-foreground text-xs">
                    {t("imports.preview_invalid")}
                  </div>
                </div>
              </div>
              {(preview.errors?.length ?? 0) > 0 ? (
                <ul className="text-destructive mb-4 max-h-40 list-disc overflow-y-auto ps-4 text-sm">
                  {preview.errors?.map((err) => (
                    <li key={`${err.index}-${err.error}`}>
                      {t("imports.preview_row_error", {
                        row: err.index,
                        error: err.error,
                      })}
                    </li>
                  ))}
                </ul>
              ) : null}
            </EntitySectionCard>
          ) : null}

          {job.error ? (
            <EntitySectionCard title={t("imports.detail.error_section")}>
              <p className="text-destructive text-sm whitespace-pre-wrap">
                {job.error}
              </p>
            </EntitySectionCard>
          ) : null}

          {canRollback ? (
            <EntitySectionCard title={t("imports.detail.rollback_section")}>
              <p className="text-muted-foreground mb-3 text-sm">
                {t("imports.rollback_hint")}
              </p>
              <Button
                variant="outline"
                disabled={rollback.isPending}
                onClick={() => rollback.mutate()}
              >
                <Undo2 className="size-4" />
                {t("imports.rollback")}
              </Button>
            </EntitySectionCard>
          ) : null}
        </div>
      ) : null}
    </EntityPage>
  );
}
