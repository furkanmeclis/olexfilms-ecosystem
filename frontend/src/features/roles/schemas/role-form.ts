import { z } from "zod";

type Translate = (key: string) => string;

export function createRoleFormSchema(t: Translate) {
  return z.object({
    name: z.string().trim().min(1, t("roles.validation.name")),
    slug: z
      .string()
      .trim()
      .min(1, t("roles.validation.slug"))
      .regex(/^[a-z0-9_]+$/, t("roles.validation.slug_format")),
    description: z.string().optional(),
    permission_slugs: z.array(z.string()),
    /** Permission slug -> scope (TEC-85); unselected keys are ignored. */
    grants: z.record(z.string(), z.string()).optional(),
  });
}

export type CreateRoleFormValues = z.infer<
  ReturnType<typeof createRoleFormSchema>
>;

export function updateRoleFormSchema(t: Translate) {
  return z.object({
    name: z.string().trim().min(1, t("roles.validation.name")),
    slug: z.string().trim().min(1, t("roles.validation.slug")),
    description: z.string().optional(),
    permission_slugs: z.array(z.string()),
    /** Permission slug -> scope (TEC-85); unselected keys are ignored. */
    grants: z.record(z.string(), z.string()).optional(),
  });
}

export type UpdateRoleFormValues = z.infer<
  ReturnType<typeof updateRoleFormSchema>
>;
