"use client";

import { CalendarClock } from "lucide-react";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { AppointmentCalendar } from "@/features/appointments/components/appointment-calendar";
import { AppointmentSettingsPanel } from "@/features/appointments/components/appointment-settings-panel";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Panel "Appointments" (TEC-326): the day / week calendar and, with
 * appointment_settings.manage, the capacity settings tab.
 */
export function AppointmentsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead = can(permissions.appointments.read);
  const canWrite = can(permissions.appointments.write);
  const canSettings = can(permissions.appointments.settingsManage);
  const title = t("appointments.title");

  return (
    <div className="space-y-6">
      <PageHeader
        title={title}
        description={t("appointments.description")}
        icon={<CalendarClock className="size-6" />}
        breadcrumbs={[
          {
            label: t("layout.breadcrumb_home"),
            href: routes.tenant.home(slug),
          },
          { label: title },
        ]}
      />
      {!canRead ? (
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("appointments.forbidden")}
        />
      ) : canSettings ? (
        <Tabs defaultValue="calendar">
          <TabsList>
            <TabsTrigger value="calendar">
              {t("appointments.tabs.calendar")}
            </TabsTrigger>
            <TabsTrigger value="settings">
              {t("appointments.tabs.settings")}
            </TabsTrigger>
          </TabsList>
          <TabsContent value="calendar" className="pt-4">
            <AppointmentCalendar slug={slug} canWrite={canWrite} />
          </TabsContent>
          <TabsContent value="settings" className="pt-4">
            <AppointmentSettingsPanel timeZone={format.timeZone} />
          </TabsContent>
        </Tabs>
      ) : (
        <AppointmentCalendar slug={slug} canWrite={canWrite} />
      )}
    </div>
  );
}
