"use client";

import { useMemo } from "react";
import { useRouter } from "next/navigation";
import { useQuery } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { RoleForm } from "@/features/roles/components/role-form";
import { rolesKeys } from "@/features/roles/hooks/query-keys";
import { useUpdateRole } from "@/features/roles/hooks/use-role-mutations";
import type { UpdateRoleFormValues } from "@/features/roles/schemas/role-form";
import { rolesService } from "@/features/roles/services/roles.service";
import { useLocale } from "@/providers/locale-provider";

export function RoleEditPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
  const router = useRouter();
  const updateRole = useUpdateRole();

  const roleQuery = useQuery({
    queryKey: rolesKeys.detail(uuid),
    queryFn: () => rolesService.get(uuid),
  });

  const defaults = useMemo<UpdateRoleFormValues | undefined>(
    () =>
      roleQuery.data
        ? {
            name: roleQuery.data.name,
            slug: roleQuery.data.slug,
            description: roleQuery.data.description ?? "",
            permission_slugs: roleQuery.data.permission_slugs ?? [],
          }
        : undefined,
    [roleQuery.data],
  );

  const title = roleQuery.data?.name ?? t("roles.edit_title");

  return (
    <EntityPage
      title={t("roles.edit_title")}
      description={t("roles.edit_description")}
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
        { label: title, href: routes.platform.roles.detail(uuid) },
        { label: t("roles.actions.edit") },
      ]}
    >
      {roleQuery.isLoading ? <Loading label={t("common.loading")} /> : null}
      {roleQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("roles.detail_not_found")}
          onRetry={() => void roleQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {roleQuery.data?.is_system ? (
        <ErrorState
          title={t("roles.system_role_title")}
          description={t("roles.system_role_edit_forbidden")}
        />
      ) : null}
      {defaults && !roleQuery.data?.is_system ? (
        <EntityForm>
          <RoleForm
            key={uuid}
            mode="edit"
            defaultValues={defaults}
            isSubmitting={updateRole.isPending}
            onCancel={() => router.push(routes.platform.roles.detail(uuid))}
            onSubmit={async (values) => {
              await updateRole.mutateAsync({
                uuid,
                body: {
                  name: values.name,
                  description: values.description,
                  permission_slugs: values.permission_slugs,
                },
              });
              router.push(routes.platform.roles.detail(uuid));
            }}
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}
