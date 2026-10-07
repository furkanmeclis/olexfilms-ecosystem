"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { CalendarOff, Trash2 } from "lucide-react";
import { useId, useState, type FormEvent } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { addDays, todayIn } from "@/features/appointments/lib/time";
import {
  validateWorkingHours,
  WEEKDAYS,
  workingHoursFromApi,
  workingHoursToApi,
  type WorkingHoursErrors,
  type WorkingHoursForm,
} from "@/features/appointments/lib/working-hours";
import {
  appointmentKeys,
  appointmentsService,
  type AppointmentSettings,
} from "@/features/appointments/services/appointments.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

const positiveInt = (v: string) => {
  const n = Number(v);
  return Number.isInteger(n) && n > 0 ? n : null;
};

function SettingsForm({ settings }: { settings: AppointmentSettings }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const id = useId();
  const [capacity, setCapacity] = useState(
    String(settings.daily_vehicle_capacity),
  );
  const [defaultMinutes, setDefaultMinutes] = useState(
    String(settings.default_estimated_minutes),
  );
  const [slotInterval, setSlotInterval] = useState(
    String(settings.slot_interval_minutes),
  );
  const [portal, setPortal] = useState(settings.portal_appointments_enabled);
  const [hours, setHours] = useState<WorkingHoursForm>(() =>
    workingHoursFromApi(settings.working_hours),
  );
  const [hourErrors, setHourErrors] = useState<WorkingHoursErrors>({});
  const [error, setError] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: appointmentsService.putSettings,
    onSuccess: (saved) => {
      qc.setQueryData(appointmentKeys.settings, saved);
      void qc.invalidateQueries({ queryKey: appointmentKeys.all });
      appToast.success(t("appointments.settings.saved"));
    },
    onError: (err) =>
      setError(
        isApiError(err) && err.code === "VALIDATION_ERROR"
          ? (err.details[0]?.message ?? err.message)
          : t("appointments.settings.save_failed"),
      ),
  });

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    const errors = validateWorkingHours(hours);
    setHourErrors(errors);
    const cap = positiveInt(capacity);
    const mins = positiveInt(defaultMinutes);
    const step = positiveInt(slotInterval);
    if (cap === null || mins === null || step === null) {
      setError(t("appointments.settings.numbers_invalid"));
      return;
    }
    if (Object.keys(errors).length) return;
    save.mutate({
      daily_vehicle_capacity: cap,
      default_estimated_minutes: mins,
      slot_interval_minutes: step,
      working_hours: workingHoursToApi(hours),
      portal_appointments_enabled: portal,
    });
  };

  const setDay = (
    day: (typeof WEEKDAYS)[number],
    patch: Partial<WorkingHoursForm[typeof day]>,
  ) => setHours((h) => ({ ...h, [day]: { ...h[day], ...patch } }));

  return (
    <form onSubmit={onSubmit} noValidate className="space-y-6">
      <Card>
        <CardHeader>
          <CardTitle>{t("appointments.settings.capacity_title")}</CardTitle>
          <CardDescription>
            {t("appointments.settings.capacity_description")}
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4 sm:grid-cols-3">
          <div className="space-y-2">
            <Label htmlFor={`${id}-cap`}>
              {t("appointments.settings.daily_capacity")}
            </Label>
            <Input
              id={`${id}-cap`}
              inputMode="numeric"
              dir="ltr"
              value={capacity}
              onChange={(e) => setCapacity(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor={`${id}-mins`}>
              {t("appointments.settings.default_minutes")}
            </Label>
            <Input
              id={`${id}-mins`}
              inputMode="numeric"
              dir="ltr"
              value={defaultMinutes}
              onChange={(e) => setDefaultMinutes(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor={`${id}-step`}>
              {t("appointments.settings.slot_interval")}
            </Label>
            <Input
              id={`${id}-step`}
              inputMode="numeric"
              dir="ltr"
              value={slotInterval}
              onChange={(e) => setSlotInterval(e.target.value)}
            />
          </div>
          <div className="flex items-center gap-3 sm:col-span-3">
            <Switch
              id={`${id}-portal`}
              checked={portal}
              onCheckedChange={setPortal}
            />
            <Label htmlFor={`${id}-portal`}>
              {t("appointments.settings.portal_enabled")}
            </Label>
          </div>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>{t("appointments.settings.hours_title")}</CardTitle>
          <CardDescription>
            {t("appointments.settings.hours_description")}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          {WEEKDAYS.map((day) => {
            const h = hours[day];
            const err = hourErrors[day];
            return (
              <div key={day} data-weekday={day} className="space-y-1">
                <div className="flex flex-wrap items-center gap-3">
                  <Switch
                    id={`${id}-${day}-open`}
                    checked={h.open}
                    onCheckedChange={(open) => setDay(day, { open })}
                  />
                  <Label htmlFor={`${id}-${day}-open`} className="w-28">
                    {t(`appointments.weekdays.${day}`)}
                  </Label>
                  {h.open ? (
                    <>
                      <Input
                        type="time"
                        dir="ltr"
                        className="w-32"
                        aria-label={t("appointments.settings.opens")}
                        name={`${day}-start`}
                        value={h.start}
                        aria-invalid={err ? true : undefined}
                        onChange={(e) => setDay(day, { start: e.target.value })}
                      />
                      <span className="text-muted-foreground">–</span>
                      <Input
                        type="time"
                        dir="ltr"
                        className="w-32"
                        aria-label={t("appointments.settings.closes")}
                        name={`${day}-end`}
                        value={h.end}
                        aria-invalid={err ? true : undefined}
                        onChange={(e) => setDay(day, { end: e.target.value })}
                      />
                    </>
                  ) : (
                    <span className="text-muted-foreground text-sm">
                      {t("appointments.settings.day_closed")}
                    </span>
                  )}
                </div>
                {err ? (
                  <p
                    className="text-destructive ps-14 text-xs"
                    data-error={day}
                  >
                    {t(err)}
                  </p>
                ) : null}
              </div>
            );
          })}
        </CardContent>
      </Card>

      {error ? (
        <p role="alert" className="text-destructive text-sm">
          {error}
        </p>
      ) : null}
      <div className="flex justify-end">
        <Button type="submit" disabled={save.isPending}>
          {save.isPending ? t("common.saving") : t("common.save")}
        </Button>
      </div>
    </form>
  );
}

function ClosuresCard({ timeZone }: { timeZone: string }) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const id = useId();
  const from = todayIn(timeZone);
  const to = addDays(from, 365);
  const key = appointmentKeys.closures(from, to);
  const closures = useQuery({
    queryKey: key,
    queryFn: () => appointmentsService.listClosures(from, to),
  });
  const [day, setDay] = useState("");
  const [reason, setReason] = useState("");

  const refresh = () => {
    void qc.invalidateQueries({ queryKey: appointmentKeys.all });
  };
  const create = useMutation({
    mutationFn: () => appointmentsService.createClosure(day, reason.trim()),
    onSuccess: () => {
      setDay("");
      setReason("");
      refresh();
      appToast.success(t("appointments.closures.added"));
    },
    onError: (err) =>
      appToast.error(
        isApiError(err) && err.code === "APPOINTMENT_CLOSURE_EXISTS"
          ? t("appointments.closures.exists")
          : t("appointments.closures.failed"),
      ),
  });
  const remove = useMutation({
    mutationFn: appointmentsService.deleteClosure,
    onSuccess: () => {
      refresh();
      appToast.success(t("appointments.closures.removed"));
    },
    onError: () => appToast.error(t("appointments.closures.failed")),
  });

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("appointments.closures.title")}</CardTitle>
        <CardDescription>
          {t("appointments.closures.description")}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <form
          className="flex flex-wrap items-end gap-3"
          noValidate
          onSubmit={(e) => {
            e.preventDefault();
            if (/^\d{4}-\d{2}-\d{2}$/.test(day)) create.mutate();
          }}
        >
          <div className="space-y-2">
            <Label htmlFor={`${id}-day`}>
              {t("appointments.closures.day")}
            </Label>
            <DatePicker id={`${id}-day`} value={day} onChange={setDay} />
          </div>
          <div className="min-w-48 flex-1 space-y-2">
            <Label htmlFor={`${id}-reason`}>
              {t("appointments.closures.reason")}
            </Label>
            <Input
              id={`${id}-reason`}
              maxLength={1000}
              value={reason}
              onChange={(e) => setReason(e.target.value)}
            />
          </div>
          <Button type="submit" disabled={!day || create.isPending}>
            <CalendarOff className="size-4" />
            {t("appointments.closures.add")}
          </Button>
        </form>
        {closures.isError ? (
          <ErrorState
            title={t("appointments.errors.load_title")}
            onRetry={() => void closures.refetch()}
          />
        ) : closures.isLoading ? (
          <Skeleton className="h-16 w-full" />
        ) : (closures.data ?? []).length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("appointments.closures.empty")}
          </p>
        ) : (
          <ul className="divide-y rounded-md border">
            {(closures.data ?? []).map((c) => (
              <li
                key={c.uuid}
                className="flex items-center justify-between gap-3 px-3 py-2"
              >
                <div className="min-w-0">
                  <p className="text-sm font-medium">
                    {format.date(`${c.closed_on}T12:00:00Z`)}
                  </p>
                  {c.reason ? (
                    <p className="text-muted-foreground truncate text-xs">
                      {c.reason}
                    </p>
                  ) : null}
                </div>
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  aria-label={t("appointments.closures.remove")}
                  disabled={remove.isPending}
                  onClick={() => remove.mutate(c.uuid)}
                >
                  <Trash2 className="size-4" />
                </Button>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

/**
 * Appointment settings of the active organization (TEC-326, needs
 * appointment_settings.manage): daily capacity, default duration, slot
 * interval, one opening per weekday, closure days and portal booking.
 */
export function AppointmentSettingsPanel({ timeZone }: { timeZone: string }) {
  const { t } = useLocale();
  const settings = useQuery({
    queryKey: appointmentKeys.settings,
    queryFn: appointmentsService.getSettings,
  });
  return (
    <div className="space-y-6" data-testid="appointment-settings">
      {settings.isError ? (
        <ErrorState
          title={t("appointments.errors.load_title")}
          description={t("appointments.errors.load_description")}
          onRetry={() => void settings.refetch()}
        />
      ) : settings.data ? (
        <SettingsForm key={settings.data.uuid} settings={settings.data} />
      ) : (
        <Skeleton className="h-96 w-full" />
      )}
      <ClosuresCard timeZone={timeZone} />
    </div>
  );
}
