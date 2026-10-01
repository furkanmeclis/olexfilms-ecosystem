import { z } from "zod";

import {
  ORGANIZATION_MEMBER_ROLES,
  ORGANIZATION_STATUS_VALUES,
} from "@/features/organizations/constants";

type Translate = (key: string) => string;

/** Structured address pickers (TEC-84): string ids, "" = none. */
const addressShape = {
  country_id: z.string().optional(),
  province_id: z.string().optional(),
  district_id: z.string().optional(),
};

export function createOrganizationFormSchema(t: Translate) {
  return z.object({
    name: z.string().trim().min(1, t("organizations.validation.name_required")),
    type: z.enum(["dealer", "distributor"]),
    ...addressShape,
    // Filled from the province/district on the server when left empty.
    city: z.string().trim(),
    district: z.string().trim(),
    phone: z
      .string()
      .trim()
      .min(1, t("organizations.validation.phone_required")),
    address: z
      .string()
      .trim()
      .min(1, t("organizations.validation.address_required")),
    owner_user_uuid: z
      .string()
      .uuid(t("organizations.validation.owner_required")),
  });
}

export type CreateOrganizationFormValues = z.infer<
  ReturnType<typeof createOrganizationFormSchema>
>;

export function updateOrganizationFormSchema(t: Translate) {
  return z.object({
    name: z.string().trim().min(1, t("organizations.validation.name_required")),
    ...addressShape,
    city: z.string().trim(),
    district: z.string().trim(),
    phone: z
      .string()
      .trim()
      .min(1, t("organizations.validation.phone_required")),
    address: z
      .string()
      .trim()
      .min(1, t("organizations.validation.address_required")),
    status: z.enum(ORGANIZATION_STATUS_VALUES),
    plan_code: z.string().trim().optional(),
    access_ends_at: z.string().optional(),
    clear_access_ends_at: z.boolean().optional(),
  });
}

export type UpdateOrganizationFormValues = z.infer<
  ReturnType<typeof updateOrganizationFormSchema>
>;

export function addOrganizationMemberFormSchema(t: Translate) {
  return z.object({
    user_uuid: z.string().uuid(t("organizations.validation.member_required")),
    role: z.enum(ORGANIZATION_MEMBER_ROLES),
  });
}

export type AddOrganizationMemberFormValues = z.infer<
  ReturnType<typeof addOrganizationMemberFormSchema>
>;
