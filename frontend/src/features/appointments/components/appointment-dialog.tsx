"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CarFront, ExternalLink } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useCallback, useId, useRef, useState, type FormEvent } from "react";

import { Alert, AlertDescription } from "@/components/ui/alert";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { TimePicker } from "@/components/ui/time-picker";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { routes } from "@/config/routes";
import {
  bookingErrorKey,
  canReschedule,
  canStartIntake,
  nextStatuses,
} from "@/features/appointments/lib/calendar";
import {
  minutesLabel,
  parseClock,
  todayIn,
  zonedParts,
  zonedToUtc,
} from "@/features/appointments/lib/time";
import {
  appointmentKeys,
  appointmentsService,
  type Appointment,
  type AppointmentStatus,
} from "@/features/appointments/services/appointments.service";
import {
  serviceWizardKeys,
  serviceWizardService,
} from "@/features/services/services/service-wizard.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const NO_VEHICLE = "__none__";

/** Status action labels (the backend status the button moves to). */
const ACTION_KEY: Partial<Record<AppointmentStatus, string>> = {
  confirmed: "appointments.actions.confirm",
  arrived: "appointments.actions.arrived",
  no_show: "appointments.actions.no_show",
  cancelled: "appointments.actions.cancel",
};

export type AppointmentDialogProps = {
  slug: string;
  timeZone: string;
  canWrite: boolean;
  /** The edited appointment; null opens a new booking. */
  appointment: Appointment | null;
  initialDate?: string;
  initialMinutes?: number;
  onClose: () => void;
};

/**
 * New / edit appointment (TEC-326): customer search (customers in scope),
 * one of the customer's vehicles, local day and time in the organization
 * zone, duration and note. An existing booking also gets the status actions
 * (confirm, arrived, no-show, cancel with reason) and "start vehicle intake"
 * once the customer has arrived, which opens the service wizard on the
 * draft service.
 */
export function AppointmentDialog({
  slug,
  timeZone,
  canWrite,
  appointment,
  initialDate,
  initialMinutes,
  onClose,
}: AppointmentDialogProps) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const router = useRouter();
  const id = useId();
  const [current, setCurrent] = useState<Appointment | null>(appointment);
  const start = current
    ? zonedParts(current.starts_at, timeZone)
    : {
        date: initialDate ?? todayIn(timeZone),
        minutes: initialMinutes ?? 9 * 60,
      };
  const [customer, setCustomer] = useState<ComboboxOption | null>(
    current?.customer_uuid
      ? {
          value: current.customer_uuid,
          label: current.customer_name || current.customer_uuid,
        }
      : null,
  );
  const [vehicleUuid, setVehicleUuid] = useState(
    current?.vehicle_uuid ?? NO_VEHICLE,
  );
  const [date, setDate] = useState(start.date);
  const [time, setTime] = useState(minutesLabel(start.minutes));
  const [duration, setDuration] = useState(
    current ? String(current.estimated_minutes) : "",
  );
  const [note, setNote] = useState(current?.note ?? "");
  const [error, setError] = useState<string | null>(null);
  const [cancelling, setCancelling] = useState(false);
  const [cancelReason, setCancelReason] = useState("");

  const editable = canWrite && (!current || canReschedule(current));

  // The combobox reports only the value; labels come from the last search.
  const seen = useRef(new Map<string, ComboboxOption>());
  const loadCustomers = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const page = await serviceWizardService.listCustomers({
        q: q.trim() || undefined,
      });
      const options = page.items.map((c) => ({
        value: c.uuid,
        label: [c.name, c.surname].filter(Boolean).join(" "),
        description: c.phone ?? c.email ?? undefined,
      }));
      for (const o of options) seen.current.set(o.value, o);
      return options;
    },
    [],
  );

  const vehicles = useQuery({
    queryKey: serviceWizardKeys.vehicles(customer?.value ?? ""),
    queryFn: () => serviceWizardService.listVehicles(customer?.value ?? ""),
    enabled: Boolean(customer) && editable,
  });

  const done = (saved: Appointment, message: string) => {
    void qc.invalidateQueries({ queryKey: appointmentKeys.all });
    setCurrent(saved);
    appToast.success(message);
  };

  const save = useMutation({
    mutationFn: (startsAt: string) => {
      const mins = duration.trim() === "" ? null : Number(duration);
      const body = {
        customer_uuid: customer?.value ?? "",
        vehicle_uuid: vehicleUuid === NO_VEHICLE ? null : vehicleUuid,
        starts_at: startsAt,
        estimated_minutes: mins,
        source: current?.source ?? ("panel" as const),
        note: note.trim(),
      };
      return current
        ? appointmentsService.patch(current.uuid, body)
        : appointmentsService.create(body);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: appointmentKeys.all });
      appToast.success(
        t(
          current
            ? "appointments.dialog.updated"
            : "appointments.dialog.created",
        ),
      );
      onClose();
    },
    onError: (err) => {
      if (isApiError(err) && err.code === "VALIDATION_ERROR") {
        setError(err.details[0]?.message ?? err.message);
      } else {
        setError(t(bookingErrorKey(err)));
      }
    },
  });

  const status = useMutation({
    mutationFn: (v: { status: AppointmentStatus; reason?: string }) =>
      appointmentsService.setStatus(current?.uuid ?? "", v.status, v.reason),
    onSuccess: (saved) => {
      setCancelling(false);
      done(saved, t(`appointments.status_changed.${saved.status}`));
    },
    onError: (err) => appToast.error(t(bookingErrorKey(err))),
  });

  const intake = useMutation({
    mutationFn: () => appointmentsService.startIntake(current?.uuid ?? ""),
    onSuccess: (saved) => {
      void qc.invalidateQueries({ queryKey: appointmentKeys.all });
      setCurrent(saved);
      if (saved.service_uuid) {
        appToast.success(t("appointments.intake.started"));
        router.push(routes.tenant.services.wizard(slug, saved.service_uuid));
      }
    },
    onError: (err) =>
      appToast.error(
        isApiError(err) && err.code === "APPOINTMENT_INTAKE_ALREADY_STARTED"
          ? t("appointments.intake.already_started")
          : t(bookingErrorKey(err)),
      ),
  });

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    if (!customer) {
      setError(t("appointments.dialog.customer_required"));
      return;
    }
    const minutes = parseClock(time);
    if (!/^\d{4}-\d{2}-\d{2}$/.test(date) || minutes === null) {
      setError(t("appointments.dialog.time_invalid"));
      return;
    }
    if (
      duration.trim() !== "" &&
      !(Number.isInteger(Number(duration)) && Number(duration) > 0)
    ) {
      setError(t("appointments.dialog.duration_invalid"));
      return;
    }
    save.mutate(zonedToUtc(date, minutes, timeZone));
  };

  const busy = save.isPending || status.isPending || intake.isPending;
  const actions = current && canWrite ? nextStatuses(current.status) : [];
  const showIntake =
    current &&
    canWrite &&
    current.status !== "cancelled" &&
    current.status !== "no_show";

  return (
    <Dialog open onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent className="sm:max-w-lg">
        <form onSubmit={onSubmit} noValidate className="space-y-4">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              {current
                ? t("appointments.dialog.edit_title")
                : t("appointments.dialog.new_title")}
              {current ? (
                <Badge variant="outline" data-testid="dialog-status">
                  {t(`appointments.status.${current.status}`)}
                </Badge>
              ) : null}
            </DialogTitle>
            <DialogDescription>
              {t("appointments.dialog.description", { zone: timeZone })}
            </DialogDescription>
          </DialogHeader>

          {current?.status === "cancelled" && current.cancel_reason ? (
            <Alert>
              <AlertDescription>
                {t("appointments.dialog.cancel_reason_shown", {
                  reason: current.cancel_reason,
                })}
              </AlertDescription>
            </Alert>
          ) : null}

          <div className="space-y-2">
            <Label htmlFor={`${id}-customer`}>
              {t("appointments.fields.customer")}
            </Label>
            <AsyncCombobox
              id={`${id}-customer`}
              className="w-full"
              value={customer?.value ?? ""}
              initialOptions={customer ? [customer] : undefined}
              disabled={!editable}
              loadOptions={loadCustomers}
              onValueChange={(value) => {
                setCustomer(
                  value
                    ? (seen.current.get(value) ?? { value, label: value })
                    : null,
                );
                setVehicleUuid(NO_VEHICLE);
              }}
              placeholder={t("appointments.fields.customer_placeholder")}
              searchPlaceholder={t("appointments.fields.customer_search")}
              emptyText={t("appointments.fields.customer_none")}
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor={`${id}-vehicle`}>
              {t("appointments.fields.vehicle")}
            </Label>
            {editable ? (
              <Select
                value={vehicleUuid}
                onValueChange={setVehicleUuid}
                disabled={!customer}
              >
                <SelectTrigger id={`${id}-vehicle`} className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value={NO_VEHICLE}>
                    {t("appointments.fields.vehicle_none")}
                  </SelectItem>
                  {current?.vehicle_uuid &&
                  !vehicles.data?.items.some(
                    (v) => v.uuid === current.vehicle_uuid,
                  ) ? (
                    <SelectItem value={current.vehicle_uuid}>
                      {[current.vehicle_plate, current.vehicle_label]
                        .filter(Boolean)
                        .join(" · ") || current.vehicle_uuid}
                    </SelectItem>
                  ) : null}
                  {(vehicles.data?.items ?? []).map((v) => (
                    <SelectItem key={v.uuid} value={v.uuid}>
                      {[
                        v.plate,
                        [v.car_brand?.name, v.car_model?.name]
                          .filter(Boolean)
                          .join(" "),
                      ]
                        .filter(Boolean)
                        .join(" · ") ||
                        t("appointments.fields.vehicle_unnamed")}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            ) : (
              <p className="text-sm" id={`${id}-vehicle`}>
                {[current?.vehicle_plate, current?.vehicle_label]
                  .filter(Boolean)
                  .join(" · ") || t("appointments.fields.vehicle_none")}
              </p>
            )}
          </div>

          <div className="grid grid-cols-3 gap-3">
            <div className="space-y-2">
              <Label htmlFor={`${id}-date`}>
                {t("appointments.fields.date")}
              </Label>
              <DatePicker
                id={`${id}-date`}
                className="w-full"
                value={date}
                disabled={!editable}
                onChange={setDate}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${id}-time`}>
                {t("appointments.fields.time")}
              </Label>
              <TimePicker
                id={`${id}-time`}
                value={time}
                disabled={!editable}
                clearable={false}
                onChange={setTime}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor={`${id}-duration`}>
                {t("appointments.fields.duration")}
              </Label>
              <Input
                id={`${id}-duration`}
                inputMode="numeric"
                dir="ltr"
                placeholder={t("appointments.fields.duration_default")}
                value={duration}
                disabled={!editable}
                onChange={(e) => setDuration(e.target.value)}
              />
            </div>
          </div>

          <div className="space-y-2">
            <Label htmlFor={`${id}-note`}>
              {t("appointments.fields.note")}
            </Label>
            <Textarea
              id={`${id}-note`}
              rows={3}
              maxLength={20000}
              value={note}
              disabled={!editable}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>

          {current ? (
            <p className="text-muted-foreground text-xs">
              {t(`appointments.source.${current.source}`)}
              {current.created_at
                ? ` · ${format.dateTime(current.created_at)}`
                : null}
            </p>
          ) : null}

          {error ? (
            <p role="alert" className="text-destructive text-sm">
              {error}
            </p>
          ) : null}

          {actions.length || showIntake ? (
            <div className="space-y-3 border-t pt-4">
              <div className="flex flex-wrap gap-2">
                {actions.map((next) => (
                  <Button
                    key={next}
                    type="button"
                    size="sm"
                    variant={next === "cancelled" ? "destructive" : "outline"}
                    data-action={next}
                    disabled={busy}
                    onClick={() =>
                      next === "cancelled"
                        ? setCancelling(true)
                        : status.mutate({ status: next })
                    }
                  >
                    {t(ACTION_KEY[next] ?? next)}
                  </Button>
                ))}
                {showIntake ? (
                  current.service_uuid ? (
                    <Button asChild type="button" size="sm" variant="secondary">
                      <Link
                        href={routes.tenant.services.wizard(
                          slug,
                          current.service_uuid,
                        )}
                      >
                        <ExternalLink className="size-4" />
                        {t("appointments.intake.open_service")}
                      </Link>
                    </Button>
                  ) : (
                    <Button
                      type="button"
                      size="sm"
                      data-action="start-intake"
                      disabled={busy || !canStartIntake(current)}
                      onClick={() => intake.mutate()}
                    >
                      <CarFront className="size-4" />
                      {t("appointments.intake.start")}
                    </Button>
                  )
                ) : null}
              </div>
              {showIntake &&
              !current.service_uuid &&
              !canStartIntake(current) ? (
                <p className="text-muted-foreground text-xs">
                  {current.vehicle_id == null
                    ? t("appointments.intake.needs_vehicle")
                    : t("appointments.intake.needs_arrival")}
                </p>
              ) : null}
              {cancelling ? (
                <div className="space-y-2">
                  <Label htmlFor={`${id}-reason`}>
                    {t("appointments.fields.cancel_reason")}
                  </Label>
                  <Textarea
                    id={`${id}-reason`}
                    rows={2}
                    maxLength={1000}
                    value={cancelReason}
                    onChange={(e) => setCancelReason(e.target.value)}
                  />
                  <div className="flex gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="destructive"
                      data-action="confirm-cancel"
                      disabled={busy || cancelReason.trim() === ""}
                      onClick={() =>
                        status.mutate({
                          status: "cancelled",
                          reason: cancelReason.trim(),
                        })
                      }
                    >
                      {t("appointments.actions.confirm_cancel")}
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="ghost"
                      onClick={() => setCancelling(false)}
                    >
                      {t("common.back")}
                    </Button>
                  </div>
                </div>
              ) : null}
            </div>
          ) : null}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.close")}
            </Button>
            {editable ? (
              <Button type="submit" disabled={busy}>
                {save.isPending ? t("common.saving") : t("common.save")}
              </Button>
            ) : null}
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
