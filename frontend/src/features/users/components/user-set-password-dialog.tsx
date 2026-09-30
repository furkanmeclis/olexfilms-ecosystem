"use client";

import { useMemo } from "react";

import { AppDialog } from "@/components/dialogs/app-dialog";
import { AppForm, AppPassword, FormActions } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { useSetUserPassword } from "@/features/users/hooks/use-user-mutations";
import {
  setUserPasswordFormSchema,
  type SetUserPasswordFormValues,
} from "@/features/users/schemas/user-form";
import { useLocale } from "@/providers/locale-provider";

type UserSetPasswordDialogProps = {
  open: boolean;
  userUuid: string | null;
  userLabel?: string;
  onOpenChange: (open: boolean) => void;
};

export function UserSetPasswordDialog({
  open,
  userUuid,
  userLabel,
  onOpenChange,
}: UserSetPasswordDialogProps) {
  const { t } = useLocale();
  const setPassword = useSetUserPassword();
  const schema = useMemo(() => setUserPasswordFormSchema(t), [t]);

  const handleSubmit = async (values: SetUserPasswordFormValues) => {
    if (!userUuid) return;
    await setPassword.mutateAsync({
      uuid: userUuid,
      body: { password: values.password },
    });
    onOpenChange(false);
  };

  return (
    <AppDialog
      open={open}
      onOpenChange={(next) => {
        if (!setPassword.isPending) onOpenChange(next);
      }}
      title={t("users.password_dialog.title")}
      description={
        userLabel
          ? t("users.password_dialog.description_named", { name: userLabel })
          : t("users.password_dialog.description")
      }
    >
      {open && userUuid ? (
        <AppForm
          key={userUuid}
          schema={schema}
          defaultValues={{ password: "" }}
          onSubmit={handleSubmit}
          className="space-y-4"
        >
          {(form) => (
            <>
              <AppPassword
                name="password"
                label={t("users.fields.password")}
                placeholder={t("users.placeholders.password")}
                description={t("users.fields.password_hint")}
                autoComplete="new-password"
              />
              <FormActions>
                <Button
                  type="button"
                  variant="outline"
                  disabled={
                    setPassword.isPending || form.formState.isSubmitting
                  }
                  onClick={() => onOpenChange(false)}
                >
                  {t("form.cancel")}
                </Button>
                <Button
                  type="submit"
                  disabled={
                    setPassword.isPending || form.formState.isSubmitting
                  }
                >
                  {t("users.actions.set_password")}
                </Button>
              </FormActions>
            </>
          )}
        </AppForm>
      ) : null}
    </AppDialog>
  );
}
