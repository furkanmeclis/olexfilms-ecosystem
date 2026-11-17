"use client";

import { useQuery } from "@tanstack/react-query";
import { Undo2 } from "lucide-react";
import Link from "next/link";
import { useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityLink, EntityPage } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  EntryAmount,
  EntryStatus,
  useCategoryLabels,
} from "@/features/accounting/components/shared";
import { VoidEntryDialog } from "@/features/accounting/components/void-entry-dialog";
import {
  accountingKeys,
  useAccountingAccess,
} from "@/features/accounting/hooks/use-accounting-access";
import { isEntryVoidable } from "@/features/accounting/lib/access";
import {
  entrySourceKind,
  entrySourceLabelKey,
  shortId,
} from "@/features/accounting/lib/entry-source";
import {
  accountingService,
  type FinanceEntry,
} from "@/features/accounting/services/accounting.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="grid gap-1 sm:grid-cols-[12rem_1fr]">
      <dt className="text-muted-foreground text-sm">{label}</dt>
      <dd className="text-sm break-words">{children}</dd>
    </div>
  );
}

/**
 * The record the row was posted from (TEC-1218): linked when the source type
 * maps to a kind with a page and the user may read it, plain text otherwise.
 * The API returns only type + uuid, so the label is "type #short-id".
 */
export function EntrySourceRecord({
  entry,
  slug,
}: {
  entry: Pick<FinanceEntry, "source_type" | "source_uuid">;
  slug: string;
}) {
  const { t } = useLocale();
  const { source_type: type, source_uuid: uuid } = entry;
  if (!type || !uuid) return <>—</>;
  const labelKey = entrySourceLabelKey(type);
  const label = t("accounting.entry_detail.source_ref", {
    source: labelKey ? t(labelKey) : type,
    id: shortId(uuid),
  });
  const kind = entrySourceKind(type);
  if (!kind) return <span>{label}</span>;
  return <EntityLink kind={kind} slug={slug} entity={{ uuid, name: label }} />;
}

function EntryRef({ slug, uuid }: { slug: string; uuid: string | null }) {
  if (!uuid) return <>—</>;
  return (
    <Link
      href={routes.tenant.accounting.entryDetail(slug, uuid)}
      className="text-primary font-mono underline-offset-4 hover:underline"
    >
      #{shortId(uuid)}
    </Link>
  );
}

/**
 * Tenant > Accounting > Entries > detail (TEC-1218): one ledger row
 * (GET /v1/accounting/entries/{uuid}) with its cari, counterparty and the
 * source record it was posted from. Open manual rows can be voided here.
 */
export function EntryDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const access = useAccountingAccess(slug);
  const enabled = access.canRead && Boolean(access.orgUuid);
  const categories = useCategoryLabels(access.orgUuid, enabled);
  const [voiding, setVoiding] = useState<FinanceEntry | null>(null);

  const query = useQuery({
    queryKey: accountingKeys.entry(access.orgUuid, uuid),
    queryFn: () => accountingService.getEntry(uuid),
    enabled: enabled && Boolean(uuid),
  });
  const e = query.data;
  const sourceKey = entrySourceLabelKey(e?.source_type);

  return (
    <EntityPage
      title={t("accounting.entry_detail.title")}
      description={e ? format.dateTime(e.created_at) : undefined}
      permission={permissions.accounting.read}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("accounting.forbidden")}
        />
      }
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("accounting.nav") },
        {
          label: t("accounting.entries.title"),
          href: routes.tenant.accounting.entries(slug),
        },
        { label: t("accounting.entry_detail.title") },
      ]}
      actions={
        e && access.canWrite && isEntryVoidable(e) ? (
          <Button
            variant="destructive"
            onClick={() => setVoiding(e)}
            data-testid="entry-void"
          >
            <Undo2 className="size-4" />
            {t("accounting.void.action")}
          </Button>
        ) : null
      }
    >
      {query.isLoading ? (
        <Loading />
      ) : query.isError || !e ? (
        <ErrorState
          title={
            isApiError(query.error) && query.error.status === 404
              ? t("accounting.entry_detail.not_found")
              : t("accounting.load_failed")
          }
          onRetry={() => void query.refetch()}
        />
      ) : (
        <div className="grid gap-6 lg:grid-cols-2">
          <Card>
            <CardHeader>
              <CardTitle>{t("accounting.entry_detail.summary")}</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="space-y-3" data-testid="entry-detail">
                <Row label={t("accounting.fields.date")}>
                  {format.dateTime(e.created_at)}
                </Row>
                <Row label={t("accounting.fields.direction")}>
                  {t(`accounting.directions.${e.direction}`)}
                </Row>
                <Row label={t("accounting.fields.category")}>
                  {categories.get(e.category) ?? e.category}
                </Row>
                <Row label={t("accounting.fields.amount")}>
                  <div className="inline-block">
                    <EntryAmount entry={e} />
                  </div>
                </Row>
                <Row label={t("accounting.fields.status")}>
                  {e.voided || e.reversal_of_uuid ? (
                    <EntryStatus entry={e} />
                  ) : (
                    "—"
                  )}
                </Row>
                <Row label={t("accounting.fields.description")}>
                  <span className="whitespace-pre-wrap">
                    {e.description || "—"}
                  </span>
                </Row>
                <Row label={t("accounting.entry_detail.revision")}>
                  {e.revision}
                </Row>
              </dl>
            </CardContent>
          </Card>
          <Card>
            <CardHeader>
              <CardTitle>{t("accounting.entry_detail.related")}</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="space-y-3" data-testid="entry-related">
                <Row label={t("accounting.fields.source")}>
                  {e.source_type
                    ? sourceKey
                      ? t(sourceKey)
                      : e.source_type
                    : "—"}
                </Row>
                <Row label={t("accounting.entry_detail.source_record")}>
                  <EntrySourceRecord entry={e} slug={slug} />
                </Row>
                <Row label={t("accounting.fields.cari")}>
                  <EntityLink
                    kind="cari"
                    slug={slug}
                    entity={
                      e.cari_uuid
                        ? {
                            uuid: e.cari_uuid,
                            name:
                              e.counterparty_organization?.name ||
                              `#${shortId(e.cari_uuid)}`,
                          }
                        : null
                    }
                  />
                </Row>
                <Row label={t("accounting.entry_detail.counterparty")}>
                  <EntityLink
                    kind="organization"
                    slug={slug}
                    entity={e.counterparty_organization}
                  />
                </Row>
                <Row label={t("accounting.fields.account")}>
                  {e.account?.name ?? "—"}
                </Row>
                {e.reversal_of_uuid ? (
                  <Row label={t("accounting.entry_detail.reversal_of")}>
                    <EntryRef slug={slug} uuid={e.reversal_of_uuid} />
                  </Row>
                ) : null}
                {e.reversed_by_uuid ? (
                  <Row label={t("accounting.entry_detail.reversed_by")}>
                    <EntryRef slug={slug} uuid={e.reversed_by_uuid} />
                  </Row>
                ) : null}
              </dl>
            </CardContent>
          </Card>
        </div>
      )}
      {access.canWrite ? (
        <VoidEntryDialog
          orgUuid={access.orgUuid}
          entry={voiding}
          onOpenChange={(open) => {
            if (!open) setVoiding(null);
          }}
        />
      ) : null}
    </EntityPage>
  );
}
