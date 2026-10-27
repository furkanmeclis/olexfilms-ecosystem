"use client";

import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";
import { useFormContext } from "react-hook-form";

import {
  AppForm,
  AppSelect,
  FormActions,
  FormFieldShell,
} from "@/components/forms";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { ORGANIZATION_MEMBER_ROLES } from "@/features/organizations/constants";
import { useAddOrganizationMember } from "@/features/organizations/hooks/use-organization-mutations";
import {
  addOrganizationMemberFormSchema,
  type AddOrganizationMemberFormValues,
} from "@/features/organizations/schemas/organization-form";
import type { OrganizationType } from "@/features/organizations/services/organizations.service";
import { roleDisplayName } from "@/features/roles/lib/role-display";
import { rolesService } from "@/features/roles/services/roles.service";
import { UserAsyncPicker } from "@/features/users/components/user-async-picker";
import { useLocale } from "@/providers/locale-provider";

type OrganizationAddMemberDialogProps = {
  organizationUuid: string;
  /** TEC-1026: narrows the organization role picker to this org type. */
  organizationType?: OrganizationType;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

/**
 * TEC-1026: organization roles of the org type (center_accounting,
 * dealer_accounting, ...). Empty means the default role of the membership
 * kind (owner → center_owner / distributor_owner / dealer_owner, staff →
 * center_staff / distributor_staff / dealer_staff); a selection replaces it,
 * so an accounting-only account is one that picks only "accounting".
 */
function OrganizationRolesField({
  organizationType,
}: {
  organizationType: OrganizationType;
}) {
  const { t } = useLocale();
  const form = useFormContext<AddOrganizationMemberFormValues>();
  const { data, isLoading } = useQuery({
    queryKey: ["platform", "roles", "picker", organizationType],
    queryFn: () =>
      rolesService.list({ limit: 100, offset: 0, is_system: "true" }),
  });
  const roles = useMemo(
    () =>
      (data?.items ?? []).filter(
        (role) => role.org_type === organizationType,
      ),
    [data, organizationType],
  );
  const value = form.watch("role_slugs") ?? [];
  const selected = new Set(value);
  const toggle = (slug: string, checked: boolean) => {
    const next = checked
      ? [...value, slug]
      : value.filter((item) => item !== slug);
    form.setValue("role_slugs", next, { shouldDirty: true });
  };

  return (
    <FormFieldShell
      name="role_slugs"
      label={t("organizations.fields.member_roles")}
      description={t("organizations.fields.member_roles_hint")}
    >
      <div
        className="space-y-2 rounded-md border p-3"
        data-testid="member-role-slugs"
      >
        {isLoading ? (
          <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
        ) : null}
        {roles.map((role) => (
          <div key={role.slug} className="flex items-start gap-2">
            <Checkbox
              id={`member-role-${role.slug}`}
              checked={selected.has(role.slug)}
              onCheckedChange={(checked) =>
                toggle(role.slug, checked === true)
              }
            />
            <Label
              htmlFor={`member-role-${role.slug}`}
              className="cursor-pointer leading-snug font-normal"
            >
              <span className="font-medium">{roleDisplayName(role, t)}</span>
              <span className="text-muted-foreground ms-2 font-mono text-xs">
                {role.slug}
              </span>
            </Label>
          </div>
        ))}
      </div>
    </FormFieldShell>
  );
}

export function OrganizationAddMemberDialog({
  organizationUuid,
  organizationType,
  open,
  onOpenChange,
}: OrganizationAddMemberDialogProps) {
  const { t } = useLocale();
  const addMember = useAddOrganizationMember();
  const schema = addOrganizationMemberFormSchema(t);

  const roleOptions = useMemo(
    () =>
      ORGANIZATION_MEMBER_ROLES.map((role) => ({
        value: role,
        label: t(`organizations.roles.${role}`),
      })),
    [t],
  );

  const handleSubmit = async (values: AddOrganizationMemberFormValues) => {
    await addMember.mutateAsync({
      uuid: organizationUuid,
      body: {
        user_uuid: values.user_uuid,
        role: values.role,
        ...(values.role_slugs && values.role_slugs.length > 0
          ? { role_slugs: values.role_slugs }
          : {}),
      },
    });
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("organizations.member_dialog.title")}</DialogTitle>
          <DialogDescription>
            {t("organizations.member_dialog.description")}
          </DialogDescription>
        </DialogHeader>
        <AppForm
          schema={schema}
          defaultValues={{ user_uuid: "", role: "staff", role_slugs: [] }}
          onSubmit={handleSubmit}
        >
          <div className="space-y-4">
            <UserAsyncPicker
              name="user_uuid"
              label={t("organizations.fields.member")}
            />
            <AppSelect
              name="role"
              label={t("organizations.fields.member_role")}
              options={roleOptions}
            />
            {organizationType ? (
              <OrganizationRolesField organizationType={organizationType} />
            ) : null}
            <FormActions>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={addMember.isPending}
              >
                {t("form.cancel")}
              </Button>
              <Button type="submit" disabled={addMember.isPending}>
                {t("organizations.actions.add_member")}
              </Button>
            </FormActions>
          </div>
        </AppForm>
      </DialogContent>
    </Dialog>
  );
}
