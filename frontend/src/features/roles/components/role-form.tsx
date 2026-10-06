"use client";

import { Shield } from "lucide-react";
import { useEffect, useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import type { UseFormReturn } from "react-hook-form";

import { Loading } from "@/components/common/loading";
import {
  AppForm,
  AppInput,
  AppTextarea,
  FormActions,
  FormLayout,
  FormSection,
  createFormSections,
} from "@/components/forms";
import { PermissionSelector } from "@/components/permissions";
import type { Permission } from "@/components/permissions/permission-groups";
import { Button } from "@/components/ui/button";
import { rolesKeys } from "@/features/roles/hooks/query-keys";
import {
  createRoleFormSchema,
  updateRoleFormSchema,
  type CreateRoleFormValues,
  type UpdateRoleFormValues,
} from "@/features/roles/schemas/role-form";
import { rolesService } from "@/features/roles/services/roles.service";
import { useLocale } from "@/providers/locale-provider";

type RoleFormValues = CreateRoleFormValues | UpdateRoleFormValues;

type RoleFormProps = {
  mode?: "create" | "edit";
  defaultValues?: RoleFormValues;
  isSubmitting?: boolean;
  onSubmit: (values: RoleFormValues) => Promise<void> | void;
  onCancel: () => void;
};

function slugifyName(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "_")
    .replace(/^_+|_+$/g, "");
}

function RoleFormFields({
  form,
  mode,
  isSubmitting,
  onCancel,
  permissionItems,
  permissionsLoading,
}: {
  form: UseFormReturn<RoleFormValues>;
  mode: "create" | "edit";
  isSubmitting?: boolean;
  onCancel: () => void;
  permissionItems: Permission[];
  permissionsLoading: boolean;
}) {
  const { t } = useLocale();
  const name = form.watch("name");

  const sections = useMemo(
    () =>
      createFormSections([
        {
          key: "details",
          label: t("roles.form.section_details"),
          description: t("roles.form.section_details_desc"),
          icon: Shield,
        },
        {
          key: "permissions",
          label: t("roles.fields.permissions"),
          description: t("roles.form.section_permissions_desc"),
          icon: Shield,
        },
      ]),
    [t],
  );

  useEffect(() => {
    if (mode !== "create") return;
    if (form.formState.dirtyFields.slug) return;
    const next = slugifyName(name);
    if (next && form.getValues("slug") !== next) {
      form.setValue("slug", next, { shouldDirty: false });
    }
  }, [form, mode, name]);

  return (
    <FormLayout navItems={sections.navItems}>
      <FormSection
        id={sections.id("details")}
        title={t("roles.form.section_details")}
        description={t("roles.form.section_details_desc")}
        columns={2}
      >
        <AppInput name="name" label={t("roles.fields.name")} />
        <AppInput
          name="slug"
          label={t("roles.fields.slug")}
          description={t("roles.fields.slug_hint")}
          disabled={mode === "edit"}
        />
        <AppTextarea
          name="description"
          label={t("roles.fields.description")}
          className="sm:col-span-2"
        />
      </FormSection>

      <FormSection
        id={sections.id("permissions")}
        title={t("roles.fields.permissions")}
        description={t("roles.form.section_permissions_desc")}
      >
        {permissionsLoading ? (
          <Loading label={t("common.loading")} />
        ) : (
          <PermissionSelector
            permissions={permissionItems}
            value={form.watch("permission_slugs")}
            onChange={(next) =>
              form.setValue("permission_slugs", next, { shouldDirty: true })
            }
            scopes={form.watch("grants") ?? {}}
            onScopesChange={(next) =>
              form.setValue("grants", next, { shouldDirty: true })
            }
          />
        )}
      </FormSection>

      <FormActions>
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("form.cancel")}
        </Button>
        <Button
          type="submit"
          disabled={isSubmitting || form.formState.isSubmitting}
        >
          {mode === "create"
            ? t("roles.actions.create")
            : t("roles.actions.save")}
        </Button>
      </FormActions>
    </FormLayout>
  );
}

export function RoleForm({
  mode = "create",
  defaultValues,
  isSubmitting,
  onSubmit,
  onCancel,
}: RoleFormProps) {
  const { t } = useLocale();
  const schema =
    mode === "create" ? createRoleFormSchema(t) : updateRoleFormSchema(t);

  const permissionsQuery = useQuery({
    queryKey: rolesKeys.permissions(),
    queryFn: () => rolesService.listPermissionCatalog(),
  });

  const permissionItems = useMemo(
    () =>
      (permissionsQuery.data?.items ?? []).map((p) => ({
        slug: p.slug,
        name: p.name,
        module: p.module,
        scopes: p.scopes,
        is_sensitive: p.is_sensitive,
        super_admin_only: p.super_admin_only,
        description: p.description ?? undefined,
      })),
    [permissionsQuery.data],
  );

  const resolvedDefaults = defaultValues ?? {
    name: "",
    slug: "",
    description: "",
    permission_slugs: [],
    grants: {},
  };

  return (
    <AppForm
      schema={schema}
      defaultValues={resolvedDefaults}
      onSubmit={onSubmit}
    >
      {(form) => (
        <RoleFormFields
          form={form}
          mode={mode}
          isSubmitting={isSubmitting}
          onCancel={onCancel}
          permissionItems={permissionItems}
          permissionsLoading={permissionsQuery.isLoading}
        />
      )}
    </AppForm>
  );
}
