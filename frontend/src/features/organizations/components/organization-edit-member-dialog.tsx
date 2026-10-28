"use client";

import { useMemo } from "react";

import { AppForm, AppSelect, FormActions } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { OrganizationMemberRolesField } from "@/features/organizations/components/organization-member-roles-field";
import { ORGANIZATION_MEMBER_ROLES } from "@/features/organizations/constants";
import { useUpdateOrganizationMember } from "@/features/organizations/hooks/use-organization-mutations";
import {
  editOrganizationMemberFormSchema,
  type EditOrganizationMemberFormValues,
} from "@/features/organizations/schemas/organization-form";
import type {
  OrganizationMember,
  OrganizationType,
} from "@/features/organizations/services/organizations.service";
import { userFullName } from "@/features/users/lib/user-display";
import { useLocale } from "@/providers/locale-provider";

type OrganizationEditMemberDialogProps = {
  organizationUuid: string;
  organizationType?: OrganizationType;
  /** The member being edited; null closes the dialog. */
  member: OrganizationMember | null;
  onClose: () => void;
};

/**
 * TEC-1036: fixes a member's membership kind (owner / staff) and its
 * organization roles; the current roles come pre-checked.
 */
export function OrganizationEditMemberDialog({
  organizationUuid,
  organizationType,
  member,
  onClose,
}: OrganizationEditMemberDialogProps) {
  const { t } = useLocale();
  const update = useUpdateOrganizationMember();
  const schema = editOrganizationMemberFormSchema();

  const roleOptions = useMemo(
    () =>
      ORGANIZATION_MEMBER_ROLES.map((role) => ({
        value: role,
        label: t(`organizations.roles.${role}`),
      })),
    [t],
  );

  const handleSubmit = async (values: EditOrganizationMemberFormValues) => {
    if (!member) return;
    await update.mutateAsync({
      uuid: organizationUuid,
      userUuid: member.uuid,
      body: { role: values.role, role_slugs: values.role_slugs ?? [] },
    });
    onClose();
  };

  const initialRole = ORGANIZATION_MEMBER_ROLES.find(
    (role) => role === member?.role,
  );

  return (
    <Dialog
      open={member !== null}
      onOpenChange={(open) => {
        if (!open && !update.isPending) onClose();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            {t("organizations.member_edit_dialog.title")}
          </DialogTitle>
          <DialogDescription>
            {member
              ? t("organizations.member_edit_dialog.description", {
                  name: userFullName(member),
                })
              : null}
          </DialogDescription>
        </DialogHeader>
        {member ? (
          <AppForm
            key={member.uuid}
            schema={schema}
            defaultValues={{
              role: initialRole ?? "staff",
              role_slugs: member.role_slugs ?? [],
            }}
            onSubmit={handleSubmit}
          >
            <div className="space-y-4">
              <AppSelect
                name="role"
                label={t("organizations.fields.member_role")}
                options={roleOptions}
              />
              {organizationType ? (
                <OrganizationMemberRolesField
                  organizationType={organizationType}
                />
              ) : null}
              <FormActions>
                <Button
                  type="button"
                  variant="outline"
                  onClick={onClose}
                  disabled={update.isPending}
                >
                  {t("form.cancel")}
                </Button>
                <Button type="submit" disabled={update.isPending}>
                  {t("form.save")}
                </Button>
              </FormActions>
            </div>
          </AppForm>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
