"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { RoleForm } from "@/features/roles/components/role-form";
import { useCreateRole } from "@/features/roles/hooks/use-role-mutations";
import type { CreateRoleFormValues } from "@/features/roles/schemas/role-form";
import { useLocale } from "@/providers/locale-provider";

export function RoleCreatePage() {
  const { t } = useLocale();
  const router = useRouter();
  const createRole = useCreateRole();

  return (
    <EntityPage
      title={t("roles.create_title")}
      description={t("roles.create_description")}
      permission={permissions.roles.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("roles.forbidden_write")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("roles.title"), href: routes.platform.roles.root },
        { label: t("roles.create_title") },
      ]}
    >
      <EntityForm>
        <RoleForm
          mode="create"
          isSubmitting={createRole.isPending}
          onCancel={() => router.push(routes.platform.roles.root)}
          onSubmit={async (values) => {
            const role = await createRole.mutateAsync(
              values as CreateRoleFormValues,
            );
            router.push(routes.platform.roles.detail(role.uuid));
          }}
        />
      </EntityForm>
    </EntityPage>
  );
}
