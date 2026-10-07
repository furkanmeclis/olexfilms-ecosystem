"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { CalendarClock, CalendarPlus, Car, Store } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { routes } from "@/config/routes";
import { PortalPage } from "@/features/portal/components/portal-page";
import {
  PORTAL_APPOINTMENT_PAGE_SIZE,
  portalAppointmentErrorKey,
  portalCancelState,
} from "@/features/portal/lib/portal-appointments";
import {
  PortalApiError,
  portalApi,
  type PortalAppointment,
  type PortalAppointmentPeriod,
} from "@/features/portal/lib/portal-client";
import { usePortalReadOnly } from "@/features/portal/lib/use-portal-read-only";
import { pageCount } from "@/features/warranty/lib/warranty-list";
import { useLocale } from "@/providers/locale-provider";

const STATUS_TONE: Record<
  PortalAppointment["status"],
  "default" | "success" | "warning" | "danger"
> = {
  scheduled: "default",
  confirmed: "success",
  arrived: "success",
  no_show: "warning",
  cancelled: "danger",
};

export const PORTAL_APPOINTMENTS_KEY = ["portal", "appointments"] as const;

/**
 * Portal > My appointments (TEC-327): upcoming (soonest first) and past
 * appointments of the user (GET /v1/portal/appointments) as cards, with
 * cancellation until two hours before the start. A fleet session is read
 * only: no "book" and no "cancel".
 */
export function PortalAppointments({ now }: { now?: Date }) {
  const { t } = useLocale();
  const readOnly = usePortalReadOnly();
  const [period, setPeriod] = useState<PortalAppointmentPeriod>("upcoming");
  const [page, setPage] = useState(0);
  const [cancelling, setCancelling] = useState<PortalAppointment | null>(null);

  const list = useQuery({
    queryKey: [...PORTAL_APPOINTMENTS_KEY, period, page],
    queryFn: () =>
      portalApi.listAppointments(
        period,
        PORTAL_APPOINTMENT_PAGE_SIZE,
        page * PORTAL_APPOINTMENT_PAGE_SIZE,
      ),
    placeholderData: keepPreviousData,
  });
  const rows = list.data?.items ?? [];
  const total = list.data?.total ?? 0;
  const pages = pageCount(total, PORTAL_APPOINTMENT_PAGE_SIZE);

  return (
    <PortalPage
      title={t("portal.appointments.title")}
      icon={<CalendarClock className="size-6" />}
      testId="portal-appointments-page"
      actions={
        readOnly ? null : (
          <Button asChild size="sm">
            <Link
              href={routes.portal.newAppointment()}
              data-testid="portal-appointment-book"
            >
              <CalendarPlus className="size-4" />
              {t("portal.appointments.book")}
            </Link>
          </Button>
        )
      }
    >
      <Tabs
        value={period}
        onValueChange={(v) => {
          setPeriod(v as PortalAppointmentPeriod);
          setPage(0);
        }}
      >
        <TabsList>
          <TabsTrigger
            value="upcoming"
            data-testid="portal-appointments-tab-upcoming"
          >
            {t("portal.appointments.tab_upcoming")}
          </TabsTrigger>
          <TabsTrigger value="past" data-testid="portal-appointments-tab-past">
            {t("portal.appointments.tab_past")}
          </TabsTrigger>
        </TabsList>
      </Tabs>

      {readOnly ? (
        <p
          className="text-muted-foreground text-xs"
          data-testid="portal-read-only"
        >
          {t("portal.read_only.notice")}
        </p>
      ) : null}

      {list.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void list.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : list.isLoading ? (
        <p className="text-muted-foreground text-sm">
          {t("portal.appointments.loading")}
        </p>
      ) : rows.length === 0 ? (
        <Card>
          <CardContent
            className="flex flex-col items-center gap-2 py-10 text-center"
            data-testid="portal-appointments-empty"
          >
            <CalendarClock className="text-muted-foreground size-10" />
            <p className="font-medium">
              {period === "upcoming"
                ? t("portal.appointments.empty_upcoming")
                : t("portal.appointments.empty_past")}
            </p>
          </CardContent>
        </Card>
      ) : (
        <ul className="space-y-3" data-testid="portal-appointments">
          {rows.map((a) => (
            <li key={a.uuid}>
              <AppointmentCard
                appointment={a}
                now={now ?? new Date()}
                canWrite={!readOnly && period === "upcoming"}
                onCancel={() => setCancelling(a)}
              />
            </li>
          ))}
        </ul>
      )}

      {pages > 1 ? (
        <div className="flex items-center justify-between gap-2">
          <p className="text-muted-foreground text-sm">
            {t("warranty.list.page", { page: page + 1, pages, total })}
          </p>
          <div className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page === 0 || list.isFetching}
              onClick={() => setPage((p) => Math.max(0, p - 1))}
            >
              {t("warranty.list.prev")}
            </Button>
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={page + 1 >= pages || list.isFetching}
              onClick={() => setPage((p) => p + 1)}
            >
              {t("warranty.list.next")}
            </Button>
          </div>
        </div>
      ) : null}

      <CancelDialog
        appointment={cancelling}
        onClose={() => setCancelling(null)}
      />
    </PortalPage>
  );
}

function AppointmentCard({
  appointment: a,
  now,
  canWrite,
  onCancel,
}: {
  appointment: PortalAppointment;
  now: Date;
  canWrite: boolean;
  onCancel: () => void;
}) {
  const { t, format } = useLocale();
  const cancel = portalCancelState(a, now);
  return (
    <Card data-testid="portal-appointment" data-uuid={a.uuid}>
      <CardContent className="flex flex-wrap items-start justify-between gap-3 pt-6">
        <div className="min-w-0 space-y-1">
          <p className="font-medium" data-testid="portal-appointment-when">
            {format.dateParts(a.starts_at, {
              weekday: "long",
              day: "numeric",
              month: "long",
              year: "numeric",
            })}{" "}
            · {format.time(a.starts_at)}
          </p>
          <p className="text-muted-foreground flex items-center gap-1 text-sm">
            <Store className="size-4 shrink-0" aria-hidden />
            <span className="truncate">{a.dealer_name}</span>
          </p>
          {a.vehicle_plate ? (
            <p className="text-muted-foreground flex items-center gap-1 text-sm">
              <Car className="size-4 shrink-0" aria-hidden />
              <span className="font-mono tracking-wider" dir="ltr">
                {a.vehicle_plate}
              </span>
            </p>
          ) : null}
        </div>
        <div className="flex flex-col items-end gap-2">
          <StatusChip
            label={t(`portal.appointments.status.${a.status}`)}
            tone={STATUS_TONE[a.status] ?? "default"}
          />
          {canWrite && cancel.kind !== "not_cancellable" ? (
            <>
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={cancel.kind !== "allowed"}
                aria-describedby={
                  cancel.kind === "window_closed"
                    ? `cancel-hint-${a.uuid}`
                    : undefined
                }
                onClick={onCancel}
                data-testid="portal-appointment-cancel"
              >
                {t("portal.appointments.cancel")}
              </Button>
              {cancel.kind === "window_closed" ? (
                <p
                  id={`cancel-hint-${a.uuid}`}
                  className="text-muted-foreground max-w-56 text-end text-xs"
                  data-testid="portal-appointment-cancel-hint"
                >
                  {t("portal.appointments.cancel_window_hint")}
                </p>
              ) : null}
            </>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}

function CancelDialog({
  appointment,
  onClose,
}: {
  appointment: PortalAppointment | null;
  onClose: () => void;
}) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const mutation = useMutation({
    mutationFn: (uuid: string) => portalApi.cancelAppointment(uuid),
    onSuccess: () => {
      toast.success(t("portal.appointments.cancelled"));
      void qc.invalidateQueries({ queryKey: PORTAL_APPOINTMENTS_KEY });
      onClose();
    },
    onError: (err) => {
      toast.error(
        t(
          portalAppointmentErrorKey(
            err instanceof PortalApiError ? err : { code: null },
          ),
        ),
      );
      void qc.invalidateQueries({ queryKey: PORTAL_APPOINTMENTS_KEY });
    },
  });
  return (
    <Dialog
      open={appointment !== null}
      onOpenChange={(open) => {
        if (!open && !mutation.isPending) onClose();
      }}
    >
      <DialogContent data-testid="portal-appointment-cancel-dialog">
        <DialogHeader>
          <DialogTitle>{t("portal.appointments.cancel_title")}</DialogTitle>
          <DialogDescription>
            {appointment
              ? t("portal.appointments.cancel_description", {
                  when: format.dateTime(appointment.starts_at),
                  dealer: appointment.dealer_name,
                })
              : null}
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            onClick={onClose}
            disabled={mutation.isPending}
          >
            {t("portal.appointments.keep")}
          </Button>
          <Button
            type="button"
            variant="destructive"
            disabled={mutation.isPending || !appointment}
            onClick={() => appointment && mutation.mutate(appointment.uuid)}
            data-testid="portal-appointment-cancel-confirm"
          >
            {t("portal.appointments.cancel_confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
