"use client";

import { ShieldCheck } from "lucide-react";
import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  accessFormSchema,
  type AccessFormValues,
} from "@/features/access/schemas/access-form";
import type { AccessPolicy } from "@/features/access/services/access.service";
import { useLocale } from "@/providers/locale-provider";

type AccessSettingsFormProps = {
  policy: AccessPolicy;
  canWrite: boolean;
  isSaving: boolean;
  onSubmit: (values: AccessFormValues) => Promise<void>;
};

export function AccessSettingsForm({
  policy,
  canWrite,
  isSaving,
  onSubmit,
}: AccessSettingsFormProps) {
  const { t } = useLocale();

  const defaultValues = useMemo<AccessFormValues>(
    () => ({
      ttl_hours: policy.ttl_hours,
      password_enabled: policy.password_enabled,
      passkey_enabled: policy.passkey_enabled,
      totp_enabled: policy.totp_enabled,
      password_login_totp_required: policy.password_login_totp_required,
    }),
    [policy],
  );

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "verification",
          label: t("access.form.section_verification"),
          description: t("access.form.section_verification_hint"),
          icon: ShieldCheck,
        },
      ]),
    [t],
  );

  return (
    <AppForm
      schema={accessFormSchema}
      defaultValues={defaultValues}
      onSubmit={onSubmit}
    >
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("verification")}
            title={t("access.form.section_verification")}
            description={t("access.form.section_verification_hint")}
          >
            <AppInput
              name="ttl_hours"
              label={t("access.form.ttl_hours")}
              description={t("access.form.ttl_hours_hint")}
              type="number"
              min={1}
              max={168}
              disabled={!canWrite || isSaving}
            />

            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="password_enabled">
                  {t("access.form.password_enabled")}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t("access.form.password_enabled_hint")}
                </p>
              </div>
              <Switch
                id="password_enabled"
                checked={form.watch("password_enabled")}
                onCheckedChange={(checked) =>
                  form.setValue("password_enabled", checked, {
                    shouldDirty: true,
                  })
                }
                disabled={!canWrite || isSaving}
              />
            </div>

            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="passkey_enabled">
                  {t("access.form.passkey_enabled")}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t("access.form.passkey_enabled_hint")}
                </p>
              </div>
              <Switch
                id="passkey_enabled"
                checked={form.watch("passkey_enabled")}
                onCheckedChange={(checked) =>
                  form.setValue("passkey_enabled", checked, {
                    shouldDirty: true,
                  })
                }
                disabled={!canWrite || isSaving}
              />
            </div>

            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="totp_enabled">
                  {t("access.form.totp_enabled")}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t("access.form.totp_enabled_hint")}
                </p>
              </div>
              <Switch
                id="totp_enabled"
                checked={form.watch("totp_enabled")}
                onCheckedChange={(checked) =>
                  form.setValue("totp_enabled", checked, {
                    shouldDirty: true,
                  })
                }
                disabled={!canWrite || isSaving}
              />
            </div>

            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="password_login_totp_required">
                  {t("access.form.password_login_totp_required")}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t("access.form.password_login_totp_required_hint")}
                </p>
              </div>
              <Switch
                id="password_login_totp_required"
                checked={form.watch("password_login_totp_required")}
                onCheckedChange={(checked) =>
                  form.setValue("password_login_totp_required", checked, {
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
                {isSaving ? t("common.loading") : t("access.form.save")}
              </Button>
            </FormActions>
          ) : null}
        </FormLayout>
      )}
    </AppForm>
  );
}
