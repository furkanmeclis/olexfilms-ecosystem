"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Ban,
  Download,
  Eye,
  FileCode,
  FileDown,
  RefreshCw,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { EinvoiceShell } from "@/features/einvoice/components/einvoice-shell";
import { VoidDialog } from "@/features/einvoice/components/void-dialog";
import {
  EINVOICE_PROFILES,
  EINVOICE_STATUSES,
  amount,
  canRetryPdf,
  canVoid,
  hasFiles,
  invoicesCsv,
  statusTone,
  validationTone,
} from "@/features/einvoice/lib/einvoice";
import {
  einvoiceKeys,
  einvoiceService,
  type Einvoice,
  type ListQuery,
} from "@/features/einvoice/services/einvoice.service";
import { download } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const INVOICES_PERSIST_KEY = "tenant-einvoices-v1";
const EXPORT_PAGE = 100;

/** Row and detail downloads; the API error toast names a failure. */
export function useEinvoiceFiles() {
  const { t } = useLocale();
  const qc = useQueryClient();
  const retry = useMutation({
    mutationFn: (uuid: string) => einvoiceService.retryPdf(uuid),
    onSuccess: (invoice) => {
      qc.setQueryData(einvoiceKeys.detail(invoice.uuid), invoice);
      void qc.invalidateQueries({ queryKey: einvoiceKeys.all });
      appToast.success(t("einvoice.pdf.retry_queued"));
    },
  });
  const fetchFile = useCallback(
    (invoice: Einvoice, kind: "xml" | "pdf") =>
      void einvoiceService.download(invoice, kind).catch(() => undefined),
    [],
  );
  return useMemo(() => ({ retry, fetchFile }), [retry, fetchFile]);
}

function InvoicesTable({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const router = useRouter();
  const files = useEinvoiceFiles();
  const [voidTarget, setVoidTarget] = useState<Einvoice | null>(null);
  const [exporting, setExporting] = useState(false);

  const columns = useMemo(
    () =>
      [
        createColumn<Einvoice>({
          id: "number",
          accessorKey: "number",
          labelKey: "einvoice.columns.number",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <Link
              href={routes.tenant.einvoices.detail(slug, row.original.uuid)}
              className="font-mono text-xs font-medium whitespace-nowrap hover:underline"
              dir="ltr"
              data-testid="einvoice-row"
              onClick={(event) => event.stopPropagation()}
            >
              {row.original.number ?? t("einvoice.draft_number")}
            </Link>
          ),
        }),
        createColumn<Einvoice>({
          id: "ettn",
          accessorKey: "uuid",
          labelKey: "einvoice.columns.ettn",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.uuid}
            </span>
          ),
        }),
        createColumn<Einvoice>({
          id: "buyer",
          accessorFn: (row) => row.buyer_organization?.name ?? row.buyer.name,
          labelKey: "einvoice.columns.buyer",
          enableSorting: false,
          gridSecondary: true,
          cell: ({ row }) =>
            row.original.buyer_organization?.name ?? row.original.buyer.name,
        }),
        createColumn<Einvoice>({
          id: "profile",
          accessorKey: "profile",
          labelKey: "einvoice.columns.profile",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: EINVOICE_PROFILES.map((value) => ({
            value,
            label: value,
            labelKey: `einvoice.profiles.${value}`,
          })),
          param: "profile",
          cell: ({ row }) => t(`einvoice.profiles.${row.original.profile}`),
        }),
        createColumn<Einvoice>({
          id: "issue_date",
          accessorKey: "issue_date",
          labelKey: "einvoice.columns.issue_date",
          enableSorting: true,
          filterVariant: "date-range",
          param: "issue_date",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.issue_date)}
            </span>
          ),
        }),
        createColumn<Einvoice>({
          id: "payable",
          accessorKey: "payable",
          labelKey: "einvoice.columns.payable",
          enableSorting: true,
          filterVariant: "number-range",
          param: "payable",
          cell: ({ row }) => (
            <span
              className="font-medium whitespace-nowrap tabular-nums"
              dir="ltr"
            >
              {format.currency(
                amount(row.original.payable),
                row.original.currency,
              )}
            </span>
          ),
        }),
        createColumn<Einvoice>({
          id: "status",
          accessorKey: "status",
          labelKey: "einvoice.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: EINVOICE_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `einvoice.statuses.${value}`,
          })),
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={t(`einvoice.statuses.${row.original.status}`)}
              tone={statusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Einvoice>({
          id: "validation",
          accessorKey: "validation_status",
          labelKey: "einvoice.columns.validation",
          enableSorting: false,
          cell: ({ row }) => (
            <StatusChip
              label={t(`einvoice.validation.${row.original.validation_status}`)}
              tone={validationTone(row.original.validation_status)}
            />
          ),
        }),
        createColumn<Einvoice>({
          id: "created_at",
          accessorKey: "created_at",
          labelKey: "einvoice.columns.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<Einvoice>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const inv = row.original;
            const items: EntityRowAction[] = [
              {
                id: "preview",
                label: t("einvoice.actions.preview"),
                icon: Eye,
                onSelect: () =>
                  router.push(routes.tenant.einvoices.detail(slug, inv.uuid)),
              },
            ];
            if (hasFiles(inv)) {
              items.push({
                id: "xml",
                label: t("einvoice.actions.download_xml"),
                icon: FileCode,
                onSelect: () => files.fetchFile(inv, "xml"),
              });
              items.push({
                id: "pdf",
                label: t("einvoice.actions.download_pdf"),
                icon: FileDown,
                disabled: !inv.has_pdf,
                onSelect: () => files.fetchFile(inv, "pdf"),
              });
            }
            if (canRetryPdf(inv)) {
              items.push({
                id: "retry-pdf",
                label: t("einvoice.actions.retry_pdf"),
                icon: RefreshCw,
                permission: permissions.einvoice.manage,
                disabled: files.retry.isPending,
                onSelect: () => files.retry.mutate(inv.uuid),
              });
            }
            if (canVoid(inv)) {
              items.push({
                id: "void",
                label: t("einvoice.actions.void"),
                icon: Ban,
                variant: "destructive",
                permission: permissions.einvoice.manage,
                separatorBefore: true,
                onSelect: () => setVoidTarget(inv),
              });
            }
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<Einvoice, unknown>[],
    [files, format, router, slug, t],
  );

  // Column meta drives the params: status / profile (CSV),
  // issue_date_from / _to, payable_min / _max; sort is one backend field.
  const listState = useServerListState({
    columns,
    initialSort: "-issue_date",
    initialPageSize: 20,
    persistKey: INVOICES_PERSIST_KEY,
  });
  const params: ListQuery = listState.params;
  const list = useQuery({
    queryKey: einvoiceKeys.list(params),
    queryFn: () => einvoiceService.list(params),
  });
  const total = list.data?.total ?? 0;
  const filtered =
    listState.tableState.columnFilters.length > 0 || Boolean(params.q);

  // The list has no export job: page through the same query into a CSV.
  const exportCsv = async () => {
    setExporting(true);
    try {
      const rows: Einvoice[] = [];
      for (let offset = 0; ; offset += EXPORT_PAGE) {
        const page = await einvoiceService.list({
          ...params,
          limit: EXPORT_PAGE,
          offset,
        });
        rows.push(...page.items);
        if (page.items.length === 0 || rows.length >= page.total) break;
      }
      download(
        new Blob([invoicesCsv(rows)], { type: "text/csv;charset=utf-8" }),
        "einvoices.csv",
      );
    } catch {
      appToast.error(t("exports.toast.failed"));
    } finally {
      setExporting(false);
    }
  };

  return (
    <>
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(inv) =>
          router.push(routes.tenant.einvoices.detail(slug, inv.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("einvoice.list.empty_title")}
        emptyDescription={
          filtered
            ? t("einvoice.list.empty_filtered")
            : t("einvoice.list.empty_description")
        }
        rowCount={total}
        state={listState.tableState}
        features={{ persistKey: INVOICES_PERSIST_KEY }}
        renderGridItem={(inv) => (
          <div className="space-y-1" data-testid="einvoice-card">
            <div className="flex items-start justify-between gap-2">
              <span className="font-mono text-xs font-medium" dir="ltr">
                {inv.number ?? t("einvoice.draft_number")}
              </span>
              <StatusChip
                label={t(`einvoice.statuses.${inv.status}`)}
                tone={statusTone(inv.status)}
              />
            </div>
            <p className="font-medium">
              {inv.buyer_organization?.name ?? inv.buyer.name}
            </p>
            <p className="text-muted-foreground text-xs">
              {format.date(inv.issue_date)} ·{" "}
              <span dir="ltr">
                {format.currency(amount(inv.payable), inv.currency)}
              </span>
            </p>
          </div>
        )}
        toolbarExtra={
          <>
            <Button
              type="button"
              variant="outline"
              size="sm"
              className="h-8"
              disabled={exporting || total === 0}
              onClick={() => void exportCsv()}
              data-testid="einvoice-export"
            >
              <Download className="size-4" />
              {t("einvoice.list.export")}
            </Button>
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
      {voidTarget ? (
        <VoidDialog
          key={voidTarget.uuid}
          invoice={voidTarget}
          open
          onOpenChange={(open) => {
            if (!open) setVoidTarget(null);
          }}
        />
      ) : null}
    </>
  );
}

/**
 * Muhasebe > e-Fatura > Faturalar (TEC-504): GET /v1/einvoices with
 * number / ETTN / buyer search, status and profile facets, issue date and
 * amount ranges, the validation badge, row actions (preview, XML / PDF,
 * PDF re-render, void with a reason) and a CSV of the filtered list.
 */
export function InvoicesPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  return (
    <EinvoiceShell
      slug={slug}
      section="list"
      title={t("einvoice.title")}
      description={t("einvoice.list.description")}
    >
      <InvoicesTable slug={slug} />
    </EinvoiceShell>
  );
}
