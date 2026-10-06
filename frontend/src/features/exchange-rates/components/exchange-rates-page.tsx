"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Coins, RefreshCw, Undo2 } from "lucide-react";
import type { ColumnDef } from "@tanstack/react-table";
import { useMemo, useState } from "react";
import { z } from "zod";

import {
  CLIENT_SIDE_MANUAL,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
} from "@/components/entity";
import {
  AppCombobox,
  AppDatePicker,
  AppForm,
  AppInput,
  FormSection,
} from "@/components/forms";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { permissions } from "@/config/permissions";
import {
  ratesService,
  type StoredExchangeRate,
} from "@/features/exchange-rates/services/rates.service";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
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

export const EXCHANGE_RATES_PERSIST_KEY = "platform-exchange-rates-v1";
const RATE_SOURCES = ["manual", "tcmb", "ecb"] as const;
const RATE_PATTERN = /^\d+(\.\d+)?$/;

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

  const clearOverride = clear.mutate;
  const clearPending = clear.isPending;
  const columns = useMemo<ColumnDef<StoredExchangeRate, unknown>[]>(
    () => [
      createColumn<StoredExchangeRate>({
        id: "pair",
        accessorFn: (row) => `${row.base}/${row.quote}`,
        labelKey: "rates.columns.pair",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row, getValue }) => (
          <span
            className={cn(
              "font-medium",
              !row.original.effective && "text-muted-foreground",
            )}
            dir="ltr"
          >
            {String(getValue())}
          </span>
        ),
      }),
      createColumn<StoredExchangeRate>({
        accessorKey: "rate",
        labelKey: "rates.columns.rate",
        enableSorting: true,
        sortingFn: (a, b) => Number(a.original.rate) - Number(b.original.rate),
        editVariant: "text",
        cell: ({ row }) => (
          <span
            className={cn(
              "font-mono",
              !row.original.effective && "text-muted-foreground",
            )}
            dir="ltr"
          >
            {row.original.rate}
          </span>
        ),
      }),
      createColumn<StoredExchangeRate>({
        accessorKey: "source",
        labelKey: "rates.columns.source",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: RATE_SOURCES.map((value) => ({
          value,
          label: value.toUpperCase(),
        })),
        gridSecondary: true,
        cell: ({ row }) => (
          <div className="flex items-center gap-2">
            <Badge variant={SOURCE_VARIANT[row.original.source] ?? "outline"}>
              {row.original.source.toUpperCase()}
            </Badge>
            {row.original.effective ? (
              <span className="text-xs">{t("rates.effective")}</span>
            ) : null}
          </div>
        ),
      }),
      createColumn<StoredExchangeRate>({
        accessorKey: "effective",
        labelKey: "rates.effective",
        enableSorting: true,
        filterVariant: "boolean",
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.effective ? t("table.true") : t("table.false"),
      }),
      createColumn<StoredExchangeRate>({
        accessorKey: "note",
        labelKey: "rates.columns.note",
        enableSorting: false,
        cell: ({ row }) => row.original.note ?? "",
      }),
      createColumn<StoredExchangeRate>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) =>
          canWrite && row.original.source === "manual" ? (
            <EntityRowActions
              actions={[
                {
                  id: "clear",
                  label: t("rates.clear_override"),
                  icon: Undo2,
                  variant: "destructive",
                  disabled: clearPending,
                  onSelect: () => clearOverride(row.original),
                },
              ]}
            />
          ) : null,
      }),
    ],
    [canWrite, clearOverride, clearPending, t],
  );

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

      <EntityTable
        columns={columns}
        data={day.data?.items ?? []}
        getRowId={(row) => `${row.base}-${row.quote}-${row.source}`}
        manual={CLIENT_SIDE_MANUAL}
        isLoading={day.isLoading}
        isError={day.isError}
        errorTitle={t("rates.error")}
        onRetry={() => void day.refetch()}
        emptyTitle={t("rates.empty")}
        emptyDescription=""
        initialState={{ pagination: { pageIndex: 0, pageSize: 50 } }}
        pageSizeOptions={[20, 50, 100]}
        features={{
          persistKey: EXCHANGE_RATES_PERSIST_KEY,
          rowSelection: false,
          inlineEdit: canWrite,
        }}
        onCellEdit={
          canWrite
            ? ({ row, columnId, value }) => {
                if (columnId !== "rate") return;
                const rate = String(value ?? "").trim();
                if (rate === row.rate) return;
                if (!RATE_PATTERN.test(rate)) {
                  appToast.error(t("rates.validation.rate"));
                  return;
                }
                override.mutate({
                  date: row.rate_date,
                  base: row.base,
                  quote: row.quote,
                  rate,
                  note: row.source === "manual" ? (row.note ?? "") : "",
                });
              }
            : undefined
        }
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void day.refetch()}
            refreshDisabled={day.isFetching}
          />
        }
      />
    </div>
  );
}
