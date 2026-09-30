"use client";

import { useMemo, useState } from "react";

export function usePagination(initialPage = 1, initialPerPage = 20) {
  const [page, setPage] = useState(initialPage);
  const [perPage, setPerPage] = useState(initialPerPage);

  return useMemo(
    () => ({
      page,
      perPage,
      setPage,
      setPerPage,
      offset: (page - 1) * perPage,
      reset: () => setPage(1),
    }),
    [page, perPage],
  );
}
