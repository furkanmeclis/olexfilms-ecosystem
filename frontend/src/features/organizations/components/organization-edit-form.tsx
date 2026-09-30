"use client";

import { Building2, CalendarClock, MapPin } from "lucide-react";
import { useMemo } from "react";
import { Controller, useFormContext } from "react-hook-form";

import {
  AppForm,
  AppInput,
  AppSelect,
  AppTextarea,
  FormActions,
  FormFieldShell,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { apiConfig } from "@/config/api";
import { ORGANIZATION_STATUS_VALUES } from "@/features/organizations/constants";
import {
  useDeleteOrganizationLogo,
  useUploadOrganizationLogo,
} from "@/features/organizations/hooks/use-organization-mutations";
import {
  updateOrganizationFormSchema,
  type UpdateOrganizationFormValues,
} from "@/features/organizations/schemas/organization-form";
import type { Organization } from "@/features/organizations/services/organizations.service";
import { FilePickButton } from "@/components/common/file-pick-button";
import { useLocale } from "@/providers/locale-provider";

type OrganizationEditFormProps = {
  organization: Organization;
  formId?: string;
  isSubmitting?: boolean;
  onSubmit: (values: UpdateOrganizationFormValues) => Promise<void> | void;
  onCancel: () => void;
};

function LogoField({ organization }: { organization: Organization }) {
  const { t } = useLocale();
  const uploadLogo = useUploadOrganizationLogo();
  const deleteLogo = useDeleteOrganizationLogo();
  const logoSrc = organization.logo_url
    ? `${apiConfig.baseUrl.replace(/\/$/, "")}${organization.logo_url}`
    : null;

  return (
    <FormFieldShell name="logo" label={t("organizations.fields.logo")}>
      <div className="flex flex-wrap items-end gap-3">
        {logoSrc ? (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={logoSrc}
            alt={organization.name}
            className="h-12 w-auto max-w-[160px] object-contain"
          />
        ) : (
          <div className="bg-muted flex h-12 w-24 items-center justify-center rounded text-xs">
            {t("organizations.detail.no_logo")}
          </div>
        )}
        <FilePickButton
          accept="image/png,image/jpeg,image/webp,image/svg+xml"
          hint={t("common.image_formats")}
          pending={uploadLogo.isPending}
          onFile={(file) =>
            uploadLogo.mutate({ uuid: organization.uuid, file })
          }
        />
        {organization.logo_url ? (
          <Button
            type="button"
            variant="outline"
            disabled={deleteLogo.isPending}
            onClick={() => deleteLogo.mutate(organization.uuid)}
          >
            {t("organizations.detail.remove_logo")}
          </Button>
        ) : null}
      </div>
    </FormFieldShell>
  );
}

function AccessEndsField() {
  const { t } = useLocale();
  const form = useFormContext<UpdateOrganizationFormValues>();
  const clearAccess = form.watch("clear_access_ends_at");

  return (
    <div className="space-y-3 sm:col-span-2">
      <Controller
        control={form.control}
        name="access_ends_at"
        render={({ field }) => (
          <FormFieldShell
            name="access_ends_at"
            label={t("organizations.fields.access_ends_at")}
          >
            <DatePicker
              value={clearAccess ? "" : (field.value?.slice(0, 10) ?? "")}
              onChange={(value) =>
                field.onChange(value ? new Date(value).toISOString() : "")
              }
              disabled={clearAccess}
            />
          </FormFieldShell>
        )}
      />
      <Controller
        control={form.control}
        name="clear_access_ends_at"
        render={({ field }) => (
          <label className="flex items-center gap-2 text-sm">
            <Checkbox
              checked={Boolean(field.value)}
              onCheckedChange={(checked) => field.onChange(Boolean(checked))}
            />
            {t("organizations.fields.clear_access_ends_at")}
          </label>
        )}
      />
    </div>
  );
}

export function OrganizationEditForm({
  organization,
  formId = "organization-edit-form",
  isSubmitting,
  onSubmit,
  onCancel,
}: OrganizationEditFormProps) {
  const { t } = useLocale();
  const schema = updateOrganizationFormSchema(t);

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "business",
          label: t("organizations.form.section_business"),
          description: t("organizations.form.section_business_desc"),
          icon: Building2,
        },
        {
          key: "subscription",
          label: t("organizations.form.section_subscription"),
          description: t("organizations.form.section_subscription_desc"),
          icon: CalendarClock,
        },
      ]),
    [t],
  );

  const statusOptions = ORGANIZATION_STATUS_VALUES.map((value) => ({
    value,
    label: t(`organizations.status.${value}`),
  }));

  return (
    <AppForm
      id={formId}
      schema={schema}
      defaultValues={{
        name: organization.name,
        city: organization.city,
        district: organization.district,
        phone: organization.phone,
        address: organization.address,
        status: organization.status,
        plan_code: organization.plan_code ?? "",
        access_ends_at: organization.access_ends_at ?? "",
        clear_access_ends_at: false,
      }}
      onSubmit={onSubmit}
    >
      <FormLayout navItems={sections.navItems}>
        <FormSection
          id={sections.id("business")}
          title={t("organizations.form.section_business")}
          description={t("organizations.form.section_business_desc")}
          columns={2}
        >
          <AppInput
            name="name"
            label={t("organizations.fields.name")}
            placeholder={t("organizations.placeholders.name")}
            startIcon={Building2}
            className="sm:col-span-2"
          />
          <FormFieldShell
            name="slug-readonly"
            label={t("organizations.fields.slug")}
            className="sm:col-span-2 sm:max-w-md"
          >
            <Input value={organization.slug} disabled readOnly />
          </FormFieldShell>
          <AppInput
            name="city"
            label={t("organizations.fields.city")}
            placeholder={t("organizations.placeholders.city")}
          />
          <AppInput
            name="district"
            label={t("organizations.fields.district")}
            placeholder={t("organizations.placeholders.district")}
          />
          <AppInput
            name="phone"
            label={t("organizations.fields.phone")}
            placeholder={t("organizations.placeholders.phone")}
            type="tel"
            startIcon={MapPin}
          />
          <AppTextarea
            name="address"
            label={t("organizations.fields.address")}
            placeholder={t("organizations.placeholders.address")}
            rows={3}
            className="sm:col-span-2"
          />
          <div className="sm:col-span-2">
            <LogoField organization={organization} />
          </div>
        </FormSection>

        <FormSection
          id={sections.id("subscription")}
          title={t("organizations.form.section_subscription")}
          description={t("organizations.form.section_subscription_desc")}
          columns={2}
        >
          <AppSelect
            name="status"
            label={t("organizations.fields.status")}
            options={statusOptions}
          />
          <AppInput
            name="plan_code"
            label={t("organizations.fields.plan_code")}
            placeholder={t("organizations.placeholders.plan_code")}
          />
          <AccessEndsField />
        </FormSection>

        <FormActions>
          <Button
            type="button"
            variant="outline"
            onClick={onCancel}
            disabled={isSubmitting}
          >
            {t("form.cancel")}
          </Button>
          <Button type="submit" disabled={isSubmitting}>
            {t("organizations.actions.save")}
          </Button>
        </FormActions>
      </FormLayout>
    </AppForm>
  );
}
