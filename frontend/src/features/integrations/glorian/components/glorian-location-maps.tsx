"use client";

import { zodResolver } from "@hookform/resolvers/zod";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Plus, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { FormProvider, useForm, useWatch } from "react-hook-form";
import { z } from "zod";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { AppCombobox, AppInput, type ComboboxOption } from "@/components/forms";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { FieldGroup } from "@/components/ui/field";
import { glorianErrorText } from "@/features/integrations/glorian/lib/errors";
import { glorianKeys } from "@/features/integrations/glorian/lib/keys";
import {
  glorianService,
  type GlorianLocationMap,
  type GlorianLocationMapFilter,
  type GlorianLocationMapInput,
  type GlorianMapLocation,
  type GlorianMapWarehouse,
} from "@/features/integrations/glorian/services/glorian.service";
import { isApiError } from "@/lib/api";
import { useDialogs } from "@/providers/dialog-provider";
import { useFormatter, useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const LOCATION_MAPS_PERSIST_KEY = "platform-glorian-location-maps-v1";

const REMOTE_CODE_MAX = 64;

/** "CODE · Name" of a map warehouse. */
export function warehouseLabel(w: Pick<GlorianMapWarehouse, "code" | "name">) {
  return w.name ? `${w.code} · ${w.name}` : w.code;
}

/** Full code of a bin, falling back to its own code. */
export function locationLabel(
  l: Pick<GlorianMapLocation, "code" | "full_code">,
) {
  return l.full_code || l.code;
}

function locationOption(l: GlorianMapLocation): ComboboxOption {
  return {
    value: l.uuid,
    label: locationLabel(l),
    description: l.name || undefined,
  };
}

/**
 * Glorian remote location → local warehouse (+ optional bin) maps
 * (TEC-769, legacy ConnectionLocationMap): server list with q, sort,
 * paging and create / edit / delete for `integrations.glorian.manage`.
 */
export function GlorianLocationMaps({ canManage }: { canManage: boolean }) {
  const { t } = useLocale();
  const format = useFormatter();
  const queryClient = useQueryClient();
  const { confirmDelete } = useDialogs();
  const [createOpen, setCreateOpen] = useState(false);
  const [editing, setEditing] = useState<GlorianLocationMap | null>(null);

  const remove = useMutation({
    mutationFn: (uuid: string) => glorianService.deleteLocationMap(uuid),
    onSuccess: async () => {
      appToast.success(t("integrations.glorian.location_maps.deleted"));
      await queryClient.invalidateQueries({
        queryKey: [...glorianKeys.all, "location-maps"],
      });
    },
    onError: (error) => appToast.error(glorianErrorText(t, error)),
  });
  const removeMap = remove.mutate;

  const columns = useMemo<ColumnDef<GlorianLocationMap, unknown>[]>(
    () => [
      createColumn<GlorianLocationMap>({
        accessorKey: "remote_location_code",
        labelKey: "integrations.glorian.location_maps.remote",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-mono">{row.original.remote_location_code}</span>
        ),
      }),
      createColumn<GlorianLocationMap>({
        id: "warehouse",
        accessorFn: (row) => warehouseLabel(row.warehouse),
        labelKey: "integrations.glorian.location_maps.warehouse",
        enableSorting: true,
        gridSecondary: true,
        cell: ({ row }) => warehouseLabel(row.original.warehouse),
      }),
      createColumn<GlorianLocationMap>({
        id: "location",
        accessorFn: (row) => (row.location ? locationLabel(row.location) : ""),
        labelKey: "integrations.glorian.location_maps.location",
        enableSorting: false,
        cell: ({ row }) =>
          row.original.location ? (
            <span className="font-mono">
              {locationLabel(row.original.location)}
            </span>
          ) : (
            "—"
          ),
      }),
      createColumn<GlorianLocationMap>({
        accessorKey: "created_at",
        labelKey: "integrations.glorian.location_maps.created",
        enableSorting: true,
        cell: ({ row }) => (
          <span className="whitespace-nowrap">
            {row.original.created_at
              ? format.dateTime(row.original.created_at)
              : "—"}
          </span>
        ),
      }),
      ...(canManage
        ? [
            createColumn<GlorianLocationMap>({
              id: "actions",
              labelKey: "common.actions",
              enableSorting: false,
              enableHiding: false,
              enableResizing: false,
              cell: ({ row }) => {
                const map = row.original;
                const actions: EntityRowAction[] = [
                  {
                    id: "edit",
                    label: t("common.edit"),
                    icon: Pencil,
                    onSelect: () => setEditing(map),
                  },
                  {
                    id: "delete",
                    label: t("common.delete"),
                    icon: Trash2,
                    variant: "destructive",
                    onSelect: () => {
                      void (async () => {
                        const ok = await confirmDelete({
                          title: t(
                            "integrations.glorian.location_maps.delete_title",
                          ),
                          description: t(
                            "integrations.glorian.location_maps.delete_confirm",
                            { code: map.remote_location_code },
                          ),
                        });
                        if (ok) removeMap(map.uuid);
                      })();
                    },
                  },
                ];
                return <EntityRowActions actions={actions} />;
              },
            }),
          ]
        : []),
    ],
    [canManage, confirmDelete, format, removeMap, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "remote_location_code",
    initialPageSize: 20,
    persistKey: LOCATION_MAPS_PERSIST_KEY,
  });
  const query = listState.params as GlorianLocationMapFilter;

  const list = useQuery({
    queryKey: glorianKeys.locationMaps(query),
    queryFn: () => glorianService.locationMapsPage(query),
    placeholderData: (previous) => previous,
  });

  return (
    <div data-testid="location-maps">
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={glorianErrorText(t, list.error)}
        onRetry={() => void list.refetch()}
        emptyTitle={t("integrations.glorian.location_maps.empty")}
        emptyDescription=""
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: LOCATION_MAPS_PERSIST_KEY }}
        toolbarExtra={
          <>
            {canManage ? (
              <Button
                type="button"
                size="sm"
                onClick={() => setCreateOpen(true)}
                data-testid="location-map-create"
              >
                <Plus className="size-4" aria-hidden />
                {t("integrations.glorian.location_maps.create")}
              </Button>
            ) : null}
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
      {canManage ? (
        <>
          <GlorianLocationMapDialog
            open={createOpen}
            onOpenChange={setCreateOpen}
          />
          <GlorianLocationMapDialog
            open={Boolean(editing)}
            onOpenChange={(open) => {
              if (!open) setEditing(null);
            }}
            map={editing}
          />
        </>
      ) : null}
    </div>
  );
}

type MapFormValues = {
  remote_location_code: string;
  warehouse_uuid: string;
  location_uuid: string;
};

const FORM_FIELDS: (keyof MapFormValues)[] = [
  "remote_location_code",
  "warehouse_uuid",
  "location_uuid",
];

export function toLocationMapBody(
  values: MapFormValues,
): GlorianLocationMapInput {
  return {
    remote_location_code: values.remote_location_code.trim(),
    warehouse_uuid: values.warehouse_uuid,
    location_uuid: values.location_uuid || null,
  };
}

/** Create / edit dialog of a remote location map. */
export function GlorianLocationMapDialog({
  open,
  onOpenChange,
  map,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Edit this map; create a new one when omitted. */
  map?: GlorianLocationMap | null;
}) {
  const { t } = useLocale();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md">
        <DialogHeader>
          <DialogTitle>
            {t(
              map
                ? "integrations.glorian.location_maps.edit_title"
                : "integrations.glorian.location_maps.create",
            )}
          </DialogTitle>
          <DialogDescription>
            {t("integrations.glorian.location_maps.form_hint")}
          </DialogDescription>
        </DialogHeader>
        {open ? (
          <LocationMapForm
            key={map?.uuid ?? "new"}
            map={map ?? null}
            onDone={() => onOpenChange(false)}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function LocationMapForm({
  map,
  onDone,
}: {
  map: GlorianLocationMap | null;
  onDone: () => void;
}) {
  const { t } = useLocale();
  const queryClient = useQueryClient();

  const schema = useMemo(
    () =>
      z.object({
        remote_location_code: z
          .string()
          .trim()
          .min(1, t("integrations.glorian.validation.required"))
          .max(REMOTE_CODE_MAX, t("integrations.glorian.validation.too_long")),
        warehouse_uuid: z
          .string()
          .min(1, t("integrations.glorian.validation.required")),
        location_uuid: z.string(),
      }),
    [t],
  );

  const form = useForm<MapFormValues>({
    resolver: zodResolver(schema as never),
    defaultValues: {
      remote_location_code: map?.remote_location_code ?? "",
      warehouse_uuid: map?.warehouse.uuid ?? "",
      location_uuid: map?.location?.uuid ?? "",
    },
  });
  const warehouseUuid = useWatch({
    control: form.control,
    name: "warehouse_uuid",
  });

  const warehouses = useQuery({
    queryKey: glorianKeys.mapWarehouses(),
    queryFn: () => glorianService.mapWarehouses(),
  });
  const warehouseOptions = useMemo<ComboboxOption[]>(() => {
    const items = warehouses.data ?? [];
    const options = items.map((w) => ({
      value: w.uuid,
      label: warehouseLabel(w),
      description: w.active
        ? undefined
        : t("integrations.glorian.location_maps.inactive"),
    }));
    // Keep the current target selectable while the list loads.
    if (map && !items.some((w) => w.uuid === map.warehouse.uuid)) {
      options.unshift({
        value: map.warehouse.uuid,
        label: warehouseLabel(map.warehouse),
        description: undefined,
      });
    }
    return options;
  }, [map, t, warehouses.data]);

  const loadLocations = useCallback(
    async (q: string) => {
      if (!warehouseUuid) return [];
      const items = await glorianService.mapLocations(warehouseUuid, q);
      return items.map(locationOption);
    },
    [warehouseUuid],
  );
  const initialLocations = useMemo(
    () =>
      map?.location && map.warehouse.uuid === warehouseUuid
        ? [locationOption(map.location)]
        : [],
    [map, warehouseUuid],
  );

  const save = useMutation({
    mutationFn: (body: GlorianLocationMapInput) =>
      map
        ? glorianService.updateLocationMap(map.uuid, body)
        : glorianService.createLocationMap(body),
    onSuccess: async () => {
      appToast.success(t("integrations.glorian.location_maps.saved"));
      await queryClient.invalidateQueries({
        queryKey: [...glorianKeys.all, "location-maps"],
      });
      onDone();
    },
    onError: (error) => {
      if (isApiError(error)) {
        if (error.code === "GLORIAN_REMOTE_LOCATION_TAKEN") {
          form.setError("remote_location_code", {
            type: "server",
            message: t("integrations.glorian.errors.remote_location_taken"),
          });
        } else if (error.isValidation) {
          for (const [field, message] of Object.entries(error.fieldErrors())) {
            if (FORM_FIELDS.includes(field as keyof MapFormValues)) {
              form.setError(field as keyof MapFormValues, {
                type: "server",
                message,
              });
            }
          }
        }
      }
      appToast.error(glorianErrorText(t, error));
    },
  });

  return (
    <FormProvider {...form}>
      <form
        noValidate
        onSubmit={form.handleSubmit((values) =>
          save.mutate(toLocationMapBody(values)),
        )}
        data-testid="location-map-form"
      >
        <FieldGroup className="gap-4">
          <AppInput
            name="remote_location_code"
            label={t("integrations.glorian.location_maps.remote")}
            description={t("integrations.glorian.location_maps.remote_hint")}
            maxLength={REMOTE_CODE_MAX}
            required
            autoFocus
            data-testid="location-map-remote"
          />
          <AppCombobox
            name="warehouse_uuid"
            label={t("integrations.glorian.location_maps.warehouse")}
            options={warehouseOptions}
            placeholder={t(
              "integrations.glorian.location_maps.warehouse_placeholder",
            )}
            emptyText={t("integrations.glorian.location_maps.no_warehouses")}
            disabled={warehouses.isLoading}
            onValueChange={(value) => {
              if (value !== warehouseUuid) form.setValue("location_uuid", "");
            }}
            data-testid="location-map-warehouse"
          />
          <AppCombobox
            key={warehouseUuid || "none"}
            name="location_uuid"
            label={t("integrations.glorian.location_maps.location")}
            description={t("integrations.glorian.location_maps.location_hint")}
            loadOptions={loadLocations}
            initialOptions={initialLocations}
            placeholder={t("integrations.glorian.location_maps.no_location")}
            emptyText={t("integrations.glorian.location_maps.no_locations")}
            disabled={!warehouseUuid}
            clearable
            data-testid="location-map-location"
          />
        </FieldGroup>
        <DialogFooter className="mt-6">
          <Button type="button" variant="outline" onClick={onDone}>
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            disabled={save.isPending}
            data-testid="location-map-save"
          >
            {save.isPending ? t("common.saving") : t("common.save")}
          </Button>
        </DialogFooter>
      </form>
    </FormProvider>
  );
}
