"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MapPinned, Trash2 } from "lucide-react";
import { useCallback } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  AppCombobox,
  AppForm,
  FormSection,
  type ComboboxOption,
} from "@/components/forms";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  AddressFields,
  addressIds,
} from "@/features/geo/components/address-fields";
import { countryName, geoKeys } from "@/features/geo/hooks/use-geo";
import { geoService } from "@/features/geo/services/geo.service";
import type { Territory } from "@/features/geo/types";
import { organizationsService } from "@/features/organizations/services/organizations.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

function assignSchema(t: Translate) {
  return z.object({
    distributor_uuid: z
      .string()
      .uuid(t("geo.territories.validation.distributor")),
    country_id: z.string().min(1, t("geo.territories.validation.country")),
    province_id: z.string().optional(),
    district_id: z.string().optional(),
  });
}

type AssignValues = z.infer<ReturnType<typeof assignSchema>>;

function areaLabel(item: Territory, locale: string) {
  return [
    countryName(
      { name_en: item.country_name_en, name_tr: item.country_name_tr },
      locale,
    ),
    item.province_name,
    item.district_name,
  ]
    .filter(Boolean)
    .join(" › ");
}

/** Platform admin: distributor territories (K5, one distributor per area). */
export function TerritoriesPage() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.territories.write);
  const queryClient = useQueryClient();
  const schema = assignSchema(t);

  const { data, isLoading, isError, refetch } = useQuery({
    queryKey: geoKeys.territories,
    queryFn: () => geoService.territories(),
  });

  const loadDistributors = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const page = await organizationsService.list({
        limit: 20,
        offset: 0,
        q,
        type: "distributor",
      });
      return page.items.map((o) => ({
        value: o.uuid,
        label: o.name,
        description: o.city,
      }));
    },
    [],
  );

  const assign = useMutation({
    mutationFn: (values: AssignValues) => {
      const ids = addressIds(values);
      return geoService.assignTerritory({
        distributor_uuid: values.distributor_uuid,
        country_id: ids.country_id ?? 0,
        province_id: ids.province_id,
        district_id: ids.district_id,
      });
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: geoKeys.territories });
      appToast.success(t("geo.territories.toast.assigned"));
    },
    onError: (error) => {
      if (isApiError(error) && error.code === "TERRITORY_CONFLICT") {
        appToast.error(t("geo.territories.toast.conflict"));
        return;
      }
      appToast.error(
        isApiError(error) ? error.message : t("geo.territories.toast.failed"),
      );
    },
  });

  const remove = useMutation({
    mutationFn: (uuid: string) => geoService.deleteTerritory(uuid),
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: geoKeys.territories });
      appToast.success(t("geo.territories.toast.removed"));
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("geo.territories.toast.failed"),
      ),
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("geo.territories.title")}
        description={t("geo.territories.description")}
        icon={<MapPinned className="size-6" />}
      />

      {canWrite ? (
        <AppForm
          schema={schema}
          defaultValues={{
            distributor_uuid: "",
            country_id: "",
            province_id: "",
            district_id: "",
          }}
          onSubmit={async (values) => {
            await assign.mutateAsync(values).catch(() => undefined);
          }}
        >
          <FormSection
            id="territory-assign"
            title={t("geo.territories.assign_title")}
            description={t("geo.territories.assign_description")}
            columns={2}
          >
            <AppCombobox
              name="distributor_uuid"
              label={t("geo.territories.distributor")}
              placeholder={t("geo.territories.distributor_placeholder")}
              searchPlaceholder={t("geo.address.search")}
              emptyText={t("geo.address.empty")}
              loadOptions={loadDistributors}
              className="sm:col-span-2"
            />
            <AddressFields />
            <div className="flex items-end justify-end sm:col-span-2">
              <Button type="submit" disabled={assign.isPending}>
                {t("geo.territories.assign")}
              </Button>
            </div>
          </FormSection>
        </AppForm>
      ) : null}

      {isLoading ? <Loading label={t("common.loading")} /> : null}
      {isError ? (
        <ErrorState
          title={t("geo.territories.error")}
          retryLabel={t("common.retry")}
          onRetry={() => refetch()}
        />
      ) : null}
      {data ? (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-muted-foreground">
              <tr>
                <th className="px-3 py-2 text-start font-medium">
                  {t("geo.territories.columns.area")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("geo.territories.columns.level")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("geo.territories.columns.distributor")}
                </th>
                <th className="px-3 py-2" />
              </tr>
            </thead>
            <tbody>
              {data.items.length === 0 ? (
                <tr>
                  <td
                    colSpan={4}
                    className="text-muted-foreground px-3 py-6 text-center"
                  >
                    {t("geo.territories.empty")}
                  </td>
                </tr>
              ) : null}
              {data.items.map((item) => (
                <tr key={item.uuid} className="border-t">
                  <td className="px-3 py-2 font-medium">
                    {areaLabel(item, locale)}
                  </td>
                  <td className="px-3 py-2">
                    <Badge variant="outline">
                      {t(`geo.levels.${item.level}`)}
                    </Badge>
                  </td>
                  <td className="px-3 py-2">{item.organization_name}</td>
                  <td className="px-3 py-2 text-end">
                    {canWrite ? (
                      <Button
                        type="button"
                        variant="ghost"
                        size="icon"
                        aria-label={t("geo.territories.remove")}
                        disabled={remove.isPending}
                        onClick={() => remove.mutate(item.uuid)}
                      >
                        <Trash2 className="size-4" />
                      </Button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </div>
  );
}
