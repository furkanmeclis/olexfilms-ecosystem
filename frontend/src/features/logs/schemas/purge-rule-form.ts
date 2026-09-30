import { z } from "zod";

type Translate = (key: string) => string;

export function purgeRuleFormSchema(t: Translate) {
  return z.object({
    name: z.string().trim().min(1, t("logs.rules.validation.name")),
    enabled: z.boolean(),
    levels: z.array(z.string()).min(1, t("logs.rules.validation.levels")),
    source: z.string().optional(),
    message_contains: z.string().optional(),
    older_than_hours: z.coerce
      .number()
      .int()
      .min(1, t("logs.rules.validation.older_than")),
    interval_minutes: z.string().min(1),
  });
}

export type PurgeRuleFormValues = z.infer<
  ReturnType<typeof purgeRuleFormSchema>
>;
