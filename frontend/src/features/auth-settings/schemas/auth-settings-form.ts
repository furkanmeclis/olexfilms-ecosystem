import { z } from "zod";

export const authSettingsFormSchema = z.object({
  registration_enabled: z.boolean(),
  default_role_uuid: z.string().uuid().nullable(),
  password_login_enabled: z.boolean(),
  password_register_enabled: z.boolean(),
  passkey_login_enabled: z.boolean(),
});

export type AuthSettingsFormValues = z.infer<typeof authSettingsFormSchema>;
