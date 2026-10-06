"use client";

import { Building2, MapPin, Pencil, Phone } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityDetail,
  EntityHeader,
  EntityPage,
  EntitySectionCard,
} from "@/components/entity";
import { Button } from "@/components/ui/button";
import { apiConfig } from "@/config/api";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ORGANIZATION_STATUS_TONE } from "@/features/organizations/constants";
import { OrganizationAddMemberDialog } from "@/features/organizations/components/organization-add-member-dialog";
import {
  OrganizationChildrenTable,
  OrganizationMembersTable,
} from "@/features/organizations/components/organization-detail-tables";
import { useOrganization } from "@/features/organizations/hooks/use-organizations-query";
import type { OrganizationStatus } from "@/features/organizations/services/organizations.service";
import { useLocale } from "@/providers/locale-provider";

type OrganizationDetailPageProps = {
  uuid: string;
};

function DetailField({ label, value }: { label: string; value: ReactNode }) {
  return (
    <div className="space-y-1">
      <dt className="text-muted-foreground text-xs font-medium">{label}</dt>
      <dd className="text-sm break-words">{value ?? "—"}</dd>
    </div>
  );
}

function statusTone(status: string) {
  if (status in ORGANIZATION_STATUS_TONE) {
    return ORGANIZATION_STATUS_TONE[status as OrganizationStatus];
  }
  return "default" as const;
}

export function OrganizationDetailPage({ uuid }: OrganizationDetailPageProps) {
  const { t, format } = useLocale();
  const router = useRouter();
  const query = useOrganization(uuid);
  const [memberDialogOpen, setMemberDialogOpen] = useState(false);

  const organization = query.data?.organization;
  const members = query.data?.members ?? [];
  const title = organization?.name ?? t("organizations.detail_title");

  const logoSrc = organization?.logo_url
    ? `${apiConfig.baseUrl.replace(/\/$/, "")}${organization.logo_url}`
    : null;

  return (
    <EntityPage
      title={title}
      description={t("organizations.detail_description")}
      permission={permissions.organizations.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("organizations.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("organizations.title"),
          href: routes.platform.organizations.root,
        },
        { label: title },
      ]}
      actions={
        organization ? (
          <EntityActions>
            <PermissionGuard permission={permissions.organizations.write}>
              <Button
                type="button"
                size="sm"
                onClick={() =>
                  router.push(
                    routes.platform.organizations.edit(organization.uuid),
                  )
                }
              >
                <Pencil className="size-4" />
                {t("organizations.actions.edit")}
              </Button>
            </PermissionGuard>
            <PermissionGuard permission={permissions.organizations.write}>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => setMemberDialogOpen(true)}
              >
                {t("organizations.actions.add_member")}
              </Button>
            </PermissionGuard>
          </EntityActions>
        ) : null
      }
    >
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("organizations.detail_not_found")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {organization ? (
        <div className="space-y-6">
          <EntityHeader
            title={organization.name}
            badges={
              <StatusChip
                label={t(`organizations.status.${organization.status}`)}
                tone={statusTone(organization.status)}
              />
            }
            leading={
              logoSrc ? (
                // eslint-disable-next-line @next/next/no-img-element
                <img
                  src={logoSrc}
                  alt={organization.name}
                  className="h-12 w-auto max-w-[160px] object-contain"
                />
              ) : (
                <div className="bg-muted flex size-12 items-center justify-center rounded-lg">
                  <Building2 className="text-muted-foreground size-5" />
                </div>
              )
            }
          >
            <dl className="grid gap-3 sm:grid-cols-2">
              <DetailField
                label={t("organizations.fields.slug")}
                value={
                  <Link
                    href={routes.tenant.home(organization.slug)}
                    className="hover:underline"
                  >
                    /{organization.slug}
                  </Link>
                }
              />
              <DetailField
                label={t("organizations.fields.uuid")}
                value={<code className="text-xs">{organization.uuid}</code>}
              />
            </dl>
          </EntityHeader>

          <EntitySectionCard title={t("organizations.detail.profile")}>
            <EntityDetail
              sections={[
                {
                  id: "profile",
                  fields: [
                    {
                      key: "city",
                      label: t("organizations.fields.city"),
                      value: organization.city,
                    },
                    {
                      key: "district",
                      label: t("organizations.fields.district"),
                      value: organization.district,
                    },
                    {
                      key: "phone",
                      label: t("organizations.fields.phone"),
                      value: (
                        <span className="inline-flex items-center gap-1.5">
                          <Phone className="text-muted-foreground size-3.5" />
                          {organization.phone}
                        </span>
                      ),
                    },
                    {
                      key: "address",
                      label: t("organizations.fields.address"),
                      value: (
                        <span className="inline-flex items-start gap-1.5">
                          <MapPin className="text-muted-foreground mt-0.5 size-3.5 shrink-0" />
                          {organization.address}
                        </span>
                      ),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <EntitySectionCard title={t("organizations.detail.subscription")}>
            <EntityDetail
              sections={[
                {
                  id: "subscription",
                  fields: [
                    {
                      key: "status",
                      label: t("organizations.fields.status"),
                      value: (
                        <StatusChip
                          label={t(
                            `organizations.status.${organization.status}`,
                          )}
                          tone={statusTone(organization.status)}
                        />
                      ),
                    },
                    {
                      key: "plan_code",
                      label: t("organizations.fields.plan_code"),
                      value: organization.plan_code ?? "—",
                    },
                    {
                      key: "access_starts_at",
                      label: t("organizations.fields.access_starts_at"),
                      value: format.dateTime(organization.access_starts_at),
                    },
                    {
                      key: "access_ends_at",
                      label: t("organizations.fields.access_ends_at"),
                      value: organization.access_ends_at
                        ? format.dateTime(organization.access_ends_at)
                        : t("organizations.unlimited_access"),
                    },
                    {
                      key: "created_at",
                      label: t("organizations.fields.created_at"),
                      value: format.dateTime(organization.created_at),
                    },
                  ],
                },
              ]}
            />
          </EntitySectionCard>

          <EntitySectionCard title={t("organizations.detail.members")}>
            <OrganizationMembersTable members={members} />
          </EntitySectionCard>

          {organization.type !== "dealer" ? (
            <EntitySectionCard title={t("organizations.detail.children")}>
              <OrganizationChildrenTable uuid={uuid} />
            </EntitySectionCard>
          ) : null}
        </div>
      ) : null}

      <OrganizationAddMemberDialog
        organizationUuid={uuid}
        open={memberDialogOpen}
        onOpenChange={setMemberDialogOpen}
      />
    </EntityPage>
  );
}
