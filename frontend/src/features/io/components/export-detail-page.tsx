"use client";

import { Download } from "lucide-react";

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
import {
  ExportJsonPreview,
  isJsonExportFormat,
} from "@/features/io/components/export-json-preview";
import {
  ExportPdfPreview,
  isPdfExportFormat,
} from "@/features/io/components/export-pdf-preview";
import { ExportSpreadsheetPreview } from "@/features/io/components/export-spreadsheet-preview";
import { useExportJob } from "@/features/io/hooks/use-io-jobs-query";
import {
  exportDownloadFilename,
  formatExportFormat,
  formatExportStatus,
  resourceLabelKey,
} from "@/features/io/lib/display";
import { isSpreadsheetExportFormat } from "@/features/io/lib/parse-export-spreadsheet";
import { exportsService } from "@/features/io/services/exports.service";
import type { ExportJobScope } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type ExportDetailPageProps = {
  uuid: string;
  scope?: ExportJobScope;
  slug?: string;
};

function statusVariant(status: string) {
  if (status === "completed") return "default" as const;
  if (status === "failed") return "danger" as const;
  if (status === "processing") return "secondary" as const;
  return "outline" as const;
}

export function ExportDetailPage({
  uuid,
  scope = "platform",
  slug,
}: ExportDetailPageProps) {
  const { t, format } = useLocale();
  const jobQuery = useExportJob(uuid, true, scope);
  const job = jobQuery.data;
  const tenant = scope === "tenant";
  const homeHref =
    tenant && slug ? routes.tenant.home(slug) : routes.platform.home;
  const listHref =
    tenant && slug
      ? routes.tenant.exports.root(slug)
      : routes.platform.exports.root;

  const resourceName = job
    ? (() => {
        const key = resourceLabelKey(job.resource);
        return key ? t(`exports.${key}`) : job.resource;
      })()
    : "";

  const title = job
    ? t("exports.detail.title_with_resource", {
        resource: resourceName,
        format: formatExportFormat(t, job.format),
      })
    : t("exports.detail.title");

  return (
    <EntityPage
      title={title}
      description={t("exports.detail.description")}
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
        { label: t("exports.title"), href: listHref },
        { label: title },
      ]}
      actions={
        job?.status === "completed" ? (
          <Button
            size="sm"
            onClick={() => {
              void exportsService.download(job, scope).catch(() => {
                appToast.error(t("exports.toast.download_failed"));
              });
            }}
          >
            <Download className="size-4" />
            {t("exports.download")}
          </Button>
        ) : null
      }
    >
      {jobQuery.isLoading ? <Loading label={t("common.loading")} /> : null}
      {jobQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("exports.detail.not_found")}
          onRetry={() => void jobQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {job ? (
        <div className="space-y-6">
          <EntityHeader
            title={resourceName}
            description={formatExportFormat(t, job.format)}
            badges={
              <Badge variant={statusVariant(job.status)}>
                {formatExportStatus(t, job.status)}
              </Badge>
            }
          />

          <EntitySectionCard title={t("exports.detail.summary")}>
            <EntityDetail
              sections={[
                {
                  id: "summary",
                  fields: [
                    {
                      key: "uuid",
                      label: t("exports.detail.fields.uuid"),
                      value: (
                        <code className="text-xs break-all">{job.uuid}</code>
                      ),
                    },
                    {
                      key: "resource",
                      label: t("exports.columns.resource"),
                      value: resourceName,
                    },
                    {
                      key: "format",
                      label: t("exports.columns.format"),
                      value: formatExportFormat(t, job.format),
                    },
                    {
                      key: "status",
                      label: t("exports.columns.status"),
                      value: formatExportStatus(t, job.status),
                    },
                    {
                      key: "row_count",
                      label: t("exports.columns.rows"),
                      value: job.row_count,
                    },
                    {
                      key: "created_at",
                      label: t("exports.columns.created_at"),
                      value: format.dateTime(job.created_at),
                    },
                    {
                      key: "filename",
                      label: t("exports.detail.fields.filename"),
                      value: exportDownloadFilename(
                        job.resource,
                        job.format,
                        new Date(job.created_at),
                      ),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          {job.error ? (
            <EntitySectionCard title={t("exports.detail.error_section")}>
              <p className="text-destructive text-sm whitespace-pre-wrap">
                {job.error}
              </p>
            </EntitySectionCard>
          ) : null}

          {job.status === "completed" ? (
            <EntitySectionCard title={t("exports.detail.preview_section")}>
              {isSpreadsheetExportFormat(job.format) ? (
                <>
                  <p className="text-muted-foreground mb-4 text-sm">
                    {t("exports.detail.preview_hint")}
                  </p>
                  <ExportSpreadsheetPreview
                    uuid={job.uuid}
                    format={job.format}
                    scope={scope}
                  />
                </>
              ) : isJsonExportFormat(job.format) ? (
                <>
                  <p className="text-muted-foreground mb-4 text-sm">
                    {t("exports.detail.preview_json_hint")}
                  </p>
                  <ExportJsonPreview uuid={job.uuid} scope={scope} />
                </>
              ) : isPdfExportFormat(job.format) ? (
                <>
                  <p className="text-muted-foreground mb-4 text-sm">
                    {t("exports.detail.preview_pdf_hint")}
                  </p>
                  <ExportPdfPreview uuid={job.uuid} scope={scope} />
                </>
              ) : null}
            </EntitySectionCard>
          ) : null}

          {job.status === "completed" ? (
            <EntitySectionCard title={t("exports.detail.download_section")}>
              <p className="text-muted-foreground mb-3 text-sm">
                {t("exports.detail.download_hint")}
              </p>
              <Button
                variant="outline"
                onClick={() => {
                  void exportsService.download(job, scope).catch(() => {
                    appToast.error(t("exports.toast.download_failed"));
                  });
                }}
              >
                <Download className="size-4" />
                {t("exports.download")}
              </Button>
            </EntitySectionCard>
          ) : null}
        </div>
      ) : null}
    </EntityPage>
  );
}
