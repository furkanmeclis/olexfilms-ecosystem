"use client";

import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { EntityForm, EntityPage } from "@/components/entity";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { addressIds } from "@/features/geo";
import { OrganizationCreateForm } from "@/features/organizations/components/organization-create-form";
import { useCreateOrganization } from "@/features/organizations/hooks/use-organization-mutations";
import type { CreateOrganizationFormValues } from "@/features/organizations/schemas/organization-form";
import type { CreatePlatformOrganizationRequest } from "@/features/organizations/services/organizations.service";
import { useLocale } from "@/providers/locale-provider";

function toCreateBody(
  values: CreateOrganizationFormValues,
): CreatePlatformOrganizationRequest {
  return {
    name: values.name.trim(),
    city: values.city.trim(),
    district: values.district.trim(),
    phone: values.phone.trim(),
    address: values.address.trim(),
    owner_user_uuid: values.owner_user_uuid,
    type: values.type,
    ...addressIds(values),
  };
}

export function OrganizationCreatePage() {
  const { t } = useLocale();
  const router = useRouter();
  const createOrganization = useCreateOrganization();

  const handleSubmit = async (values: CreateOrganizationFormValues) => {
    const organization = await createOrganization.mutateAsync(
      toCreateBody(values),
    );
    router.push(routes.platform.organizations.detail(organization.uuid));
  };

  return (
    <EntityPage
      title={t("organizations.create_title")}
      description={t("organizations.create_description")}
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
        { label: t("organizations.create_title") },
      ]}
    >
      <EntityForm>
        <OrganizationCreateForm
          isSubmitting={createOrganization.isPending}
          onSubmit={handleSubmit}
          onCancel={() => router.push(routes.platform.organizations.root)}
        />
      </EntityForm>
    </EntityPage>
  );
}
