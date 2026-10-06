"use client";

import { useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { CircleCheck, CircleOff, Pencil, Trash2 } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityActions,
  EntityCreateButton,
  EntityHeader,
  EntityPage,
  EntityRowActions,
  EntitySectionCard,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  useBulkSelection,
  type BulkActionDef,
} from "@/features/bulk-engine";
import { VehicleBrandDialog } from "@/features/vehicle-catalog/components/vehicle-brand-dialog";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import { VehicleHeroImage } from "@/features/vehicle-catalog/components/vehicle-hero-image";
import { VehicleImageField } from "@/features/vehicle-catalog/components/vehicle-image-field";
import { VehicleModelDialog } from "@/features/vehicle-catalog/components/vehicle-model-dialog";
import {
  useVehicleBrand,
  useVehicleBrandMutations,
  useVehicleModelFacets,
  useVehicleModelMutations,
  useVehicleModels,
  vehicleCatalogKeys,
} from "@/features/vehicle-catalog/hooks/use-vehicle-catalog";
import { logoVersion } from "@/features/vehicle-catalog/lib/images";
import type {
  VehicleModel,
  VehicleModelListParams,
} from "@/features/vehicle-catalog/services/vehicle-catalog.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";

export const VEHICLE_MODELS_PERSIST_KEY = "platform-vehicle-models-v2";

/** `POST /v1/platform/vehicle-catalog/models/bulk` (TEC-369), undoable. */
export const VEHICLE_MODEL_BULK_ACTIONS: BulkActionDef[] = [
  {
    id: "activate",
    label_key: "bulk.actions.vehicle_models.activate",
    permission: permissions.vehicleCatalog.write,
    reversible: true,
    icon: CircleCheck,
  },
  {
    id: "deactivate",
    label_key: "bulk.actions.vehicle_models.deactivate",
    permission: permissions.vehicleCatalog.write,
    reversible: true,
    confirm_key: "bulk.confirm.vehicle_models.deactivate",
    icon: CircleOff,
  },
];

function yearRange(model: VehicleModel) {
  if (!model.year_start && !model.year_stop) return "—";
  return `${model.year_start ?? "…"} – ${model.year_stop ?? "…"}`;
}

/** super_admin brand detail: form, logo / hero upload and the model list. */
export function VehicleBrandDetailPage({ uuid }: { uuid: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const { confirmDelete } = useDialogs();
  const brandQuery = useVehicleBrand(uuid);
  const brandMutations = useVehicleBrandMutations();
  const modelMutations = useVehicleModelMutations();
  const [editing, setEditing] = useState(false);
  const [modelDialog, setModelDialog] = useState<{
    open: boolean;
    model?: VehicleModel;
  }>({ open: false });

  const brand = brandQuery.data;
  const queryClient = useQueryClient();
  const removeModel = modelMutations.remove.mutate;
  const updateModel = modelMutations.update.mutate;
  const facetsQuery = useVehicleModelFacets(uuid);
  const bodyTypeOptions = useMemo(
    () =>
      (facetsQuery.data?.body_type ?? []).map((f) => ({
        value: f.value,
        label: `${f.value} (${f.count})`,
      })),
    [facetsQuery.data?.body_type],
  );
  const powertrainOptions = useMemo(
    () =>
      (facetsQuery.data?.powertrain ?? []).map((f) => ({
        value: f.value,
        label: `${f.value} (${f.count})`,
      })),
    [facetsQuery.data?.powertrain],
  );

  const baseColumns = useMemo<ColumnDef<VehicleModel, unknown>[]>(
    () => [
      createColumn<VehicleModel>({
        accessorKey: "name",
        labelKey: "vehicles.columns.model_name",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.name}</span>
        ),
      }) as ColumnDef<VehicleModel, unknown>,
      createColumn<VehicleModel>({
        id: "years",
        labelKey: "vehicles.columns.years",
        accessorFn: (row) => yearRange(row),
        // Sorts by first year; the filter matches overlapping spans.
        enableSorting: true,
        sortParam: "year_start",
        filterVariant: "number-range",
        param: "year",
      }) as ColumnDef<VehicleModel, unknown>,
      createColumn<VehicleModel>({
        accessorKey: "body_type",
        labelKey: "vehicles.columns.body_type",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: bodyTypeOptions,
        enableColumnFilter: bodyTypeOptions.length > 0,
        param: "body_type",
        cell: ({ row }) => row.original.body_type ?? "—",
      }) as ColumnDef<VehicleModel, unknown>,
      createColumn<VehicleModel>({
        accessorKey: "powertrain",
        labelKey: "vehicles.columns.powertrain",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: powertrainOptions,
        enableColumnFilter: powertrainOptions.length > 0,
        param: "powertrain",
        cell: ({ row }) => row.original.powertrain ?? "—",
      }) as ColumnDef<VehicleModel, unknown>,
      createColumn<VehicleModel>({
        accessorKey: "active",
        labelKey: "vehicles.columns.status",
        enableSorting: true,
        filterVariant: "boolean",
        param: "active",
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.active ? t("common.active") : t("common.passive")
            }
            tone={row.original.active ? "success" : "default"}
          />
        ),
      }) as ColumnDef<VehicleModel, unknown>,
      createColumn<VehicleModel>({
        accessorKey: "updated_at",
        labelKey: "vehicles.columns.updated_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) => format.dateTime(row.original.updated_at),
      }) as ColumnDef<VehicleModel, unknown>,
      createColumn<VehicleModel>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) => {
          const model = row.original;
          const actions: EntityRowAction[] = [
            {
              id: "edit",
              label: t("common.edit"),
              icon: Pencil,
              onSelect: () => setModelDialog({ open: true, model }),
            },
            {
              id: "toggle_active",
              label: model.active
                ? t("bulk.actions.vehicle_models.deactivate")
                : t("bulk.actions.vehicle_models.activate"),
              icon: model.active ? CircleOff : CircleCheck,
              onSelect: () =>
                updateModel({
                  uuid: model.uuid,
                  body: { active: !model.active },
                }),
            },
            {
              id: "delete",
              label: t("common.delete"),
              icon: Trash2,
              variant: "destructive",
              onSelect: () => {
                void (async () => {
                  const ok = await confirmDelete({
                    title: t("vehicles.model.delete_title"),
                    description: t("vehicles.model.delete_confirm", {
                      name: model.name,
                    }),
                  });
                  if (ok) removeModel(model.uuid);
                })();
              },
            },
          ];
          return <EntityRowActions actions={actions} />;
        },
      }) as ColumnDef<VehicleModel, unknown>,
    ],
    [
      bodyTypeOptions,
      confirmDelete,
      format,
      powertrainOptions,
      removeModel,
      t,
      updateModel,
    ],
  );
  const columns = useMemo(
    () => [createSelectColumnDef<VehicleModel>(), ...baseColumns],
    [baseColumns],
  );

  // Column meta drives the params: body_type / powertrain (CSV), year
  // (year_min / year_max), active; single-field sort (TEC-369).
  const listState = useServerListState({
    columns,
    initialSort: "name",
    initialPageSize: 20,
    persistKey: VEHICLE_MODELS_PERSIST_KEY,
  });
  const modelParams = useMemo<VehicleModelListParams>(
    () => ({ ...listState.params, brand_uuid: uuid }),
    [listState.params, uuid],
  );
  const modelsQuery = useVehicleModels(modelParams);
  const models = useMemo(
    () => modelsQuery.data?.items ?? [],
    [modelsQuery.data?.items],
  );
  // Always the fresh row, so the dialog shows a new hero after upload.
  const editingModel = modelDialog.model
    ? (models.find((m) => m.uuid === modelDialog.model?.uuid) ??
      modelDialog.model)
    : null;
  const modelTotal = modelsQuery.data?.total ?? 0;
  const bulkQuery = useMemo(
    () => ({
      ...listState.filterParams,
      q: modelParams.q,
      sort: modelParams.sort,
    }),
    [listState.filterParams, modelParams.q, modelParams.sort],
  );
  const bulkSelection = useBulkSelection({
    listQueryKey: modelParams,
    bulkQuery,
    total: modelTotal,
  });

  const deleteBrand = async () => {
    if (!brand) return;
    const ok = await confirmDelete({
      title: t("vehicles.brand.delete_title"),
      description: t("vehicles.brand.delete_confirm", { name: brand.name }),
    });
    if (!ok) return;
    await brandMutations.remove.mutateAsync(brand.uuid);
    router.push(routes.platform.vehicleCatalog.root);
  };

  const imagePending =
    brandMutations.uploadImage.isPending ||
    brandMutations.deleteImage.isPending;

  return (
    <EntityPage
      title={brand?.name ?? t("vehicles.title")}
      description={t("vehicles.brand.detail_description")}
      permission={permissions.vehicleCatalog.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("vehicles.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.platform.home },
        {
          label: t("vehicles.title"),
          href: routes.platform.vehicleCatalog.root,
        },
        { label: brand?.name ?? "…" },
      ]}
      actions={
        brand ? (
          <EntityActions>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setEditing(true)}
            >
              <Pencil className="size-4" />
              {t("common.edit")}
            </Button>
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={brandMutations.remove.isPending}
              onClick={() => void deleteBrand()}
            >
              <Trash2 className="size-4" />
              {t("common.delete")}
            </Button>
          </EntityActions>
        ) : null
      }
    >
      {brandQuery.isLoading ? <Loading label={t("common.loading")} /> : null}
      {brandQuery.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          description={t("vehicles.error")}
          onRetry={() => void brandQuery.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}

      {brand ? (
        <div className="flex flex-col gap-6">
          <EntityHeader
            title={brand.name}
            subtitle={t("vehicles.brand.model_count", {
              count: String(brand.model_count),
            })}
            badges={
              <StatusChip
                label={brand.active ? t("common.active") : t("common.passive")}
                tone={brand.active ? "success" : "default"}
              />
            }
            leading={
              <VehicleBrandLogo
                uuid={brand.uuid}
                name={brand.name}
                version={logoVersion(brand.logo_url)}
                height={48}
              />
            }
          />

          <EntitySectionCard title={t("vehicles.brand.images")}>
            <div className="grid gap-6 md:grid-cols-2">
              <VehicleImageField
                kind="logo"
                label={t("vehicles.fields.logo")}
                hasImage={brand.has_logo}
                pending={imagePending}
                preview={
                  <VehicleBrandLogo
                    uuid={brand.uuid}
                    name={brand.name}
                    version={logoVersion(brand.logo_url)}
                    height={brand.logo_height ?? 64}
                  />
                }
                onUpload={(file) =>
                  brandMutations.uploadImage.mutate({
                    uuid: brand.uuid,
                    kind: "logo",
                    file,
                  })
                }
                onRemove={() =>
                  brandMutations.deleteImage.mutate({
                    uuid: brand.uuid,
                    kind: "logo",
                  })
                }
              />
              <VehicleImageField
                kind="hero"
                label={t("vehicles.fields.hero")}
                hasImage={brand.has_hero}
                pending={imagePending}
                preview={
                  <VehicleHeroImage
                    src={brand.hero_url}
                    alt={brand.name}
                    inherited={!brand.has_hero}
                  />
                }
                onUpload={(file) =>
                  brandMutations.uploadImage.mutate({
                    uuid: brand.uuid,
                    kind: "hero",
                    file,
                  })
                }
                onRemove={() =>
                  brandMutations.deleteImage.mutate({
                    uuid: brand.uuid,
                    kind: "hero",
                  })
                }
              />
            </div>
          </EntitySectionCard>

          <EntitySectionCard
            title={t("vehicles.model.title")}
            badge={modelsQuery.data?.total ?? brand.model_count}
            action={
              <EntityCreateButton
                onClick={() => setModelDialog({ open: true })}
                label={t("vehicles.model.create")}
              />
            }
          >
            <EntityTable
              columns={columns}
              data={models}
              getRowId={(row) => row.uuid}
              onRowClick={(row) => setModelDialog({ open: true, model: row })}
              isLoading={modelsQuery.isLoading}
              isError={modelsQuery.isError}
              errorDescription={t("vehicles.error")}
              onRetry={() => void modelsQuery.refetch()}
              emptyTitle={t("vehicles.model.empty_title")}
              emptyDescription={t("vehicles.model.empty_description")}
              rowCount={modelTotal}
              state={{
                ...listState.tableState,
                rowSelection: bulkSelection.rowSelection,
                onRowSelectionChange: bulkSelection.onRowSelectionChange,
              }}
              features={{
                persistKey: VEHICLE_MODELS_PERSIST_KEY,
                rowSelection: true,
                viewMode: true,
              }}
              renderGridItem={(model) => (
                <div className="space-y-3">
                  <VehicleHeroImage
                    src={model.hero_url}
                    alt={model.name}
                    inherited={!model.has_hero}
                  />
                  <div className="space-y-1">
                    <p className="font-display font-semibold">{model.name}</p>
                    <p className="text-muted-foreground text-xs">
                      {[yearRange(model), model.body_type, model.powertrain]
                        .filter((part) => part && part !== "—")
                        .join(" · ") || "—"}
                    </p>
                  </div>
                  <StatusChip
                    label={
                      model.active ? t("common.active") : t("common.passive")
                    }
                    tone={model.active ? "success" : "default"}
                  />
                </div>
              )}
              toolbarExtra={
                <>
                  <BulkActionMenu
                    resource="vehicle_catalog.models"
                    actions={VEHICLE_MODEL_BULK_ACTIONS}
                    scope={bulkSelection.scope}
                    selectedCount={bulkSelection.selectedCount}
                    onComplete={() => {
                      bulkSelection.clearSelection();
                      void queryClient.invalidateQueries({
                        queryKey: vehicleCatalogKeys.all,
                      });
                    }}
                  />
                  <EntityToolbar
                    onRefresh={() => void modelsQuery.refetch()}
                    refreshDisabled={modelsQuery.isFetching}
                  />
                </>
              }
            />
          </EntitySectionCard>
        </div>
      ) : null}

      <VehicleBrandDialog
        open={editing}
        onOpenChange={setEditing}
        brand={brand}
      />
      <VehicleModelDialog
        open={modelDialog.open}
        onOpenChange={(open) =>
          setModelDialog((prev) => (open ? prev : { open: false }))
        }
        brandUuid={uuid}
        model={editingModel}
      />
    </EntityPage>
  );
}
