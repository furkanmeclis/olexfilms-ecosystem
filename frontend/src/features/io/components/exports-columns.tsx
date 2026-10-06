"use client";

import Link from "next/link";
import { Eye } from "lucide-react";
import { useMemo } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  formatExportFormat,
  formatExportStatus,
  resourceLabelKey,
} from "@/features/io/lib/display";
import {
  EXPORT_FORMAT_VALUES,
  EXPORT_STATUS_VALUES,
  ioResourceOptions,
} from "@/features/io/lib/filter-options";
import { exportsService } from "@/features/io/services/exports.service";
import type { ExportJob, ExportJobScope } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

function statusVariant(status: string) {
  if (status === "completed") return "default" as const;
  if (status === "failed") return "danger" as const;
  if (status === "processing") return "secondary" as const;
  return "outline" as const;
}

type ExportsColumnsOptions = {
  scope?: ExportJobScope;
  detailHref?: (uuid: string) => string;
};

export function useExportsColumns(options: ExportsColumnsOptions = {}) {
  const { t, format } = useLocale();
  const scope = options.scope ?? "platform";
  const customDetailHref = options.detailHref;
  const detailHref = useMemo(
    () =>
      customDetailHref ??
      ((uuid: string) => routes.platform.exports.detail(uuid)),
    [customDetailHref],
  );

  return useMemo<ColumnDef<ExportJob>[]>(
    () => [
      createColumn<ExportJob>({
        id: "resource",
        accessorKey: "resource",
        labelKey: "exports.columns.resource",
        enableSorting: true,
        filterVariant: "faceted",
        param: "resource",
        filterOptions: ioResourceOptions("exports", scope),
        gridPrimary: true,
        cell: ({ row }) => {
          const key = resourceLabelKey(row.original.resource);
          return key ? t(`exports.${key}`) : row.original.resource;
        },
      }),
      createColumn<ExportJob>({
        id: "format",
        accessorKey: "format",
        labelKey: "exports.columns.format",
        enableSorting: true,
        filterVariant: "faceted",
        param: "format",
        filterOptions: EXPORT_FORMAT_VALUES.map((value) => ({
          value,
          labelKey: `exports.formats.${value}`,
          label: value.toUpperCase(),
        })),
        cell: ({ row }) => formatExportFormat(t, row.original.format),
      }),
      createColumn<ExportJob>({
        id: "status",
        accessorKey: "status",
        labelKey: "exports.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        param: "status",
        filterOptions: EXPORT_STATUS_VALUES.map((value) => ({
          value,
          labelKey: `exports.status.${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <Badge variant={statusVariant(row.original.status)}>
            {formatExportStatus(t, row.original.status)}
          </Badge>
        ),
      }),
      createColumn<ExportJob>({
        id: "row_count",
        accessorKey: "row_count",
        labelKey: "exports.columns.rows",
        enableSorting: false,
      }),
      // TEC-211: who requested the job and which file it produces.
      createColumn<ExportJob>({
        id: "actor",
        labelKey: "exports.columns.actor",
        enableColumnFilter: false,
        enableSorting: false,
        cell: ({ row }) => row.original.actor?.name || "—",
      }),
      createColumn<ExportJob>({
        id: "filename",
        accessorKey: "filename",
        labelKey: "exports.columns.file",
        enableColumnFilter: false,
        enableSorting: false,
        cell: ({ row }) => (
          <span className="font-mono text-xs">
            {row.original.filename || "—"}
          </span>
        ),
      }),
      createColumn<ExportJob>({
        id: "created_at",
        accessorKey: "created_at",
        labelKey: "exports.columns.created_at",
        enableSorting: true,
        filterVariant: "date-range",
        param: "created",
        cell: ({ row }) => format.dateTime(row.original.created_at),
      }),
      createColumn<ExportJob>({
        id: "actions",
        labelKey: "exports.columns.actions",
        enableColumnFilter: false,
        enableHiding: false,
        enableSorting: false,
        cell: ({ row }) => {
          const job = row.original;
          return (
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="outline" asChild>
                <Link href={detailHref(job.uuid)}>
                  <Eye className="size-4" />
                  {t("exports.actions.view")}
                </Link>
              </Button>
              {job.status === "completed" ? (
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => {
                    void exportsService.download(job, scope).catch(() => {
                      appToast.error(t("exports.toast.download_failed"));
                    });
                  }}
                >
                  {t("exports.download")}
                </Button>
              ) : null}
            </div>
          );
        },
      }),
    ],
    [t, scope, detailHref, format],
  );
}
