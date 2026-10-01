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
        enableColumnFilter: false,
        cell: ({ row }) => {
          const key = resourceLabelKey(row.original.resource);
          return key ? t(`exports.${key}`) : row.original.resource;
        },
      }),
      createColumn<ExportJob>({
        id: "format",
        accessorKey: "format",
        labelKey: "exports.columns.format",
        enableColumnFilter: false,
        cell: ({ row }) => formatExportFormat(t, row.original.format),
      }),
      createColumn<ExportJob>({
        id: "status",
        accessorKey: "status",
        labelKey: "exports.columns.status",
        enableColumnFilter: false,
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
        enableColumnFilter: false,
      }),
      createColumn<ExportJob>({
        id: "created_at",
        accessorKey: "created_at",
        labelKey: "exports.columns.created_at",
        enableColumnFilter: false,
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
