"use client";

import { Building2, MapPin, UserRound } from "lucide-react";
import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { UserAsyncPicker } from "@/features/users/components/user-async-picker";
import {
  createOrganizationFormSchema,
  type CreateOrganizationFormValues,
} from "@/features/organizations/schemas/organization-form";
import { useLocale } from "@/providers/locale-provider";

type OrganizationCreateFormProps = {
  formId?: string;
  isSubmitting?: boolean;
  onSubmit: (values: CreateOrganizationFormValues) => Promise<void> | void;
  onCancel: () => void;
};

export function OrganizationCreateForm({
  formId = "organization-create-form",
  isSubmitting,
  onSubmit,
  onCancel,
}: OrganizationCreateFormProps) {
  const { t } = useLocale();
  const schema = createOrganizationFormSchema(t);

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
          key: "owner",
          label: t("organizations.form.section_owner"),
          description: t("organizations.form.section_owner_desc"),
          icon: UserRound,
        },
      ]),
    [t],
  );

  return (
    <AppForm
      id={formId}
      schema={schema}
      defaultValues={{
        name: "",
        city: "",
        district: "",
        phone: "",
        address: "",
        owner_user_uuid: "",
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
        </FormSection>

        <FormSection
          id={sections.id("owner")}
          title={t("organizations.form.section_owner")}
          description={t("organizations.form.section_owner_desc")}
          columns={1}
        >
          <UserAsyncPicker
            name="owner_user_uuid"
            label={t("organizations.fields.owner")}
            description={t("organizations.fields.owner_hint")}
          />
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
            {t("organizations.actions.create")}
          </Button>
        </FormActions>
      </FormLayout>
    </AppForm>
  );
}
