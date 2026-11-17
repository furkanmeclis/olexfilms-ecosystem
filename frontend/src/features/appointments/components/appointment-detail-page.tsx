"use client";

import { useQuery } from "@tanstack/react-query";
import { CalendarClock } from "lucide-react";
import type { ReactNode } from "react";

import { EntityLink } from "@/components/entity";
import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { STATUS_TONE } from "@/features/appointments/lib/calendar";
import {
  appointmentKeys,
  appointmentsService,
  type Appointment,
} from "@/features/appointments/services/appointments.service";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/** Customer ref, or the legacy panel fields when the ref is missing. */
function customerRef(a: Appointment) {
  if (a.customer) return a.customer;
  return a.customer_uuid || a.customer_name
    ? { uuid: a.customer_uuid, name: a.customer_name }
    : null;
}

/** Vehicle ref, or the legacy plate / label fields. */
function vehicleRef(a: Appointment) {
  if (a.vehicle) return a.vehicle;
  const name = [a.vehicle_plate, a.vehicle_label].filter(Boolean).join(" · ");
  return a.vehicle_uuid || name ? { uuid: a.vehicle_uuid, name } : null;
}

/**
 * Tenant > Appointments > detail (TEC-1218): one booking with its status,
 * times and every related record (organization, customer, vehicle, service,
 * lead, creator) linked to its own page.
 */
export function AppointmentDetailPage({
  slug,
  uuid,
}: {
  slug: string;
  uuid: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.appointments.read);

  const detail = useQuery({
    queryKey: appointmentKeys.detail(uuid),
    queryFn: () => appointmentsService.get(uuid),
    enabled: canRead && uuid !== "",
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });

  const a = detail.data;
  const listTitle = t("appointments.title");
  const title = a
    ? customerRef(a)?.name || t("appointments.dialog.edit_title")
    : t("appointments.dialog.edit_title");
  const header = (
    <PageHeader
      title={title}
      icon={<CalendarClock className="size-6" />}
      description={a ? format.dateTime(a.starts_at) : undefined}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: listTitle, href: routes.tenant.appointments.calendar(slug) },
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
          description={t("appointments.forbidden")}
        />
      </div>
    );
  }
  if (detail.isError) {
    const notFound = isApiError(detail.error) && detail.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound
              ? t("appointments.detail.not_found")
              : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }
  if (!a) {
    return (
      <div className="space-y-6">
        {header}
        <p className="text-muted-foreground text-sm">
          {t("appointments.detail.loading")}
        </p>
      </div>
    );
  }

  const when = (v: string | null | undefined) => (v ? format.dateTime(v) : "—");

  return (
    <div className="space-y-6" data-testid="appointment-detail">
      {header}
      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
          <CardTitle>{t("appointments.detail.summary")}</CardTitle>
          <span
            data-testid="appointment-status"
            className={cn(
              "rounded-md border-s-4 px-2 py-0.5 text-xs font-medium",
              STATUS_TONE[a.status],
            )}
          >
            {t(`appointments.status.${a.status}`)}
          </span>
        </CardHeader>
        <CardContent className="space-y-4">
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Field label={t("appointments.fields.starts_at")}>
              {when(a.starts_at)}
            </Field>
            <Field label={t("appointments.fields.ends_at")}>
              {when(a.ends_at)}
            </Field>
            <Field label={t("appointments.fields.estimated_minutes")}>
              {t("appointments.detail.minutes", {
                minutes: a.estimated_minutes,
              })}
            </Field>
            <Field label={t("appointments.fields.customer")}>
              <EntityLink kind="customer" entity={customerRef(a)} slug={slug} />
            </Field>
            <Field label={t("appointments.fields.vehicle")}>
              <EntityLink
                kind="vehicle"
                entity={vehicleRef(a)}
                slug={slug}
                fallback={t("appointments.fields.vehicle_none")}
              />
            </Field>
            <Field label={t("appointments.fields.organization")}>
              <EntityLink
                kind="organization"
                entity={a.organization}
                slug={slug}
              />
            </Field>
            <Field label={t("appointments.fields.service")}>
              <EntityLink kind="service" entity={a.service} slug={slug} />
            </Field>
            <Field label={t("appointments.fields.lead")}>
              <EntityLink kind="lead" entity={a.lead} slug={slug} />
            </Field>
            <Field label={t("appointments.fields.source")}>
              {t(`appointments.source.${a.source}`)}
            </Field>
            <Field label={t("appointments.fields.created_by")}>
              <EntityLink kind="user" entity={a.created_by} slug={slug} />
            </Field>
            <Field label={t("appointments.fields.created_at")}>
              {when(a.created_at)}
            </Field>
            <Field label={t("appointments.fields.updated_at")}>
              {when(a.updated_at)}
            </Field>
          </dl>
          {a.status === "cancelled" && a.cancel_reason ? (
            <div data-testid="appointment-cancel-reason">
              <div className="text-muted-foreground text-xs">
                {t("appointments.fields.cancel_reason")}
              </div>
              <p className="text-sm whitespace-pre-wrap">{a.cancel_reason}</p>
            </div>
          ) : null}
          {a.note ? (
            <div>
              <div className="text-muted-foreground text-xs">
                {t("appointments.fields.note")}
              </div>
              <p className="text-sm whitespace-pre-wrap">{a.note}</p>
            </div>
          ) : null}
        </CardContent>
      </Card>
    </div>
  );
}
