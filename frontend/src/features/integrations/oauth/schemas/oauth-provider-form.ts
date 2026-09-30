import { z } from "zod";

export const oauthProviderFormSchema = z.object({
  login_enabled: z.boolean(),
  register_enabled: z.boolean(),
  client_id: z.string().trim(),
  client_secret: z.string().trim().optional(),
});

export type OAuthProviderFormValues = z.infer<typeof oauthProviderFormSchema>;
