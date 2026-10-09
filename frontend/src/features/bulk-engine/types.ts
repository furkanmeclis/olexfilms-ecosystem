import type { LucideIcon } from "lucide-react";

/** Extra input collected before a parameterized bulk action. */
export type BulkActionParam = {
  key: string;
  kind: "percent" | "number" | "uuid" | "enum" | "text";
  required?: boolean;
  label_key: string;
  /**
   * Allowed values of an `enum` param (TEC-371); the label of a value is
   * `label_key + "." + value`.
   */
  options?: string[];
};

/** Bulk action contract from resource meta API (no icon). */
export type BulkActionMeta = {
  id: string;
  label_key: string;
  permission: string;
  destructive?: boolean;
  reversible?: boolean;
  confirm_key?: string;
  params?: BulkActionParam[];
};

/** Bulk action for UI — icon is required. */
export type BulkActionDef = BulkActionMeta & {
  icon: LucideIcon;
};

export type BulkTarget = {
  scope: "ids" | "query";
  ids?: string[];
  query?: Record<string, string>;
  params?: Record<string, string>;
};

export type BulkSummary = {
  total: number;
  succeeded: number;
  failed: number;
};

export type BulkJob = {
  uuid: string;
  resource: string;
  action: string;
  status: string;
  target?: BulkTarget;
  summary?: BulkSummary;
  error?: string | null;
  rollback_until?: string | null;
  applied_at?: string | null;
  created_at: string;
};

/** Undo outcome of a logged bulk operation (TEC-212). */
export type BulkUndoResult = {
  restored: number;
  skipped: { entity_uuid: string; reason: "conflict" | "gone" }[];
  failed: { entity_uuid: string; error: string }[];
};

/** Logged bulk operation: the undoable record of a run (TEC-212). */
export type BulkOperation = {
  uuid: string;
  resource: string;
  action: string;
  job_uuid?: string | null;
  summary: BulkSummary;
  undo_status: "none" | "available" | "undone" | "partial";
  undo_until?: string | null;
  undone_at?: string | null;
  undo_result?: BulkUndoResult | null;
  created_at: string;
};

export type BulkExecuteSyncResult = {
  sync: true;
  summary: BulkSummary;
  operation?: BulkOperation | null;
};

export type BulkResource =
  | "platform.users"
  | "platform.roles"
  | "catalog.products"
  | "tasks"
  | "platform.organizations"
  | "catalog.categories"
  | "vehicle_catalog.brands"
  | "vehicle_catalog.models"
  | "leads"
  | "conversations"
  | "einvoices.billable"
  | "performance.bonuses";

export const BULK_PATHS: Record<BulkResource, string> = {
  "platform.users": "/v1/platform/users/bulk",
  "platform.roles": "/v1/platform/roles/bulk",
  "catalog.products": "/v1/catalog/products/bulk",
  tasks: "/v1/tasks/bulk",
  "platform.organizations": "/v1/platform/organizations/bulk",
  "catalog.categories": "/v1/catalog/categories/bulk",
  "vehicle_catalog.brands": "/v1/platform/vehicle-catalog/brands/bulk",
  "vehicle_catalog.models": "/v1/platform/vehicle-catalog/models/bulk",
  leads: "/v1/leads/bulk",
  conversations: "/v1/conversations/bulk",
  // TEC-504: e-invoice drafts of the billable center sales.
  "einvoices.billable": "/v1/einvoices/billable/bulk",
  "performance.bonuses": "/v1/performance/bonuses/bulk",
};

/** Platform-scoped resources whose name has no `platform.` prefix. */
const PLATFORM_BULK_RESOURCES = new Set<string>([
  "vehicle_catalog.brands",
  "vehicle_catalog.models",
  // WhatsApp inbox: platform admin only (S2), undo under /v1/platform.
  "conversations",
]);

/** Tenant resources log their operations under the active organization. */
export function isTenantBulkResource(resource: BulkResource | string) {
  return (
    !resource.startsWith("platform.") && !PLATFORM_BULK_RESOURCES.has(resource)
  );
}

/** Undo route of an operation: tenant or platform scope. */
export function bulkUndoPath(resource: string, operationUuid: string) {
  return isTenantBulkResource(resource)
    ? `/v1/tenant/bulk-operations/${operationUuid}/undo`
    : `/v1/platform/bulk-operations/${operationUuid}/undo`;
}

export type SelectionScope =
  | { mode: "none" }
  | { mode: "page"; ids: string[] }
  | { mode: "all"; query: Record<string, string>; total: number };

export type ListJobsResult = {
  items: BulkJob[];
  total: number;
  limit: number;
  offset: number;
};
