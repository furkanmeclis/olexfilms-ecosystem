"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { RectangleHorizontal } from "lucide-react";
import { useState } from "react";
import { useFormContext, useWatch } from "react-hook-form";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  AppCombobox,
  AppForm,
  AppInput,
  AppSwitch,
  FormSection,
} from "@/components/forms";
import { PageHeader } from "@/components/layout/page-header";
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

  const { data, isLoading, isError, refetch } = useQuery({
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
      {isLoading ? <Loading label={t("common.loading")} /> : null}
      {isError ? (
        <ErrorState
          title={t("geo.plates.error")}
          retryLabel={t("common.retry")}
          onRetry={() => refetch()}
        />
      ) : null}

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

      {data ? (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-muted-foreground">
              <tr>
                <th className="px-3 py-2 text-start font-medium">
                  {t("geo.plates.fields.country")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("geo.plates.columns.preview")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("geo.plates.fields.regex")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("geo.plates.fields.active")}
                </th>
                <th className="px-3 py-2" />
              </tr>
            </thead>
            <tbody>
              {data.items.map((f) => (
                <tr key={f.country_iso2} className="border-t">
                  <td className="px-3 py-2 font-medium">
                    {countryName(
                      {
                        name_en: f.country_name_en,
                        name_tr: f.country_name_tr,
                      },
                      locale,
                    )}
                  </td>
                  <td className="px-3 py-2">
                    <PlateBadge plate={f.example} format={f} />
                  </td>
                  <td className="text-muted-foreground max-w-xs truncate px-3 py-2 font-mono text-xs">
                    {f.regex}
                  </td>
                  <td className="px-3 py-2">
                    <Badge variant={f.is_active ? "secondary" : "outline"}>
                      {f.is_active
                        ? t("geo.plates.active")
                        : t("geo.plates.inactive")}
                    </Badge>
                  </td>
                  <td className="px-3 py-2 text-end">
                    {canWrite ? (
                      <Button
                        type="button"
                        variant="outline"
                        size="sm"
                        onClick={() => setEditing(f)}
                      >
                        {t("geo.plates.edit")}
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
