"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  CalendarPlus,
  Car,
  ChevronLeft,
  ChevronRight,
  LocateFixed,
  Store,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useEffect, useMemo, useState, type ReactNode } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import type { LatLng } from "@/config/map";
import { routes } from "@/config/routes";
import {
  FALLBACK_RADIUS_KM,
  LOCATED_RADIUS_KM,
  RADIUS_OPTIONS,
  fetchNearbyDealers,
  formatDistanceKm,
  locateUser,
  type NearbyResult,
  type PositionResult,
  type RadiusKm,
} from "@/features/dealers/lib/dealers";
import { PORTAL_APPOINTMENTS_KEY } from "@/features/portal/components/portal-appointments";
import { PortalPage } from "@/features/portal/components/portal-page";
import {
  BOOKING_MAX_AHEAD_DAYS,
  BOOKING_WINDOW_DAYS,
  addDaysIso,
  dayState,
  isDayBookable,
  isoDate,
  portalAppointmentErrorKey,
} from "@/features/portal/lib/portal-appointments";
import {
  PortalApiError,
  portalApi,
  type AppointmentAvailabilityDay,
  type AppointmentSlot,
} from "@/features/portal/lib/portal-client";
import { portalVehicleTitle } from "@/features/portal/lib/portal-vehicles";
import { usePortalReadOnly } from "@/features/portal/lib/use-portal-read-only";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

export type BookingDealer = { uuid: string; name: string };

/** Vehicles offered in the picker (a customer rarely has more). */
const VEHICLE_PICKER_LIMIT = 100;

function browserGeolocation() {
  return typeof navigator !== "undefined" && "geolocation" in navigator
    ? navigator.geolocation
    : null;
}

/** Noon UTC of a YYYY-MM-DD: the same calendar day in every time zone. */
function dayInstant(date: string): string {
  return `${date}T12:00:00Z`;
}

/**
 * Portal > Book an appointment (TEC-327): dealer (nearby dealers that take
 * portal bookings), own vehicle, day and free start time from the dealer's
 * availability, then confirmation (POST /v1/portal/appointments). A dealer
 * card or the vehicle page can preselect the dealer or the vehicle. Success
 * goes to "my appointments". A fleet session cannot book.
 */
export function PortalAppointmentBooking({
  initialDealer,
  initialVehicle,
  today,
}: {
  initialDealer?: BookingDealer | null;
  initialVehicle?: string | null;
  today?: Date;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const qc = useQueryClient();
  const readOnly = usePortalReadOnly();

  const [dealer, setDealer] = useState<BookingDealer | null>(
    initialDealer ?? null,
  );
  const [vehicle, setVehicle] = useState<string | null>(initialVehicle ?? null);
  const [day, setDay] = useState<string | null>(null);
  const [slot, setSlot] = useState<AppointmentSlot | null>(null);
  const [note, setNote] = useState("");
  const [error, setError] = useState<string | null>(null);

  const chooseDealer = (d: BookingDealer | null) => {
    setDealer(d);
    setDay(null);
    setSlot(null);
    setError(null);
  };

  const create = useMutation({
    mutationFn: () =>
      portalApi.createAppointment({
        dealer_uuid: dealer!.uuid,
        vehicle_uuid: vehicle!,
        starts_at: slot!.start,
        ...(note.trim() ? { note: note.trim() } : {}),
      }),
    onSuccess: () => {
      toast.success(t("portal.appointments.booked"));
      void qc.invalidateQueries({ queryKey: PORTAL_APPOINTMENTS_KEY });
      router.push(routes.portal.appointments);
    },
    onError: (err) => {
      const e = err instanceof PortalApiError ? err : null;
      setError(portalAppointmentErrorKey(e ?? { code: null }));
      if (
        e?.code === "APPOINTMENT_CAPACITY_FULL" ||
        e?.code === "APPOINTMENT_DAY_CLOSED"
      ) {
        setSlot(null);
        void qc.invalidateQueries({
          queryKey: ["portal", "appointments", "availability"],
        });
      }
    },
  });

  const back = {
    href: routes.portal.appointments,
    label: t("portal.appointments.back"),
  };

  if (readOnly) {
    return (
      <PortalPage
        title={t("portal.appointments.new_title")}
        icon={<CalendarPlus className="size-6" />}
        back={back}
        testId="portal-appointment-booking"
      >
        <p
          className="text-muted-foreground text-sm"
          data-testid="portal-read-only"
        >
          {t("portal.read_only.notice")}
        </p>
      </PortalPage>
    );
  }

  const ready = Boolean(dealer && vehicle && slot);

  return (
    <PortalPage
      title={t("portal.appointments.new_title")}
      icon={<CalendarPlus className="size-6" />}
      back={back}
      testId="portal-appointment-booking"
    >
      <Step n={1} title={t("portal.appointments.step_dealer")}>
        {dealer ? (
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p
              className="flex items-center gap-2 font-medium"
              data-testid="booking-dealer-selected"
            >
              <Store className="size-4" aria-hidden />
              {dealer.name || t("portal.appointments.dealer_selected")}
            </p>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={() => chooseDealer(null)}
            >
              {t("portal.appointments.change")}
            </Button>
          </div>
        ) : (
          <DealerPicker onPick={chooseDealer} />
        )}
      </Step>

      <Step n={2} title={t("portal.appointments.step_vehicle")}>
        <VehiclePicker value={vehicle} onChange={setVehicle} />
      </Step>

      <Step n={3} title={t("portal.appointments.step_time")}>
        {dealer ? (
          <SlotPicker
            dealerUuid={dealer.uuid}
            today={today}
            day={day}
            slot={slot}
            onDay={(d) => {
              setDay(d);
              setSlot(null);
              setError(null);
            }}
            onSlot={(s) => {
              setSlot(s);
              setError(null);
            }}
          />
        ) : (
          <p className="text-muted-foreground text-sm">
            {t("portal.appointments.pick_dealer_first")}
          </p>
        )}
      </Step>

      <Step n={4} title={t("portal.appointments.step_confirm")}>
        <div className="space-y-2">
          <Label htmlFor="booking-note">{t("portal.appointments.note")}</Label>
          <Textarea
            id="booking-note"
            value={note}
            maxLength={2000}
            onChange={(e) => setNote(e.target.value)}
            placeholder={t("portal.appointments.note_hint")}
          />
        </div>
        {error ? (
          <Alert variant="destructive" data-testid="booking-error">
            <AlertDescription>{t(error)}</AlertDescription>
          </Alert>
        ) : null}
        <div className="flex justify-end">
          <Button
            type="button"
            disabled={!ready || create.isPending}
            onClick={() => create.mutate()}
            data-testid="booking-submit"
          >
            <CalendarPlus className="size-4" />
            {create.isPending
              ? t("portal.appointments.booking")
              : t("portal.appointments.confirm")}
          </Button>
        </div>
      </Step>
    </PortalPage>
  );
}

function Step({
  n,
  title,
  children,
}: {
  n: number;
  title: string;
  children: ReactNode;
}) {
  return (
    <Card data-testid={`booking-step-${n}`}>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <span className="bg-primary text-primary-foreground inline-flex size-6 items-center justify-center rounded-full text-xs">
            {n}
          </span>
          {title}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">{children}</CardContent>
    </Card>
  );
}

/**
 * Nearby dealers that take portal bookings (`accepts_appointments`), around
 * the browser position or, when it is refused, the whole country.
 */
function DealerPicker({ onPick }: { onPick: (d: BookingDealer) => void }) {
  const { t, locale } = useLocale();
  const [center, setCenter] = useState<LatLng | null>(null);
  const [radius, setRadius] = useState<RadiusKm>(LOCATED_RADIUS_KM);
  const [result, setResult] = useState<{
    key: string;
    value: NearbyResult;
  } | null>(null);
  const [retry, setRetry] = useState(0);

  const applyPosition = (pos: PositionResult) => {
    setCenter(pos.center);
    setRadius(pos.kind === "located" ? LOCATED_RADIUS_KM : FALLBACK_RADIUS_KM);
  };
  useEffect(() => {
    let active = true;
    void locateUser(browserGeolocation()).then((pos) => {
      if (active) applyPosition(pos);
    });
    return () => {
      active = false;
    };
  }, []);
  const relocate = () => {
    setCenter(null);
    void locateUser(browserGeolocation()).then(applyPosition);
  };

  const key = center ? `${center.lat},${center.lng},${radius}#${retry}` : null;
  useEffect(() => {
    if (!center || !key) return;
    const controller = new AbortController();
    void fetchNearbyDealers(
      { lat: center.lat, lng: center.lng, radiusKm: radius },
      fetch,
      controller.signal,
    ).then((value) => {
      if (!controller.signal.aborted) setResult({ key, value });
    });
    return () => controller.abort();
  }, [center, radius, key]);

  const loading = !key || result?.key !== key;
  const dealers =
    !loading && result?.value.kind === "ok"
      ? result.value.dealers.filter((d) => d.accepts_appointments && d.uuid)
      : [];

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-end gap-3">
        <div className="flex min-w-40 flex-col gap-1">
          <span id="booking-radius-label" className="text-sm font-medium">
            {t("portal.dealers.radius_label")}
          </span>
          <Select
            value={String(radius)}
            onValueChange={(v) => setRadius(Number(v) as RadiusKm)}
          >
            <SelectTrigger aria-labelledby="booking-radius-label">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {RADIUS_OPTIONS.map((km) => (
                <SelectItem key={km} value={String(km)}>
                  {t("portal.dealers.radius_option", { km })}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={relocate}
          disabled={!center}
        >
          <LocateFixed className="size-4" aria-hidden />
          {t("portal.dealers.use_my_location")}
        </Button>
      </div>
      {loading ? (
        <p className="text-muted-foreground text-sm" aria-live="polite">
          {t("portal.dealers.loading")}
        </p>
      ) : result?.value.kind !== "ok" ? (
        <Alert variant="destructive">
          <AlertDescription className="flex flex-wrap items-center gap-3">
            {result?.value.kind === "rate_limited"
              ? t("portal.dealers.rate_limited")
              : t("portal.dealers.error")}
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => setRetry((n) => n + 1)}
            >
              {t("portal.dealers.retry")}
            </Button>
          </AlertDescription>
        </Alert>
      ) : dealers.length === 0 ? (
        <p
          className="text-muted-foreground text-sm"
          data-testid="booking-dealers-empty"
        >
          {t("portal.appointments.no_dealers")}
        </p>
      ) : (
        <ul className="space-y-2" data-testid="booking-dealers">
          {dealers.map((d) => {
            const place = [d.district, d.city].filter(Boolean).join(", ");
            return (
              <li
                key={d.uuid}
                className="flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3"
                data-testid="booking-dealer"
              >
                <div className="min-w-0">
                  <p className="truncate font-medium">{d.name}</p>
                  <p className="text-muted-foreground text-xs">
                    {place ? `${place} · ` : ""}
                    {formatDistanceKm(d.distance_km, locale)}
                  </p>
                </div>
                <Button
                  type="button"
                  size="sm"
                  onClick={() => onPick({ uuid: d.uuid, name: d.name })}
                >
                  {t("portal.appointments.select")}
                </Button>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}

function VehiclePicker({
  value,
  onChange,
}: {
  value: string | null;
  onChange: (uuid: string) => void;
}) {
  const { t } = useLocale();
  const list = useQuery({
    queryKey: ["portal", "vehicles", "picker"],
    queryFn: () => portalApi.listVehicles(VEHICLE_PICKER_LIMIT, 0),
  });
  const vehicles = useMemo(() => list.data?.items ?? [], [list.data]);

  // One vehicle: nothing to choose.
  useEffect(() => {
    if (!value && vehicles.length === 1) onChange(vehicles[0].uuid);
  }, [value, vehicles, onChange]);

  if (list.isError) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void list.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  if (list.isLoading) {
    return (
      <p className="text-muted-foreground text-sm">
        {t("portal.vehicles.loading")}
      </p>
    );
  }
  if (vehicles.length === 0) {
    return (
      <p
        className="text-muted-foreground text-sm"
        data-testid="booking-no-vehicles"
      >
        {t("portal.appointments.no_vehicles")}
      </p>
    );
  }
  return (
    <ul
      className="grid gap-2 sm:grid-cols-2"
      role="radiogroup"
      aria-label={t("portal.appointments.step_vehicle")}
      data-testid="booking-vehicles"
    >
      {vehicles.map((v) => {
        const selected = v.uuid === value;
        return (
          <li key={v.uuid}>
            <button
              type="button"
              role="radio"
              aria-checked={selected}
              onClick={() => onChange(v.uuid)}
              data-testid="booking-vehicle"
              data-uuid={v.uuid}
              className={cn(
                "hover:border-primary/50 flex w-full items-center gap-2 rounded-lg border p-3 text-start transition-colors",
                selected && "border-primary bg-primary/5",
              )}
            >
              <Car className="text-muted-foreground size-4 shrink-0" />
              <span className="min-w-0">
                <span className="block truncate font-medium">
                  {portalVehicleTitle(v) ||
                    t("portal.vehicles.unknown_vehicle")}
                </span>
                {v.plate ? (
                  <span
                    className="text-muted-foreground block font-mono text-xs tracking-wider"
                    dir="ltr"
                  >
                    {v.plate}
                  </span>
                ) : null}
              </span>
            </button>
          </li>
        );
      })}
    </ul>
  );
}

const DAY_NOTE_KEYS: Record<string, string> = {
  closed: "portal.appointments.day_closed",
  full: "portal.appointments.day_full",
  none: "portal.appointments.day_none",
};

/**
 * Two weeks of the dealer's days at a time; a closed, full or slotless day
 * cannot be picked, so its start times are never offered.
 */
function SlotPicker({
  dealerUuid,
  today,
  day,
  slot,
  onDay,
  onSlot,
}: {
  dealerUuid: string;
  today?: Date;
  day: string | null;
  slot: AppointmentSlot | null;
  onDay: (day: string | null) => void;
  onSlot: (slot: AppointmentSlot) => void;
}) {
  const { t, format } = useLocale();
  const [first] = useState(() => isoDate(today ?? new Date()));
  const last = addDaysIso(first, BOOKING_MAX_AHEAD_DAYS);
  const [from, setFrom] = useState(first);
  const to = addDaysIso(from, BOOKING_WINDOW_DAYS - 1);

  const availability = useQuery({
    queryKey: ["portal", "appointments", "availability", dealerUuid, from, to],
    queryFn: () => portalApi.dealerAvailability(dealerUuid, from, to),
  });
  const days = useMemo(() => availability.data ?? [], [availability.data]);
  const selectedDay: AppointmentAvailabilityDay | undefined = days.find(
    (d) => d.date === day,
  );

  // A refetch can fill the chosen day: drop the choice then.
  useEffect(() => {
    if (day && selectedDay && !isDayBookable(selectedDay)) onDay(null);
  }, [day, selectedDay, onDay]);

  const move = (delta: number) => {
    const next = addDaysIso(from, delta);
    setFrom(next < first ? first : next);
    onDay(null);
  };

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between gap-2">
        <Button
          type="button"
          variant="outline"
          size="icon"
          disabled={from <= first}
          onClick={() => move(-BOOKING_WINDOW_DAYS)}
          aria-label={t("portal.appointments.prev_days")}
        >
          <ChevronLeft className="size-4 rtl:rotate-180" />
        </Button>
        <p className="text-muted-foreground text-sm">
          {format.date(dayInstant(from))} – {format.date(dayInstant(to))}
        </p>
        <Button
          type="button"
          variant="outline"
          size="icon"
          disabled={addDaysIso(from, BOOKING_WINDOW_DAYS) > last}
          onClick={() => move(BOOKING_WINDOW_DAYS)}
          aria-label={t("portal.appointments.next_days")}
        >
          <ChevronRight className="size-4 rtl:rotate-180" />
        </Button>
      </div>

      {availability.isError ? (
        <ErrorState
          title={t("portal.appointments.availability_error")}
          onRetry={() => void availability.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : availability.isLoading ? (
        <p className="text-muted-foreground text-sm">
          {t("portal.appointments.loading_days")}
        </p>
      ) : (
        <ul
          className="grid grid-cols-3 gap-2 sm:grid-cols-7"
          data-testid="booking-days"
        >
          {days.map((d) => {
            const state = dayState(d);
            const bookable = state === "bookable";
            const selected = d.date === day;
            return (
              <li key={d.date}>
                <button
                  type="button"
                  disabled={!bookable}
                  aria-pressed={selected}
                  onClick={() => onDay(d.date)}
                  data-testid="booking-day"
                  data-date={d.date}
                  data-state={state}
                  className={cn(
                    "flex w-full flex-col items-center rounded-lg border p-2 text-center text-sm transition-colors",
                    bookable
                      ? "hover:border-primary/50"
                      : "bg-muted/40 text-muted-foreground cursor-not-allowed",
                    selected && "border-primary bg-primary/5",
                  )}
                >
                  <span className="text-xs">
                    {format.dateParts(dayInstant(d.date), {
                      weekday: "short",
                    })}
                  </span>
                  <span className="font-medium">
                    {format.dateParts(dayInstant(d.date), {
                      day: "numeric",
                      month: "short",
                    })}
                  </span>
                  <span className="text-[11px]">
                    {bookable
                      ? t("portal.appointments.day_free", {
                          count: d.remaining_capacity,
                        })
                      : t(DAY_NOTE_KEYS[state])}
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      )}

      {selectedDay && isDayBookable(selectedDay) ? (
        <div className="space-y-2">
          <p className="text-sm font-medium">
            {t("portal.appointments.pick_time")}
          </p>
          <ul className="flex flex-wrap gap-2" data-testid="booking-slots">
            {selectedDay.slots.map((s) => (
              <li key={s.start}>
                <Button
                  type="button"
                  size="sm"
                  variant={slot?.start === s.start ? "default" : "outline"}
                  aria-pressed={slot?.start === s.start}
                  onClick={() => onSlot(s)}
                  data-testid="booking-slot"
                  data-start={s.start}
                >
                  <span dir="ltr">{format.time(s.start)}</span>
                </Button>
              </li>
            ))}
          </ul>
        </div>
      ) : (
        <p className="text-muted-foreground text-sm">
          {t("portal.appointments.pick_day")}
        </p>
      )}
    </div>
  );
}
