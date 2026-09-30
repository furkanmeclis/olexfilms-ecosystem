import { useCallback, useMemo, useState } from "react";
import type { RowSelectionState, Updater } from "@tanstack/react-table";

import type { SelectionScope } from "@/features/bulk-engine/types";

type UseBulkSelectionOptions = {
  listQueryKey: unknown;
  bulkQuery: Record<string, string | undefined>;
  total: number;
};

export function useBulkSelection({
  listQueryKey,
  bulkQuery,
  total,
}: UseBulkSelectionOptions) {
  const bulkQueryKey = JSON.stringify(bulkQuery);
  const listQueryKeySerialized = JSON.stringify(listQueryKey);
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const [allMatching, setAllMatching] = useState<{
    query: Record<string, string>;
    total: number;
  } | null>(null);
  const [prevListQueryKey, setPrevListQueryKey] = useState(
    listQueryKeySerialized,
  );
  const [prevBulkQueryKey, setPrevBulkQueryKey] = useState(bulkQueryKey);

  if (
    listQueryKeySerialized !== prevListQueryKey ||
    bulkQueryKey !== prevBulkQueryKey
  ) {
    setPrevListQueryKey(listQueryKeySerialized);
    setPrevBulkQueryKey(bulkQueryKey);
    setRowSelection({});
    setAllMatching(null);
  }

  const onRowSelectionChange = useCallback(
    (updater: Updater<RowSelectionState>) => {
      setRowSelection((prev) =>
        typeof updater === "function" ? updater(prev) : updater,
      );
      setAllMatching(null);
    },
    [],
  );

  const pageIds = useMemo(
    () => Object.keys(rowSelection).filter((id) => rowSelection[id]),
    [rowSelection],
  );

  const scope: SelectionScope = allMatching
    ? { mode: "all", query: allMatching.query, total: allMatching.total }
    : pageIds.length > 0
      ? { mode: "page", ids: pageIds }
      : { mode: "none" };

  const clearSelection = useCallback(() => {
    setAllMatching(null);
    setRowSelection({});
  }, []);

  const selectAllMatching = useCallback(() => {
    const query: Record<string, string> = {};
    for (const [key, value] of Object.entries(bulkQuery)) {
      if (value) query[key] = value;
    }
    setAllMatching({ query, total });
    setRowSelection({});
  }, [bulkQuery, total]);

  const selectedCount =
    scope.mode === "all"
      ? scope.total
      : scope.mode === "page"
        ? scope.ids.length
        : 0;

  const showSelectAllBanner =
    scope.mode === "page" && pageIds.length > 0 && total > pageIds.length;

  return {
    scope,
    rowSelection,
    onRowSelectionChange,
    selectedCount,
    showSelectAllBanner,
    clearSelection,
    selectAllMatching,
  };
}

export function buildBulkTarget(
  scope: SelectionScope,
  params?: Record<string, string>,
) {
  const extra = params && Object.keys(params).length > 0 ? { params } : {};
  if (scope.mode === "page") {
    return { scope: "ids" as const, ids: scope.ids, ...extra };
  }
  if (scope.mode === "all") {
    return { scope: "query" as const, query: scope.query, ...extra };
  }
  throw new Error("No rows selected");
}
