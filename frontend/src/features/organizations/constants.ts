import type { OrganizationStatus } from "@/features/organizations/services/organizations.service";

export const ORGANIZATION_STATUS_VALUES = [
  "pending",
  "active",
  "suspended",
  "expired",
] as const satisfies readonly OrganizationStatus[];

export const ORGANIZATION_STATUS_TONE: Record<
  OrganizationStatus,
  "success" | "danger" | "warning" | "default"
> = {
  active: "success",
  pending: "warning",
  suspended: "danger",
  expired: "danger",
};

export const ORGANIZATION_MEMBER_ROLES = ["owner", "staff"] as const;
