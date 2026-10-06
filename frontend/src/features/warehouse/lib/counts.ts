import type {
  StockCount,
  StockCountLine,
  StockCountMethod,
  StockCountResolution,
  StockCountScan,
  StockCountScopeType,
  StockCountStatus,
  StockCountVisibility,
} from "@/features/warehouse/services/warehouse.service";

/** Stock count states (TEC-206). */
export const COUNT_STATUSES: StockCountStatus[] = [
  "draft",
  "in_progress",
  "pending_review",
  "approved",
  "cancelled",
];

/** The four counting methods. */
export const COUNT_METHODS: StockCountMethod[] = [
  "location_first",
  "unit_first",
  "product_qty",
  "initial_placement",
];

export const COUNT_VISIBILITIES: StockCountVisibility[] = ["blind", "guided"];

export const COUNT_SCOPES: StockCountScopeType[] = [
  "warehouse",
  "room",
  "location",
  "product",
];

/** Results of a difference line (matched lines need no resolution). */
export const COUNT_RESULTS: StockCountLine["result"][] = [
  "matched",
  "missing",
  "wrong_location",
  "unlocated",
  "unexpected",
  "qty_variance",
  "meter_variance",
];

/** Resolutions a difference line may allow. */
export const COUNT_RESOLUTIONS: StockCountResolution[] = [
  "ignore",
  "relocate",
  "void_missing",
  "increase_unlocated",
];

/** Counted scan kinds (location scans only set the context). */
export const COUNT_SCAN_KINDS: StockCountScan["kind"][] = [
  "serial",
  "fixed",
  "product",
];

/** initial_placement has no product scope (TEC-206). */
export function scopesFor(method: StockCountMethod): StockCountScopeType[] {
  return method === "initial_placement"
    ? COUNT_SCOPES.filter((s) => s !== "product")
    : COUNT_SCOPES;
}

export function countStatusTone(
  status: StockCountStatus,
): "default" | "success" | "warning" | "danger" {
  switch (status) {
    case "approved":
      return "success";
    case "draft":
    case "in_progress":
    case "pending_review":
      return "warning";
    case "cancelled":
      return "danger";
    default:
      return "default";
  }
}

/** Approve-start is needed (initial_placement) and not given yet. */
export function needsStartApproval(c: StockCount): boolean {
  return (
    c.status === "draft" && c.start_approval_required && !c.start_approved_at
  );
}

export function canStart(c: StockCount): boolean {
  return c.status === "draft" && !needsStartApproval(c);
}

export function canCancelCount(c: StockCount): boolean {
  return (
    c.status === "draft" ||
    c.status === "in_progress" ||
    c.status === "pending_review"
  );
}

/** Quantity is asked for fixed barcodes and SKUs (not for location_first). */
export function showsQuantity(method: StockCountMethod): boolean {
  return method === "product_qty" || method === "unit_first";
}

/** Lines the approval must resolve: every line that is not matched. */
export function linesToResolve(lines: StockCountLine[]): StockCountLine[] {
  return lines.filter((l) => l.result !== "matched");
}

/**
 * Default resolution of a line: the one it already has, else "ignore" when
 * allowed (the conservative choice: no stock change), else the first
 * allowed resolution.
 */
export function defaultResolution(
  line: StockCountLine,
): StockCountResolution | null {
  if (line.resolution && line.allowed_resolutions.includes(line.resolution)) {
    return line.resolution;
  }
  if (line.allowed_resolutions.includes("ignore")) return "ignore";
  return line.allowed_resolutions[0] ?? null;
}

/**
 * Approval body: one resolution per line to resolve. Lines without a valid
 * choice are listed in `missing`; the caller must not send the body then.
 */
export function approveBody(
  lines: StockCountLine[],
  choices: Record<string, StockCountResolution | undefined>,
): {
  resolutions: { line_uuid: string; resolution: StockCountResolution }[];
  missing: string[];
} {
  const resolutions: {
    line_uuid: string;
    resolution: StockCountResolution;
  }[] = [];
  const missing: string[] = [];
  for (const line of linesToResolve(lines)) {
    const choice = choices[line.uuid] ?? defaultResolution(line) ?? undefined;
    if (choice && line.allowed_resolutions.includes(choice)) {
      resolutions.push({ line_uuid: line.uuid, resolution: choice });
    } else {
      missing.push(line.uuid);
    }
  }
  return { resolutions, missing };
}

export type CountFormState = {
  warehouse_uuid: string;
  method: StockCountMethod;
  visibility: StockCountVisibility;
  scope_type: StockCountScopeType;
  room_uuid: string;
  location_uuid: string;
  product_uuid: string;
  note: string;
};

export const EMPTY_COUNT_FORM: CountFormState = {
  warehouse_uuid: "",
  method: "location_first",
  visibility: "blind",
  scope_type: "warehouse",
  room_uuid: "",
  location_uuid: "",
  product_uuid: "",
  note: "",
};

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

/** Field errors of the new count form (empty object when valid). */
export function countFormErrors(
  v: CountFormState,
  t: Translate,
): Partial<Record<keyof CountFormState, string>> {
  const errors: Partial<Record<keyof CountFormState, string>> = {};
  if (!v.warehouse_uuid) {
    errors.warehouse_uuid = t("warehouse.validation.warehouse");
  }
  if (!scopesFor(v.method).includes(v.scope_type)) {
    errors.scope_type = t("warehouse.validation.scope");
  }
  if (v.scope_type === "room" && !v.room_uuid) {
    errors.room_uuid = t("warehouse.validation.room");
  }
  if (v.scope_type === "location" && !v.location_uuid) {
    errors.location_uuid = t("warehouse.validation.location");
  }
  if (v.scope_type === "product" && !v.product_uuid) {
    errors.product_uuid = t("warehouse.validation.product");
  }
  if (v.note.trim().length > 500) {
    errors.note = t("warehouse.validation.too_long", { max: 500 });
  }
  return errors;
}

/** Request body: only the target of the chosen scope is sent. */
export function countBody(v: CountFormState) {
  return {
    warehouse_uuid: v.warehouse_uuid,
    method: v.method,
    visibility: v.visibility,
    scope_type: v.scope_type,
    ...(v.scope_type === "room" ? { room_uuid: v.room_uuid } : {}),
    ...(v.scope_type === "location" ? { location_uuid: v.location_uuid } : {}),
    ...(v.scope_type === "product" ? { product_uuid: v.product_uuid } : {}),
    note: v.note.trim() || null,
  };
}
