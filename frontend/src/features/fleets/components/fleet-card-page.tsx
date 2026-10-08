"use client";

import { useQuery } from "@tanstack/react-query";
import { Truck } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import type { ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { FleetPlansTab } from "@/features/fleets/components/fleet-plans-tab";
import { FleetReportsTab } from "@/features/fleets/components/fleet-reports-tab";
import { FleetServicesTab } from "@/features/fleets/components/fleet-services-tab";
import { FleetStatementTab } from "@/features/fleets/components/fleet-statement-tab";
import { FleetUsersTab } from "@/features/fleets/components/fleet-users-tab";
import { FleetVehiclesTab } from "@/features/fleets/components/fleet-vehicles-tab";
import { linkTone } from "@/features/fleets/components/fleets-list-page";
import {
  fleetKeys,
  fleetsService,
  type FleetCard,
} from "@/features/fleets/services/fleets.service";
import { useFeature } from "@/features/modules/hooks/use-features";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const FLEET_TABS = [
  "summary",
  "vehicles",
  "users",
  "services",
  "statement",
  "reports",
  "plans",
] as const;
export type FleetTab = (typeof FLEET_TABS)[number];

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children || "—"}</dd>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: ReactNode }) {
  return (
    <Card>
      <CardContent className="space-y-1 pt-6">
        <div className="text-muted-foreground text-xs">{label}</div>
        <div className="text-2xl font-semibold tabular-nums">{value}</div>
      </CardContent>
    </Card>
  );
}

function FleetSummary({ fleet }: { fleet: FleetCard }) {
  const { t, format } = useLocale();
  const p = fleet.profile;
  return (
    <div className="space-y-4" data-testid="fleet-summary">
      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Stat
          label={t("fleets.summary.vehicles")}
          value={format.number(fleet.vehicle_count)}
        />
        <Stat
          label={t("fleets.summary.active_warranties")}
          value={format.number(fleet.active_warranty_count)}
        />
        <Stat
          label={t("fleets.summary.services")}
          value={format.number(fleet.service_count)}
        />
        <Stat
          label={t("fleets.summary.balance")}
          value={
            fleet.cari ? (
              <span dir="ltr">
                {format.currency(
                  Number(fleet.cari.balance),
                  fleet.cari.currency,
                )}
              </span>
            ) : (
              "—"
            )
          }
        />
      </div>
      <div className="grid gap-4 lg:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle>{t("fleets.summary.profile")}</CardTitle>
          </CardHeader>
          <CardContent>
            <dl className="grid gap-3 sm:grid-cols-2">
              <Field label={t("fleets.fields.legal_name")}>
                {p.legal_name}
              </Field>
              <Field label={t("fleets.fields.tax_number")}>
                <span className="font-mono" dir="ltr">
                  {p.tax_number}
                </span>
              </Field>
              <Field label={t("fleets.fields.tax_office")}>
                {p.tax_office}
              </Field>
              <Field label={t("fleets.fields.contact_name")}>
                {p.contact_name}
              </Field>
              <Field label={t("fleets.fields.contact_phone")}>
                {p.contact_phone ? (
                  <span dir="ltr">{p.contact_phone}</span>
                ) : null}
              </Field>
              <Field label={t("fleets.fields.billing_email")}>
                {p.billing_email}
              </Field>
              <Field label={t("fleets.fields.report_frequency")}>
                {t(`fleets.report_frequency.${p.report_frequency}`)}
              </Field>
              <Field label={t("fleets.fields.report_locale")}>
                {p.report_locale}
              </Field>
            </dl>
            {!fleet.has_primary_user ? (
              <p
                className="mt-4 text-sm text-amber-700 dark:text-amber-400"
                data-testid="fleet-no-primary-user"
              >
                {t("fleets.summary.no_primary_user")}
              </p>
            ) : null}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t("fleets.summary.links")}</CardTitle>
          </CardHeader>
          <CardContent>
            <ul className="space-y-2">
              {fleet.links.map((link) => (
                <li
                  key={link.uuid}
                  className="flex items-center justify-between gap-2 text-sm"
                >
                  <span>{link.dealer_name}</span>
                  <span className="flex items-center gap-2">
                    {link.started_at ? (
                      <span className="text-muted-foreground text-xs">
                        {format.date(link.started_at)}
                      </span>
                    ) : null}
                    <StatusChip
                      label={t(`fleets.link_status.${link.status}`)}
                      tone={linkTone(link.status)}
                    />
                  </span>
                </li>
              ))}
            </ul>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}

/**
 * Fleet card (TEC-477): summary, vehicles (selection → bulk service plan,
 * import with dry-run preview), users (invite, disable), the caller's
 * services, the consolidated statement (bulk invoice view, PDF / XLSX),
 * periodic reports and service plans. The tab is kept in `?tab=`.
 */
export function FleetCardPage({ slug, uuid }: { slug: string; uuid: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const pathname = usePathname();
  const search = useSearchParams();
  const appointments = useFeature(slug, "appointments");
  const canRead = can(permissions.fleets.read);
  const canPlan = can(permissions.fleets.plan) && appointments.enabled;
  const canServices = can(permissions.services.read);

  const card = useQuery({
    queryKey: fleetKeys.card(uuid),
    queryFn: () => fleetsService.card(uuid),
    enabled: canRead,
  });

  const visible = FLEET_TABS.filter(
    (tab) =>
      (tab !== "plans" || canPlan) && (tab !== "services" || canServices),
  );
  const requested = search.get("tab") as FleetTab | null;
  const tab: FleetTab =
    requested && visible.includes(requested) ? requested : "summary";
  const setTab = (next: string) => {
    const params = new URLSearchParams(search.toString());
    params.set("tab", next);
    router.replace(`${pathname}?${params.toString()}`, { scroll: false });
  };

  const fleet = card.data;
  const title = fleet?.name ?? t("fleets.card.title");
  const header = (
    <PageHeader
      title={title}
      description={fleet?.profile.legal_name}
      icon={<Truck className="size-6" />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("fleets.title"), href: routes.tenant.fleets.list(slug) },
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
          description={t("fleets.forbidden")}
        />
      </div>
    );
  }
  if (card.isError) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_generic")}
          retryLabel={t("common.retry")}
          onRetry={() => void card.refetch()}
        />
      </div>
    );
  }
  if (!fleet) {
    return (
      <div className="space-y-6">
        {header}
        <p className="text-muted-foreground text-sm">{t("fleets.loading")}</p>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <Tabs value={tab} onValueChange={setTab}>
        <TabsList className="flex-wrap">
          {visible.map((value) => (
            <TabsTrigger
              key={value}
              value={value}
              data-testid={`fleet-tab-${value}`}
            >
              {t(`fleets.tabs.${value}`)}
            </TabsTrigger>
          ))}
        </TabsList>
        <TabsContent value="summary" className="pt-4">
          <FleetSummary fleet={fleet} />
        </TabsContent>
        <TabsContent value="vehicles" className="pt-4">
          {tab === "vehicles" ? (
            <FleetVehiclesTab slug={slug} fleet={fleet} canPlan={canPlan} />
          ) : null}
        </TabsContent>
        <TabsContent value="users" className="pt-4">
          {tab === "users" ? <FleetUsersTab fleet={fleet} /> : null}
        </TabsContent>
        {canServices ? (
          <TabsContent value="services" className="pt-4">
            {tab === "services" ? (
              <FleetServicesTab slug={slug} fleet={fleet} />
            ) : null}
          </TabsContent>
        ) : null}
        <TabsContent value="statement" className="pt-4">
          {tab === "statement" ? <FleetStatementTab fleet={fleet} /> : null}
        </TabsContent>
        <TabsContent value="reports" className="pt-4">
          {tab === "reports" ? <FleetReportsTab fleet={fleet} /> : null}
        </TabsContent>
        {canPlan ? (
          <TabsContent value="plans" className="pt-4">
            {tab === "plans" ? (
              <FleetPlansTab slug={slug} fleet={fleet} />
            ) : null}
          </TabsContent>
        ) : null}
      </Tabs>
    </div>
  );
}
