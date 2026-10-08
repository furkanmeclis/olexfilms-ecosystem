"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  CalendarCheck,
  Loader2,
  RefreshCw,
  Search,
} from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo, useRef, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { vehicleLabel } from "@/features/fleets/components/fleet-vehicles-tab";
import {
  dayInZone,
  moveToDay,
  parsePreferredTimes,
  planWarningCount,
  rowWarnings,
  unscheduledVehicles,
} from "@/features/fleets/lib/plan";
import {
  fleetKeys,
  fleetsService,
  isStalePlanError,
  type FleetPlanAppointment,
  type FleetPlanPreview,
  type FleetPlanRequest,
  type FleetVehicle,
} from "@/features/fleets/services/fleets.service";
import { useDebounce } from "@/hooks/use-debounce";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

type Step = "vehicles" | "settings" | "preview";
const STEPS: Step[] = ["vehicles", "settings", "preview"];

function newKey() {
  return typeof crypto !== "undefined" && "randomUUID" in crypto
    ? crypto.randomUUID()
    : `${Date.now()}-${Math.random()}`;
}

function tomorrow() {
  const d = new Date();
  d.setDate(d.getDate() + 1);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

function StepIndicator({ step }: { step: Step }) {
  const { t } = useLocale();
  const current = STEPS.indexOf(step);
  return (
    <ol className="flex flex-wrap gap-2 text-sm" data-testid="plan-steps">
      {STEPS.map((s, i) => (
        <li
          key={s}
          className={cn(
            "rounded-full border px-3 py-1",
            i === current
              ? "border-primary bg-primary text-primary-foreground"
              : i < current
                ? "border-primary text-primary"
                : "text-muted-foreground",
          )}
        >
          {i + 1}. {t(`fleets.plan.steps.${s}`)}
        </li>
      ))}
    </ol>
  );
}

/**
 * Bulk service plan wizard (TEC-477; the wizard is a DataTable exception):
 * vehicles → service, start day, daily maximum and preferred hours →
 * preview grouped by day (server warnings, daily capacity warnings, the day
 * of a row can be changed) → confirm. A 409 FLEET_SERVICE_PLAN_STALE
 * (capacity changed after the preview) writes nothing; the wizard offers
 * to preview again.
 */
export function FleetPlanWizard({
  slug,
  fleetUuid,
  initialVehicleUuids = [],
}: {
  slug: string;
  fleetUuid: string;
  initialVehicleUuids?: string[];
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const timeZone = format.timeZone;

  const [step, setStep] = useState<Step>(
    initialVehicleUuids.length ? "settings" : "vehicles",
  );
  const [selected, setSelected] = useState<string[]>(initialVehicleUuids);
  const [search, setSearch] = useState("");
  const q = useDebounce(search, 300);
  // Every vehicle the picker has listed (plates of the preview rows).
  const [known, setKnown] = useState(() => new Map<string, FleetVehicle>());

  const [serviceType, setServiceType] = useState("");
  const [startDate, setStartDate] = useState(tomorrow);
  const [dailyMax, setDailyMax] = useState("");
  const [times, setTimes] = useState("");
  const [note, setNote] = useState("");
  const [settingsError, setSettingsError] = useState<string | null>(null);

  const [preview, setPreview] = useState<FleetPlanPreview | null>(null);
  const [rows, setRows] = useState<FleetPlanAppointment[]>([]);
  const [stale, setStale] = useState(false);
  const idempotencyKey = useRef(newKey());

  const card = useQuery({
    queryKey: fleetKeys.card(fleetUuid),
    queryFn: () => fleetsService.card(fleetUuid),
  });
  const vehicles = useQuery({
    queryKey: fleetKeys.vehiclePicker(fleetUuid, q),
    queryFn: async () => {
      const page = await fleetsService.listVehicles(fleetUuid, {
        limit: 100,
        offset: 0,
        sort: "plate",
        q: q || undefined,
      });
      setKnown((prev) => {
        const next = new Map(prev);
        for (const v of page.items) next.set(v.uuid, v);
        return next;
      });
      return page;
    },
  });

  const dailyMaxNumber = dailyMax ? Number(dailyMax) : undefined;
  const request = (): FleetPlanRequest | null => {
    const preferred = parsePreferredTimes(times);
    if (!serviceType.trim()) {
      setSettingsError(t("fleets.plan.service_type_required"));
      return null;
    }
    if (preferred === null) {
      setSettingsError(t("fleets.plan.times_invalid"));
      return null;
    }
    if (
      dailyMax &&
      (!Number.isInteger(dailyMaxNumber) || dailyMaxNumber! < 1)
    ) {
      setSettingsError(t("fleets.plan.daily_max_invalid"));
      return null;
    }
    setSettingsError(null);
    return {
      vehicle_uuids: selected,
      service_type: serviceType.trim(),
      start_date: startDate,
      ...(note.trim() ? { note: note.trim() } : {}),
      ...(dailyMaxNumber ? { daily_max_vehicles: dailyMaxNumber } : {}),
      ...(preferred.length ? { preferred_times: preferred } : {}),
    };
  };

  const previewMutation = useMutation({
    mutationFn: (body: FleetPlanRequest) =>
      fleetsService.previewPlan(fleetUuid, body),
    onSuccess: (out) => {
      setPreview(out);
      setRows(out.appointments);
      setStale(false);
      idempotencyKey.current = newKey();
      setStep("preview");
    },
  });

  const create = useMutation({
    mutationFn: (body: FleetPlanRequest) =>
      fleetsService.createPlan(fleetUuid, body, idempotencyKey.current),
    onSuccess: (plan) => {
      appToast.success(t("fleets.plan.created"));
      router.push(routes.tenant.fleets.plan(slug, fleetUuid, plan.uuid));
    },
    onError: (error) => {
      if (isStalePlanError(error)) setStale(true);
    },
  });

  const runPreview = () => {
    const body = request();
    if (body) previewMutation.mutate(body);
  };

  const confirm = () => {
    const body = request();
    if (body) create.mutate({ ...body, appointments: rows });
  };

  const warnings = useMemo(() => preview?.warnings ?? [], [preview]);
  const warningCount = useMemo(
    () => planWarningCount(rows, warnings, dailyMaxNumber, timeZone),
    [dailyMaxNumber, rows, timeZone, warnings],
  );
  const missing = useMemo(
    () => unscheduledVehicles(selected, rows),
    [rows, selected],
  );
  const days = useMemo(() => {
    const byDay = new Map<string, FleetPlanAppointment[]>();
    for (const row of [...rows].sort((a, b) =>
      a.starts_at.localeCompare(b.starts_at),
    )) {
      const day = dayInZone(row.starts_at, timeZone);
      byDay.set(day, [...(byDay.get(day) ?? []), row]);
    }
    return [...byDay.entries()];
  }, [rows, timeZone]);

  const plate = (uuid: string) => {
    const v = known.get(uuid);
    return v?.plate ?? uuid.slice(0, 8);
  };

  const fleetName = card.data?.name ?? "";
  const title = t("fleets.plan.wizard_title");
  const header = (
    <PageHeader
      title={title}
      description={fleetName}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: t("fleets.title"), href: routes.tenant.fleets.list(slug) },
        {
          label: fleetName || t("fleets.card.title"),
          href: routes.tenant.fleets.detail(slug, fleetUuid, "plans"),
        },
        { label: title },
      ]}
    />
  );

  if (!can(permissions.fleets.plan)) {
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

  const apiError = (error: unknown) =>
    isApiError(error) ? error.message : t("common.error_generic");

  return (
    <div className="space-y-6">
      {header}
      <StepIndicator step={step} />

      {step === "vehicles" ? (
        <Card data-testid="plan-step-vehicles">
          <CardHeader>
            <CardTitle>{t("fleets.plan.steps.vehicles")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="relative max-w-sm">
              <Search className="text-muted-foreground absolute start-2.5 top-2.5 size-4" />
              <Input
                className="ps-8"
                value={search}
                placeholder={t("fleets.plan.search_vehicles")}
                onChange={(e) => setSearch(e.target.value)}
              />
            </div>
            <div className="flex items-center gap-2 text-sm">
              <Checkbox
                id="plan-select-all"
                checked={
                  (vehicles.data?.items.length ?? 0) > 0 &&
                  vehicles.data!.items.every((v) => selected.includes(v.uuid))
                }
                onCheckedChange={(checked) => {
                  const page = vehicles.data?.items.map((v) => v.uuid) ?? [];
                  setSelected((prev) =>
                    checked
                      ? [...new Set([...prev, ...page])]
                      : prev.filter((id) => !page.includes(id)),
                  );
                }}
              />
              <Label htmlFor="plan-select-all">
                {t("fleets.plan.select_all")}
              </Label>
              <Badge variant="secondary" data-testid="plan-selected-count">
                {t("fleets.plan.selected", { count: selected.length })}
              </Badge>
            </div>
            <ul className="grid max-h-[50vh] gap-1 overflow-y-auto sm:grid-cols-2 lg:grid-cols-3">
              {(vehicles.data?.items ?? []).map((v) => (
                <li key={v.uuid}>
                  <label className="hover:bg-muted flex cursor-pointer items-center gap-2 rounded-md border p-2">
                    <Checkbox
                      checked={selected.includes(v.uuid)}
                      onCheckedChange={(checked) =>
                        setSelected((prev) =>
                          checked
                            ? [...prev, v.uuid]
                            : prev.filter((id) => id !== v.uuid),
                        )
                      }
                    />
                    <span className="font-mono text-sm" dir="ltr">
                      {v.plate ?? "—"}
                    </span>
                    <span className="text-muted-foreground truncate text-xs">
                      {vehicleLabel(v)}
                    </span>
                  </label>
                </li>
              ))}
            </ul>
            {vehicles.data && vehicles.data.items.length === 0 ? (
              <p className="text-muted-foreground text-sm">
                {t("fleets.vehicles.empty_title")}
              </p>
            ) : null}
            <div className="flex justify-end">
              <Button
                type="button"
                disabled={selected.length === 0}
                data-testid="plan-next-settings"
                onClick={() => setStep("settings")}
              >
                {t("fleets.plan.next")}
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {step === "settings" ? (
        <Card data-testid="plan-step-settings">
          <CardHeader>
            <CardTitle>{t("fleets.plan.steps.settings")}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-4">
            <p className="text-muted-foreground text-sm">
              {t("fleets.plan.selected", { count: selected.length })}
            </p>
            <div className="grid gap-4 sm:grid-cols-2">
              <div className="space-y-1.5">
                <Label htmlFor="plan-service-type">
                  {t("fleets.plan.service_type")}
                </Label>
                <Input
                  id="plan-service-type"
                  maxLength={120}
                  value={serviceType}
                  data-testid="plan-service-type"
                  onChange={(e) => setServiceType(e.target.value)}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="plan-start-date">
                  {t("fleets.plan.start_date")}
                </Label>
                <DatePicker
                  id="plan-start-date"
                  value={startDate}
                  onChange={setStartDate}
                />
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="plan-daily-max">
                  {t("fleets.plan.daily_max")}
                </Label>
                <Input
                  id="plan-daily-max"
                  type="number"
                  min={1}
                  value={dailyMax}
                  data-testid="plan-daily-max"
                  onChange={(e) => setDailyMax(e.target.value)}
                />
                <p className="text-muted-foreground text-xs">
                  {t("fleets.plan.daily_max_hint")}
                </p>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="plan-times">
                  {t("fleets.plan.preferred_times")}
                </Label>
                <Input
                  id="plan-times"
                  dir="ltr"
                  placeholder="09:00, 14:00"
                  value={times}
                  data-testid="plan-times"
                  onChange={(e) => setTimes(e.target.value)}
                />
                <p className="text-muted-foreground text-xs">
                  {t("fleets.plan.preferred_times_hint")}
                </p>
              </div>
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="plan-note">{t("fleets.plan.note")}</Label>
              <Textarea
                id="plan-note"
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </div>
            {settingsError ? (
              <p className="text-destructive text-sm" role="alert">
                {settingsError}
              </p>
            ) : null}
            {previewMutation.isError ? (
              <p className="text-destructive text-sm" role="alert">
                {apiError(previewMutation.error)}
              </p>
            ) : null}
            <div className="flex justify-between gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => setStep("vehicles")}
              >
                {t("common.back")}
              </Button>
              <Button
                type="button"
                disabled={previewMutation.isPending || selected.length === 0}
                data-testid="plan-preview"
                onClick={runPreview}
              >
                {previewMutation.isPending ? (
                  <Loader2 className="size-4 animate-spin" />
                ) : null}
                {t("fleets.plan.preview")}
              </Button>
            </div>
          </CardContent>
        </Card>
      ) : null}

      {step === "preview" && preview ? (
        <div className="space-y-4" data-testid="plan-step-preview">
          {stale ? (
            <Alert variant="destructive" data-testid="plan-stale">
              <AlertTriangle className="size-4" />
              <AlertTitle>{t("fleets.plan.stale_title")}</AlertTitle>
              <AlertDescription className="space-y-2">
                <p>{t("fleets.plan.stale_description")}</p>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  data-testid="plan-repreview"
                  disabled={previewMutation.isPending}
                  onClick={runPreview}
                >
                  <RefreshCw className="size-4" />
                  {t("fleets.plan.preview_again")}
                </Button>
              </AlertDescription>
            </Alert>
          ) : null}
          {warningCount > 0 ? (
            <Alert data-testid="plan-warnings">
              <AlertTriangle className="size-4" />
              <AlertTitle>
                {t("fleets.plan.warnings_title", { count: warningCount })}
              </AlertTitle>
              <AlertDescription>
                {t("fleets.plan.warnings_description")}
              </AlertDescription>
            </Alert>
          ) : null}
          {missing.length > 0 ? (
            <Alert data-testid="plan-unscheduled">
              <AlertTriangle className="size-4" />
              <AlertTitle>
                {t("fleets.plan.unscheduled", { count: missing.length })}
              </AlertTitle>
              <AlertDescription>
                {missing.map(plate).join(", ")}
              </AlertDescription>
            </Alert>
          ) : null}
          {create.isError && !stale ? (
            <p className="text-destructive text-sm" role="alert">
              {apiError(create.error)}
            </p>
          ) : null}

          <div className="grid gap-4 lg:grid-cols-2">
            {days.map(([day, items]) => {
              const over = dailyMaxNumber
                ? items.length > dailyMaxNumber
                : false;
              return (
                <Card key={day} data-testid="plan-day" data-day={day}>
                  <CardHeader className="flex flex-row items-center justify-between gap-2 pb-2">
                    <CardTitle className="text-base">
                      {format.date(`${day}T12:00:00Z`, "long")}
                    </CardTitle>
                    <Badge variant={over ? "warning" : "secondary"}>
                      {dailyMaxNumber
                        ? t("fleets.plan.day_load", {
                            count: items.length,
                            limit: dailyMaxNumber,
                          })
                        : t("fleets.plan.day_count", { count: items.length })}
                    </Badge>
                  </CardHeader>
                  <CardContent>
                    <ul className="divide-y">
                      {items.map((row) => {
                        const rowWarn = rowWarnings(
                          row,
                          rows,
                          warnings,
                          dailyMaxNumber,
                          timeZone,
                        );
                        return (
                          <li
                            key={row.vehicle_uuid}
                            className="flex flex-wrap items-center gap-2 py-2"
                            data-testid="plan-row"
                            data-vehicle={row.vehicle_uuid}
                          >
                            <span className="w-28 font-mono text-sm" dir="ltr">
                              {plate(row.vehicle_uuid)}
                            </span>
                            <span className="text-sm tabular-nums">
                              {format.time(row.starts_at)}
                            </span>
                            <DatePicker
                              id={`plan-row-date-${row.vehicle_uuid}`}
                              className="h-8 w-44"
                              placeholder={t("fleets.plan.change_day")}
                              value={day}
                              onChange={(next) => {
                                if (!next) return;
                                setRows((prev) =>
                                  prev.map((r) =>
                                    r.vehicle_uuid === row.vehicle_uuid
                                      ? {
                                          ...r,
                                          starts_at: moveToDay(
                                            r.starts_at,
                                            next,
                                            timeZone,
                                          ),
                                        }
                                      : r,
                                  ),
                                );
                              }}
                            />
                            {rowWarn.map((w, i) => (
                              <Badge
                                key={i}
                                variant="warning"
                                data-testid={`plan-row-warning-${w.kind}`}
                              >
                                {w.kind === "capacity"
                                  ? t("fleets.plan.capacity_warning", {
                                      count: w.count,
                                      limit: w.limit,
                                    })
                                  : t(`fleets.plan.warning_codes.${w.code}`)}
                              </Badge>
                            ))}
                          </li>
                        );
                      })}
                    </ul>
                  </CardContent>
                </Card>
              );
            })}
          </div>

          <div className="flex justify-between gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => setStep("settings")}
            >
              {t("common.back")}
            </Button>
            <Button
              type="button"
              data-testid="plan-confirm"
              disabled={create.isPending || stale || rows.length === 0}
              onClick={confirm}
            >
              {create.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <CalendarCheck className="size-4" />
              )}
              {t("fleets.plan.confirm", { count: rows.length })}
            </Button>
          </div>
        </div>
      ) : null}
    </div>
  );
}
