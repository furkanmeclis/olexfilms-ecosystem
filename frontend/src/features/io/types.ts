export type ExportFormat = "pdf" | "xlsx" | "csv" | "json";
export type ImportFormat = "json" | "xlsx" | "csv" | "tsv";

import type { BulkActionMeta } from "@/features/bulk-engine/types";
import type { components } from "@/generated/api";

export type ResourceCapabilities = {
  create?: boolean;
  read?: boolean;
  update?: boolean;
  delete?: boolean;
  search?: boolean;
  filter?: boolean;
  sort?: boolean;
  export?: boolean;
  import?: boolean;
  bulk?: boolean;
};

export type ResourceMeta = {
  resource?: string;
  capabilities?: ResourceCapabilities;
  sortable?: string[];
  filterable?: string[];
  /** Endpoint default sort (`field` / `-field`); absent on older metas. */
  default_sort?: string;
  bulk_actions?: BulkActionMeta[];
};

/** The user who requested an export or uploaded an import (TEC-211). */
export type IoJobActor = {
  uuid: string;
  name: string;
};

export type ExportJob = {
  uuid: string;
  resource: string;
  format: string;
  status: string;
  row_count: number;
  error?: string | null;
  download_url?: string | null;
  filename?: string;
  actor?: IoJobActor | null;
  created_at: string;
};

export type ImportJob = {
  uuid: string;
  resource: string;
  format: string;
  status: string;
  mapping?: Record<string, string>;
  defaults?: Record<string, string>;
  preview_summary?: ImportPreviewSummary;
  error?: string | null;
  rollback_until?: string | null;
  applied_at?: string | null;
  source_filename?: string;
  actor?: IoJobActor | null;
  created_at: string;
};

export type ImportPreviewSummary = {
  total: number;
  valid: number;
  invalid: number;
  rows?: { index: number; data: Record<string, unknown> }[];
  errors?: { index: number; field?: string; error: string }[];
};

export type AppSettings = {
  company_name: string;
  tagline: string;
  primary_color: string;
  address: string;
  city?: string;
  district?: string;
  phone: string;
  email: string;
  website: string;
  footer_text: string;
  paper_size: string;
  logo_url?: string | null;
  /** TEC-240 map position; tenant settings only (null when unset). */
  latitude?: TenantSettingsSchema["latitude"];
  longitude?: TenantSettingsSchema["longitude"];
};

type TenantSettingsSchema = components["schemas"]["TenantSettings"];

export type ActivityEvent = {
  uuid: string;
  actor_user_id?: number | null;
  action: string;
  resource: string;
  resource_uuid?: string | null;
  payload: Record<string, unknown>;
  created_at: string;
};

export type ExportJobScope = "platform" | "tenant";

export type IoResource =
  | "platform.users"
  | "platform.roles"
  | "platform.notifications"
  | "platform.activity"
  | "tenant.catalog.products";

export const EXPORT_PATHS: Record<IoResource, string> = {
  "platform.users": "/v1/platform/users/export",
  "platform.roles": "/v1/platform/roles/export",
  "platform.notifications": "/v1/platform/notifications/export",
  "platform.activity": "/v1/platform/activity/export",
  "tenant.catalog.products": "/v1/catalog/products/export",
};

export const IMPORT_PATHS: Partial<
  Record<IoResource, { upload: string; sample: string }>
> = {
  "platform.users": {
    upload: "/v1/platform/users/import",
    sample: "/v1/platform/users/import/sample",
  },
  "platform.roles": {
    upload: "/v1/platform/roles/import",
    sample: "/v1/platform/roles/import/sample",
  },
  "tenant.catalog.products": {
    upload: "/v1/catalog/products/import",
    sample: "/v1/catalog/products/import/sample",
  },
};

export const IMPORT_SCHEMA: Partial<
  Record<IoResource, { key: string; labelKey: string; required?: boolean }[]>
> = {
  "platform.users": [
    { key: "email", labelKey: "users.fields.email", required: true },
    { key: "name", labelKey: "users.fields.name", required: true },
    { key: "surname", labelKey: "users.fields.surname", required: true },
    { key: "status", labelKey: "users.fields.status" },
    { key: "locale", labelKey: "users.fields.locale" },
    { key: "role_slugs", labelKey: "users.fields.roles" },
  ],
  "platform.roles": [
    { key: "name", labelKey: "roles.fields.name", required: true },
    { key: "slug", labelKey: "roles.fields.slug", required: true },
    { key: "description", labelKey: "roles.fields.description" },
    { key: "permission_slugs", labelKey: "roles.fields.permissions" },
  ],
  // Category is matched by name and must exist (TEC-145).
  "tenant.catalog.products": [
    { key: "sku", labelKey: "catalog.fields.sku", required: true },
    { key: "name", labelKey: "catalog.fields.name", required: true },
    { key: "category", labelKey: "catalog.fields.category", required: true },
    { key: "unit_type", labelKey: "catalog.fields.unit_type" },
    {
      key: "warranty_duration_months",
      labelKey: "catalog.fields.warranty_duration_months",
    },
    { key: "micron_thickness", labelKey: "catalog.fields.micron_thickness" },
    {
      key: "uses_fixed_barcode",
      labelKey: "catalog.fields.uses_fixed_barcode",
    },
    { key: "active", labelKey: "catalog.fields.active" },
    { key: "description_md", labelKey: "catalog.fields.description_md" },
  ],
};
