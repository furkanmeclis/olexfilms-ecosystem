"use client";

import Link from "next/link";
import { Pencil } from "lucide-react";
import { useQuery } from "@tanstack/react-query";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import {
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { PermissionMatrixView } from "@/components/permissions";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { rolesKeys } from "@/features/roles/hooks/query-keys";
import { rolesService } from "@/features/roles/services/roles.service";
import { useLocale } from "@/providers/locale-provider";

export function RoleDetailPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();

  const roleQuery = useQuery({
    queryKey: rolesKeys.detail(uuid),
    queryFn: () => rolesService.get(uuid),
  });

  const permissionsQuery = useQuery({
    queryKey: rolesKeys.permissions(),
    queryFn: () => rolesService.listPermissionCatalog(),
  });

  const data = roleQuery.data;
  const title = data?.name ?? t("roles.detail_title");

  const grantScopes = new Map(
    (data?.grants ?? []).map((g) => [g.permission, g.scope]),
  );
  const permissionCatalog = (permissionsQuery.data?.items ?? []).map((p) => {
    const scope = grantScopes.get(p.slug);
    return {
      slug: p.slug,
      module: p.module,
      name: scope ? `${p.name} · ${t(`permissions.scopes.${scope}`)}` : p.name,
    };
  });

  return (
    <EntityPage
      title={title}
      description={t("roles.detail_description")}
      permission={permissions.roles.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("roles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        { label: t("roles.title"), href: routes.platform.roles.root },
        { label: title },
      ]}
      actions={
        data && !data.is_system ? (
          <PermissionGuard permission={permissions.roles.write}>
            <Button asChild size="sm">
              <Link href={routes.platform.roles.edit(uuid)}>
                <Pencil className="size-4" />
                {t("roles.actions.edit")}
              </Link>
            </Button>
          </PermissionGuard>
        ) : null
      }
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

      {data ? (
        <div className="space-y-6">
          <EntityHeader
            title={data.name}
            description={data.slug}
            badges={
              data.is_system ? (
                <Badge variant="secondary">{t("roles.labels.system")}</Badge>
              ) : null
            }
          />

          <EntitySectionCard title={t("roles.detail.profile")}>
            <EntityDetail
              sections={[
                {
                  id: "profile",
                  fields: [
                    {
                      key: "name",
                      label: t("roles.fields.name"),
                      value: data.name,
                    },
                    {
                      key: "slug",
                      label: t("roles.fields.slug"),
                      value: <code className="text-xs">{data.slug}</code>,
                    },
                    {
                      key: "description",
                      label: t("roles.fields.description"),
                      value: data.description ?? "—",
                    },
                    {
                      key: "uuid",
                      label: t("roles.fields.uuid"),
                      value: <code className="text-xs">{data.uuid}</code>,
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <EntitySectionCard title={t("roles.fields.permissions")}>
            <PermissionMatrixView
              slugs={data.permission_slugs}
              catalog={permissionCatalog}
              emptyTitle={t("roles.permissions_empty_title")}
              emptyDescription={t("roles.permissions_empty_description")}
            />
          </EntitySectionCard>
        </div>
      ) : null}
    </EntityPage>
  );
}
