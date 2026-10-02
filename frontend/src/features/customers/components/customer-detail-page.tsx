"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUpCircle,
  Car,
  Download,
  Pencil,
  Plus,
  ShieldOff,
  UserRound,
} from "lucide-react";
import Link from "next/link";
import { useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  AnonymizeDialog,
  DataExportDialog,
  UpgradeDialog,
  type CustomerActionKind,
} from "@/features/customers/components/customer-actions";
import { customerStatusTone } from "@/features/customers/components/customers-list-page";
import { VehicleFormDialog } from "@/features/customers/components/vehicle-form-dialog";
import { resolveCustomerDetailAccess } from "@/features/customers/lib/access";
import { customerDisplayName } from "@/features/customers/lib/form";
import {
  customerKeys,
  customersService,
  type Vehicle,
} from "@/features/customers/services/customers.service";
import { VehicleBrandLogo } from "@/features/vehicle-catalog/components/vehicle-brand-logo";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm break-words">{children}</dd>
    </div>
  );
}

const dash = (v: string | number | null | undefined) =>
  v === null || v === undefined || v === "" ? "—" : v;

/**
 * Tenant > Customers > detail (TEC-163): profile, serving organizations,
 * vehicles (add / edit) and the actions the user may run: edit,
 * anonymize, personal data export (center, customers.anonymize, step-up)
 * and upgrade to dealer (center / distributor).
 */
export function CustomerDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const qc = useQueryClient();
  const canRead = can(permissions.customers.read);
  const [action, setAction] = useState<CustomerActionKind | null>(null);
  const [vehicleDialog, setVehicleDialog] = useState<{
    vehicle: Vehicle | null;
  } | null>(null);

  const detail = useQuery({
    queryKey: customerKeys.detail(uuid),
    queryFn: () => customersService.get(uuid),
    enabled: canRead && uuid !== "",
  });
  const c = detail.data;
  const access = c ? resolveCustomerDetailAccess(can, org?.type, c) : null;
  const vehicles = useQuery({
    queryKey: customerKeys.vehicles(uuid),
    queryFn: () => customersService.listVehicles(uuid),
    enabled: Boolean(access?.canReadVehicles),
  });

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: customerKeys.detail(uuid) });
    void qc.invalidateQueries({ queryKey: customerKeys.vehicles(uuid) });
    void qc.invalidateQueries({ queryKey: ["customers", "list"] });
  };

  const title = c ? customerDisplayName(c) : t("customers.detail.title");
  const header = (
    <PageHeader
      title={title}
      icon={<UserRound className="size-6" />}
      description={t("customers.detail.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("customers.list.title"),
          href: routes.tenant.customers.list(slug),
        },
        { label: title },
      ]}
      actions={
        c && access ? (
          <div className="flex flex-wrap gap-2" data-testid="customer-actions">
            {access.canEdit ? (
              <Button asChild variant="outline" data-action="edit">
                <Link href={routes.tenant.customers.edit(slug, c.uuid)}>
                  <Pencil className="size-4" />
                  {t("customers.actions.edit")}
                </Link>
              </Button>
            ) : null}
            {access.canUpgrade ? (
              <Button
                type="button"
                variant="outline"
                data-action="upgrade"
                onClick={() => setAction("upgrade")}
              >
                <ArrowUpCircle className="size-4" />
                {t("customers.actions.upgrade.button")}
              </Button>
            ) : null}
            {access.canExport ? (
              <Button
                type="button"
                variant="outline"
                data-action="export"
                onClick={() => setAction("export")}
              >
                <Download className="size-4" />
                {t("customers.actions.export.button")}
              </Button>
            ) : null}
            {access.canAnonymize ? (
              <Button
                type="button"
                variant="destructive"
                data-action="anonymize"
                onClick={() => setAction("anonymize")}
              >
                <ShieldOff className="size-4" />
                {t("customers.actions.anonymize.button")}
              </Button>
            ) : null}
          </div>
        ) : null
      }
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("customers.list.forbidden")}
        />
      </div>
    );
  }
  if (detail.isLoading) return <Loading />;
  if (detail.isError || !c || !access) {
    const notFound = isApiError(detail.error) && detail.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound
              ? t("customers.detail.not_found")
              : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }

  const rows = vehicles.data?.items ?? [];
  return (
    <div className="space-y-6" data-testid="customer-detail">
      {header}
      {c.anonymized ? (
        <p
          className="bg-muted text-muted-foreground rounded-md p-3 text-sm"
          data-testid="anonymized-notice"
        >
          {t("customers.detail.anonymized")}
        </p>
      ) : null}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-base">
            {t("customers.detail.profile")}
            <StatusChip
              label={t(`customers.status.${c.status}`)}
              tone={customerStatusTone(c.status)}
            />
          </CardTitle>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-3 sm:grid-cols-3">
            <Field label={t("customers.fields.phone")}>
              <span dir="ltr">{dash(c.phone)}</span>
            </Field>
            <Field label={t("customers.fields.email")}>{dash(c.email)}</Field>
            <Field label={t("customers.fields.type")}>
              {t(`customers.type.${c.type}`)}
            </Field>
            {c.type === "corporate" ? (
              <>
                <Field label={t("customers.fields.company_name")}>
                  {dash(c.company_name)}
                </Field>
                <Field label={t("customers.fields.tax_office")}>
                  {dash(c.tax_office)}
                </Field>
                <Field label={t("customers.fields.tax_no")}>
                  <span dir="ltr">
                    {c.tax_no_last4 ? `•••• ${c.tax_no_last4}` : "—"}
                  </span>
                </Field>
              </>
            ) : null}
            <Field label={t("customers.fields.national_id")}>
              <span dir="ltr">
                {c.national_id_last4 ? `•••• ${c.national_id_last4}` : "—"}
              </span>
            </Field>
            <Field label={t("customers.fields.linked_at")}>
              {c.linked_at ? format.date(c.linked_at) : "—"}
            </Field>
            <Field label={t("customers.fields.first_service_at")}>
              {c.first_service_at ? format.date(c.first_service_at) : "—"}
            </Field>
          </dl>
          {c.organizations.length > 0 ? (
            <div className="mt-4 space-y-1">
              <p className="text-muted-foreground text-xs">
                {t("customers.detail.organizations")}
              </p>
              <ul className="flex flex-wrap gap-2" data-testid="customer-orgs">
                {c.organizations.map((o) => (
                  <li
                    key={o.uuid}
                    className="bg-muted rounded-md px-2 py-1 text-xs"
                  >
                    {o.name}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
        </CardContent>
      </Card>

      {access.canReadVehicles ? (
        <Card>
          <CardHeader className="flex flex-row items-center justify-between gap-2">
            <CardTitle className="flex items-center gap-2 text-base">
              <Car className="size-4" />
              {t("customers.detail.vehicles")}
            </CardTitle>
            {access.canWriteVehicles ? (
              <Button
                type="button"
                size="sm"
                data-action="add-vehicle"
                onClick={() => setVehicleDialog({ vehicle: null })}
              >
                <Plus className="size-4" />
                {t("customers.vehicle.add")}
              </Button>
            ) : null}
          </CardHeader>
          <CardContent>
            {vehicles.isLoading ? (
              <p className="text-muted-foreground text-sm">
                {t("customers.loading")}
              </p>
            ) : rows.length === 0 ? (
              <p
                className="text-muted-foreground text-sm"
                data-testid="vehicles-empty"
              >
                {t("customers.detail.no_vehicles")}
              </p>
            ) : (
              <ul className="divide-y" data-testid="vehicle-list">
                {rows.map((v) => (
                  <li
                    key={v.uuid}
                    className="flex flex-wrap items-center gap-3 py-2"
                    data-testid="vehicle-row"
                    data-uuid={v.uuid}
                  >
                    <VehicleBrandLogo
                      uuid={v.car_brand?.uuid}
                      name={v.car_brand?.name}
                      height={28}
                    />
                    <div className="min-w-0 flex-1">
                      <Link
                        href={routes.tenant.vehicles.detail(slug, v.uuid)}
                        className="font-mono font-medium hover:underline"
                        dir="ltr"
                      >
                        {dash(v.plate)}
                        {v.plate_country ? ` (${v.plate_country})` : ""}
                      </Link>
                      <p className="text-muted-foreground text-xs">
                        {dash(
                          [v.car_brand?.name, v.car_model?.name, v.model_year]
                            .filter(Boolean)
                            .join(" "),
                        )}
                        {v.vin ? (
                          <span className="ms-2 font-mono" dir="ltr">
                            {v.vin}
                          </span>
                        ) : null}
                      </p>
                    </div>
                    {access.canWriteVehicles ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        data-action="edit-vehicle"
                        onClick={() => setVehicleDialog({ vehicle: v })}
                      >
                        <Pencil className="size-4" />
                        {t("customers.vehicle.edit")}
                      </Button>
                    ) : null}
                  </li>
                ))}
              </ul>
            )}
          </CardContent>
        </Card>
      ) : null}

      <VehicleFormDialog
        open={vehicleDialog !== null}
        customerUuid={c.uuid}
        vehicle={vehicleDialog?.vehicle ?? null}
        onClose={() => setVehicleDialog(null)}
        onSaved={() => {
          setVehicleDialog(null);
          void qc.invalidateQueries({ queryKey: customerKeys.vehicles(uuid) });
        }}
      />
      {access.canAnonymize ? (
        <AnonymizeDialog
          customer={c}
          open={action === "anonymize"}
          onClose={() => setAction(null)}
          onDone={() => {
            setAction(null);
            refresh();
          }}
        />
      ) : null}
      {access.canExport ? (
        <DataExportDialog
          customer={c}
          open={action === "export"}
          onClose={() => setAction(null)}
        />
      ) : null}
      {access.canUpgrade ? (
        <UpgradeDialog
          customer={c}
          open={action === "upgrade"}
          onClose={() => setAction(null)}
          onDone={() => {
            setAction(null);
            refresh();
          }}
        />
      ) : null}
    </div>
  );
}
