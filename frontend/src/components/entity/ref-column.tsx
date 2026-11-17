"use client";

import type { ColumnDef, RowData } from "@tanstack/react-table";

import { createColumn } from "@/components/tables";
import type { EntityKind } from "@/config/routes";

import { EntityLink, entityRefLabel, type EntityRefValue } from "./entity-link";

type CreateColumnOptions<TData extends RowData> = Parameters<
  typeof createColumn<TData, string>
>[0];

export type RefColumnOptions<TData extends RowData> = Omit<
  CreateColumnOptions<TData>,
  "accessorFn" | "accessorKey" | "cell" | "id"
> & {
  /** Column id (also the backend sort field unless `sortParam` is set). */
  id: string;
  kind: EntityKind;
  /** Reads the ref from the row. */
  getRef: (row: TData) => EntityRefValue | null | undefined;
  /** Shows the kind and `sub` in a hover card. */
  hoverCard?: boolean;
};

/**
 * TEC-1217: EntityTable column of a related record. The cell is an
 * `EntityLink` (plain text without permission); the column value is the
 * ref's name, so client sorting, grid cards and exports keep working.
 * Sorting is off unless `enableSorting` is passed (the backend must accept
 * the column id or `sortParam`).
 */
export function refColumn<TData extends RowData>({
  id,
  kind,
  getRef,
  hoverCard,
  enableSorting = false,
  ...rest
}: RefColumnOptions<TData>): ColumnDef<TData, string> {
  return createColumn<TData, string>({
    ...rest,
    id,
    enableSorting,
    accessorFn: (row) => entityRefLabel(getRef(row)),
    cell: ({ row }) => (
      <EntityLink
        kind={kind}
        entity={getRef(row.original)}
        hoverCard={hoverCard}
      />
    ),
  } as CreateColumnOptions<TData>);
}
