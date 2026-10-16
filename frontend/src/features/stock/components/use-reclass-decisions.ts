"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";

import { Permission } from "@/config/permissions";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { stockErrorMessage } from "@/features/stock/lib/errors";
import {
  reclassActions,
  type ReclassActions,
} from "@/features/stock/lib/stock-ops";
import {
  stockOpsKeys,
  stockOpsService,
  type StockReclassification,
} from "@/features/stock/services/stock-ops.service";
import { useStepUp } from "@/features/step-up-engine";
import { useAuth } from "@/providers/auth-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export type ReclassDecision = "approve" | "reject" | "cancel";

/**
 * Decisions on reclassification requests (TEC-157, TEC-763), shared by
 * the list's row actions and the detail page: approve asks for a
 * confirmation and a fresh step-up (sensitive), reject asks for a reason,
 * cancel for a confirmation.
 */
export function useReclassDecisions(slug: string) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const { can } = usePermission();
  const { user } = useAuth();
  const org = useActiveOrganization(slug);
  const { confirm, prompt } = useDialogs();
  const { ensure } = useStepUp();

  const canReclassify = can(Permission.StockReclassify);
  const canWrite = can(Permission.StockWrite);
  const userUuid = user?.uuid ?? null;
  const organizationUuid = org?.uuid ?? null;

  const actionsFor = useCallback(
    (row: StockReclassification): ReclassActions =>
      reclassActions(row, {
        userUuid,
        organizationUuid,
        canReclassify,
        canWrite,
      }),
    [canReclassify, canWrite, organizationUuid, userUuid],
  );

  const mutation = useMutation({
    mutationFn: (v: { uuid: string; action: ReclassDecision; note?: string }) =>
      stockOpsService.decideReclassification(v.uuid, v.action, v.note),
    onSuccess: async (next, v) => {
      qc.setQueryData(stockOpsKeys.reclassification(next.uuid), next);
      await qc.invalidateQueries({ queryKey: ["stock"] });
      appToast.success(t(`stock.reclass.${v.action}_done`));
    },
    onError: (err) =>
      appToast.error(stockErrorMessage(err, t, t("stock.reclass.failed"))),
  });
  const { mutate } = mutation;

  const decide = useCallback(
    async (row: StockReclassification, action: ReclassDecision) => {
      const summary = {
        barcode: row.unit.barcode,
        from: row.from_product.name,
        to: row.to_product.name,
      };
      if (action === "reject") {
        const note = await prompt({
          title: t("stock.reclass.reject_title"),
          description: t("stock.reclass.reject_description", summary),
          placeholder: t("stock.reclass.reject_placeholder"),
          confirmLabel: t("stock.reclass.reject"),
        });
        if (note === null) return;
        if (!note.trim()) {
          appToast.error(t("stock.reclass.errors.note_required"));
          return;
        }
        mutate({ uuid: row.uuid, action, note: note.trim() });
        return;
      }
      const ok = await confirm({
        title: t(`stock.reclass.${action}_title`),
        description: t(`stock.reclass.${action}_description`, summary),
        confirmLabel: t(`stock.reclass.${action}`),
        variant: action === "cancel" ? "destructive" : "default",
      });
      if (!ok) return;
      // Approval is sensitive: a fresh step-up first (403 otherwise).
      if (action === "approve" && !(await ensure())) return;
      mutate({ uuid: row.uuid, action });
    },
    [confirm, ensure, mutate, prompt, t],
  );

  return { actionsFor, decide, pending: mutation.isPending };
}
