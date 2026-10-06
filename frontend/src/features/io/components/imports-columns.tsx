"use client";

import Link from "next/link";
import { Eye } from "lucide-react";
import { useMemo, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  formatImportFormat,
  formatImportStatus,
  resourceLabelKey,
  rollbackWindowOpen,
} from "@/features/io/lib/display";
import {
  IMPORT_FORMAT_VALUES,
  IMPORT_STATUS_VALUES,
  ioResourceOptions,
} from "@/features/io/lib/filter-options";
import type { ExportJobScope, ImportJob } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";

function statusVariant(status: string) {
  if (status === "applied") return "default" as const;
  if (status === "failed") return "danger" as const;
  if (status === "applying" || status === "queued") return "secondary" as const;
  return "outline" as const;
}

type UseImportsColumnsOptions = {
  onRollback: (uuid: string) => void;
  rollbackPending: boolean;
  detailHref?: (uuid: string) => string;
  scope?: ExportJobScope;
};

export function useImportsColumns({
  onRollback,
  rollbackPending,
  detailHref,
  scope = "platform",
}: UseImportsColumnsOptions) {
  const { t, format } = useLocale();
  const [nowMs] = useState(() => Date.now());

  return useMemo<ColumnDef<ImportJob>[]>(
    () => [
      createColumn<ImportJob>({
        id: "resource",
        accessorKey: "resource",
        labelKey: "imports.columns.resource",
        enableSorting: true,
        filterVariant: "faceted",
        param: "resource",
        filterOptions: ioResourceOptions("imports", scope),
        gridPrimary: true,
        cell: ({ row }) => {
          const key = resourceLabelKey(row.original.resource);
          return key ? t(`imports.${key}`) : row.original.resource;
        },
      }),
      createColumn<ImportJob>({
        id: "format",
        accessorKey: "format",
        labelKey: "imports.columns.format",
        enableSorting: true,
        filterVariant: "faceted",
        param: "format",
        filterOptions: IMPORT_FORMAT_VALUES.map((value) => ({
          value,
          labelKey: `imports.formats.${value}`,
          label: value.toUpperCase(),
        })),
        cell: ({ row }) => formatImportFormat(t, row.original.format),
      }),
      createColumn<ImportJob>({
        id: "status",
        accessorKey: "status",
        labelKey: "imports.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        param: "status",
        filterOptions: IMPORT_STATUS_VALUES.map((value) => ({
          value,
          labelKey: `imports.status.${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <Badge variant={statusVariant(row.original.status)}>
            {formatImportStatus(t, row.original.status)}
          </Badge>
        ),
      }),
      // TEC-211: who uploaded the job and the uploaded file name.
      createColumn<ImportJob>({
        id: "actor",
        labelKey: "imports.columns.actor",
        enableColumnFilter: false,
        enableSorting: false,
        cell: ({ row }) => row.original.actor?.name || "—",
      }),
      createColumn<ImportJob>({
        id: "source_filename",
        accessorKey: "source_filename",
        labelKey: "imports.columns.file",
        enableColumnFilter: false,
        enableSorting: false,
        cell: ({ row }) => (
          <span className="font-mono text-xs">
            {row.original.source_filename || "—"}
          </span>
        ),
      }),
      createColumn<ImportJob>({
        id: "created_at",
        accessorKey: "created_at",
        labelKey: "imports.columns.created_at",
        enableSorting: true,
        filterVariant: "date-range",
        param: "created",
        cell: ({ row }) => format.dateTime(row.original.created_at),
      }),
      createColumn<ImportJob>({
        id: "actions",
        labelKey: "imports.columns.actions",
        enableColumnFilter: false,
        enableHiding: false,
        enableSorting: false,
        cell: ({ row }) => {
          const job = row.original;
          const canRollback =
            job.status === "applied" &&
            rollbackWindowOpen(job.rollback_until, nowMs);
          return (
            <div className="flex flex-wrap gap-2">
              <Button size="sm" variant="outline" asChild>
                <Link
                  href={
                    detailHref
                      ? detailHref(job.uuid)
                      : routes.platform.imports.detail(job.uuid)
                  }
                >
                  <Eye className="size-4" />
                  {t("imports.actions.view")}
                </Link>
              </Button>
              {canRollback ? (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={rollbackPending}
                  onClick={() => onRollback(job.uuid)}
                >
                  {t("imports.rollback")}
                </Button>
              ) : null}
            </div>
          );
        },
      }),
    ],
    [nowMs, onRollback, rollbackPending, detailHref, scope, t, format],
  );
}
