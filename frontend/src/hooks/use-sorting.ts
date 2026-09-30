"use client";

import { useState } from "react";

export type SortDirection = "asc" | "desc";

export function useSorting(
  initialId?: string,
  initialDir: SortDirection = "asc",
) {
  const [sortBy, setSortBy] = useState(initialId);
  const [sortDir, setSortDir] = useState<SortDirection>(initialDir);

  const toggle = (id: string) => {
    if (sortBy === id) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
      return;
    }
    setSortBy(id);
    setSortDir("asc");
  };

  return { sortBy, sortDir, setSortBy, setSortDir, toggle };
}
