import { useQuery } from "@tanstack/react-query";

import { api, unwrap, uploadForm } from "@/core/api/client";
import type { components } from "@/core/api/schema";
import { PAGE_SIZE, usePagedQuery } from "@/features/admin/kit";
import type { StatusTone } from "@/ui/status-pill";

type S = components["schemas"];
/** TEC-1226: stock adjustment documents and the return inspection queue (no price). */
export type Adjustment = S["StockAdjustment"];
export type AdjustmentType = S["StockAdjustmentType"];
export type AdjustmentStatus = S["StockAdjustmentStatus"];
export type QuarantineItem = S["QuarantineItem"];
export type InspectionInput = S["ReturnInspectionInput"];
export type InspectionDecision = InspectionInput["decision"];
export type ReturnReason = InspectionInput["return_reason"];

export const ADJUSTMENT_TYPES: AdjustmentType[] = [
  "waste",
  "damage",
  "scrap",
  "loss",
  "found",
  "other",
];
export const INSPECTION_DECISIONS: InspectionDecision[] = [
  "restock",
  "scrap",
  "supplier_return",
];
export const RETURN_REASONS: ReturnReason[] = [
  "defective",
  "damaged",
  "wrong_product",
  "excess",
  "customer_return",
  "other",
];

export type AdjustFilter = "open" | "posted" | "all";

const FILTERS: Record<AdjustFilter, string | undefined> = {
  open: "draft,pending_approval",
  posted: "posted",
  all: undefined,
};

export const adjustKeys = {
  list: (filter: AdjustFilter, q: string) =>
    ["warehouse", "adjust", "list", filter, q] as const,
  one: (uuid: string) => ["warehouse", "adjust", uuid] as const,
  quarantine: (q: string) => ["warehouse", "quarantine", q] as const,
};

export function adjustTone(s: AdjustmentStatus): StatusTone {
  if (s === "posted") return "done";
  if (s === "cancelled") return "neutral";
  if (s === "pending_approval") return "progress";
  return "neutral";
}

export { needsPhoto, normalizeMeters } from "./adjust-meters";

export function useAdjustments(filter: AdjustFilter, q: string) {
  return usePagedQuery<Adjustment>(adjustKeys.list(filter, q), (offset) =>
    unwrap(
      api.GET("/v1/warehouse/adjustments", {
        params: {
          query: {
            status: FILTERS[filter],
            q: q || undefined,
            sort: "-created_at",
            limit: PAGE_SIZE,
            offset,
          },
        },
      }),
    ),
  );
}

export function useAdjustment(uuid: string) {
  return useQuery({
    queryKey: adjustKeys.one(uuid),
    queryFn: () =>
      unwrap(
        api.GET("/v1/warehouse/adjustments/{uuid}", {
          params: { path: { uuid } },
        }),
      ),
    enabled: Boolean(uuid),
  });
}

export function createAdjustment(
  type: AdjustmentType,
  warehouseUuid?: string | null,
) {
  return unwrap(
    api.POST("/v1/warehouse/adjustments", {
      body: { type, warehouse_uuid: warehouseUuid ?? null },
    }),
  );
}

export function addAdjustmentLine(
  uuid: string,
  barcode: string,
  meters?: string,
  quantity?: number,
) {
  return unwrap(
    api.POST("/v1/warehouse/adjustments/{uuid}/lines", {
      params: { path: { uuid } },
      body: { barcode, meters: meters ?? null, quantity: quantity ?? null },
    }),
  );
}

export function deleteAdjustmentLine(uuid: string, line: string) {
  return unwrap(
    api.DELETE("/v1/warehouse/adjustments/{uuid}/lines/{line_uuid}", {
      params: { path: { uuid, line_uuid: line } },
    }),
  );
}

export function adjustmentAction(
  uuid: string,
  action: "submit" | "approve" | "cancel",
) {
  const params = { path: { uuid } };
  if (action === "submit")
    return unwrap(
      api.POST("/v1/warehouse/adjustments/{uuid}/submit", { params }),
    );
  if (action === "approve")
    return unwrap(
      api.POST("/v1/warehouse/adjustments/{uuid}/approve", { params }),
    );
  return unwrap(
    api.POST("/v1/warehouse/adjustments/{uuid}/cancel", { params, body: {} }),
  );
}

/** JPEG from the camera, sent as a React Native FormData file part. */
export function uploadAdjustmentPhoto(uuid: string, uri: string) {
  const form = new FormData();
  form.append("file", {
    uri,
    name: "adjustment.jpg",
    type: "image/jpeg",
  } as unknown as Blob);
  return uploadForm<Adjustment>(
    `/v1/warehouse/adjustments/${encodeURIComponent(uuid)}/photos`,
    form,
  );
}

export function useQuarantine(q: string) {
  return usePagedQuery<QuarantineItem>(adjustKeys.quarantine(q), (offset) =>
    unwrap(
      api.GET("/v1/warehouse/return-inspections", {
        params: { query: { q: q || undefined, limit: PAGE_SIZE, offset } },
      }),
    ),
  );
}

export function inspectReturn(body: InspectionInput) {
  return unwrap(api.POST("/v1/warehouse/return-inspections", { body }));
}
