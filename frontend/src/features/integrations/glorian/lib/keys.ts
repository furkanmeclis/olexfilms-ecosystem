/** TanStack Query keys of the Glorian admin page. */
export const glorianKeys = {
  all: ["platform", "integrations", "glorian"] as const,
  connection: () => [...glorianKeys.all, "connection"] as const,
  runs: (kind: string, status: string) =>
    [...glorianKeys.all, "sync-runs", kind, status] as const,
  outbounds: (state: string) =>
    [...glorianKeys.all, "outbounds", state] as const,
  reconcile: () => [...glorianKeys.all, "reconcile"] as const,
};
