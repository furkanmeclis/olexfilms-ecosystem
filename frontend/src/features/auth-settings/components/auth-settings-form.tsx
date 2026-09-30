"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { Settings2 } from "lucide-react";

import {
  AppForm,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  authSettingsFormSchema,
  type AuthSettingsFormValues,
} from "@/features/auth-settings/schemas/auth-settings-form";
import type { AuthSettings } from "@/features/auth-settings/services/auth-settings.service";
import { rolesService } from "@/features/roles/services/roles.service";
import { useLocale } from "@/providers/locale-provider";

type AuthSettingsFormProps = {
  settings: AuthSettings;
  canWrite: boolean;
  isSaving: boolean;
  onSubmit: (values: AuthSettingsFormValues) => Promise<void>;
};

const NONE = "__none__";

export function AuthSettingsForm({
  settings,
  canWrite,
  isSaving,
  onSubmit,
}: AuthSettingsFormProps) {
  const { t } = useLocale();

  const { data: rolesData } = useQuery({
    queryKey: ["platform", "roles", "auth-settings-picker"],
    queryFn: () => rolesService.list({ limit: 100, offset: 0 }),
    enabled: canWrite || Boolean(settings.default_role),
  });

  const defaultValues = useMemo<AuthSettingsFormValues>(
    () => ({
      registration_enabled: settings.registration_enabled,
      default_role_uuid: settings.default_role?.uuid ?? null,
      password_login_enabled: settings.password_login_enabled,
      password_register_enabled: settings.password_register_enabled,
      passkey_login_enabled: settings.passkey_login_enabled,
    }),
    [settings],
  );

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "policy",
          label: t("auth.settings.form.section_title"),
          description: t("auth.settings.form.section_hint"),
          icon: Settings2,
        },
      ]),
    [t],
  );

  return (
    <AppForm
      schema={authSettingsFormSchema}
      defaultValues={defaultValues}
      onSubmit={onSubmit}
    >
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("policy")}
            title={t("auth.settings.form.section_title")}
            description={t("auth.settings.form.section_hint")}
          >
            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="registration_enabled">
                  {t("auth.settings.form.registration_enabled")}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t("auth.settings.form.registration_enabled_hint")}
                </p>
              </div>
              <Switch
                id="registration_enabled"
                checked={form.watch("registration_enabled")}
                onCheckedChange={(checked) =>
                  form.setValue("registration_enabled", checked, {
                    shouldDirty: true,
                  })
                }
                disabled={!canWrite || isSaving}
              />
            </div>

            <div className="space-y-2 sm:col-span-2">
              <Label>{t("auth.settings.form.default_role")}</Label>
              <p className="text-muted-foreground text-sm">
                {t("auth.settings.form.default_role_hint")}
              </p>
              <Select
                value={form.watch("default_role_uuid") ?? NONE}
                onValueChange={(value) =>
                  form.setValue(
                    "default_role_uuid",
                    value === NONE ? null : value,
                    { shouldDirty: true },
                  )
                }
                disabled={!canWrite || isSaving}
              >
                <SelectTrigger>
                  <SelectValue
                    placeholder={t("auth.settings.form.default_role_none")}
                  />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NONE}>
                    {t("auth.settings.form.default_role_none")}
                  </SelectItem>
                  {(rolesData?.items ?? []).map((role) => (
                    <SelectItem key={role.uuid} value={role.uuid}>
                      {role.name} ({role.slug})
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="password_login_enabled">
                  {t("auth.settings.form.password_login")}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t("auth.settings.form.password_login_hint")}
                </p>
              </div>
              <Switch
                id="password_login_enabled"
                checked={form.watch("password_login_enabled")}
                onCheckedChange={(checked) =>
                  form.setValue("password_login_enabled", checked, {
                    shouldDirty: true,
                  })
                }
                disabled={!canWrite || isSaving}
              />
            </div>

            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="password_register_enabled">
                  {t("auth.settings.form.password_register")}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t("auth.settings.form.password_register_hint")}
                </p>
              </div>
              <Switch
                id="password_register_enabled"
                checked={form.watch("password_register_enabled")}
                onCheckedChange={(checked) =>
                  form.setValue("password_register_enabled", checked, {
                    shouldDirty: true,
                  })
                }
                disabled={!canWrite || isSaving}
              />
            </div>

            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="passkey_login_enabled">
                  {t("auth.settings.form.passkey_login")}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t("auth.settings.form.passkey_login_hint")}
                </p>
              </div>
              <Switch
                id="passkey_login_enabled"
                checked={form.watch("passkey_login_enabled")}
                onCheckedChange={(checked) =>
                  form.setValue("passkey_login_enabled", checked, {
                    shouldDirty: true,
                  })
                }
                disabled={!canWrite || isSaving}
              />
            </div>
          </FormSection>

          {canWrite ? (
            <FormActions>
              <Button
                type="button"
                variant="outline"
                onClick={() => form.reset(defaultValues)}
                disabled={isSaving}
              >
                {t("common.cancel")}
              </Button>
              <Button type="submit" disabled={isSaving}>
                {isSaving ? t("common.loading") : t("auth.settings.form.save")}
              </Button>
            </FormActions>
          ) : null}
        </FormLayout>
      )}
    </AppForm>
  );
}
