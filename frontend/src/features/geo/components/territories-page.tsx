"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { MapPinned, Trash2 } from "lucide-react";
import type { ColumnDef } from "@tanstack/react-table";
import { useCallback, useMemo } from "react";
import { z } from "zod";

import {
  CLIENT_SIDE_MANUAL,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import {
  AppCombobox,
  AppForm,
  FormSection,
  type ComboboxOption,
} from "@/components/forms";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
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

export const TERRITORIES_PERSIST_KEY = "platform-territories-v1";
const TERRITORY_LEVELS = ["country", "province", "district"] as const;

function uniqueOptions(options: { value: string; label: string }[]) {
  const seen = new Map<string, string>();
  for (const option of options) {
    if (!seen.has(option.value)) seen.set(option.value, option.label);
  }
  return [...seen.entries()]
    .map(([value, label]) => ({ value, label }))
    .sort((a, b) => a.label.localeCompare(b.label));
}

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

  const { data, isLoading, isError, isFetching, refetch } = useQuery({
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

  const removeTerritory = remove.mutate;
  const removePending = remove.isPending;
  const columns = useMemo<ColumnDef<Territory, unknown>[]>(() => {
    const items = data?.items ?? [];
    const countries = uniqueOptions(
      items.map((item) => ({
        value: item.country_iso2,
        label: countryName(
          { name_en: item.country_name_en, name_tr: item.country_name_tr },
          locale,
        ),
      })),
    );
    const distributors = uniqueOptions(
      items.map((item) => ({
        value: item.organization_name,
        label: item.organization_name,
      })),
    );
    return [
      createColumn<Territory>({
        id: "area",
        accessorFn: (row) => areaLabel(row, locale),
        labelKey: "geo.territories.columns.area",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ getValue }) => (
          <span className="font-medium">{String(getValue())}</span>
        ),
      }),
      createColumn<Territory>({
        accessorKey: "country_iso2",
        labelKey: "geo.territories.columns.country",
        enableSorting: true,
        filterVariant: "select",
        filterFn: "equalsString",
        filterOptions: countries,
        defaultHidden: true,
      }),
      createColumn<Territory>({
        accessorKey: "level",
        labelKey: "geo.territories.columns.level",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: TERRITORY_LEVELS.map((value) => ({
          value,
          labelKey: `geo.levels.${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <Badge variant="outline">
            {t(`geo.levels.${row.original.level}`)}
          </Badge>
        ),
      }),
      createColumn<Territory>({
        id: "distributor",
        // Name (not uuid) so the toolbar search matches distributors too.
        accessorFn: (row) => row.organization_name,
        labelKey: "geo.territories.columns.distributor",
        enableSorting: false,
        filterVariant: "select",
        filterFn: "equalsString",
        filterOptions: distributors,
        gridSecondary: true,
        cell: ({ row }) => row.original.organization_name,
      }),
      createColumn<Territory>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => (
          <EntityRowActions
            actions={[
              {
                id: "remove",
                label: t("geo.territories.remove"),
                icon: Trash2,
                variant: "destructive",
                permission: permissions.territories.write,
                disabled: removePending,
                onSelect: () => removeTerritory(row.original.uuid),
              },
            ]}
          />
        ),
      }),
    ];
  }, [data?.items, locale, removePending, removeTerritory, t]);

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

      <EntityTable
        columns={columns}
        data={data?.items ?? []}
        getRowId={(row) => row.uuid}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={isLoading}
        isError={isError}
        errorTitle={t("geo.territories.error")}
        onRetry={() => void refetch()}
        emptyTitle={t("geo.territories.empty")}
        emptyDescription=""
        initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
        pageSizeOptions={[20, 50, 100]}
        features={{
          persistKey: TERRITORIES_PERSIST_KEY,
          // No bulk delete endpoint.
          rowSelection: false,
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void refetch()}
            refreshDisabled={isFetching}
          />
        }
      />
    </div>
  );
}
