"use client";

import { ArrowLeft, CalendarClock } from "lucide-react";
import Link from "next/link";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  AppointmentCalendar,
  type CalendarView,
} from "@/features/appointments/components/appointment-calendar";
import { useActiveOrganization } from "@/hooks/use-active-organization";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const DATE_RE = /^\d{4}-\d{2}-\d{2}$/;

/**
 * Read-only calendar of one organization of the network (TEC-328), opened
 * from the occupancy table: the TEC-326 calendar narrowed with
 * organization_uuid, without booking, drag and drop or status actions.
 * `name`, `date` and `view` come from the occupancy row (query string).
 */
export function OrganizationCalendarPage({
  slug,
  organizationUuid,
  name,
  date,
  view,
}: {
  slug: string;
  organizationUuid: string;
  name?: string | null;
  date?: string | null;
  view?: string | null;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const org = useActiveOrganization(slug);
  const canRead = can(permissions.appointments.read);
  const networkOrg = org?.type === "center" || org?.type === "distributor";
  const occupancyTitle = t("appointments.occupancy.title");
  const title = name || t("appointments.occupancy.calendar_title");
  const initialView: CalendarView = view === "day" ? "day" : "week";
  const initialDate = date && DATE_RE.test(date) ? date : undefined;

  return (
    <div className="space-y-6">
      <PageHeader
        title={title}
        description={t("appointments.occupancy.calendar_description")}
        icon={<CalendarClock className="size-6" />}
        breadcrumbs={[
          {
            label: t("layout.breadcrumb_home"),
            href: routes.tenant.home(slug),
          },
          {
            label: occupancyTitle,
            href: routes.tenant.appointments.occupancy(slug),
          },
          { label: title },
        ]}
        actions={
          <Button asChild variant="outline">
            <Link
              href={routes.tenant.appointments.occupancy(slug)}
              data-testid="occupancy-back"
            >
              <ArrowLeft className="size-4 rtl:-scale-x-100" />
              {t("appointments.occupancy.back")}
            </Link>
          </Button>
        }
      />
      {!canRead || (org && !networkOrg) ? (
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("appointments.occupancy.forbidden")}
        />
      ) : (
        <AppointmentCalendar
          slug={slug}
          canWrite={false}
          readOnly
          organizationUuid={organizationUuid}
          initialView={initialView}
          initialDate={initialDate}
        />
      )}
    </div>
  );
}
