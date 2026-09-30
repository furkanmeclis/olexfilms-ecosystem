"use client";

import type { ReactNode } from "react";

import {
  DataTable,
  type DataTableBulkAction,
  type DataTableProps,
} from "@/components/tables";
import { ErrorState } from "@/components/common/error-state";
import { useLocale } from "@/providers/locale-provider";

type EntityTableProps<TData> = Omit<
  DataTableProps<TData>,
  "isLoading" | "emptyTitle" | "emptyDescription"
> & {
  isLoading?: boolean;
  isError?: boolean;
  errorTitle?: string;
  errorDescription?: string;
  onRetry?: () => void;
  emptyTitle?: string;
  emptyDescription?: string;
  bulkActions?: DataTableBulkAction<TData>[];
  toolbarExtra?: ReactNode;
};

/**
 * DataTable wrapper with shared loading / empty / error conventions
 * for platform entity list pages.
 */
export function EntityTable<TData>({
  isLoading,
  isError,
  errorTitle,
  errorDescription,
  onRetry,
  emptyTitle,
  emptyDescription,
  toolbar,
  toolbarExtra,
  ...props
}: EntityTableProps<TData>) {
  const { t } = useLocale();

  if (isError) {
    return (
      <ErrorState
        title={errorTitle ?? t("common.error_generic")}
        description={errorDescription}
        onRetry={onRetry}
        retryLabel={t("common.retry")}
      />
    );
  }

  const mergedToolbar =
    toolbar || toolbarExtra ? (
      <>
        {toolbarExtra}
        {typeof toolbar === "function" ? null : toolbar}
      </>
    ) : (
      toolbar
    );

  return (
    <DataTable
      {...props}
      isLoading={isLoading}
      emptyTitle={emptyTitle ?? t("common.empty_title")}
      emptyDescription={emptyDescription ?? t("common.empty_description")}
      toolbar={
        typeof toolbar === "function"
          ? (table) => (
              <>
                {toolbarExtra}
                {toolbar(table)}
              </>
            )
          : mergedToolbar
      }
      manual={{
        filtering: true,
        sorting: true,
        pagination: true,
        ...props.manual,
      }}
      features={{
        sorting: true,
        globalFilter: true,
        columnFilters: true,
        columnVisibility: true,
        columnResizing: true,
        rowSelection: true,
        pagination: true,
        density: true,
        facetedFilters: true,
        ...props.features,
      }}
    />
  );
}
