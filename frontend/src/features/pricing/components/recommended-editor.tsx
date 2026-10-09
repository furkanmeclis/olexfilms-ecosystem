"use client";

import { useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Download, Percent, Trash2, Upload } from "lucide-react";
import { useMemo, useState } from "react";

import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { ToolbarIconButton } from "@/components/tables/toolbar-icon-button";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import { Money } from "@/features/accounting/components/shared";
import { catalogKeys } from "@/features/catalog/hooks/use-catalog-access";
import {
  catalogService,
  type CatalogProduct,
} from "@/features/catalog/services/catalog.service";
import { useCountries } from "@/features/geo/hooks/use-geo";
import { ImportWizard } from "@/features/io/components/import-wizard";
import {
  addDays,
  applyPercent,
  basketKey,
  dayValue,
  isoDay,
  normalizePublishPrice,
  recommendedCsv,
  type BasketRow,
} from "@/features/pricing/lib/recommended";
import {
  pricingKeys,
  recommendedService,
  RECOMMENDED_IMPORT_PATHS,
  type CurrentRecommendedPrice,
  type RecommendedPriceVersion,
} from "@/features/pricing/services/recommended.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const RECOMMENDED_EDITOR_PERSIST_KEY = "tenant-recommended-prices-v1";

/** Value of the country picker for the currency-wide price. */
const CURRENCY_WIDE = "none";
const FALLBACK_CURRENCIES = ["TRY", "EUR", "USD"];
const EXPORT_PAGE = 100;

export type EditorRow = {
  product: CatalogProduct;
  current: CurrentRecommendedPrice | null;
  pending: RecommendedPriceVersion | null;
  draft: BasketRow | null;
};

type RecommendedEditorProps = {
  slug: string;
  canWrite: boolean;
  basket: ReadonlyMap<string, BasketRow>;
  onBasketChange: (rows: BasketRow[], remove?: string[]) => void;
};

/**
 * Center > Recommended prices (TEC-507). The brand's products for one
 * country (or currency-wide) and currency: the price in force, its day and
 * the next scheduled version. A double-clicked "New price" cell or the
 * percent bulk action drafts a row into the publication basket.
 */
export function RecommendedEditor({
  slug,
  canWrite,
  basket,
  onBasketChange,
}: RecommendedEditorProps) {
  const { t, format, locale } = useLocale();
  const [country, setCountry] = useState(CURRENCY_WIDE);
  const [currency, setCurrency] = useState("TRY");
  const [percentFor, setPercentFor] = useState<EditorRow[] | null>(null);
  const [importOpen, setImportOpen] = useState(false);
  const [exporting, setExporting] = useState(false);
  const countryIso = country === CURRENCY_WIDE ? "" : country;

  const countries = useCountries();
  const countryOptions = useMemo(
    () =>
      (countries.data ?? []).map((c) => ({
        value: c.iso2,
        label: locale === "tr" ? c.name_tr : c.name_en,
      })),
    [countries.data, locale],
  );
  const currencyOptions = useMemo(() => {
    const set = new Set(FALLBACK_CURRENCIES);
    for (const c of countries.data ?? []) {
      if (c.default_currency) set.add(c.default_currency);
    }
    return [...set].sort();
  }, [countries.data]);

  const columns = useMemo(
    () =>
      [
        ...(canWrite ? [createSelectColumnDef<EditorRow>()] : []),
        createColumn<EditorRow>({
          id: "name",
          accessorFn: (row) => row.product.name,
          labelKey: "catalog.fields.name",
          enableSorting: true,
          enableHiding: false,
          enableColumnFilter: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.product.name}</span>
          ),
        }),
        createColumn<EditorRow>({
          id: "sku",
          accessorFn: (row) => row.product.sku,
          labelKey: "catalog.fields.sku",
          enableSorting: true,
          enableColumnFilter: false,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.product.sku}
            </span>
          ),
        }),
        createColumn<EditorRow>({
          id: "country",
          accessorFn: () => countryIso,
          labelKey: "catalog.recommended.fields.country",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.current?.scope === "currency" && countryIso ? (
              <Badge
                variant="outline"
                title={t("catalog.recommended.scope_currency_hint")}
              >
                {t("catalog.recommended.scope_currency")}
              </Badge>
            ) : countryIso ? (
              <span className="font-mono text-xs">{countryIso}</span>
            ) : (
              <span className="text-muted-foreground text-xs">
                {t("catalog.recommended.scope_currency")}
              </span>
            ),
        }),
        createColumn<EditorRow>({
          id: "currency",
          accessorFn: () => currency,
          labelKey: "catalog.recommended.fields.currency",
          enableSorting: false,
          enableColumnFilter: false,
          cell: () => <span className="font-mono text-xs">{currency}</span>,
        }),
        createColumn<EditorRow>({
          id: "current_price",
          accessorFn: (row) => row.current?.price ?? "",
          labelKey: "catalog.recommended.fields.current_price",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.current ? (
              <Money amount={row.original.current.price} currency={currency} />
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<EditorRow>({
          id: "effective_from",
          accessorFn: (row) => row.current?.effective_from ?? "",
          labelKey: "catalog.recommended.fields.effective_from",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.current ? (
              format.date(dayValue(row.original.current.effective_from))
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<EditorRow>({
          id: "pending",
          accessorFn: (row) => row.pending?.price ?? "",
          labelKey: "catalog.recommended.fields.pending",
          enableSorting: false,
          enableColumnFilter: false,
          cell: ({ row }) =>
            row.original.pending ? (
              <span className="flex flex-wrap items-center gap-2">
                <Money
                  amount={row.original.pending.price}
                  currency={currency}
                />
                <Badge variant="secondary">
                  {format.date(dayValue(row.original.pending.effective_from))}
                </Badge>
              </span>
            ) : (
              <span className="text-muted-foreground">—</span>
            ),
        }),
        createColumn<EditorRow>({
          id: "draft",
          accessorFn: (row) => row.draft?.price ?? "",
          labelKey: "catalog.recommended.fields.new_price",
          enableSorting: false,
          enableColumnFilter: false,
          enableHiding: false,
          ...(canWrite ? { editVariant: "text" as const } : {}),
          cell: ({ row }) =>
            row.original.draft ? (
              <Badge variant="warning" data-testid="draft-price">
                <Money amount={row.original.draft.price} currency={currency} />
              </Badge>
            ) : (
              <span className="text-muted-foreground text-xs">
                {canWrite ? t("catalog.recommended.edit_hint") : "—"}
              </span>
            ),
        }),
        createColumn<EditorRow>({
          id: "active",
          accessorFn: (row) => row.product.active,
          labelKey: "catalog.fields.active",
          enableSorting: true,
          filterVariant: "boolean",
          param: "active",
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.product.active
              ? t("catalog.status.active")
              : t("catalog.status.inactive"),
        }),
      ] as ColumnDef<EditorRow, unknown>[],
    [canWrite, countryIso, currency, format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "name",
    persistKey: RECOMMENDED_EDITOR_PERSIST_KEY,
  });
  const params = listState.params;

  const products = useQuery({
    queryKey: catalogKeys.products(params),
    queryFn: () => catalogService.listProducts(params),
  });
  const uuids = useMemo(
    () => (products.data?.items ?? []).map((p) => p.uuid),
    [products.data?.items],
  );
  const scopeQuery = {
    product: uuids.join(","),
    country,
    currency,
    limit: EXPORT_PAGE,
  };
  const current = useQuery({
    queryKey: pricingKeys.current(scopeQuery),
    queryFn: () => recommendedService.current(scopeQuery),
    enabled: uuids.length > 0,
  });
  const today = isoDay(new Date());
  const pendingQuery = {
    ...scopeQuery,
    effective_from_from: addDays(today, 1),
    sort: "effective_from",
  };
  const pending = useQuery({
    queryKey: pricingKeys.versions(pendingQuery),
    queryFn: () => recommendedService.versions(pendingQuery),
    enabled: uuids.length > 0,
  });

  const rows = useMemo<EditorRow[]>(() => {
    const byProduct = new Map(
      (current.data?.items ?? []).map((c) => [c.product_uuid, c]),
    );
    const next = new Map<string, RecommendedPriceVersion>();
    for (const v of pending.data?.items ?? []) {
      if (v.status === "scheduled" && !next.has(v.product_uuid)) {
        next.set(v.product_uuid, v);
      }
    }
    return (products.data?.items ?? []).map((product) => ({
      product,
      current: byProduct.get(product.uuid) ?? null,
      pending: next.get(product.uuid) ?? null,
      draft:
        basket.get(
          basketKey({
            productUuid: product.uuid,
            country: countryIso,
            currency,
          }),
        ) ?? null,
    }));
  }, [
    basket,
    countryIso,
    currency,
    current.data?.items,
    pending.data?.items,
    products.data?.items,
  ]);

  const draftFor = (row: EditorRow, price: string): BasketRow => ({
    productUuid: row.product.uuid,
    productName: row.product.name,
    productSku: row.product.sku,
    country: countryIso,
    currency,
    price,
    previous: row.current?.price ?? null,
  });

  const exportCsv = async () => {
    setExporting(true);
    try {
      const items: CurrentRecommendedPrice[] = [];
      for (let offset = 0; ; offset += EXPORT_PAGE) {
        const page = await recommendedService.current({
          country,
          currency,
          limit: EXPORT_PAGE,
          offset,
        });
        items.push(...page.items);
        if (page.items.length === 0 || items.length >= page.total) break;
      }
      const csv = recommendedCsv(
        items.map((i) => ({
          sku: i.product_sku,
          country: i.country_iso2,
          currency: i.currency,
          price: i.price,
        })),
      );
      const url = URL.createObjectURL(
        new Blob([csv], { type: "text/csv;charset=utf-8" }),
      );
      const a = document.createElement("a");
      a.href = url;
      a.download = `recommended-prices-${countryIso || "all"}-${currency}.csv`;
      a.click();
      URL.revokeObjectURL(url);
    } catch {
      appToast.error(t("exports.toast.failed"));
    } finally {
      setExporting(false);
    }
  };

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <Label>{t("catalog.recommended.fields.country")}</Label>
          <Select value={country} onValueChange={setCountry}>
            <SelectTrigger className="w-56" data-testid="country-picker">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value={CURRENCY_WIDE}>
                {t("catalog.recommended.scope_currency")}
              </SelectItem>
              {countryOptions.map((o) => (
                <SelectItem key={o.value} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1">
          <Label>{t("catalog.recommended.fields.currency")}</Label>
          <Select value={currency} onValueChange={setCurrency}>
            <SelectTrigger className="w-28" data-testid="currency-picker">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {currencyOptions.map((c) => (
                <SelectItem key={c} value={c}>
                  {c}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <p className="text-muted-foreground max-w-prose text-xs">
          {t("catalog.recommended.scope_hint")}
        </p>
      </div>
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.product.uuid}
        isLoading={products.isLoading}
        isError={products.isError}
        onRetry={() => void products.refetch()}
        emptyTitle={t("catalog.recommended.empty")}
        rowCount={products.data?.total ?? 0}
        state={listState.tableState}
        features={{
          persistKey: RECOMMENDED_EDITOR_PERSIST_KEY,
          rowSelection: canWrite,
          inlineEdit: canWrite,
          columnOrdering: true,
          columnPinning: true,
        }}
        bulkActions={
          canWrite
            ? [
                {
                  id: "percent",
                  label: t("catalog.recommended.bulk.percent"),
                  icon: Percent,
                  onClick: (selected) => setPercentFor(selected),
                },
                {
                  id: "remove",
                  label: t("catalog.recommended.bulk.remove"),
                  icon: Trash2,
                  onClick: (selected) =>
                    onBasketChange(
                      [],
                      selected.map((r) =>
                        basketKey({
                          productUuid: r.product.uuid,
                          country: countryIso,
                          currency,
                        }),
                      ),
                    ),
                },
              ]
            : undefined
        }
        onCellEdit={({ row, columnId, value }) => {
          if (columnId !== "draft") return;
          const raw = String(value ?? "").trim();
          const key = basketKey({
            productUuid: row.product.uuid,
            country: countryIso,
            currency,
          });
          if (!raw) {
            onBasketChange([], [key]);
            return;
          }
          const price = normalizePublishPrice(raw);
          if (price === null) {
            appToast.error(t("catalog.recommended.invalid_price"));
            return;
          }
          onBasketChange([draftFor(row, price)]);
        }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => {
              void products.refetch();
              void current.refetch();
              void pending.refetch();
            }}
            refreshDisabled={products.isFetching}
          >
            <ToolbarIconButton
              label={t("catalog.recommended.export")}
              onClick={() => void exportCsv()}
              disabled={exporting}
            >
              <Download className="size-4" />
            </ToolbarIconButton>
            {canWrite ? (
              <ToolbarIconButton
                label={t("imports.menu_label")}
                onClick={() => setImportOpen(true)}
              >
                <Upload className="size-4" />
              </ToolbarIconButton>
            ) : null}
          </EntityToolbar>
        }
      />
      {percentFor ? (
        <PercentDialog
          count={percentFor.length}
          onOpenChange={(open) => {
            if (!open) setPercentFor(null);
          }}
          onApply={(pct) => {
            const drafts: BasketRow[] = [];
            for (const row of percentFor) {
              const base = row.draft?.price ?? row.current?.price;
              const next = base ? applyPercent(base, pct) : null;
              if (next !== null) drafts.push(draftFor(row, next));
            }
            onBasketChange(drafts);
            if (drafts.length < percentFor.length) {
              appToast.warning(t("catalog.recommended.bulk.skipped"));
            }
            setPercentFor(null);
          }}
        />
      ) : null}
      {canWrite ? (
        <ImportWizard
          resource="tenant.pricing.recommended"
          paths={RECOMMENDED_IMPORT_PATHS}
          scope="tenant"
          open={importOpen}
          onOpenChange={setImportOpen}
          jobsHref={routes.tenant.imports.root(slug)}
          onComplete={() => {
            void current.refetch();
            void pending.refetch();
          }}
        />
      ) : null}
    </div>
  );
}

/** Percent change of the selected rows' price (in force, else drafted). */
function PercentDialog({
  count,
  onOpenChange,
  onApply,
}: {
  count: number;
  onOpenChange: (open: boolean) => void;
  onApply: (pct: number) => void;
}) {
  const { t } = useLocale();
  const [raw, setRaw] = useState("");
  const pct = Number(raw.replace(",", "."));
  const valid = raw.trim() !== "" && Number.isFinite(pct) && pct > -100;
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent data-testid="percent-dialog">
        <DialogHeader>
          <DialogTitle>{t("catalog.recommended.bulk.percent")}</DialogTitle>
          <DialogDescription>
            {t("catalog.recommended.bulk.percent_description", { count })}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(event) => {
            event.preventDefault();
            if (valid) onApply(pct);
          }}
        >
          <div className="space-y-2">
            <Label htmlFor="recommended-percent">
              {t("catalog.recommended.bulk.percent_label")}
            </Label>
            <Input
              id="recommended-percent"
              inputMode="decimal"
              dir="ltr"
              value={raw}
              onChange={(event) => setRaw(event.target.value)}
            />
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!valid}>
              {t("catalog.recommended.bulk.apply")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
