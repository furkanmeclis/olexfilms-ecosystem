"use client";

import { Contact, Palette } from "lucide-react";
import { useMemo } from "react";
import {
  Controller,
  useFormContext,
  type UseFormReturn,
} from "react-hook-form";

import {
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { FormFieldShell } from "@/components/forms/form-field";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { brand } from "@/config/brand";
import { apiConfig } from "@/config/api";
import { CoordinatesField } from "@/features/io/components/coordinates-field";
import {
  PAPER_SIZES,
  settingsFormSchema,
  toPatchPayload,
  toSettingsFormValues,
  type SettingsFormValues,
} from "@/features/io/schemas/settings-form";
import type { AppSettings } from "@/features/io/types";
import { FilePickButton } from "@/components/common/file-pick-button";
import { useLocale } from "@/providers/locale-provider";

type SettingsFormProps = {
  settings: AppSettings;
  canWrite: boolean;
  isSaving: boolean;
  isLogoUploading: boolean;
  isLogoRemoving: boolean;
  showLocation?: boolean;
  onSubmit: (payload: ReturnType<typeof toPatchPayload>) => Promise<void>;
  onUploadLogo: (file: File) => void;
  onRemoveLogo: () => void;
};

function SettingsFormFields({
  form,
  settings,
  canWrite,
  isSaving,
  isLogoUploading,
  isLogoRemoving,
  showLocation,
  onCancel,
  onUploadLogo,
  onRemoveLogo,
}: {
  form: UseFormReturn<SettingsFormValues>;
  settings: AppSettings;
  canWrite: boolean;
  isSaving: boolean;
  isLogoUploading: boolean;
  isLogoRemoving: boolean;
  showLocation?: boolean;
  onCancel: () => void;
  onUploadLogo: (file: File) => void;
  onRemoveLogo: () => void;
}) {
  const { t } = useLocale();
  const values = form.watch();

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "branding",
          label: t("settings.form.section_branding"),
          description: t("settings.branding_hint"),
          icon: Palette,
        },
        {
          key: "contact",
          label: t("settings.form.section_contact"),
          description: t("settings.contact_hint"),
          icon: Contact,
        },
      ]),
    [t],
  );

  const logoSrc = settings.logo_url
    ? `${apiConfig.baseUrl.replace(/\/$/, "")}${settings.logo_url}`
    : null;

  const paperOptions = PAPER_SIZES.map((size) => ({
    value: size,
    label: t(`settings.paper_sizes.${size.toLowerCase()}`),
  }));

  return (
    <FormLayout navItems={sections.navItems}>
      <FormSection
        id={sections.id("branding")}
        title={t("settings.form.section_branding")}
        description={t("settings.branding_hint")}
        columns={2}
      >
        <div className="sm:col-span-2">
          <p className="text-muted-foreground mb-2 text-xs font-medium">
            {t("settings.preview_strip")}
          </p>
          <div
            className="rounded-lg border p-4"
            style={{
              borderTopColor: values.primary_color,
              borderTopWidth: 4,
            }}
          >
            <div className="flex items-center gap-4">
              {logoSrc ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={logoSrc}
                  alt={values.company_name}
                  className="h-12 w-auto max-w-[160px] object-contain"
                />
              ) : (
                <div className="bg-muted flex h-12 w-12 items-center justify-center rounded text-xs">
                  {t("settings.no_logo")}
                </div>
              )}
              <div>
                <div className="font-semibold">{values.company_name}</div>
                <div className="text-muted-foreground text-sm">
                  {values.tagline}
                </div>
              </div>
            </div>
          </div>
        </div>

        <AppInput
          name="company_name"
          label={t("settings.company_name")}
          disabled={!canWrite}
        />
        <AppInput
          name="tagline"
          label={t("settings.tagline")}
          disabled={!canWrite}
        />

        <ColorField disabled={!canWrite} label={t("settings.primary_color")} />

        <AppSelect
          name="paper_size"
          label={t("settings.paper_size")}
          placeholder={t("settings.paper_size_placeholder")}
          options={paperOptions}
          disabled={!canWrite}
        />

        <div className="flex flex-wrap items-end gap-2 sm:col-span-2">
          <FormFieldShell name="logo" label={t("settings.upload_logo")}>
            <FilePickButton
              id="logo"
              accept="image/png,image/jpeg,image/webp,image/svg+xml"
              hint={t("common.image_formats")}
              disabled={!canWrite}
              pending={isLogoUploading}
              onFile={onUploadLogo}
            />
          </FormFieldShell>
          {settings.logo_url && canWrite ? (
            <Button
              type="button"
              variant="outline"
              disabled={isLogoRemoving}
              onClick={onRemoveLogo}
            >
              {t("settings.remove_logo")}
            </Button>
          ) : null}
        </div>
      </FormSection>

      <FormSection
        id={sections.id("contact")}
        title={t("settings.form.section_contact")}
        description={t("settings.contact_hint")}
        columns={2}
      >
        <AppTextarea
          name="address"
          label={t("settings.address")}
          disabled={!canWrite}
          className="sm:col-span-2"
        />
        {showLocation ? (
          <>
            <AppInput
              name="city"
              label={t("settings.city")}
              disabled={!canWrite}
            />
            <AppInput
              name="district"
              label={t("settings.district")}
              disabled={!canWrite}
            />
            <CoordinatesField disabled={!canWrite} />
          </>
        ) : null}
        <AppInput
          name="phone"
          label={t("settings.phone")}
          disabled={!canWrite}
        />
        <AppInput
          name="email"
          label={t("settings.email")}
          type="email"
          disabled={!canWrite}
        />
        <AppInput
          name="website"
          label={t("settings.website")}
          disabled={!canWrite}
          className="sm:col-span-2"
        />
        <AppTextarea
          name="footer_text"
          label={t("settings.footer_text")}
          disabled={!canWrite}
          className="sm:col-span-2"
        />
      </FormSection>

      {canWrite ? (
        <FormActions>
          <Button type="button" variant="outline" onClick={onCancel}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" disabled={isSaving}>
            {isSaving ? t("common.loading") : t("settings.save")}
          </Button>
        </FormActions>
      ) : (
        <p className="text-muted-foreground text-sm">
          {t("settings.read_only")}
        </p>
      )}
    </FormLayout>
  );
}

function ColorField({
  label,
  disabled,
}: {
  label: string;
  disabled?: boolean;
}) {
  const { control } = useFormContext<SettingsFormValues>();

  return (
    <Controller
      name="primary_color"
      control={control}
      render={({ field, fieldState }) => (
        <FormFieldShell
          name="primary_color"
          label={label}
          error={fieldState.error?.message}
        >
          <div className="flex items-center gap-2">
            <Input
              type="color"
              value={field.value}
              disabled={disabled}
              className="h-9 w-14 shrink-0 cursor-pointer p-1"
              onChange={(event) => field.onChange(event.target.value)}
            />
            <Input
              value={field.value}
              disabled={disabled}
              className="font-mono uppercase"
              onChange={(event) => {
                const next = event.target.value.trim();
                if (/^#?[0-9A-Fa-f]{0,6}$/.test(next)) {
                  field.onChange(next.startsWith("#") ? next : `#${next}`);
                }
              }}
              onBlur={() => {
                if (!/^#[0-9A-Fa-f]{6}$/.test(field.value)) {
                  field.onChange(brand.colors.primary);
                }
              }}
            />
          </div>
        </FormFieldShell>
      )}
    />
  );
}

export function SettingsForm({
  settings,
  canWrite,
  isSaving,
  isLogoUploading,
  isLogoRemoving,
  showLocation,
  onSubmit,
  onUploadLogo,
  onRemoveLogo,
}: SettingsFormProps) {
  const defaultValues = useMemo(
    () => toSettingsFormValues(settings, brand.colors.primary),
    [settings],
  );

  return (
    <AppForm
      schema={settingsFormSchema}
      defaultValues={defaultValues}
      onSubmit={async (values) => {
        await onSubmit(
          toPatchPayload(values, { withCoordinates: Boolean(showLocation) }),
        );
      }}
    >
      {(form) => (
        <SettingsFormFields
          form={form}
          settings={settings}
          canWrite={canWrite}
          isSaving={isSaving}
          isLogoUploading={isLogoUploading}
          isLogoRemoving={isLogoRemoving}
          showLocation={showLocation}
          onCancel={() => form.reset(defaultValues)}
          onUploadLogo={onUploadLogo}
          onRemoveLogo={onRemoveLogo}
        />
      )}
    </AppForm>
  );
}
