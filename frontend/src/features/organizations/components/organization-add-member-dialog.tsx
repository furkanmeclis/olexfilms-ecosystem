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
import { ORGANIZATION_MEMBER_ROLES } from "@/features/organizations/constants";
import { useAddOrganizationMember } from "@/features/organizations/hooks/use-organization-mutations";
import {
  addOrganizationMemberFormSchema,
  type AddOrganizationMemberFormValues,
} from "@/features/organizations/schemas/organization-form";
import { UserAsyncPicker } from "@/features/users/components/user-async-picker";
import { useLocale } from "@/providers/locale-provider";

type OrganizationAddMemberDialogProps = {
  organizationUuid: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
};

export function OrganizationAddMemberDialog({
  organizationUuid,
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
          defaultValues={{ user_uuid: "", role: "staff" }}
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
