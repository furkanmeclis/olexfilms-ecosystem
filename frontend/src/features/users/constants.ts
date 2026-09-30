import type { UserStatus } from "@/features/users/services/users.service";

/** Aligns with OpenAPI PublicUser / CreatePlatformUserRequest status examples. */
export const USER_STATUS_VALUES = [
  "active",
  "pending",
  "disabled",
] as const satisfies readonly UserStatus[];

export const USER_STATUS_TONE: Record<
  UserStatus,
  "success" | "danger" | "warning" | "default"
> = {
  active: "success",
  pending: "warning",
  disabled: "danger",
};
