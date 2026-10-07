import { z } from "zod";

import {
  AI_INSTRUCTIONS_MAX_CHARS,
  AI_KNOWLEDGE_MAX_BYTES,
  byteLength,
} from "@/features/ai-admin/lib/quota";
import type {
  AISettings,
  AISettingsUpdate,
} from "@/features/ai-admin/services/ai-admin.service";

/** Error messages are i18n keys, translated where they are shown. */
const quotaField = z
  .number()
  .int()
  .min(0, "ai_admin.errors.quota_negative")
  .nullable()
  .refine((v) => v != null, "ai_admin.errors.quota_required");

export const aiSettingsSchema = z.object({
  default_model: z.string().min(1, "ai_admin.errors.model_required"),
  fast_model: z.string().min(1, "ai_admin.errors.model_required"),
  default_monthly_token_quota: quotaField,
  system_pool_monthly_quota: quotaField,
  /** Tool name → enabled. */
  tools: z.record(z.string(), z.boolean()),
  extra_instructions: z
    .string()
    .max(AI_INSTRUCTIONS_MAX_CHARS, "ai_admin.errors.instructions_too_long"),
  knowledge_text: z
    .string()
    .refine(
      (v) => byteLength(v) <= AI_KNOWLEDGE_MAX_BYTES,
      "ai_admin.errors.knowledge_too_long",
    ),
});

export type AISettingsFormValues = z.infer<typeof aiSettingsSchema>;

export function toAISettingsFormValues(
  settings: AISettings,
): AISettingsFormValues {
  return {
    default_model: settings.default_model,
    fast_model: settings.fast_model,
    default_monthly_token_quota: settings.default_monthly_token_quota,
    system_pool_monthly_quota: settings.system_pool_monthly_quota,
    tools: Object.fromEntries(
      settings.tools.map((tool) => [tool.name, tool.enabled]),
    ),
    extra_instructions: settings.extra_instructions,
    knowledge_text: settings.knowledge_text,
  };
}

/** PUT body: every tool is sent (the backend keeps only `false`). */
export function toAISettingsUpdate(
  values: AISettingsFormValues,
): AISettingsUpdate {
  return {
    default_model: values.default_model,
    fast_model: values.fast_model,
    default_monthly_token_quota: values.default_monthly_token_quota ?? 0,
    system_pool_monthly_quota: values.system_pool_monthly_quota ?? 0,
    tool_toggles: { ...values.tools },
    extra_instructions: values.extra_instructions,
    knowledge_text: values.knowledge_text,
  };
}
