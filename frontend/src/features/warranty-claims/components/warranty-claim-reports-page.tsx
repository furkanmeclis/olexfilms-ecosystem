"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChartColumn } from "lucide-react";
import { useMemo, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { ClaimReportExportButton } from "@/features/warranty-claims/components/claim-report-export-button";
import { FailureRateChart } from "@/features/warranty-claims/components/failure-rate-chart";
import {
  buildPeriodQuery,
  CLAIM_REPORT_TABS,
  dealerFilterOptions,
  failureRowKey,
  filterDealerRows,
  formatClaimRate,
  isPeriodValid,
  tabGroup,
  type ClaimReportTab,
} from "@/features/warranty-claims/lib/claim-reports";
import {
  claimReportKeys,
  claimReportsService,
  type ClaimReportPeriod,
} from "@/features/warranty-claims/services/claim-reports.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const selectClass =
  "border-input bg-background h-9 w-full rounded-md border px-2 text-sm";

/**
 * Tenant > Warranty claim report (TEC-340 on the TEC-338 endpoints): period
 * filter, product / lot failure rates (table + bar chart), claims by dealer
 * (with a dealer filter limited to the caller's scope) and part
 * distribution; each tab exports to CSV / XLSX through the export center.
 */
export function WarrantyClaimReportsPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.warrantyClaims.read);
  const [tab, setTab] = useState<ClaimReportTab>("product");
  const [period, setPeriod] = useState<ClaimReportPeriod>({});
  const periodValid = isPeriodValid(period);
  const query = useMemo(() => buildPeriodQuery(period), [period]);

  const title = t("warranty.claim_reports.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ChartColumn className="size-6" />}
      description={t("warranty.claim_reports.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("warranty.claim_reports.forbidden")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardContent className="flex flex-wrap items-end gap-4 pt-6">
          <div className="space-y-1.5">
            <Label htmlFor="claim-report-from">
              {t("warranty.claim_reports.from")}
            </Label>
            <DatePicker
              id="claim-report-from"
              className="w-44"
              value={period.from ?? ""}
              onChange={(from) => setPeriod((p) => ({ ...p, from }))}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="claim-report-to">
              {t("warranty.claim_reports.to")}
            </Label>
            <DatePicker
              id="claim-report-to"
              className="w-44"
              value={period.to ?? ""}
              aria-invalid={periodValid ? undefined : true}
              onChange={(to) => setPeriod((p) => ({ ...p, to }))}
            />
          </div>
          <ToggleGroup
            type="single"
            variant="outline"
            value={tab}
            aria-label={t("warranty.claim_reports.tabs_label")}
            onValueChange={(v) => v && setTab(v as ClaimReportTab)}
          >
            {CLAIM_REPORT_TABS.map((value) => (
              <ToggleGroupItem
                key={value}
                value={value}
                data-testid={`claim-report-tab-${value}`}
              >
                {t(`warranty.claim_reports.tabs.${value}`)}
              </ToggleGroupItem>
            ))}
          </ToggleGroup>
          <div className="ms-auto">
            <ClaimReportExportButton
              slug={slug}
              tab={tab}
              period={query}
              disabled={!periodValid}
            />
          </div>
          {periodValid ? null : (
            <p
              role="alert"
              className="text-destructive w-full text-sm"
              data-testid="claim-report-period-error"
            >
              {t("warranty.claim_reports.period_invalid")}
            </p>
          )}
        </CardContent>
      </Card>

      {tab === "product" || tab === "lot" ? (
        <FailureRateSection tab={tab} query={query} enabled={periodValid} />
      ) : tab === "dealer" ? (
        <ByDealerSection query={query} enabled={periodValid} />
      ) : (
        <PartsSection query={query} enabled={periodValid} />
      )}
    </div>
  );
}

function Th({ children, end }: { children: ReactNode; end?: boolean }) {
  return (
    <th className={`p-2 font-medium ${end ? "text-end" : "text-start"}`}>
      {children}
    </th>
  );
}

function Td({ children, end }: { children: ReactNode; end?: boolean }) {
  return (
    <td className={`p-2 ${end ? "text-end tabular-nums" : ""}`}>{children}</td>
  );
}

function ReportState({
  isLoading,
  isError,
  empty,
  onRetry,
  children,
}: {
  isLoading: boolean;
  isError: boolean;
  empty: boolean;
  onRetry: () => void;
  children: ReactNode;
}) {
  const { t } = useLocale();
  if (isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={onRetry}
        retryLabel={t("common.retry")}
      />
    );
  }
  if (isLoading) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("warranty.claim_reports.loading")}
      </p>
    );
  }
  if (empty) {
    return (
      <p
        className="text-muted-foreground py-8 text-center text-sm"
        data-testid="claim-report-empty"
      >
        {t("warranty.claim_reports.empty")}
      </p>
    );
  }
  return <>{children}</>;
}

function FailureRateSection({
  tab,
  query,
  enabled,
}: {
  tab: "product" | "lot";
  query: ClaimReportPeriod;
  enabled: boolean;
}) {
  const { t, format, locale } = useLocale();
  const group = tabGroup(tab) ?? "product";
  const params = useMemo(() => ({ ...query, group }), [query, group]);
  const report = useQuery({
    queryKey: claimReportKeys.failureRate(params),
    queryFn: () => claimReportsService.failureRate(params),
    enabled,
    placeholderData: keepPreviousData,
  });
  const rows = report.data?.group === group ? report.data.items : [];
  const rate = (v: number) => formatClaimRate(v, { locale });

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t(`warranty.claim_reports.sections.${tab}`)}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-6">
        <ReportState
          isLoading={report.isLoading}
          isError={report.isError}
          empty={rows.length === 0}
          onRetry={() => void report.refetch()}
        >
          <FailureRateChart rows={rows} />
          <div className="overflow-x-auto">
            <table
              className="w-full text-sm"
              data-testid="claim-report-failure-table"
            >
              <thead>
                <tr className="text-muted-foreground border-b text-xs">
                  <Th>{t("warranty.claim_reports.columns.product")}</Th>
                  {tab === "lot" ? (
                    <Th>{t("warranty.claim_reports.columns.lot")}</Th>
                  ) : null}
                  <Th end>{t("warranty.claim_reports.columns.warranties")}</Th>
                  <Th end>{t("warranty.claim_reports.columns.claims")}</Th>
                  <Th end>{t("warranty.claim_reports.columns.approved")}</Th>
                  <Th end>{t("warranty.claim_reports.columns.claim_rate")}</Th>
                  <Th end>
                    {t("warranty.claim_reports.columns.approved_rate")}
                  </Th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr
                    key={failureRowKey(r)}
                    className="border-b last:border-0"
                    data-testid="claim-report-failure-row"
                  >
                    <Td>
                      <div>{r.product_name}</div>
                      <div
                        className="text-muted-foreground font-mono text-xs"
                        dir="ltr"
                      >
                        {r.product_sku}
                      </div>
                    </Td>
                    {tab === "lot" ? (
                      <Td>
                        <span className="font-mono text-xs" dir="ltr">
                          {r.lot_code ?? "—"}
                        </span>
                      </Td>
                    ) : null}
                    <Td end>{format.number(r.warranty_count)}</Td>
                    <Td end>{format.number(r.claim_count)}</Td>
                    <Td end>{format.number(r.approved_claim_count)}</Td>
                    <Td end>
                      <span data-testid="claim-rate">{rate(r.claim_rate)}</span>
                    </Td>
                    <Td end>
                      <span data-testid="approved-rate">
                        {rate(r.approved_rate)}
                      </span>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </ReportState>
      </CardContent>
    </Card>
  );
}

function ByDealerSection({
  query,
  enabled,
}: {
  query: ClaimReportPeriod;
  enabled: boolean;
}) {
  const { t, format, locale } = useLocale();
  const [dealer, setDealer] = useState("");
  const report = useQuery({
    queryKey: claimReportKeys.byDealer(query),
    queryFn: () => claimReportsService.byDealer(query),
    enabled,
    placeholderData: keepPreviousData,
  });
  const all = useMemo(() => report.data?.items ?? [], [report.data]);
  const options = useMemo(() => dealerFilterOptions(all), [all]);
  // A dealer that left the scope (new period) falls back to "all".
  const selected = options.some((o) => o.value === dealer) ? dealer : "";
  const rows = filterDealerRows(all, selected);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("warranty.claim_reports.sections.dealer")}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="max-w-xs space-y-1.5">
          <Label htmlFor="claim-report-dealer">
            {t("warranty.claim_reports.dealer_filter")}
          </Label>
          <select
            id="claim-report-dealer"
            data-testid="claim-report-dealer-filter"
            className={selectClass}
            value={selected}
            onChange={(e) => setDealer(e.target.value)}
          >
            <option value="">{t("warranty.claim_reports.dealer_all")}</option>
            {options.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </div>
        <ReportState
          isLoading={report.isLoading}
          isError={report.isError}
          empty={rows.length === 0}
          onRetry={() => void report.refetch()}
        >
          <div className="overflow-x-auto">
            <table
              className="w-full text-sm"
              data-testid="claim-report-dealer-table"
            >
              <thead>
                <tr className="text-muted-foreground border-b text-xs">
                  <Th>{t("warranty.claim_reports.columns.organization")}</Th>
                  <Th>{t("warranty.claim_reports.columns.org_type")}</Th>
                  <Th end>{t("warranty.claim_reports.columns.claims")}</Th>
                  <Th end>{t("warranty.claim_reports.columns.approved")}</Th>
                  <Th end>{t("warranty.claim_reports.columns.rejected")}</Th>
                  <Th end>
                    {t("warranty.claim_reports.columns.approval_rate")}
                  </Th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr
                    key={r.organization_uuid}
                    className="border-b last:border-0"
                    data-testid="claim-report-dealer-row"
                    data-uuid={r.organization_uuid}
                  >
                    <Td>{r.organization_name}</Td>
                    <Td>
                      {t(
                        `warranty.claim_reports.org_type.${r.organization_type}`,
                      )}
                    </Td>
                    <Td end>{format.number(r.claim_count)}</Td>
                    <Td end>{format.number(r.approved_claim_count)}</Td>
                    <Td end>{format.number(r.rejected_claim_count)}</Td>
                    <Td end>{formatClaimRate(r.approval_rate, { locale })}</Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </ReportState>
      </CardContent>
    </Card>
  );
}

function PartsSection({
  query,
  enabled,
}: {
  query: ClaimReportPeriod;
  enabled: boolean;
}) {
  const { t, format } = useLocale();
  const report = useQuery({
    queryKey: claimReportKeys.parts(query),
    queryFn: () => claimReportsService.parts(query),
    enabled,
    placeholderData: keepPreviousData,
  });
  const rows = report.data?.items ?? [];
  // Car part names of the service wizard; unknown keys stay as they are.
  const partLabel = (key: string) => {
    const msg = `services.parts.names.${key}`;
    const text = t(msg);
    return text === msg ? key : text;
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("warranty.claim_reports.sections.parts")}</CardTitle>
      </CardHeader>
      <CardContent>
        <ReportState
          isLoading={report.isLoading}
          isError={report.isError}
          empty={rows.length === 0}
          onRetry={() => void report.refetch()}
        >
          <div className="overflow-x-auto">
            <table
              className="w-full text-sm"
              data-testid="claim-report-parts-table"
            >
              <thead>
                <tr className="text-muted-foreground border-b text-xs">
                  <Th>{t("warranty.claim_reports.columns.part")}</Th>
                  <Th>{t("warranty.claim_reports.columns.product")}</Th>
                  <Th end>{t("warranty.claim_reports.columns.part_count")}</Th>
                  <Th end>{t("warranty.claim_reports.columns.claims")}</Th>
                  <Th end>{t("warranty.claim_reports.columns.approved")}</Th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr
                    key={`${r.part_key}:${r.product_uuid ?? ""}`}
                    className="border-b last:border-0"
                    data-testid="claim-report-parts-row"
                  >
                    <Td>
                      <div>{partLabel(r.part_key)}</div>
                      <div
                        className="text-muted-foreground font-mono text-xs"
                        dir="ltr"
                      >
                        {r.part_key}
                      </div>
                    </Td>
                    <Td>
                      <div>{r.product_name || "—"}</div>
                      {r.product_sku ? (
                        <div
                          className="text-muted-foreground font-mono text-xs"
                          dir="ltr"
                        >
                          {r.product_sku}
                        </div>
                      ) : null}
                    </Td>
                    <Td end>{format.number(r.part_count)}</Td>
                    <Td end>{format.number(r.claim_count)}</Td>
                    <Td end>{format.number(r.approved_claim_count)}</Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </ReportState>
      </CardContent>
    </Card>
  );
}
