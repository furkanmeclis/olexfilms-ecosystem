"use client";

import type { ColumnDef, RowData } from "@tanstack/react-table";

import { Checkbox } from "@/components/ui/checkbox";
import { useLocale } from "@/providers/locale-provider";

function SelectLabel({
  children,
}: {
  children: (label: (key: string) => string) => React.ReactNode;
}) {
  const { t } = useLocale();
  return <>{children(t)}</>;
}

export function createSelectColumnDef<TData extends RowData>(): ColumnDef<
  TData,
  unknown
> {
  return {
    id: "__select",
    size: 40,
    enableSorting: false,
    enableHiding: false,
    enableColumnFilter: false,
    enableResizing: false,
    enablePinning: true,
    meta: {
      labelKey: "table.select",
      enableHiding: false,
    },
    header: ({ table }) => (
      <SelectLabel>
        {(t) => (
          <Checkbox
            checked={
              table.getIsAllPageRowsSelected() ||
              (table.getIsSomePageRowsSelected() && "indeterminate")
            }
            onCheckedChange={(value) =>
              table.toggleAllPageRowsSelected(Boolean(value))
            }
            aria-label={t("table.select_all")}
          />
        )}
      </SelectLabel>
    ),
    cell: ({ row }) => (
      <SelectLabel>
        {(t) => (
          <Checkbox
            checked={row.getIsSelected()}
            disabled={!row.getCanSelect()}
            onCheckedChange={(value) => row.toggleSelected(Boolean(value))}
            aria-label={t("table.select_row")}
          />
        )}
      </SelectLabel>
    ),
  };
}
