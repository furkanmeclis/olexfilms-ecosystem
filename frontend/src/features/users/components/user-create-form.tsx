"use client";

import { Mail, Shield, UserRound } from "lucide-react";
import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  AppPassword,
  AppSelect,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { RoleMultiSelect } from "@/features/users/components/role-multi-select";
import { USER_STATUS_VALUES } from "@/features/users/constants";
import {
  createUserFormSchema,
  type CreateUserFormValues,
} from "@/features/users/schemas/user-form";
import { useLocale } from "@/providers/locale-provider";

type UserCreateFormProps = {
  formId?: string;
  isSubmitting?: boolean;
  onSubmit: (values: CreateUserFormValues) => Promise<void> | void;
  onCancel: () => void;
};

export function UserCreateForm({
  formId = "user-create-form",
  isSubmitting,
  onSubmit,
  onCancel,
}: UserCreateFormProps) {
  const { t } = useLocale();
  const schema = createUserFormSchema(t);

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "profile",
          label: t("users.form.section_profile"),
          description: t("users.form.section_profile_desc"),
          icon: UserRound,
        },
        {
          key: "account",
          label: t("users.form.section_account"),
          description: t("users.form.section_account_desc"),
          icon: Mail,
        },
        {
          key: "access",
          label: t("users.form.section_access"),
          description: t("users.form.section_access_desc"),
          icon: Shield,
        },
      ]),
    [t],
  );

  const statusOptions = USER_STATUS_VALUES.map((value) => ({
    value,
    label: t(`users.status.${value}`),
  }));

  return (
    <AppForm
      id={formId}
      schema={schema}
      defaultValues={{
        name: "",
        surname: "",
        email: "",
        password: "",
        status: "active",
        role_uuids: [],
      }}
      onSubmit={onSubmit}
    >
      {(form) => (
        <FormLayout navItems={sections.navItems}>
          <FormSection
            id={sections.id("profile")}
            title={t("users.form.section_profile")}
            description={t("users.form.section_profile_desc")}
            columns={2}
          >
            <AppInput
              name="name"
              label={t("users.fields.name")}
              placeholder={t("users.placeholders.name")}
              startIcon={UserRound}
              autoComplete="given-name"
            />
            <AppInput
              name="surname"
              label={t("users.fields.surname")}
              placeholder={t("users.placeholders.surname")}
              startIcon={UserRound}
              autoComplete="family-name"
            />
          </FormSection>

          <FormSection
            id={sections.id("account")}
            title={t("users.form.section_account")}
            description={t("users.form.section_account_desc")}
            columns={2}
          >
            <AppInput
              name="email"
              type="email"
              label={t("users.fields.email")}
              placeholder={t("users.placeholders.email")}
              startIcon={Mail}
              autoComplete="email"
            />
            <AppPassword
              name="password"
              label={t("users.fields.password")}
              placeholder={t("users.placeholders.password")}
              description={t("users.fields.password_hint")}
              autoComplete="new-password"
            />
          </FormSection>

          <FormSection
            id={sections.id("access")}
            title={t("users.form.section_access")}
            description={t("users.form.section_access_desc")}
            columns={2}
          >
            <AppSelect
              name="status"
              label={t("users.fields.status")}
              options={statusOptions}
            />
            <RoleMultiSelect
              className="sm:col-span-2"
              value={form.watch("role_uuids")}
              onChange={(next) =>
                form.setValue("role_uuids", next, { shouldDirty: true })
              }
              error={form.formState.errors.role_uuids?.message as string}
            />
          </FormSection>

          <FormActions>
            <Button
              type="button"
              variant="outline"
              onClick={onCancel}
              disabled={isSubmitting || form.formState.isSubmitting}
            >
              {t("form.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={isSubmitting || form.formState.isSubmitting}
            >
              {t("users.actions.create")}
            </Button>
          </FormActions>
        </FormLayout>
      )}
    </AppForm>
  );
}
