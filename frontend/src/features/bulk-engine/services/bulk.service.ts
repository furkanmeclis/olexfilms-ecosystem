import type {
  BulkExecuteSyncResult,
  BulkJob,
  BulkOperation,
  BulkResource,
  BulkTarget,
  ListJobsResult,
} from "@/features/bulk-engine/types";
import { BULK_PATHS, bulkUndoPath } from "@/features/bulk-engine/types";
import { platformRequest } from "@/lib/api/platform-request";

export const bulkService = {
  async execute(
    resource: BulkResource,
    body: { action: string; target: BulkTarget; locale: string },
  ) {
    return platformRequest<BulkJob | BulkExecuteSyncResult>(
      "POST",
      BULK_PATHS[resource],
      { body },
    );
  },

  async get(uuid: string) {
    return platformRequest<BulkJob>("GET", `/v1/platform/bulk/${uuid}`);
  },

  async rollback(uuid: string) {
    return platformRequest<BulkJob>(
      "POST",
      `/v1/platform/bulk/${uuid}/rollback`,
    );
  },

  /** Undo a logged operation (TEC-212); 409 when already undone or expired. */
  async undo(resource: string, operationUuid: string) {
    return platformRequest<BulkOperation>(
      "POST",
      bulkUndoPath(resource, operationUuid),
    );
  },

  async list(params: { limit?: number; offset?: number }) {
    return platformRequest<ListJobsResult>("GET", "/v1/platform/bulk", {
      query: params,
    });
  },
};
