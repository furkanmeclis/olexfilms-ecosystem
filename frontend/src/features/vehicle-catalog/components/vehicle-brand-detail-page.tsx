"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Trash2 } from "lucide-react";
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
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { VehicleBrandDialog } from "@/features/vehicle-catalog/components/vehicle-brand-dialog";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import { VehicleHeroImage } from "@/features/vehicle-catalog/components/vehicle-hero-image";
import { VehicleImageField } from "@/features/vehicle-catalog/components/vehicle-image-field";
import { VehicleModelDialog } from "@/features/vehicle-catalog/components/vehicle-model-dialog";
import {
  useVehicleBrand,
  useVehicleBrandMutations,
  useVehicleModelMutations,
  useVehicleModels,
} from "@/features/vehicle-catalog/hooks/use-vehicle-catalog";
import { logoVersion } from "@/features/vehicle-catalog/lib/images";
import type {
  VehicleModel,
  VehicleModelListParams,
} from "@/features/vehicle-catalog/services/vehicle-catalog.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";

function yearRange(model: VehicleModel) {
  if (!model.year_start && !model.year_stop) return "—";
  return `${model.year_start ?? "…"} – ${model.year_stop ?? "…"}`;
}

/** super_admin brand detail: form, logo / hero upload and the model list. */
export function VehicleBrandDetailPage({ uuid }: { uuid: string }) {
  const { t } = useLocale();
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

  const listState = useServerListState({
    initialSort: "name",
    initialPageSize: 20,
  });
  const modelParams = useMemo<VehicleModelListParams>(
    () => ({
      brand_uuid: uuid,
      limit: listState.params.limit,
      offset: listState.params.offset,
      q: listState.params.q,
    }),
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

  const brand = brandQuery.data;
  const removeModel = modelMutations.remove;

  const columns = useMemo<ColumnDef<VehicleModel>[]>(
    () => [
      createColumn<VehicleModel>({
        accessorKey: "name",
        labelKey: "vehicles.columns.model_name",
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.name}</span>
        ),
      }),
      createColumn<VehicleModel>({
        id: "years",
        labelKey: "vehicles.columns.years",
        enableSorting: false,
        accessorFn: (row) => yearRange(row),
      }),
      createColumn<VehicleModel>({
        accessorKey: "body_type",
        labelKey: "vehicles.columns.body_type",
        enableSorting: false,
        cell: ({ row }) => row.original.body_type ?? "—",
      }),
      createColumn<VehicleModel>({
        accessorKey: "powertrain",
        labelKey: "vehicles.columns.powertrain",
        enableSorting: false,
        cell: ({ row }) => row.original.powertrain ?? "—",
      }),
      createColumn<VehicleModel>({
        accessorKey: "active",
        labelKey: "vehicles.columns.status",
        enableSorting: false,
        cell: ({ row }) => (
          <StatusChip
            label={
              row.original.active ? t("common.active") : t("common.passive")
            }
            tone={row.original.active ? "success" : "default"}
          />
        ),
      }),
      {
        id: "actions",
        enableSorting: false,
        enableHiding: false,
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
                  if (ok) removeModel.mutate(model.uuid);
                })();
              },
            },
          ];
          return <EntityRowActions actions={actions} />;
        },
      },
    ],
    [confirmDelete, removeModel, t],
  );

  const pageCount = Math.max(
    1,
    Math.ceil((modelsQuery.data?.total ?? 0) / (modelParams.limit || 20)),
  );

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
              pageCount={pageCount}
              state={listState.tableState}
              features={{
                persistKey: "platform-vehicle-models-v1",
                sorting: false,
                columnFilters: false,
                facetedFilters: false,
                rowSelection: false,
              }}
              toolbarExtra={
                <EntityToolbar
                  onRefresh={() => void modelsQuery.refetch()}
                  refreshDisabled={modelsQuery.isFetching}
                />
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
