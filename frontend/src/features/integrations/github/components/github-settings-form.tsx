"use client";

import { useMemo } from "react";

import { githubNavIcon } from "@/components/icons/github-icon";
import {
  AppForm,
  AppInput,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  githubSettingsFormSchema,
  type GitHubSettingsFormValues,
} from "@/features/integrations/github/schemas/github-settings-form";
import type { GitHubIntegrationSettings } from "@/features/integrations/github/services/github-integration.service";
import { useLocale } from "@/providers/locale-provider";

type GitHubSettingsFormProps = {
  settings: GitHubIntegrationSettings;
  canWrite: boolean;
  isSaving: boolean;
  callbackUrl: string;
  onSubmit: (values: GitHubSettingsFormValues) => Promise<void>;
};

export function GitHubSettingsForm({
  settings,
  canWrite,
  isSaving,
  callbackUrl,
  onSubmit,
}: GitHubSettingsFormProps) {
  const { t } = useLocale();

  const defaultValues = useMemo<GitHubSettingsFormValues>(
    () => ({
      enabled: settings.enabled,
      register_enabled: settings.register_enabled,
      app_id: settings.app_id,
      client_id: settings.client_id,
      client_secret: "",
      private_key: "",
    }),
    [settings],
  );

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "github",
          label: t("integrations.github.form.section_title"),
          description: t("integrations.github.form.section_hint"),
          icon: githubNavIcon,
        },
      ]),
    [t],
  );

  return (
    <AppForm
      schema={githubSettingsFormSchema}
      defaultValues={defaultValues}
      onSubmit={onSubmit}
    >
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("github")}
            title={t("integrations.github.form.section_title")}
            description={t("integrations.github.form.section_hint")}
          >
            <Alert className="sm:col-span-2">
              <AlertDescription>
                {t("integrations.github.form.callback_hint", {
                  url: callbackUrl,
                })}
              </AlertDescription>
            </Alert>

            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="enabled">
                  {t("integrations.github.form.enabled")}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t("integrations.github.form.enabled_hint")}
                </p>
              </div>
              <Switch
                id="enabled"
                checked={form.watch("enabled")}
                onCheckedChange={(checked) =>
                  form.setValue("enabled", checked, { shouldDirty: true })
                }
                disabled={!canWrite || isSaving}
              />
            </div>

            <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
              <div className="space-y-1">
                <Label htmlFor="register_enabled">
                  {t("integrations.github.form.register_enabled")}
                </Label>
                <p className="text-muted-foreground text-sm">
                  {t("integrations.github.form.register_enabled_hint")}
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
              name="app_id"
              label={t("integrations.github.form.app_id")}
              description={t("integrations.github.form.app_id_hint")}
              disabled={!canWrite || isSaving}
            />

            <AppInput
              name="client_id"
              label={t("integrations.github.form.client_id")}
              description={t("integrations.github.form.client_id_hint")}
              disabled={!canWrite || isSaving}
            />

            <AppInput
              name="client_secret"
              label={t("integrations.github.form.client_secret")}
              description={t("integrations.github.form.client_secret_hint")}
              type="password"
              placeholder={
                settings.client_secret_configured
                  ? t("integrations.github.form.secret_configured")
                  : t("integrations.github.form.secret_placeholder")
              }
              disabled={!canWrite || isSaving}
            />

            <AppTextarea
              name="private_key"
              label={t("integrations.github.form.private_key")}
              description={t("integrations.github.form.private_key_hint")}
              rows={8}
              placeholder={
                settings.private_key_configured
                  ? t("integrations.github.form.secret_configured")
                  : t("integrations.github.form.private_key_placeholder")
              }
              disabled={!canWrite || isSaving}
              className="font-mono text-xs sm:col-span-2"
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
                {isSaving
                  ? t("common.loading")
                  : t("integrations.github.form.save")}
              </Button>
            </FormActions>
          ) : null}
        </FormLayout>
      )}
    </AppForm>
  );
}
