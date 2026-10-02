import { useMutation, useQueryClient } from "@tanstack/react-query";

import { bulkService } from "@/features/bulk-engine/services/bulk.service";
import type {
  BulkExecuteSyncResult,
  BulkJob,
  BulkOperation,
  BulkResource,
  BulkTarget,
} from "@/features/bulk-engine/types";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { toast } from "sonner";

function isSyncResult(
  data: BulkJob | BulkExecuteSyncResult,
): data is BulkExecuteSyncResult {
  return "sync" in data && data.sync === true;
}

type ExecuteInput = {
  resource: BulkResource;
  action: string;
  target: BulkTarget;
  onComplete?: () => void;
};

/** Error code → message key of the undo endpoint (TEC-212). */
function undoErrorKey(error: unknown) {
  if (isApiError(error)) {
    if (error.code === "BULK_UNDO_EXPIRED") return "bulk.undo_expired";
    if (error.code === "BULK_UNDO_UNAVAILABLE") return "bulk.undo_unavailable";
  }
  return "common.error_generic";
}

/**
 * Undo of a logged bulk operation (TEC-212): the "Geri al" action offered
 * right after a sync run and on the operation log.
 */
export function useBulkUndoMutation() {
  const { t } = useLocale();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({
      resource,
      operationUuid,
    }: {
      resource: string;
      operationUuid: string;
    }) => bulkService.undo(resource, operationUuid),
    onSuccess: (op: BulkOperation) => {
      const result = op.undo_result;
      const skipped = result?.skipped.length ?? 0;
      const failed = result?.failed.length ?? 0;
      if (skipped > 0 || failed > 0) {
        toast.warning(
          t("bulk.undo_partial", {
            restored: result?.restored ?? 0,
            skipped: skipped + failed,
          }),
        );
      } else {
        toast.success(
          t("bulk.undo_success", { restored: result?.restored ?? 0 }),
        );
      }
      void queryClient.invalidateQueries({ queryKey: ["bulk-jobs"] });
      void queryClient.invalidateQueries({ queryKey: ["bulk-operations"] });
    },
    onError: (error: unknown) => {
      toast.error(t(undoErrorKey(error)));
    },
  });
}

export function useBulkMutation() {
  const { locale, t } = useLocale();
  const queryClient = useQueryClient();
  const undo = useBulkUndoMutation();

  return useMutation({
    mutationFn: async ({ resource, action, target }: ExecuteInput) => {
      return bulkService.execute(resource, {
        action,
        target,
        locale,
      });
    },
    onSuccess: (data, variables) => {
      if (isSyncResult(data)) {
        const operation = data.operation;
        const message = t("bulk.result.sync", {
          succeeded: data.summary.succeeded,
          failed: data.summary.failed,
        });
        if (operation && operation.undo_status === "available") {
          toast.success(message, {
            duration: 10000,
            action: {
              label: t("bulk.undo"),
              onClick: () => {
                undo.mutate(
                  { resource: variables.resource, operationUuid: operation.uuid },
                  { onSuccess: () => variables.onComplete?.() },
                );
              },
            },
          });
        } else {
          toast.success(message);
        }
        variables.onComplete?.();
        return;
      }
      toast.info(t("bulk.job.queued"));
      void queryClient.invalidateQueries({ queryKey: ["bulk-jobs"] });
      variables.onComplete?.();
    },
    onError: () => {
      toast.error(t("bulk.job.failed"));
    },
  });
}

export function useBulkRollbackMutation() {
  const { t } = useLocale();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: (uuid: string) => bulkService.rollback(uuid),
    onSuccess: () => {
      toast.success(t("bulk.rollback_success"));
      void queryClient.invalidateQueries({ queryKey: ["bulk-jobs"] });
    },
    onError: () => {
      toast.error(t("common.error_generic"));
    },
  });
}
