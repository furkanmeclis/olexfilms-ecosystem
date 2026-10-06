/** TanStack Query keys of the Glorian admin page. */
export const glorianKeys = {
  all: ["platform", "integrations", "glorian"] as const,
  connection: () => [...glorianKeys.all, "connection"] as const,
  runs: (query: object) => [...glorianKeys.all, "sync-runs", query] as const,
  outbounds: (query: object) =>
    [...glorianKeys.all, "outbounds", query] as const,
  reconcile: () => [...glorianKeys.all, "reconcile"] as const,
};
