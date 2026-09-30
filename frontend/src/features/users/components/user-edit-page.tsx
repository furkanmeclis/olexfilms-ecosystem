"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { UserEditForm } from "@/features/users/components/user-edit-form";
import { useUpdateUser } from "@/features/users/hooks/use-user-mutations";
import { useUser } from "@/features/users/hooks/use-users-query";
import type { UpdateUserFormValues } from "@/features/users/schemas/user-form";
import { userFullName } from "@/features/users/lib/user-display";
import { useLocale } from "@/providers/locale-provider";

type UserEditPageProps = {
  uuid: string;
};

export function UserEditPage({ uuid }: UserEditPageProps) {
  const { t } = useLocale();
  const router = useRouter();
  const userQuery = useUser(uuid);
  const updateUser = useUpdateUser();

  const handleSubmit = async (values: UpdateUserFormValues) => {
    await updateUser.mutateAsync({
      uuid,
      body: {
        name: values.name,
        surname: values.surname,
        status: values.status,
        role_uuids: values.role_uuids,
      },
    });
    router.push(routes.platform.users.detail(uuid));
  };

  const title = userQuery.data
    ? userFullName(userQuery.data)
    : t("users.edit_title");

  return (
    <EntityPage
      title={t("users.edit_title")}
      description={t("users.edit_description")}
      permission={permissions.users.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("users.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("users.title"), href: routes.platform.users.root },
        {
          label: title,
          href: routes.platform.users.detail(uuid),
        },
        { label: t("users.actions.edit") },
      ]}
    >
      {userQuery.isLoading ? <Loading label={t("common.loading")} /> : null}
      {userQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("users.detail_not_found")}
          onRetry={() => void userQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {userQuery.data ? (
        <EntityForm>
          <UserEditForm
            key={userQuery.data.uuid}
            user={userQuery.data}
            isSubmitting={updateUser.isPending}
            onSubmit={handleSubmit}
            onCancel={() => router.push(routes.platform.users.detail(uuid))}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}
