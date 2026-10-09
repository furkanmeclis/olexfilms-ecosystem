"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  AlertTriangle,
  Archive,
  Ban,
  FileCode,
  FileDown,
  RefreshCw,
} from "lucide-react";
import Link from "next/link";
import { useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { Timeline } from "@/components/common/timeline";
import { ConfirmDialog } from "@/components/dialogs/confirm-dialog";
import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  EinvoiceShell,
  useEinvoiceAccess,
} from "@/features/einvoice/components/einvoice-shell";
import { useEinvoiceFiles } from "@/features/einvoice/components/invoices-page";
import { VoidDialog } from "@/features/einvoice/components/void-dialog";
import {
  amount,
  canArchive,
  canRetryPdf,
  canVoid,
  hasFiles,
  statusTimeline,
  statusTone,
  validationTone,
} from "@/features/einvoice/lib/einvoice";
import {
  einvoiceKeys,
  einvoiceService,
  type Einvoice,
} from "@/features/einvoice/services/einvoice.service";
import { useStepUp } from "@/features/step-up-engine";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/**
 * Sandboxed iframe of the invoice HTML: the unnumbered preview of a draft
 * (PREVIEW watermark) or the archived XML through the stylesheet (VOID
 * watermark once voided). `sandbox=""` runs no script.
 */
function InvoicePreview({ invoice }: { invoice: Einvoice }) {
  const { t } = useLocale();
  const kind =
    invoice.status === "draft" || invoice.status === "failed"
      ? "preview"
      : "html";
  const html = useQuery({
    queryKey: [...einvoiceKeys.html(invoice.uuid, kind), invoice.updated_at],
    queryFn: () => einvoiceService.html(invoice),
  });
  if (html.isLoading) return <Loading />;
  if (html.isError || html.data === undefined) {
    return (
      <ErrorState
        title={t("einvoice.detail.preview_error")}
        description={
          isApiError(html.error) ? t(`errors.codes.${html.error.code}`) : ""
        }
        onRetry={() => void html.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  return (
    <iframe
      title={t("einvoice.detail.preview")}
      sandbox=""
      srcDoc={html.data}
      data-testid="einvoice-preview"
      className="h-[min(80vh,900px)] w-full rounded-md border bg-white"
    />
  );
}

function SourceLink({ slug, invoice }: { slug: string; invoice: Einvoice }) {
  const { t } = useLocale();
  const href =
    invoice.source_type === "order"
      ? routes.tenant.orders.detail(slug, invoice.source_uuid)
      : routes.tenant.serviceSubscriptions.list(slug);
  return (
    <Link
      href={href}
      className="text-primary hover:underline"
      data-testid="einvoice-source-link"
    >
      {t(`einvoice.source_types.${invoice.source_type}`)}
    </Link>
  );
}

function DetailBody({ slug, invoice }: { slug: string; invoice: Einvoice }) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const { ensure } = useStepUp();
  const { canManage } = useEinvoiceAccess(slug);
  const files = useEinvoiceFiles();
  const [confirmArchive, setConfirmArchive] = useState(false);
  const [voidOpen, setVoidOpen] = useState(false);

  const archive = useMutation({
    mutationFn: async () => {
      if (!(await ensure())) return null;
      return einvoiceService.archive(invoice.uuid);
    },
    onSuccess: (updated) => {
      setConfirmArchive(false);
      if (!updated) return;
      qc.setQueryData(einvoiceKeys.detail(invoice.uuid), updated);
      void qc.invalidateQueries({ queryKey: einvoiceKeys.all });
      if (updated.status === "archived") {
        appToast.success(
          t("einvoice.archive.success", { number: updated.number ?? "" }),
        );
      } else {
        appToast.warning(t("einvoice.archive.failed"));
      }
    },
    onError: () => setConfirmArchive(false),
  });

  const archivable = canArchive(invoice);
  const money = (v: string) => format.currency(amount(v), invoice.currency);
  const steps = statusTimeline(invoice);

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-2">
        <StatusChip
          label={t(`einvoice.statuses.${invoice.status}`)}
          tone={statusTone(invoice.status)}
        />
        <StatusChip
          label={t(`einvoice.validation.${invoice.validation_status}`)}
          tone={validationTone(invoice.validation_status)}
        />
        <div className="ms-auto flex flex-wrap gap-2">
          {canManage &&
          (invoice.status === "draft" || invoice.status === "failed") ? (
            <Button
              type="button"
              size="sm"
              data-testid="einvoice-archive"
              disabled={!archivable || archive.isPending}
              title={
                archivable ? undefined : t("einvoice.archive.invalid_hint")
              }
              onClick={() => setConfirmArchive(true)}
            >
              <Archive className="size-4" />
              {t("einvoice.actions.archive")}
            </Button>
          ) : null}
          {hasFiles(invoice) ? (
            <>
              <Button
                type="button"
                size="sm"
                variant="outline"
                data-testid="einvoice-download-xml"
                onClick={() => files.fetchFile(invoice, "xml")}
              >
                <FileCode className="size-4" />
                {t("einvoice.actions.download_xml")}
              </Button>
              <Button
                type="button"
                size="sm"
                variant="outline"
                data-testid="einvoice-download-pdf"
                disabled={!invoice.has_pdf}
                title={invoice.has_pdf ? undefined : t("einvoice.pdf.pending")}
                onClick={() => files.fetchFile(invoice, "pdf")}
              >
                <FileDown className="size-4" />
                {t("einvoice.actions.download_pdf")}
              </Button>
            </>
          ) : null}
          {canManage && canRetryPdf(invoice) ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              data-testid="einvoice-retry-pdf"
              disabled={files.retry.isPending}
              onClick={() => files.retry.mutate(invoice.uuid)}
            >
              <RefreshCw className="size-4" />
              {t("einvoice.actions.retry_pdf")}
            </Button>
          ) : null}
          {canManage && canVoid(invoice) ? (
            <Button
              type="button"
              size="sm"
              variant="destructive"
              data-testid="einvoice-void"
              onClick={() => setVoidOpen(true)}
            >
              <Ban className="size-4" />
              {t("einvoice.actions.void")}
            </Button>
          ) : null}
        </div>
      </div>

      {!archivable &&
      (invoice.status === "draft" || invoice.status === "failed") ? (
        <p
          className="text-destructive flex items-center gap-2 text-sm"
          data-testid="einvoice-invalid-hint"
        >
          <AlertTriangle className="size-4 shrink-0" />
          {t("einvoice.archive.invalid_hint")}
        </p>
      ) : null}
      {invoice.error ? (
        <p
          className="bg-destructive/10 text-destructive rounded-md p-3 text-sm"
          data-testid="einvoice-error"
          dir="auto"
        >
          {invoice.error}
        </p>
      ) : null}

      <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,1fr)]">
        <EntitySectionCard title={t("einvoice.detail.preview")}>
          <InvoicePreview invoice={invoice} />
        </EntitySectionCard>
        <div className="space-y-6">
          <EntitySectionCard title={t("einvoice.detail.summary")}>
            <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-1">
              <Field label={t("einvoice.columns.number")}>
                <span className="font-mono" dir="ltr">
                  {invoice.number ?? t("einvoice.draft_number")}
                </span>
              </Field>
              <Field label={t("einvoice.columns.ettn")}>
                <span className="font-mono text-xs break-all" dir="ltr">
                  {invoice.uuid}
                </span>
              </Field>
              <Field label={t("einvoice.columns.buyer")}>
                {invoice.buyer_organization?.name ?? invoice.buyer.name}
                <span className="text-muted-foreground block text-xs" dir="ltr">
                  {invoice.buyer.vkn
                    ? `VKN ${invoice.buyer.vkn}`
                    : invoice.buyer.tckn
                      ? `TCKN ${invoice.buyer.tckn}`
                      : ""}
                </span>
              </Field>
              <Field label={t("einvoice.columns.profile")}>
                {t(`einvoice.profiles.${invoice.profile}`)}
              </Field>
              <Field label={t("einvoice.columns.issue_date")}>
                {format.date(invoice.issue_date)}
              </Field>
              <Field label={t("einvoice.detail.source")}>
                <SourceLink slug={slug} invoice={invoice} />
              </Field>
              <Field label={t("einvoice.columns.tax_exclusive")}>
                <span dir="ltr">{money(invoice.tax_exclusive)}</span>
              </Field>
              <Field label={t("einvoice.columns.tax_total")}>
                <span dir="ltr">{money(invoice.tax_total)}</span>
              </Field>
              <Field label={t("einvoice.columns.payable")}>
                <span className="font-semibold" dir="ltr">
                  {money(invoice.payable)}
                </span>
              </Field>
              {invoice.finance_entry ? (
                <Field label={t("einvoice.detail.finance_entry")}>
                  <Link
                    href={routes.tenant.accounting.entries(slug)}
                    className="text-primary hover:underline"
                  >
                    <span dir="ltr">
                      {format.currency(
                        amount(invoice.finance_entry.amount),
                        invoice.finance_entry.currency,
                      )}
                    </span>
                  </Link>
                </Field>
              ) : null}
            </dl>
          </EntitySectionCard>

          <EntitySectionCard
            title={t("einvoice.detail.validation_messages")}
            badge={invoice.validation_messages.length || undefined}
          >
            {invoice.validation_messages.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("einvoice.detail.no_messages")}
              </p>
            ) : (
              <ul
                className="list-disc space-y-1 ps-5 text-sm"
                data-testid="einvoice-validation-messages"
              >
                {invoice.validation_messages.map((m, i) => (
                  <li key={i} dir="auto">
                    {m}
                  </li>
                ))}
              </ul>
            )}
          </EntitySectionCard>

          <EntitySectionCard title={t("einvoice.detail.timeline")}>
            <Timeline
              items={steps.map((s) => ({
                id: s.id,
                title: t(`einvoice.timeline.${s.id}`),
                description: s.note ?? undefined,
                meta:
                  s.id === "archived"
                    ? format.date(s.at)
                    : format.dateTime(s.at),
              }))}
            />
          </EntitySectionCard>
        </div>
      </div>

      <ConfirmDialog
        open={confirmArchive}
        title={t("einvoice.archive.confirm_title")}
        description={t("einvoice.archive.confirm_description")}
        confirmLabel={t("einvoice.actions.archive")}
        isPending={archive.isPending}
        onConfirm={() => archive.mutate()}
        onCancel={() => setConfirmArchive(false)}
      />
      {voidOpen ? (
        <VoidDialog
          invoice={invoice}
          open
          onOpenChange={(open) => setVoidOpen(open)}
        />
      ) : null}
    </div>
  );
}

function DetailLoader({ slug, uuid }: { slug: string; uuid: string }) {
  const { t } = useLocale();
  const query = useQuery({
    queryKey: einvoiceKeys.detail(uuid),
    queryFn: () => einvoiceService.get(uuid),
  });
  if (query.isLoading) return <Loading />;
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  return <DetailBody slug={slug} invoice={query.data} />;
}

/**
 * Muhasebe > e-Fatura > detail (TEC-504): HTML preview (sandboxed), the
 * validation messages (Schematron warnings), the source record, the
 * status history; archive (step-up, "nothing is sent, only archived"),
 * XML / PDF, PDF re-render and void.
 */
export function InvoiceDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t } = useLocale();
  return (
    <EinvoiceShell
      slug={slug}
      title={t("einvoice.detail.title")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("einvoice.title"),
          href: routes.tenant.einvoices.list(slug),
        },
        { label: t("einvoice.detail.title") },
      ]}
    >
      <DetailLoader slug={slug} uuid={uuid} />
    </EinvoiceShell>
  );
}
