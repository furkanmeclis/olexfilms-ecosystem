"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { UserCreateForm } from "@/features/users/components/user-create-form";
import { useCreateUser } from "@/features/users/hooks/use-user-mutations";
import type { CreateUserFormValues } from "@/features/users/schemas/user-form";
import type { CreatePlatformUserRequest } from "@/features/users/services/users.service";
import { useLocale } from "@/providers/locale-provider";

function toCreateBody(values: CreateUserFormValues): CreatePlatformUserRequest {
  return {
    email: values.email.trim(),
    password: values.password,
    name: values.name.trim(),
    surname: values.surname.trim(),
    status: values.status,
    role_uuids: values.role_uuids,
  };
}

export function UserCreatePage() {
  const { t } = useLocale();
  const router = useRouter();
  const createUser = useCreateUser();

  const handleSubmit = async (values: CreateUserFormValues) => {
    const user = await createUser.mutateAsync(toCreateBody(values));
    router.push(routes.platform.users.detail(user.uuid));
  };

  return (
    <EntityPage
      title={t("users.create_title")}
      description={t("users.create_description")}
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
        { label: t("users.create_title") },
      ]}
    >
      <EntityForm>
        <UserCreateForm
          isSubmitting={createUser.isPending}
          onSubmit={handleSubmit}
          onCancel={() => router.push(routes.platform.users.root)}
        />
      </EntityForm>
    </EntityPage>
  );
}
