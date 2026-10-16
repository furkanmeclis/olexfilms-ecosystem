"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { useMemo } from "react";

import { DataTable } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { useImportStatusLabel } from "@/features/io/components/import-status-cards";
import {
  importErrorMessage,
  importStatusTone,
  importTargetParts,
  type ImportTableRow,
} from "@/features/io/lib/import-summary";
import type { ImportSchemaField } from "@/features/io/types";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

/** Mapped fields shown as columns (the rest stays in the row data). */
const MAX_FIELD_COLUMNS = 4;

const TONE_BADGE = {
  success: "success",
  warning: "warning",
  danger: "danger",
  default: "secondary",
} as const;

function cellText(value: unknown): string {
  if (value == null) return "";
  if (typeof value === "object") return JSON.stringify(value);
  return String(value);
}

type ImportRowsTableProps = {
  rows: ImportTableRow[];
  schema: ImportSchemaField[];
  emptyTitle?: string;
};

/**
 * Preview / report rows of an import: row number, the main mapped fields,
 * the target, the status pill and the error text. A cell with an error of
 * its field is highlighted. Client-side paging (the summary holds up to
 * 1000 rows).
 */
export function ImportRowsTable({
  rows,
  schema,
  emptyTitle,
}: ImportRowsTableProps) {
  const { t } = useLocale();
  const statusLabel = useImportStatusLabel();

  const fields = useMemo(() => {
    const present = new Set<string>();
    for (const row of rows) {
      for (const key of Object.keys(row.data)) present.add(key);
    }
    const fromSchema = schema
      .filter((f) => present.has(f.key))
      .map((f) => ({ key: f.key, label: t(f.labelKey) }));
    if (fromSchema.length || schema.length) {
      return fromSchema.slice(0, MAX_FIELD_COLUMNS);
    }
    return [...present]
      .slice(0, MAX_FIELD_COLUMNS)
      .map((key) => ({ key, label: key }));
  }, [rows, schema, t]);

  const hasTarget = useMemo(
    () => rows.some((row) => importTargetParts(row.target).length > 0),
    [rows],
  );

  const columns = useMemo<ColumnDef<ImportTableRow, unknown>[]>(() => {
    const cols: ColumnDef<ImportTableRow, unknown>[] = [
      {
        id: "index",
        accessorKey: "index",
        header: t("imports.table.row"),
        meta: {
          label: t("imports.table.row"),
          cellClassName: "w-16 tabular-nums",
        },
        cell: ({ row }) => row.original.index,
      },
    ];
    for (const field of fields) {
      cols.push({
        id: `field_${field.key}`,
        accessorFn: (row) => cellText(row.data[field.key]),
        header: field.label,
        meta: { label: field.label },
        cell: ({ row }) => {
          const value = cellText(row.original.data[field.key]);
          const bad = row.original.errors.some((e) => e.field === field.key);
          return (
            <span
              className={cn(
                "inline-block max-w-56 truncate align-middle",
                bad &&
                  "bg-destructive/10 text-destructive ring-destructive/40 rounded px-1 ring-1",
              )}
              data-error-cell={bad ? field.key : undefined}
              title={value}
            >
              {value || (bad ? t("imports.table.empty_value") : "")}
            </span>
          );
        },
      });
    }
    if (hasTarget) {
      cols.push({
        id: "target",
        accessorFn: (row) =>
          importTargetParts(row.target)
            .map((p) => p.value)
            .join(" "),
        header: t("imports.table.target"),
        meta: { label: t("imports.table.target") },
        cell: ({ row }) => {
          const parts = importTargetParts(row.original.target);
          if (!parts.length)
            return <span className="text-muted-foreground">—</span>;
          return (
            <div className="space-y-0.5 text-xs">
              {parts.map((p) => (
                <div key={`${p.kind}-${p.value}`}>
                  {p.kind !== "other" ? (
                    <span className="text-muted-foreground">
                      {t(`imports.target.${p.kind}`)}:{" "}
                    </span>
                  ) : null}
                  {p.value}
                </div>
              ))}
            </div>
          );
        },
      });
    }
    cols.push(
      {
        id: "status",
        accessorKey: "status",
        header: t("imports.table.status"),
        meta: { label: t("imports.table.status") },
        cell: ({ row }) => (
          <Badge
            variant={TONE_BADGE[importStatusTone(row.original.status)]}
            data-testid="import-row-status"
          >
            {statusLabel(row.original.status, row.original.statusLabel)}
          </Badge>
        ),
      },
      {
        id: "message",
        accessorFn: (row) =>
          row.errors.map((e) => importErrorMessage(e, t)).join(" "),
        header: t("imports.table.message"),
        meta: { label: t("imports.table.message") },
        enableSorting: false,
        cell: ({ row }) =>
          row.original.errors.length ? (
            <ul className="text-destructive space-y-0.5 text-xs">
              {row.original.errors.map((err, i) => (
                <li key={`${err.code ?? err.error}-${i}`}>
                  {importErrorMessage(err, t)}
                </li>
              ))}
            </ul>
          ) : (
            <span className="text-muted-foreground">—</span>
          ),
      },
    );
    return cols;
  }, [fields, hasTarget, statusLabel, t]);

  return (
    <DataTable
      columns={columns}
      data={rows}
      getRowId={(row) => String(row.index)}
      emptyTitle={emptyTitle ?? t("imports.table.empty")}
      emptyDescription=""
      features={{
        sorting: true,
        globalFilter: true,
        columnFilters: false,
        columnVisibility: false,
        columnOrdering: false,
        columnPinning: false,
        columnResizing: false,
        rowSelection: false,
        density: false,
        viewMode: false,
        facetedFilters: false,
        pagination: true,
      }}
      initialState={{ pagination: { pageIndex: 0, pageSize: 25 } }}
      pageSizeOptions={[25, 50, 100]}
      getRowClassName={(row) =>
        row.errors.length ? "bg-destructive/[0.03]" : undefined
      }
      className="import-rows-table"
    />
  );
}
