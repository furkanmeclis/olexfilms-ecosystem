import { useQuery } from "@tanstack/react-query";

import { bulkService } from "@/features/bulk-engine/services/bulk.service";

const ACTIVE_STATUSES = new Set(["queued", "processing"]);

export function useBulkJobQuery(uuid: string | null, enabled = true) {
  return useQuery({
    queryKey: ["bulk-jobs", uuid],
    queryFn: () => bulkService.get(uuid!),
    enabled: Boolean(uuid) && enabled,
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (status && ACTIVE_STATUSES.has(status)) return 2000;
      return false;
    },
  });
}
