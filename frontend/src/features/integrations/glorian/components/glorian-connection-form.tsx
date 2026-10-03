"use client";

import { useMemo } from "react";

import { AppForm, AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  glorianFormSchema,
  toFormValues,
  type GlorianFormValues,
} from "@/features/integrations/glorian/lib/form";
import type { GlorianConnection } from "@/features/integrations/glorian/services/glorian.service";
import { useLocale } from "@/providers/locale-provider";

type Props = {
  connection: GlorianConnection;
  canManage: boolean;
  isSaving: boolean;
  onSubmit: (values: GlorianFormValues) => Promise<void>;
};

/**
 * Connection settings. The API key is write only: the field starts empty,
 * its placeholder is the backend mask when a key is stored, and a blank
 * field keeps the stored key.
 */
export function GlorianConnectionForm({
  connection,
  canManage,
  isSaving,
  onSubmit,
}: Props) {
  const { t } = useLocale();
  const schema = useMemo(
    () => glorianFormSchema(t, connection.api_key_set),
    [t, connection.api_key_set],
  );
  const defaultValues = useMemo(() => toFormValues(connection), [connection]);
  const disabled = !canManage || isSaving;

  return (
    <AppForm
      key={connection.updated_at ?? "new"}
      schema={schema}
      defaultValues={defaultValues}
      onSubmit={onSubmit}
      className="grid gap-4 sm:grid-cols-2"
    >
      {(form) => (
        <>
          <AppInput
            name="base_url"
            label={t("integrations.glorian.form.base_url")}
            description={t("integrations.glorian.form.base_url_hint")}
            placeholder="https://hub.example.com"
            inputMode="url"
            autoComplete="off"
            disabled={disabled}
            className="sm:col-span-2"
          />

          <AppInput
            name="api_key"
            type="password"
            label={t("integrations.glorian.form.api_key")}
            description={
              connection.api_key_set
                ? t("integrations.glorian.form.api_key_keep_hint")
                : t("integrations.glorian.form.api_key_hint")
            }
            placeholder={
              connection.api_key_set
                ? (connection.api_key_masked ?? "********")
                : t("integrations.glorian.form.api_key_placeholder")
            }
            autoComplete="new-password"
            disabled={disabled}
            data-testid="glorian-api-key"
          />

          <AppInput
            name="default_warehouse_uuid"
            label={t("integrations.glorian.form.default_warehouse")}
            description={t("integrations.glorian.form.default_warehouse_hint")}
            autoComplete="off"
            disabled={disabled}
            className="font-mono"
          />

          <div className="flex items-center justify-between gap-4 rounded-lg border p-4 sm:col-span-2">
            <div className="space-y-1">
              <Label htmlFor="glorian-active">
                {t("integrations.glorian.form.active")}
              </Label>
              <p className="text-muted-foreground text-sm">
                {t("integrations.glorian.form.active_hint")}
              </p>
            </div>
            <Switch
              id="glorian-active"
              checked={form.watch("active")}
              onCheckedChange={(checked) =>
                form.setValue("active", checked, {
                  shouldDirty: true,
                  shouldValidate: form.formState.isSubmitted,
                })
              }
              disabled={disabled}
            />
          </div>

          {canManage ? (
            <div className="flex justify-end gap-2 sm:col-span-2">
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
                  : t("integrations.glorian.form.save")}
              </Button>
            </div>
          ) : (
            <p className="text-muted-foreground text-sm sm:col-span-2">
              {t("integrations.glorian.read_only")}
            </p>
          )}
        </>
      )}
    </AppForm>
  );
}
