import { z } from "zod";

export const githubSettingsFormSchema = z.object({
  enabled: z.boolean(),
  register_enabled: z.boolean(),
  app_id: z.string().trim(),
  client_id: z.string().trim(),
  client_secret: z.string().trim().optional(),
  private_key: z.string().trim().optional(),
});

export type GitHubSettingsFormValues = z.infer<typeof githubSettingsFormSchema>;
