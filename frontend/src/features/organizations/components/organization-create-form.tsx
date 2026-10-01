"use client";

import { Building2, MapPin, Map as MapIcon, UserRound } from "lucide-react";
import { useMemo } from "react";

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
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { AddressFields } from "@/features/geo";
import { UserAsyncPicker } from "@/features/users/components/user-async-picker";
import {
  createOrganizationFormSchema,
  type CreateOrganizationFormValues,
} from "@/features/organizations/schemas/organization-form";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

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
  const { can } = usePermission();
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
          key: "address",
          label: t("organizations.form.section_address"),
          description: t("organizations.form.section_address_desc"),
          icon: MapIcon,
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
        type: "dealer",
        country_id: "",
        province_id: "",
        district_id: "",
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
          <AppSelect
            name="type"
            label={t("organizations.fields.type")}
            options={[
              { value: "dealer", label: t("organizations.types.dealer") },
              {
                value: "distributor",
                label: t("organizations.types.distributor"),
              },
            ]}
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
          id={sections.id("address")}
          title={t("organizations.form.section_address")}
          description={t("organizations.form.section_address_desc")}
          columns={2}
        >
          <AddressFields showTerritory={can(permissions.territories.read)} />
          <AppInput
            name="city"
            label={t("organizations.fields.city")}
            placeholder={t("organizations.placeholders.city")}
            description={t("organizations.fields.city_hint")}
          />
          <AppInput
            name="district"
            label={t("organizations.fields.district")}
            placeholder={t("organizations.placeholders.district")}
            description={t("organizations.fields.district_hint")}
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
