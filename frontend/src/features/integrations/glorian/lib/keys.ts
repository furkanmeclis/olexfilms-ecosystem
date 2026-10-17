/** TanStack Query keys of the Glorian admin page. */
export const glorianKeys = {
  all: ["platform", "integrations", "glorian"] as const,
  connection: () => [...glorianKeys.all, "connection"] as const,
  runs: (query: object) => [...glorianKeys.all, "sync-runs", query] as const,
  outbounds: (query: object) =>
    [...glorianKeys.all, "outbounds", query] as const,
  locationMaps: (query: object) =>
    [...glorianKeys.all, "location-maps", query] as const,
  mapWarehouses: () =>
    [...glorianKeys.all, "location-maps-warehouses"] as const,
  reconcile: () => [...glorianKeys.all, "reconcile"] as const,
};
