"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, RectangleHorizontal } from "lucide-react";
import { useMemo, useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
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
  AppInput,
  AppSwitch,
  FormSection,
} from "@/components/forms";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { permissions } from "@/config/permissions";
import { PlateBadge } from "@/features/geo/components/plate-badge";
import {
  countryName,
  geoKeys,
  useCountries,
} from "@/features/geo/hooks/use-geo";
import { matchPlate } from "@/features/geo/lib/plate";
import { geoService } from "@/features/geo/services/geo.service";
import type { PlateFormat } from "@/features/geo/types";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

const COLOR = /^#[0-9A-Fa-f]{6}$/;

export const PLATE_FORMATS_PERSIST_KEY = "platform-plate-formats-v1";

function plateSchema(t: Translate) {
  const color = z.string().regex(COLOR, t("geo.plates.validation.color"));
  return z.object({
    country: z.string().length(2, t("geo.plates.validation.country")),
    regex: z
      .string()
      .trim()
      .regex(/^\^.*\$$/, t("geo.plates.validation.regex")),
    input_mask: z.string().max(64),
    example: z.string().max(32),
    country_label: z
      .string()
      .trim()
      .min(1, t("geo.plates.validation.label"))
      .max(4, t("geo.plates.validation.label")),
    strip_color: color,
    background_color: color,
    text_color: color,
    is_active: z.boolean(),
  });
}

type PlateValues = z.infer<ReturnType<typeof plateSchema>>;

const EMPTY: PlateValues = {
  country: "",
  regex: "^$",
  input_mask: "",
  example: "",
  country_label: "",
  strip_color: "#003399",
  background_color: "#FFFFFF",
  text_color: "#000000",
  is_active: true,
};

function toValues(f: PlateFormat): PlateValues {
  return {
    country: f.country_iso2,
    regex: f.regex,
    input_mask: f.input_mask,
    example: f.example,
    country_label: f.country_label,
    strip_color: f.strip_color,
    background_color: f.background_color,
    text_color: f.text_color,
    is_active: f.is_active,
  };
}

function Preview() {
  const { t } = useLocale();
  const form = useFormContext<PlateValues>();
  const values = useWatch({ control: form.control }) as PlateValues;
  const [sample, setSample] = useState("");
  const plate = sample || values.example || "";
  const valid = plate ? matchPlate(values.regex ?? "", plate) : null;
  return (
    <div className="space-y-3 sm:col-span-2">
      <div className="flex flex-wrap items-center gap-3">
        <PlateBadge plate={plate} format={values} />
        {valid === null ? null : (
          <Badge variant={valid ? "success" : "danger"}>
            {valid ? t("geo.plates.valid") : t("geo.plates.invalid")}
          </Badge>
        )}
      </div>
      <Input
        value={sample}
        onChange={(e) => setSample(e.target.value)}
        placeholder={t("geo.plates.try_placeholder")}
        aria-label={t("geo.plates.try")}
      />
    </div>
  );
}

/** Admin > System settings > Plate formats (regex, mask, badge colors). */
export function PlateFormatsPage() {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.settings.write);
  const queryClient = useQueryClient();
  const schema = plateSchema(t);
  const [editing, setEditing] = useState<PlateFormat | "new" | null>(null);
  const countries = useCountries();

  const { data, isLoading, isError, isFetching, refetch } = useQuery({
    queryKey: geoKeys.platformPlateFormats,
    queryFn: () => geoService.platformPlateFormats(),
  });

  const invalidate = () =>
    Promise.all([
      queryClient.invalidateQueries({ queryKey: geoKeys.platformPlateFormats }),
      queryClient.invalidateQueries({ queryKey: geoKeys.plateFormats }),
    ]);

  const save = useMutation({
    mutationFn: (values: PlateValues) => {
      const { country, ...rest } = values;
      return editing === "new"
        ? geoService.createPlateFormat({ country, ...rest })
        : geoService.updatePlateFormat(country, rest);
    },
    onSuccess: async () => {
      await invalidate();
      setEditing(null);
      appToast.success(t("geo.plates.toast.saved"));
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("geo.plates.toast.failed"),
      ),
  });

  const remove = useMutation({
    mutationFn: (iso2: string) => geoService.deletePlateFormat(iso2),
    onSuccess: async () => {
      await invalidate();
      setEditing(null);
      appToast.success(t("geo.plates.toast.deleted"));
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("geo.plates.toast.failed"),
      ),
  });

  // Row drag-and-drop → PUT /v1/platform/plate-formats/order (optimistic).
  const reorder = useMutation({
    mutationFn: (rows: PlateFormat[]) =>
      geoService.reorderPlateFormats(rows.map((row) => row.country_iso2)),
    onMutate: async (rows) => {
      await queryClient.cancelQueries({
        queryKey: geoKeys.platformPlateFormats,
      });
      const previous = queryClient.getQueryData<{ items: PlateFormat[] }>(
        geoKeys.platformPlateFormats,
      );
      queryClient.setQueryData(geoKeys.platformPlateFormats, { items: rows });
      return { previous };
    },
    onSuccess: async (result) => {
      queryClient.setQueryData(geoKeys.platformPlateFormats, result);
      await queryClient.invalidateQueries({ queryKey: geoKeys.plateFormats });
      appToast.success(t("table.reorder_saved"));
    },
    onError: (error, _rows, context) => {
      if (context?.previous) {
        queryClient.setQueryData(
          geoKeys.platformPlateFormats,
          context.previous,
        );
      }
      appToast.error(
        isApiError(error) ? error.message : t("geo.plates.toast.failed"),
      );
    },
  });

  // Inline edit of the active flag (double-click) → PATCH is_active.
  const toggleActive = useMutation({
    mutationFn: ({ iso2, isActive }: { iso2: string; isActive: boolean }) =>
      geoService.updatePlateFormat(iso2, { is_active: isActive }),
    onSuccess: async () => {
      await invalidate();
      appToast.success(t("geo.plates.toast.saved"));
    },
    onError: (error) =>
      appToast.error(
        isApiError(error) ? error.message : t("geo.plates.toast.failed"),
      ),
  });

  const columns = useMemo<ColumnDef<PlateFormat, unknown>[]>(
    () => [
      createColumn<PlateFormat>({
        id: "country",
        accessorFn: (row) =>
          countryName(
            { name_en: row.country_name_en, name_tr: row.country_name_tr },
            locale,
          ),
        labelKey: "geo.plates.fields.country",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row, getValue }) => (
          <div className="flex min-w-0 flex-col">
            <span className="font-medium">{String(getValue())}</span>
            <span className="text-muted-foreground font-mono text-xs">
              {row.original.country_iso2}
            </span>
          </div>
        ),
      }),
      createColumn<PlateFormat>({
        id: "preview",
        accessorKey: "example",
        labelKey: "geo.plates.columns.preview",
        enableSorting: false,
        cell: ({ row }) => (
          <PlateBadge plate={row.original.example} format={row.original} />
        ),
      }),
      createColumn<PlateFormat>({
        accessorKey: "regex",
        labelKey: "geo.plates.fields.regex",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="text-muted-foreground block max-w-xs truncate font-mono text-xs">
            {row.original.regex}
          </span>
        ),
      }),
      createColumn<PlateFormat>({
        accessorKey: "is_active",
        labelKey: "geo.plates.fields.active",
        enableSorting: true,
        filterVariant: "boolean",
        editVariant: "boolean",
        gridSecondary: true,
        cell: ({ row }) => (
          <Badge variant={row.original.is_active ? "secondary" : "outline"}>
            {row.original.is_active
              ? t("geo.plates.active")
              : t("geo.plates.inactive")}
          </Badge>
        ),
      }),
      createColumn<PlateFormat>({
        accessorKey: "sort_order",
        labelKey: "table.reorder",
        enableSorting: true,
        defaultHidden: true,
      }),
      createColumn<PlateFormat>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) =>
          canWrite ? (
            <EntityRowActions
              actions={[
                {
                  id: "edit",
                  label: t("geo.plates.edit"),
                  icon: Pencil,
                  onSelect: () => setEditing(row.original),
                },
              ]}
            />
          ) : null,
      }),
    ],
    [canWrite, locale, t],
  );

  const used = new Set((data?.items ?? []).map((f) => f.country_iso2));
  const countryOptions = (countries.data ?? [])
    .filter((c) => !used.has(c.iso2))
    .map((c) => ({
      value: c.iso2,
      label: countryName(c, locale),
      description: c.iso2,
    }));

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("geo.plates.title")}
        description={t("geo.plates.description")}
        icon={<RectangleHorizontal className="size-6" />}
        actions={
          canWrite ? (
            <Button type="button" onClick={() => setEditing("new")}>
              {t("geo.plates.add")}
            </Button>
          ) : null
        }
      />

      {editing ? (
        <AppForm
          key={editing === "new" ? "new" : editing.country_iso2}
          schema={schema}
          defaultValues={editing === "new" ? EMPTY : toValues(editing)}
          onSubmit={async (values) => {
            await save.mutateAsync(values).catch(() => undefined);
          }}
        >
          <FormSection
            id="plate-format"
            title={
              editing === "new"
                ? t("geo.plates.new_title")
                : t("geo.plates.edit_title", { country: editing.country_iso2 })
            }
            description={t("geo.plates.form_description")}
            columns={2}
          >
            {editing === "new" ? (
              <AppCombobox
                name="country"
                label={t("geo.plates.fields.country")}
                options={countryOptions}
                searchPlaceholder={t("geo.address.search")}
                emptyText={t("geo.address.empty")}
                className="sm:col-span-2"
              />
            ) : null}
            <AppInput
              name="regex"
              label={t("geo.plates.fields.regex")}
              description={t("geo.plates.fields.regex_hint")}
              className="font-mono sm:col-span-2"
            />
            <AppInput
              name="input_mask"
              label={t("geo.plates.fields.input_mask")}
            />
            <AppInput name="example" label={t("geo.plates.fields.example")} />
            <AppInput
              name="country_label"
              label={t("geo.plates.fields.country_label")}
            />
            <AppInput
              name="strip_color"
              label={t("geo.plates.fields.strip_color")}
            />
            <AppInput
              name="background_color"
              label={t("geo.plates.fields.background_color")}
            />
            <AppInput
              name="text_color"
              label={t("geo.plates.fields.text_color")}
            />
            <AppSwitch name="is_active" label={t("geo.plates.fields.active")} />
            <Preview />
            <div className="flex flex-wrap items-center justify-end gap-2 sm:col-span-2">
              {editing !== "new" ? (
                <Button
                  type="button"
                  variant="destructive"
                  disabled={remove.isPending}
                  onClick={() => remove.mutate(editing.country_iso2)}
                >
                  {t("geo.plates.delete")}
                </Button>
              ) : null}
              <Button
                type="button"
                variant="outline"
                onClick={() => setEditing(null)}
              >
                {t("form.cancel")}
              </Button>
              <Button type="submit" disabled={save.isPending}>
                {t("geo.plates.save")}
              </Button>
            </div>
          </FormSection>
        </AppForm>
      ) : null}

      <EntityTable
        columns={columns}
        data={data?.items ?? []}
        getRowId={(row) => row.country_iso2}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={isLoading}
        isError={isError}
        errorTitle={t("geo.plates.error")}
        onRetry={() => void refetch()}
        initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
        pageSizeOptions={[20, 50, 100]}
        features={{
          persistKey: PLATE_FORMATS_PERSIST_KEY,
          rowSelection: false,
          rowReorder: canWrite,
          inlineEdit: canWrite,
        }}
        onRowReorder={canWrite ? (rows) => reorder.mutate(rows) : undefined}
        onCellEdit={
          canWrite
            ? ({ row, columnId, value }) => {
                if (columnId !== "is_active") return;
                const next = Boolean(value);
                if (next === row.is_active) return;
                toggleActive.mutate({ iso2: row.country_iso2, isActive: next });
              }
            : undefined
        }
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
