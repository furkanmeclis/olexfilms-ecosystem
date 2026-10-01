"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { addressIds } from "@/features/geo";
import { OrganizationEditForm } from "@/features/organizations/components/organization-edit-form";
import { useUpdateOrganization } from "@/features/organizations/hooks/use-organization-mutations";
import { useOrganization } from "@/features/organizations/hooks/use-organizations-query";
import type { UpdateOrganizationFormValues } from "@/features/organizations/schemas/organization-form";
import { useLocale } from "@/providers/locale-provider";

type OrganizationEditPageProps = {
  uuid: string;
};

export function OrganizationEditPage({ uuid }: OrganizationEditPageProps) {
  const { t } = useLocale();
  const router = useRouter();
  const organizationQuery = useOrganization(uuid);
  const updateOrganization = useUpdateOrganization();

  const handleSubmit = async (values: UpdateOrganizationFormValues) => {
    await updateOrganization.mutateAsync({
      uuid,
      body: {
        name: values.name,
        city: values.city,
        district: values.district,
        phone: values.phone,
        address: values.address,
        // The parent does not follow the address (K25: admin moves dealers).
        ...addressIds(values),
        status: values.status,
        plan_code: values.plan_code?.trim() || undefined,
        ...(values.clear_access_ends_at
          ? { clear_access_ends_at: true }
          : values.access_ends_at
            ? { access_ends_at: values.access_ends_at }
            : {}),
      },
    });
    router.push(routes.platform.organizations.detail(uuid));
  };

  const title =
    organizationQuery.data?.organization.name ?? t("organizations.edit_title");

  return (
    <EntityPage
      title={t("organizations.edit_title")}
      description={t("organizations.edit_description")}
      permission={permissions.organizations.write}
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
        {
          label: title,
          href: routes.platform.organizations.detail(uuid),
        },
        { label: t("organizations.actions.edit") },
      ]}
    >
      {organizationQuery.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : null}
      {organizationQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("organizations.detail_not_found")}
          onRetry={() => void organizationQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {organizationQuery.data?.organization ? (
        <EntityForm>
          <OrganizationEditForm
            key={organizationQuery.data.organization.uuid}
            organization={organizationQuery.data.organization}
            isSubmitting={updateOrganization.isPending}
            onSubmit={handleSubmit}
            onCancel={() =>
              router.push(routes.platform.organizations.detail(uuid))
            }
          />
        </EntityForm>
      ) : null}
    </EntityPage>
  );
}
