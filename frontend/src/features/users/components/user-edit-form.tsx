"use client";

import { Shield, UserRound } from "lucide-react";
import { useMemo } from "react";

import {
  AppForm,
  AppInput,
  AppSelect,
  FormActions,
  FormFieldShell,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { RoleMultiSelect } from "@/features/users/components/role-multi-select";
import { USER_STATUS_VALUES } from "@/features/users/constants";
import type { PlatformUserDetail } from "@/features/users/services/users.service";
import {
  updateUserFormSchema,
  type UpdateUserFormValues,
} from "@/features/users/schemas/user-form";
import { useLocale } from "@/providers/locale-provider";

type UserEditFormProps = {
  user: PlatformUserDetail;
  formId?: string;
  isSubmitting?: boolean;
  onSubmit: (values: UpdateUserFormValues) => Promise<void> | void;
  onCancel: () => void;
};

function normalizeStatus(status: string): UpdateUserFormValues["status"] {
  if (status === "pending" || status === "disabled") return status;
  return "active";
}

export function UserEditForm({
  user,
  formId = "user-edit-form",
  isSubmitting,
  onSubmit,
  onCancel,
}: UserEditFormProps) {
  const { t } = useLocale();
  const schema = updateUserFormSchema(t);

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
        name: user.name,
        surname: user.surname,
        status: normalizeStatus(user.status),
        role_uuids: user.roles?.map((role) => role.uuid) ?? [],
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
            <FormFieldShell
              name="email-readonly"
              label={t("users.fields.email")}
              description={t("users.fields.email_hint")}
              className="sm:col-span-2 sm:max-w-md"
            >
              <Input value={user.email} disabled readOnly />
            </FormFieldShell>
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
              {t("users.actions.save")}
            </Button>
          </FormActions>
        </FormLayout>
      )}
    </AppForm>
  );
}
