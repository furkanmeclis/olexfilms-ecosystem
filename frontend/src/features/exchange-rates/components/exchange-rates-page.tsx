"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Coins, RefreshCw, Undo2 } from "lucide-react";
import { useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  AppCombobox,
  AppDatePicker,
  AppForm,
  AppInput,
  FormSection,
} from "@/components/forms";
import { PageHeader } from "@/components/layout/page-header";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import { ratesService } from "@/features/exchange-rates/services/rates.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

const ratesKeys = {
  all: ["exchange-rates"] as const,
  day: (date: string) => ["exchange-rates", "day", date] as const,
  currencies: ["exchange-rates", "currencies"] as const,
};

function overrideSchema(t: Translate) {
  const code = z.string().regex(/^[A-Z]{3}$/, t("rates.validation.currency"));
  return z
    .object({
      date: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, t("rates.validation.date")),
      base: code,
      quote: code,
      rate: z
        .string()
        .trim()
        .regex(/^\d+(\.\d+)?$/, t("rates.validation.rate")),
      note: z.string().max(200),
    })
    .refine((v) => v.base !== v.quote, {
      path: ["quote"],
      message: t("rates.validation.same_pair"),
    });
}

type OverrideValues = z.infer<ReturnType<typeof overrideSchema>>;

const SOURCE_VARIANT: Record<string, "default" | "secondary" | "outline"> = {
  manual: "default",
  tcmb: "secondary",
  ecb: "outline",
};

function today() {
  return new Date().toISOString().slice(0, 10);
}

/** Admin > System settings > Exchange rates (TCMB/ECB daily, overrides). */
export function ExchangeRatesPage() {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(permissions.rates.write);
  const queryClient = useQueryClient();
  const schema = overrideSchema(t);
  const [date, setDate] = useState("");

  const day = useQuery({
    queryKey: ratesKeys.day(date),
    queryFn: () => ratesService.day(date || undefined),
  });
  const currencies = useQuery({
    queryKey: ratesKeys.currencies,
    queryFn: () => ratesService.currencies(),
    staleTime: 30 * 60 * 1000,
  });
  const currencyOptions = (currencies.data?.items ?? []).map((c) => ({
    value: c.code,
    label: `${c.code} · ${c.name}`,
  }));

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ratesKeys.all });
  const onError = (error: unknown) =>
    appToast.error(isApiError(error) ? error.message : t("rates.toast.failed"));

  const override = useMutation({
    mutationFn: (values: OverrideValues) =>
      ratesService.setOverride({
        ...values,
        note: values.note || undefined,
      }),
    onSuccess: async () => {
      await invalidate();
      appToast.success(t("rates.toast.override_saved"));
    },
    onError,
  });
  const clear = useMutation({
    mutationFn: (row: { rate_date: string; base: string; quote: string }) =>
      ratesService.clearOverride(row.rate_date, row.base, row.quote),
    onSuccess: async () => {
      await invalidate();
      appToast.success(t("rates.toast.override_cleared"));
    },
    onError,
  });
  const fetchNow = useMutation({
    mutationFn: () => ratesService.fetchNow(),
    onSuccess: async (rep) => {
      await invalidate();
      const failed = rep.sources.filter((s) => s.error);
      if (failed.length > 0) {
        appToast.error(
          t("rates.toast.fetch_partial", {
            sources: failed.map((s) => s.source.toUpperCase()).join(", "),
          }),
        );
      } else {
        appToast.success(t("rates.toast.fetched"));
      }
    },
    onError,
  });

  return (
    <div className="space-y-6">
      <PageHeader
        title={t("rates.title")}
        description={t("rates.description")}
        icon={<Coins className="size-6" />}
        actions={
          canWrite ? (
            <Button
              type="button"
              variant="outline"
              disabled={fetchNow.isPending}
              onClick={() => fetchNow.mutate()}
            >
              <RefreshCw className="size-4" />
              {t("rates.fetch_now")}
            </Button>
          ) : null
        }
      />

      {canWrite ? (
        <AppForm
          schema={schema}
          defaultValues={{
            date: today(),
            base: "EUR",
            quote: "TRY",
            rate: "",
            note: "",
          }}
          onSubmit={async (values) => {
            await override.mutateAsync(values).catch(() => undefined);
          }}
        >
          <FormSection
            id="rate-override"
            title={t("rates.override_title")}
            description={t("rates.override_description")}
            columns={2}
          >
            <AppDatePicker name="date" label={t("rates.fields.date")} />
            <AppInput
              name="rate"
              inputMode="decimal"
              label={t("rates.fields.rate")}
              placeholder="48.75"
            />
            <AppCombobox
              name="base"
              label={t("rates.fields.base")}
              options={currencyOptions}
            />
            <AppCombobox
              name="quote"
              label={t("rates.fields.quote")}
              options={currencyOptions}
            />
            <AppInput
              name="note"
              label={t("rates.fields.note")}
              className="sm:col-span-2"
            />
            <div className="flex justify-end sm:col-span-2">
              <Button type="submit" disabled={override.isPending}>
                {t("rates.save_override")}
              </Button>
            </div>
          </FormSection>
        </AppForm>
      ) : null}

      <div className="flex flex-wrap items-end gap-3">
        <label className="space-y-1 text-sm">
          <span className="text-muted-foreground">{t("rates.fields.day")}</span>
          <DatePicker
            value={date || day.data?.date || ""}
            onChange={(value) => setDate(value)}
            className="w-44"
          />
        </label>
        <p className="text-muted-foreground pb-2 text-sm">
          {t("rates.priority_note")}
        </p>
      </div>

      {day.isLoading ? <Loading label={t("common.loading")} /> : null}
      {day.isError ? (
        <ErrorState
          title={t("rates.error")}
          retryLabel={t("common.retry")}
          onRetry={() => day.refetch()}
        />
      ) : null}
      {day.data ? (
        <div className="overflow-x-auto rounded-lg border">
          <table className="w-full text-sm">
            <thead className="bg-muted/50 text-muted-foreground">
              <tr>
                <th className="px-3 py-2 text-start font-medium">
                  {t("rates.columns.pair")}
                </th>
                <th className="px-3 py-2 text-end font-medium">
                  {t("rates.columns.rate")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("rates.columns.source")}
                </th>
                <th className="px-3 py-2 text-start font-medium">
                  {t("rates.columns.note")}
                </th>
                <th className="px-3 py-2" />
              </tr>
            </thead>
            <tbody>
              {day.data.items.length === 0 ? (
                <tr>
                  <td
                    colSpan={5}
                    className="text-muted-foreground px-3 py-6 text-center"
                  >
                    {t("rates.empty")}
                  </td>
                </tr>
              ) : null}
              {day.data.items.map((row) => (
                <tr
                  key={`${row.base}-${row.quote}-${row.source}`}
                  className={
                    row.effective
                      ? "border-t"
                      : "text-muted-foreground border-t"
                  }
                >
                  <td className="px-3 py-2 font-medium" dir="ltr">
                    {row.base}/{row.quote}
                  </td>
                  <td className="px-3 py-2 text-end font-mono" dir="ltr">
                    {row.rate}
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex items-center gap-2">
                      <Badge variant={SOURCE_VARIANT[row.source] ?? "outline"}>
                        {row.source.toUpperCase()}
                      </Badge>
                      {row.effective ? (
                        <span className="text-xs">{t("rates.effective")}</span>
                      ) : null}
                    </div>
                  </td>
                  <td className="px-3 py-2">{row.note ?? ""}</td>
                  <td className="px-3 py-2 text-end">
                    {canWrite && row.source === "manual" ? (
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={clear.isPending}
                        onClick={() => clear.mutate(row)}
                      >
                        <Undo2 className="size-4" />
                        {t("rates.clear_override")}
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
