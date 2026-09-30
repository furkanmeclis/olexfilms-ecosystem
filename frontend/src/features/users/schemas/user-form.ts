import { z } from "zod";

import { passwordPolicySchema } from "@/features/auth/schemas";
import { USER_STATUS_VALUES } from "@/features/users/constants";

type Translate = (key: string) => string;

/** Aligns with platform user create API. */
export function createUserFormSchema(t: Translate) {
  return z.object({
    name: z.string().trim().min(1, t("users.validation.name_required")),
    surname: z.string().trim().min(1, t("users.validation.surname_required")),
    email: z.email(t("users.validation.email")),
    password: passwordPolicySchema(t),
    status: z.enum(USER_STATUS_VALUES),
    role_uuids: z.array(z.string().uuid()),
  });
}

export type CreateUserFormValues = z.infer<
  ReturnType<typeof createUserFormSchema>
>;

/** Aligns with platform user patch API. */
export function updateUserFormSchema(t: Translate) {
  return z.object({
    name: z.string().trim().min(1, t("users.validation.name_required")),
    surname: z.string().trim().min(1, t("users.validation.surname_required")),
    status: z.enum(USER_STATUS_VALUES),
    role_uuids: z.array(z.string().uuid()),
  });
}

export type UpdateUserFormValues = z.infer<
  ReturnType<typeof updateUserFormSchema>
>;

/** Aligns with set-password API. */
export function setUserPasswordFormSchema(t: Translate) {
  return z.object({
    password: passwordPolicySchema(t),
  });
}

export type SetUserPasswordFormValues = z.infer<
  ReturnType<typeof setUserPasswordFormSchema>
>;
