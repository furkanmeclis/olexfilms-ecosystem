"use client";

import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  useDraggable,
  useDroppable,
  useSensor,
  useSensors,
  type DragEndEvent,
} from "@dnd-kit/core";
import { CSS } from "@dnd-kit/utilities";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, Plus } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { AppointmentDialog } from "@/features/appointments/components/appointment-dialog";
import {
  bookingErrorKey,
  canReschedule,
  parseSlotId,
  placeAppointments,
  slotId,
  STATUS_TONE,
  visibleSpan,
  type PlacedAppointment,
} from "@/features/appointments/lib/calendar";
import {
  addDays,
  daysRange,
  minutesLabel,
  todayIn,
  weekDays,
  zonedToUtc,
} from "@/features/appointments/lib/time";
import {
  appointmentKeys,
  appointmentsService,
  rescheduleBody,
  type Appointment,
  type AppointmentAvailabilityDay,
} from "@/features/appointments/services/appointments.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export type CalendarView = "day" | "week";

/** Grid step of the drop slots, minutes. */
const STEP = 30;
/** Pixel height of one STEP row. */
const ROW = 36;

const px = (minutes: number) => (minutes / STEP) * ROW;

function AppointmentCard({
  placed,
  timeZone,
  draggable,
  onOpen,
}: {
  placed: PlacedAppointment & { spanStart: number };
  timeZone: string;
  draggable: boolean;
  onOpen: (a: Appointment) => void;
}) {
  const { t, locale } = useLocale();
  const a = placed.appointment;
  const { attributes, listeners, setNodeRef, transform, isDragging } =
    useDraggable({ id: a.uuid, disabled: !draggable });
  const time = new Intl.DateTimeFormat(locale, {
    timeZone,
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(a.starts_at));
  const width = 100 / placed.lanes;
  return (
    <button
      ref={setNodeRef}
      type="button"
      data-testid={`appointment-${a.uuid}`}
      data-status={a.status}
      className={cn(
        "absolute z-10 overflow-hidden rounded-md border border-s-4 px-1.5 py-0.5 text-start text-xs shadow-sm",
        "focus-visible:ring-ring focus-visible:ring-2 focus-visible:outline-none",
        STATUS_TONE[a.status],
        a.status === "cancelled" && "line-through",
        draggable ? "cursor-grab" : "cursor-pointer",
        isDragging && "z-20 opacity-80 shadow-lg",
      )}
      style={{
        top: px(placed.start - placed.spanStart),
        height: Math.max(px(placed.end - placed.start) - 2, 20),
        insetInlineStart: `${placed.lane * width}%`,
        width: `calc(${width}% - 2px)`,
        transform: CSS.Translate.toString(transform),
      }}
      onClick={() => onOpen(a)}
      {...attributes}
      {...listeners}
    >
      <span className="block truncate font-medium">
        <span dir="ltr">{time}</span>{" "}
        {a.customer_name || t("appointments.card.customer_unknown")}
      </span>
      {a.vehicle_plate || a.vehicle_label ? (
        <span className="block truncate opacity-80">
          {a.vehicle_plate ? (
            <span dir="ltr" className="font-mono">
              {a.vehicle_plate}
            </span>
          ) : null}
          {a.vehicle_label ? ` ${a.vehicle_label}` : null}
        </span>
      ) : null}
      <span className="sr-only">{t(`appointments.status.${a.status}`)}</span>
    </button>
  );
}

function DropSlot({
  date,
  minutes,
  top,
  disabled,
  onPick,
}: {
  date: string;
  minutes: number;
  top: number;
  disabled: boolean;
  onPick?: () => void;
}) {
  const { t } = useLocale();
  const { setNodeRef, isOver } = useDroppable({
    id: slotId(date, minutes),
    disabled,
  });
  return (
    <div
      ref={setNodeRef}
      data-slot={slotId(date, minutes)}
      className={cn(
        "absolute inset-x-0 border-t border-dashed",
        minutes % 60 === 0 && "border-solid",
        isOver && "bg-primary/10",
        onPick && "hover:bg-muted/60 cursor-pointer",
      )}
      style={{ top, height: ROW }}
      onClick={onPick}
      role={onPick ? "button" : undefined}
      aria-label={
        onPick
          ? t("appointments.calendar.new_at", {
              time: minutesLabel(minutes),
            })
          : undefined
      }
    />
  );
}

type DialogState =
  | { mode: "edit"; appointment: Appointment }
  | { mode: "create"; date: string; minutes: number }
  | null;

/**
 * Day / week calendar of the active organization (TEC-326). Renders in the
 * organization's time zone (availability `timezone`, else the session's
 * effective zone). Cards move by drag and drop (PATCH, optimistic): a 422
 * (capacity full, closed day) puts the card back and shows a toast.
 */
export function AppointmentCalendar({
  slug,
  canWrite,
  initialView = "week",
  initialDate,
}: {
  slug: string;
  canWrite: boolean;
  initialView?: CalendarView;
  initialDate?: string;
}) {
  const { t, locale, format } = useLocale();
  const qc = useQueryClient();
  const [view, setView] = useState<CalendarView>(initialView);
  const [anchor, setAnchor] = useState(
    () => initialDate ?? todayIn(format.timeZone),
  );
  const [dialog, setDialog] = useState<DialogState>(null);

  const days = useMemo(
    () => (view === "week" ? weekDays(anchor) : [anchor]),
    [view, anchor],
  );
  const firstDay = days[0] ?? anchor;
  const lastDay = days[days.length - 1] ?? anchor;

  const availability = useQuery({
    queryKey: appointmentKeys.availability(firstDay, lastDay),
    queryFn: () => appointmentsService.availability(firstDay, lastDay),
  });
  const timeZone = availability.data?.[0]?.timezone || format.timeZone;
  const byDay = useMemo(() => {
    const m = new Map<string, AppointmentAvailabilityDay>();
    for (const d of availability.data ?? []) m.set(d.date, d);
    return m;
  }, [availability.data]);

  const range = useMemo(() => daysRange(days, timeZone), [days, timeZone]);
  const listKey = appointmentKeys.range(range.from, range.to);
  const list = useQuery({
    queryKey: listKey,
    queryFn: () => appointmentsService.listRange(range.from, range.to),
  });

  const placed = useMemo(
    () => placeAppointments(list.data ?? [], days, timeZone),
    [list.data, days, timeZone],
  );
  const span = visibleSpan(placed);
  const rows: number[] = [];
  for (let m = span.start; m < span.end; m += STEP) rows.push(m);

  const reschedule = useMutation({
    mutationFn: (v: { appointment: Appointment; startsAt: string }) =>
      appointmentsService.patch(
        v.appointment.uuid,
        rescheduleBody(v.appointment, v.startsAt),
      ),
    onMutate: async (v) => {
      await qc.cancelQueries({ queryKey: listKey });
      const previous = qc.getQueryData<Appointment[]>(listKey);
      const ms =
        new Date(v.appointment.ends_at).getTime() -
        new Date(v.appointment.starts_at).getTime();
      qc.setQueryData<Appointment[]>(listKey, (old) =>
        (old ?? []).map((a) =>
          a.uuid === v.appointment.uuid
            ? {
                ...a,
                starts_at: v.startsAt,
                ends_at: new Date(new Date(v.startsAt).getTime() + ms)
                  .toISOString()
                  .replace(".000Z", "Z"),
              }
            : a,
        ),
      );
      return { previous };
    },
    onError: (error, _v, ctx) => {
      qc.setQueryData(listKey, ctx?.previous);
      appToast.error(t(bookingErrorKey(error)));
    },
    onSuccess: (saved) => {
      qc.setQueryData<Appointment[]>(listKey, (old) =>
        (old ?? []).map((a) => (a.uuid === saved.uuid ? saved : a)),
      );
      appToast.success(t("appointments.calendar.moved"));
    },
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: appointmentKeys.all });
    },
  });

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 6 } }),
    useSensor(KeyboardSensor),
  );

  const onDragEnd = (event: DragEndEvent) => {
    const target = event.over ? parseSlotId(event.over.id) : null;
    const appointment = (list.data ?? []).find(
      (a) => a.uuid === String(event.active.id),
    );
    if (!target || !appointment || !canReschedule(appointment)) return;
    const startsAt = zonedToUtc(target.date, target.minutes, timeZone);
    if (
      new Date(startsAt).getTime() === new Date(appointment.starts_at).getTime()
    )
      return;
    reschedule.mutate({ appointment, startsAt });
  };

  const step = view === "week" ? 7 : 1;
  const dayHeader = (date: string) => {
    const at = new Date(`${date}T12:00:00Z`);
    const f = (o: Intl.DateTimeFormatOptions) =>
      new Intl.DateTimeFormat(locale, { timeZone: "UTC", ...o }).format(at);
    return {
      weekday: f({ weekday: "short" }),
      day: f({ day: "numeric", month: "short" }),
    };
  };
  const title =
    view === "week"
      ? `${format.date(`${firstDay}T12:00:00Z`)} – ${format.date(`${lastDay}T12:00:00Z`)}`
      : new Intl.DateTimeFormat(locale, {
          timeZone: "UTC",
          weekday: "long",
          day: "numeric",
          month: "long",
          year: "numeric",
        }).format(new Date(`${anchor}T12:00:00Z`));
  const today = todayIn(timeZone);

  return (
    <div className="space-y-4" data-testid="appointment-calendar">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label={t("appointments.calendar.previous")}
            onClick={() => setAnchor(addDays(anchor, -step))}
          >
            <ChevronLeft className="size-4 rtl:-scale-x-100" />
          </Button>
          <Button
            type="button"
            variant="outline"
            onClick={() => setAnchor(todayIn(timeZone))}
          >
            {t("appointments.calendar.today")}
          </Button>
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label={t("appointments.calendar.next")}
            onClick={() => setAnchor(addDays(anchor, step))}
          >
            <ChevronRight className="size-4 rtl:-scale-x-100" />
          </Button>
          <h2
            className="ms-2 text-lg font-semibold"
            data-testid="calendar-title"
          >
            {title}
          </h2>
        </div>
        <div className="flex items-center gap-2">
          <span className="text-muted-foreground text-xs" dir="ltr">
            {timeZone}
          </span>
          <ToggleGroup
            type="single"
            variant="outline"
            value={view}
            onValueChange={(v) => {
              if (v === "day" || v === "week") setView(v);
            }}
          >
            <ToggleGroupItem value="day">
              {t("appointments.calendar.day")}
            </ToggleGroupItem>
            <ToggleGroupItem value="week">
              {t("appointments.calendar.week")}
            </ToggleGroupItem>
          </ToggleGroup>
          {canWrite ? (
            <Button
              type="button"
              onClick={() =>
                setDialog({ mode: "create", date: anchor, minutes: 9 * 60 })
              }
            >
              <Plus className="size-4" />
              {t("appointments.new")}
            </Button>
          ) : null}
        </div>
      </div>

      {list.isError ? (
        <ErrorState
          title={t("appointments.errors.load_title")}
          description={t("appointments.errors.load_description")}
          onRetry={() => void list.refetch()}
        />
      ) : list.isLoading ? (
        <Skeleton className="h-96 w-full" />
      ) : (
        <DndContext sensors={sensors} onDragEnd={onDragEnd}>
          <div className="overflow-x-auto rounded-lg border">
            <div
              className="grid min-w-full"
              style={{
                gridTemplateColumns: `4rem repeat(${days.length}, minmax(${view === "week" ? "8rem" : "12rem"}, 1fr))`,
              }}
            >
              <div className="bg-muted/40 sticky top-0 border-b" />
              {days.map((date) => {
                const h = dayHeader(date);
                const av = byDay.get(date);
                return (
                  <div
                    key={date}
                    data-day-header={date}
                    className={cn(
                      "bg-muted/40 border-s border-b px-2 py-1.5 text-center",
                      date === today && "bg-primary/10",
                    )}
                  >
                    <div className="text-muted-foreground text-xs uppercase">
                      {h.weekday}
                    </div>
                    <div className="text-sm font-medium">{h.day}</div>
                    {av ? (
                      av.closed ? (
                        <Badge variant="outline" className="mt-1">
                          {t("appointments.calendar.closed")}
                        </Badge>
                      ) : (
                        <Badge
                          variant={
                            av.remaining_capacity > 0 ? "secondary" : "danger"
                          }
                          className="mt-1"
                        >
                          {t("appointments.calendar.capacity", {
                            occupied: av.occupied,
                            capacity: av.capacity,
                          })}
                        </Badge>
                      )
                    ) : null}
                  </div>
                );
              })}
              <div className="relative" style={{ height: rows.length * ROW }}>
                {rows
                  .filter((m) => m % 60 === 0)
                  .map((m) => (
                    <span
                      key={m}
                      dir="ltr"
                      className="text-muted-foreground absolute end-1 -translate-y-1/2 text-[11px]"
                      style={{ top: px(m - span.start) }}
                    >
                      {m === span.start ? "" : minutesLabel(m)}
                    </span>
                  ))}
              </div>
              {days.map((date) => {
                const closed = byDay.get(date)?.closed ?? false;
                return (
                  <div
                    key={date}
                    data-day={date}
                    className={cn("relative border-s", closed && "bg-muted/50")}
                    style={{ height: rows.length * ROW }}
                  >
                    {rows.map((m) => (
                      <DropSlot
                        key={m}
                        date={date}
                        minutes={m}
                        top={px(m - span.start)}
                        disabled={!canWrite || closed}
                        onPick={
                          canWrite && !closed
                            ? () =>
                                setDialog({ mode: "create", date, minutes: m })
                            : undefined
                        }
                      />
                    ))}
                    {(placed[date] ?? []).map((p) => (
                      <AppointmentCard
                        key={p.appointment.uuid}
                        placed={{ ...p, spanStart: span.start }}
                        timeZone={timeZone}
                        draggable={canWrite && canReschedule(p.appointment)}
                        onOpen={(a) =>
                          setDialog({ mode: "edit", appointment: a })
                        }
                      />
                    ))}
                  </div>
                );
              })}
            </div>
          </div>
        </DndContext>
      )}
      {list.data && list.data.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("appointments.calendar.empty")}
        </p>
      ) : null}

      {dialog ? (
        <AppointmentDialog
          key={dialog.mode === "edit" ? dialog.appointment.uuid : "new"}
          slug={slug}
          timeZone={timeZone}
          canWrite={canWrite}
          appointment={dialog.mode === "edit" ? dialog.appointment : null}
          initialDate={dialog.mode === "create" ? dialog.date : undefined}
          initialMinutes={dialog.mode === "create" ? dialog.minutes : undefined}
          onClose={() => setDialog(null)}
        />
      ) : null}
    </div>
  );
}
