"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, FilterFn } from "@tanstack/react-table";
import {
  Bell,
  Check,
  Eye,
  FileText,
  Plus,
  Save,
  Send,
  Trash2,
  X,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  EntityRowActions,
  EntityTable,
  clientDateRangeFilter,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { catalogService } from "@/features/catalog/services/catalog.service";
import { leadInputClass } from "@/features/leads/components/lead-fields";
import {
  fromDateInput,
  linesBody,
  linesOf,
  newLine,
  quoteEditable,
  quoteStatusTone,
  toDateInput,
  validateLines,
  type QuoteLineDraft,
} from "@/features/leads/lib/quotes";
import { leadKeys } from "@/features/leads/services/leads.service";
import {
  quoteKeys,
  quotesService,
  type DocumentRender,
  type Quote,
  type QuoteStatus,
} from "@/features/leads/services/quotes.service";
import { serviceCatalogService } from "@/features/service-catalog/services/service-catalog.service";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { foldSearch } from "@/lib/utils/search";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const LEAD_QUOTES_PERSIST_KEY = "tenant-lead-quotes-v1";
const QUOTE_STATUSES: QuoteStatus[] = [
  "draft",
  "sent",
  "accepted",
  "rejected",
  "expired",
];
const PDF_POLL_MS = 1500;
const PDF_POLL_MAX = 40;

function Money({ value, currency }: { value: string; currency: string }) {
  return (
    <span className="tabular-nums" dir="ltr">
      {value} {currency}
    </span>
  );
}

function errorMessage(err: unknown, fallback: string) {
  return isApiError(err) && err.message ? err.message : fallback;
}

/**
 * "Teklifler" tab of the lead detail (TEC-319): the lead's quotes as a
 * client-side DataTable (one lead has a handful of quotes, the endpoint
 * returns them all) and the editor of the selected quote.
 */
export function LeadQuotesTab({ leadUuid }: { leadUuid: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canRead = can(permissions.quotes.read);
  const canWrite = can(permissions.quotes.write);
  const [selected, setSelected] = useState<string | null>(null);
  const list = useQuery({
    queryKey: quoteKeys.byLead(leadUuid),
    queryFn: () => quotesService.listByLead(leadUuid),
    enabled: canRead,
  });
  const create = useMutation({
    mutationFn: () => quotesService.create(leadUuid, { lines: [] }),
    onSuccess: async (quote) => {
      setSelected(quote.uuid);
      await qc.invalidateQueries({ queryKey: quoteKeys.byLead(leadUuid) });
      await qc.invalidateQueries({ queryKey: leadKeys.events(leadUuid) });
      toast.success(t("leads.quotes.created"));
    },
    onError: (err) => toast.error(errorMessage(err, t("leads.quotes.error"))),
  });

  const items = useMemo(() => list.data?.items ?? [], [list.data]);
  const current =
    items.find((q) => q.uuid === selected) ?? (selected ? null : items[0]);

  const columns = useMemo(
    () =>
      [
        createColumn<Quote>({
          accessorKey: "display_no",
          labelKey: "leads.quotes.columns.no",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono font-medium" dir="ltr">
              {row.original.display_no}
            </span>
          ),
        }),
        createColumn<Quote>({
          accessorKey: "status",
          labelKey: "leads.quotes.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: QUOTE_STATUSES.map((s) => ({
            value: s,
            label: t(`leads.quotes.status.${s}`),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`leads.quotes.status.${row.original.status}`)}
              tone={quoteStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Quote>({
          id: "grand_total",
          accessorFn: (row) => Number(row.grand_total),
          labelKey: "leads.quotes.columns.grand_total",
          enableSorting: true,
          cell: ({ row }) => (
            <Money
              value={row.original.grand_total}
              currency={row.original.currency}
            />
          ),
        }),
        createColumn<Quote>({
          accessorKey: "valid_until",
          labelKey: "leads.quotes.columns.valid_until",
          enableSorting: true,
          sortUndefined: "last",
          cell: ({ row }) =>
            row.original.valid_until
              ? format.date(row.original.valid_until)
              : "—",
        }),
        createColumn<Quote>({
          accessorKey: "created_at",
          labelKey: "leads.quotes.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          filterFn: clientDateRangeFilter as FilterFn<Quote>,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<Quote>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const actions: EntityRowAction[] = [
              {
                id: "open",
                label: t("common.open"),
                icon: Eye,
                onSelect: () => setSelected(row.original.uuid),
              },
            ];
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<Quote, unknown>[],
    [format, t],
  );

  if (!canRead) {
    return (
      <ErrorState
        title={t("common.error_forbidden")}
        description={t("leads.quotes.forbidden")}
      />
    );
  }

  return (
    <div className="space-y-6" data-testid="lead-quotes">
      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
          <CardTitle>{t("leads.quotes.title")}</CardTitle>
          {canWrite ? (
            <Button
              type="button"
              size="sm"
              data-testid="quote-new"
              disabled={create.isPending}
              onClick={() => create.mutate()}
            >
              <Plus className="size-4" />
              {t("leads.quotes.new")}
            </Button>
          ) : null}
        </CardHeader>
        <CardContent>
          {list.isError ? (
            <ErrorState
              title={t("common.error_generic")}
              retryLabel={t("common.retry")}
              onRetry={() => void list.refetch()}
            />
          ) : (
            <EntityTable
              columns={columns}
              data={items}
              getRowId={(row) => row.uuid}
              manual={CLIENT_SIDE_MANUAL}
              isLoading={list.isLoading}
              onRowClick={(row) => setSelected(row.uuid)}
              initialState={{
                pagination: { pageIndex: 0, pageSize: 10 },
                sorting: [{ id: "created_at", desc: true }],
              }}
              emptyTitle={t("leads.quotes.empty")}
              emptyDescription=""
              features={{ persistKey: LEAD_QUOTES_PERSIST_KEY }}
            />
          )}
        </CardContent>
      </Card>
      {current ? (
        <QuoteEditor
          key={`${current.uuid}:${current.updated_at}`}
          quote={current}
          leadUuid={leadUuid}
        />
      ) : null}
    </div>
  );
}

function useSearchOptions() {
  const { t } = useLocale();
  const labels = useRef(new Map<string, string>());
  const remember = (options: ComboboxOption[]) => {
    for (const o of options) labels.current.set(o.value, o.label);
    return options;
  };
  const loadProducts = useCallback(
    async (query: string) =>
      remember(
        (await catalogService.listProducts({ q: query, limit: 20 })).items.map(
          (p) => ({
            value: p.uuid,
            label: p.name,
            description: p.sku,
          }),
        ),
      ),
    [],
  );
  const loadCatalog = useCallback(
    async (query: string) => {
      const items = (await serviceCatalogService.listVisible()).items;
      const needle = foldSearch(query);
      return remember(
        items
          .filter((i) => i.is_active)
          .filter((i) => !needle || foldSearch(i.name).includes(needle))
          .slice(0, 50)
          .map((i) => ({
            value: i.uuid,
            label: i.name,
            description: t(`leads.quotes.recurrence.${i.recurrence}`),
          })),
      );
    },
    [t],
  );
  return { labels, loadProducts, loadCatalog };
}

/**
 * Editor of one quote. Lines are a local editable form (the editor is
 * exempt from the DataTable rule); saving replaces them as a whole and
 * every total — line and quote — is the backend's value. Sent and decided
 * quotes are read-only.
 */
export function QuoteEditor({
  quote,
  leadUuid,
}: {
  quote: Quote;
  leadUuid: string;
}) {
  const { t, format, locale } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canWrite = can(permissions.quotes.write);
  const canPrice = can(permissions.pricing.saleWrite);
  const editable = canWrite && quoteEditable(quote);
  const [lines, setLines] = useState<QuoteLineDraft[]>(() => linesOf(quote));
  const [validUntil, setValidUntil] = useState(() =>
    toDateInput(quote.valid_until),
  );
  const [dirty, setDirty] = useState(false);
  const [errors, setErrors] = useState<Record<string, string>>({});
  const [publicUrl, setPublicUrl] = useState("");
  const [rejectOpen, setRejectOpen] = useState(false);
  const [reason, setReason] = useState("");
  const [pdfOpen, setPdfOpen] = useState(false);
  const { labels, loadProducts, loadCatalog } = useSearchOptions();

  const refresh = async (updated?: Quote) => {
    if (updated) qc.setQueryData(quoteKeys.detail(updated.uuid), updated);
    await qc.invalidateQueries({ queryKey: quoteKeys.byLead(leadUuid) });
    await qc.invalidateQueries({ queryKey: leadKeys.events(leadUuid) });
    await qc.invalidateQueries({ queryKey: leadKeys.detail(leadUuid) });
  };
  const fail = (err: unknown) =>
    toast.error(errorMessage(err, t("leads.quotes.error")));

  const save = useMutation({
    mutationFn: async () => {
      let out = await quotesService.replaceLines(
        quote.uuid,
        linesBody(lines, canPrice),
      );
      if (validUntil !== toDateInput(quote.valid_until)) {
        out = await quotesService.patch(quote.uuid, {
          valid_until: fromDateInput(validUntil),
        });
      }
      return out;
    },
    onSuccess: async (updated) => {
      await refresh(updated);
      toast.success(t("leads.quotes.saved"));
    },
    onError: fail,
  });
  const send = useMutation({
    mutationFn: () => quotesService.send(quote.uuid),
    onSuccess: async (out) => {
      setPublicUrl(out.public_url);
      await refresh(out.quote);
      toast.success(t("leads.quotes.sent"));
    },
    onError: fail,
  });
  const remind = useMutation({
    mutationFn: () => quotesService.remind(quote.uuid),
    onSuccess: async (updated) => {
      await refresh(updated);
      toast.success(t("leads.quotes.reminded"));
    },
    onError: fail,
  });
  const decide = useMutation({
    mutationFn: (accepted: boolean) =>
      accepted
        ? quotesService.accept(quote.uuid)
        : quotesService.reject(quote.uuid, reason.trim() || undefined),
    onSuccess: async (updated) => {
      setRejectOpen(false);
      await refresh(updated);
      toast.success(
        t(
          updated.status === "accepted"
            ? "leads.quotes.accepted"
            : "leads.quotes.rejected",
        ),
      );
    },
    onError: fail,
  });

  const edit = (key: string, patch: Partial<QuoteLineDraft>) => {
    setDirty(true);
    setLines((all) =>
      all.map((l) =>
        l.key === key ? { ...l, ...patch, line_total: null } : l,
      ),
    );
  };
  const add = (type: QuoteLineDraft["line_type"], ref: string) => {
    if (!ref) return;
    setDirty(true);
    setLines((all) => [
      ...all,
      newLine(type, ref, labels.current.get(ref) ?? ""),
    ]);
  };
  const onSave = () => {
    const local = validateLines(lines);
    setErrors(local);
    if (Object.keys(local).length === 0) save.mutate();
  };
  const busy =
    save.isPending || send.isPending || remind.isPending || decide.isPending;

  const numberInput = (
    line: QuoteLineDraft,
    field: "quantity" | "unit_price" | "discount_amount",
    readOnly: boolean,
  ) => {
    const err = errors[`${line.key}.${field}`];
    return (
      <div className="space-y-1">
        <Label className="text-xs md:sr-only">
          {t(`leads.quotes.line.${field}`)}
        </Label>
        <input
          data-testid={`quote-line-${field}`}
          className={cn(
            leadInputClass,
            "tabular-nums",
            err && "border-destructive",
          )}
          inputMode="decimal"
          dir="ltr"
          value={line[field]}
          placeholder={
            field === "unit_price" ? t("leads.quotes.line.auto_price") : ""
          }
          readOnly={readOnly}
          disabled={!editable}
          onChange={(e) => edit(line.key, { [field]: e.target.value })}
        />
        {err ? <p className="text-destructive text-xs">{t(err)}</p> : null}
      </div>
    );
  };

  return (
    <Card data-testid="quote-editor" data-status={quote.status}>
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
        <CardTitle className="flex items-center gap-2">
          <span dir="ltr">{quote.display_no}</span>
          <StatusChip
            label={t(`leads.quotes.status.${quote.status}`)}
            tone={quoteStatusTone(quote.status)}
          />
        </CardTitle>
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            data-testid="quote-pdf"
            onClick={() => setPdfOpen(true)}
          >
            <FileText className="size-4" />
            {t("leads.quotes.pdf")}
          </Button>
          {editable ? (
            <>
              <Button
                type="button"
                size="sm"
                variant="outline"
                data-testid="quote-save"
                disabled={busy}
                onClick={onSave}
              >
                <Save className="size-4" />
                {t("leads.quotes.save")}
              </Button>
              <Button
                type="button"
                size="sm"
                data-testid="quote-send"
                disabled={busy || dirty || quote.lines.length === 0}
                title={dirty ? t("leads.quotes.save_first") : undefined}
                onClick={() => send.mutate()}
              >
                <Send className="size-4" />
                {t("leads.quotes.send")}
              </Button>
            </>
          ) : null}
          {canWrite && quote.status === "sent" ? (
            <>
              <Button
                type="button"
                size="sm"
                variant="outline"
                data-testid="quote-remind"
                disabled={busy}
                onClick={() => remind.mutate()}
              >
                <Bell className="size-4" />
                {t("leads.quotes.remind")}
              </Button>
              <Button
                type="button"
                size="sm"
                data-testid="quote-accept"
                disabled={busy}
                onClick={() => decide.mutate(true)}
              >
                <Check className="size-4" />
                {t("leads.quotes.accept")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="destructive"
                data-testid="quote-reject"
                disabled={busy}
                onClick={() => setRejectOpen(true)}
              >
                <X className="size-4" />
                {t("leads.quotes.reject")}
              </Button>
            </>
          ) : null}
        </div>
      </CardHeader>
      <CardContent className="space-y-6">
        {!quoteEditable(quote) ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="quote-locked"
          >
            {t("leads.quotes.locked")}
          </p>
        ) : null}
        {publicUrl ? (
          <p className="text-sm" data-testid="quote-public-url">
            {t("leads.quotes.public_url")}:{" "}
            <a
              href={publicUrl}
              target="_blank"
              rel="noreferrer"
              className="underline"
              dir="ltr"
            >
              {publicUrl}
            </a>
          </p>
        ) : null}
        {rejectOpen ? (
          <div className="space-y-2" data-testid="quote-reject-form">
            <Label htmlFor="quote-reject-reason">
              {t("leads.quotes.reject_reason")}
            </Label>
            <Textarea
              id="quote-reject-reason"
              rows={2}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
            <div className="flex justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() => setRejectOpen(false)}
              >
                {t("leads.form.cancel")}
              </Button>
              <Button
                type="button"
                variant="destructive"
                size="sm"
                data-testid="quote-reject-confirm"
                disabled={busy}
                onClick={() => decide.mutate(false)}
              >
                {t("leads.quotes.reject")}
              </Button>
            </div>
          </div>
        ) : null}

        <div className="grid gap-4 sm:grid-cols-2">
          <div className="space-y-1.5">
            <Label htmlFor="quote-valid-until">
              {t("leads.quotes.valid_until")}
            </Label>
            <div data-testid="quote-valid-until">
              <DatePicker
                id="quote-valid-until"
                value={validUntil}
                disabled={!editable}
                onChange={(value) => {
                  setDirty(true);
                  setValidUntil(value);
                }}
              />
            </div>
          </div>
          <div className="text-muted-foreground self-end text-xs">
            {t("leads.quotes.created_at")}: {format.dateTime(quote.created_at)}
          </div>
        </div>

        <div className="space-y-3" data-testid="quote-lines">
          <div className="text-muted-foreground hidden gap-2 text-xs font-medium md:grid md:grid-cols-[minmax(0,2fr)_100px_130px_130px_130px_40px]">
            <span>{t("leads.quotes.line.description")}</span>
            <span>{t("leads.quotes.line.quantity")}</span>
            <span>{t("leads.quotes.line.unit_price")}</span>
            <span>{t("leads.quotes.line.discount_amount")}</span>
            <span className="text-end">
              {t("leads.quotes.line.line_total")}
            </span>
            <span />
          </div>
          {lines.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("leads.quotes.no_lines")}
            </p>
          ) : null}
          {lines.map((line) => (
            <div
              key={line.key}
              data-testid="quote-line"
              className="grid items-start gap-2 border-b pb-3 md:grid-cols-[minmax(0,2fr)_100px_130px_130px_130px_40px] md:border-0 md:pb-0"
            >
              <div className="space-y-1">
                <Label className="text-xs md:sr-only">
                  {t("leads.quotes.line.description")}
                </Label>
                <input
                  data-testid="quote-line-description"
                  className={leadInputClass}
                  value={line.description}
                  maxLength={500}
                  disabled={!editable}
                  onChange={(e) =>
                    edit(line.key, { description: e.target.value })
                  }
                />
                <span className="text-muted-foreground text-xs">
                  {t(`leads.quotes.line_type.${line.line_type}`)}
                </span>
              </div>
              {numberInput(line, "quantity", false)}
              {numberInput(line, "unit_price", !canPrice)}
              {numberInput(line, "discount_amount", false)}
              <div
                className="flex h-10 items-center justify-end text-sm font-medium"
                data-testid="quote-line-total"
              >
                {line.line_total !== null ? (
                  <Money value={line.line_total} currency={quote.currency} />
                ) : (
                  <span className="text-muted-foreground text-xs">
                    {t("leads.quotes.line.pending")}
                  </span>
                )}
              </div>
              {editable ? (
                <Button
                  type="button"
                  size="icon"
                  variant="ghost"
                  aria-label={t("leads.quotes.line.remove")}
                  data-testid="quote-line-remove"
                  onClick={() => {
                    setDirty(true);
                    setLines((all) => all.filter((l) => l.key !== line.key));
                  }}
                >
                  <Trash2 className="size-4" />
                </Button>
              ) : null}
            </div>
          ))}
          {editable ? (
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1.5" data-testid="quote-add-product">
                <Label>{t("leads.quotes.add_product")}</Label>
                <AsyncCombobox
                  value=""
                  loadOptions={loadProducts}
                  placeholder={t("leads.quotes.search_product")}
                  searchPlaceholder={t("leads.quotes.search_product")}
                  emptyText={t("leads.quotes.search_empty")}
                  onValueChange={(v) => add("product", v)}
                />
              </div>
              <div className="space-y-1.5" data-testid="quote-add-service">
                <Label>{t("leads.quotes.add_service")}</Label>
                <AsyncCombobox
                  value=""
                  loadOptions={loadCatalog}
                  placeholder={t("leads.quotes.search_service")}
                  searchPlaceholder={t("leads.quotes.search_service")}
                  emptyText={t("leads.quotes.search_empty")}
                  onValueChange={(v) => add("catalog_service", v)}
                />
              </div>
            </div>
          ) : null}
          {editable && !canPrice ? (
            <p
              className="text-muted-foreground text-xs"
              data-testid="quote-price-locked"
            >
              {t("leads.quotes.price_locked")}
            </p>
          ) : null}
        </div>

        <dl
          className="ms-auto grid max-w-xs grid-cols-2 gap-x-6 gap-y-1 text-sm"
          data-testid="quote-totals"
        >
          <dt className="text-muted-foreground">
            {t("leads.quotes.subtotal")}
          </dt>
          <dd className="text-end" data-testid="quote-subtotal">
            <Money value={quote.subtotal} currency={quote.currency} />
          </dd>
          <dt className="text-muted-foreground">
            {t("leads.quotes.discount_total")}
          </dt>
          <dd className="text-end">
            <Money value={quote.discount_total} currency={quote.currency} />
          </dd>
          <dt className="text-muted-foreground">
            {t("leads.quotes.tax_total")}
          </dt>
          <dd className="text-end">
            <Money value={quote.tax_total} currency={quote.currency} />
          </dd>
          <dt className="font-medium">{t("leads.quotes.grand_total")}</dt>
          <dd
            className="text-end font-semibold"
            data-testid="quote-grand-total"
          >
            <Money value={quote.grand_total} currency={quote.currency} />
          </dd>
        </dl>
        {dirty ? (
          <p className="text-muted-foreground text-xs">
            {t("leads.quotes.totals_after_save")}
          </p>
        ) : null}
      </CardContent>
      <QuotePdfDialog
        quote={quote}
        locale={locale}
        open={pdfOpen}
        onOpenChange={setPdfOpen}
      />
    </Card>
  );
}

function sleep(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

async function renderPdf(quoteUuid: string, locale?: string) {
  let render: DocumentRender = await quotesService.requestPdf(
    quoteUuid,
    locale,
  );
  for (let i = 0; render.status !== "ready" && i < PDF_POLL_MAX; i++) {
    if (render.status === "failed") break;
    await sleep(PDF_POLL_MS);
    render = await quotesService.getRender(render.uuid);
  }
  if (render.status !== "ready") {
    throw new Error(render.error ?? "render failed");
  }
  return quotesService.downloadRender(render);
}

/** PDF preview of a quote (documents module render, polled). */
function QuotePdfDialog({
  quote,
  locale,
  open,
  onOpenChange,
}: {
  quote: Quote;
  locale?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const pdf = useQuery({
    queryKey: [
      ...quoteKeys.detail(quote.uuid),
      "pdf",
      quote.updated_at,
      locale,
    ],
    queryFn: () => renderPdf(quote.uuid, locale),
    enabled: open,
    staleTime: Infinity,
    gcTime: 0,
    retry: false,
  });
  const blob = pdf.data?.blob;
  const url = useMemo(() => (blob ? URL.createObjectURL(blob) : ""), [blob]);
  useEffect(
    () => () => {
      if (url) URL.revokeObjectURL(url);
    },
    [url],
  );
  const state = pdf.isError ? "error" : pdf.isFetching ? "loading" : "idle";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl" data-testid="quote-pdf-dialog">
        <DialogHeader>
          <DialogTitle>
            {t("leads.quotes.pdf")} · <span dir="ltr">{quote.display_no}</span>
          </DialogTitle>
        </DialogHeader>
        {state === "loading" ? (
          <p className="text-muted-foreground text-sm">
            {t("leads.quotes.pdf_loading")}
          </p>
        ) : state === "error" ? (
          <p className="text-destructive text-sm">
            {t("leads.quotes.pdf_error")}
          </p>
        ) : url ? (
          <div className="space-y-2">
            <iframe
              title={quote.display_no}
              src={url}
              className="h-[70vh] w-full rounded-md border"
            />
            <div className="flex justify-end">
              <Button asChild size="sm" variant="outline">
                <a href={url} download={`${quote.display_no}.pdf`}>
                  {t("leads.quotes.pdf_download")}
                </a>
              </Button>
            </div>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
