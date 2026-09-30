"use client";

import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { oauthProviderIcons } from "@/features/integrations/oauth/oauth-provider-icons";
import {
  oauthProviderFormSchema,
  type OAuthProviderFormValues,
} from "@/features/integrations/oauth/schemas/oauth-provider-form";
import type {
  OAuthProviderSettings,
  OAuthProviderSlug,
} from "@/features/integrations/oauth/services/oauth-provider.service";
import { useLocale } from "@/providers/locale-provider";

type OAuthProviderSettingsFormProps = {
  provider: OAuthProviderSlug;
  settings: OAuthProviderSettings;
  canWrite: boolean;
  isSaving: boolean;
  callbackUrl: string;
  onSubmit: (values: OAuthProviderFormValues) => Promise<void>;
};

export function OAuthProviderSettingsForm({
  provider,
  settings,
  canWrite,
  isSaving,
  callbackUrl,
  onSubmit,
}: OAuthProviderSettingsFormProps) {
  const Icon = oauthProviderIcons[provider];
  const { t } = useLocale();
  const ns = `integrations.${provider}`;

  const defaultValues = useMemo<OAuthProviderFormValues>(
    () => ({
      login_enabled: settings.login_enabled,
      register_enabled: settings.register_enabled,
      client_id: settings.client_id,
      client_secret: "",
    }),
    [settings],
  );

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: provider,
          label: t(`${ns}.form.section_title`),
          description: t(`${ns}.form.section_hint`),
          icon: Icon,
        },
      ]),
    [Icon, ns, provider, t],
  );

  return (
    <AppForm
      schema={oauthProviderFormSchema}
      defaultValues={defaultValues}
      onSubmit={onSubmit}
    >
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id(provider)}
            title={t(`${ns}.form.section_title`)}
            description={t(`${ns}.form.section_hint`)}
          >
            <Alert className="sm:col-span-2">
              <AlertDescription>
                {t(`${ns}.form.callback_hint`, { url: callbackUrl })}
              </AlertDescription>
            </Alert>

            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="login_enabled">
                  {t(`${ns}.form.login_enabled`)}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t(`${ns}.form.login_enabled_hint`)}
                </p>
              </div>
              <Switch
                id="login_enabled"
                checked={form.watch("login_enabled")}
                onCheckedChange={(checked) =>
                  form.setValue("login_enabled", checked, { shouldDirty: true })
                }
                disabled={!canWrite || isSaving}
              />
            </div>

            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="register_enabled">
                  {t(`${ns}.form.register_enabled`)}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t(`${ns}.form.register_enabled_hint`)}
                </p>
              </div>
              <Switch
                id="register_enabled"
                checked={form.watch("register_enabled")}
                onCheckedChange={(checked) =>
                  form.setValue("register_enabled", checked, {
                    shouldDirty: true,
                  })
                }
                disabled={!canWrite || isSaving}
              />
            </div>

            <AppInput
              name="client_id"
              label={t(`${ns}.form.client_id`)}
              description={t(`${ns}.form.client_id_hint`)}
              disabled={!canWrite || isSaving}
            />

            <AppInput
              name="client_secret"
              label={t(`${ns}.form.client_secret`)}
              description={t(`${ns}.form.client_secret_hint`)}
              type="password"
              placeholder={
                settings.client_secret_configured
                  ? t(`${ns}.form.secret_configured`)
                  : t(`${ns}.form.secret_placeholder`)
              }
              disabled={!canWrite || isSaving}
            />
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
                {isSaving ? t("common.loading") : t(`${ns}.form.save`)}
              </Button>
            </FormActions>
          ) : null}
        </FormLayout>
      )}
    </AppForm>
  );
}
