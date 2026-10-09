"use client";

import { keepPreviousData, useMutation, useQuery } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  Building2,
  Car,
  Download,
  LinkIcon,
  ShieldCheck,
  Wrench,
  X,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState, type ReactNode } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { routes } from "@/config/routes";
import { serviceStatusTone } from "@/features/services/lib/detail";
import {
  portalApi,
  portalFleetReportUrl,
  type FleetLink,
  type FleetPortalAccountLine,
  type FleetPortalReport,
  type FleetPortalService,
  type FleetPortalVehicle,
  type FleetPortalVehicleDetail,
  type FleetPortalWarranty,
} from "@/features/portal/lib/portal-client";
import { portalVehicleTitle } from "@/features/portal/lib/portal-vehicles";
import { useLocale } from "@/providers/locale-provider";

const PAGE_SIZE = 20;
const VEHICLES_PERSIST_KEY = "portal-fleet-vehicles-v1";
const SERVICES_PERSIST_KEY = "portal-fleet-services-v1";
const WARRANTIES_PERSIST_KEY = "portal-fleet-warranties-v1";
const REPORTS_PERSIST_KEY = "portal-fleet-reports-v1";

function money(
  value: string,
  currency: string,
  format: ReturnType<typeof useLocale>["format"],
) {
  const n = Number(value);
  return format.currency(Number.isFinite(n) ? n : 0, currency);
}

function FleetReadOnlyNotice() {
  const { t } = useLocale();
  return (
    <p
      className="text-muted-foreground text-sm"
      data-testid="portal-fleet-read-only"
    >
      {t("portal.fleet.read_only")}
    </p>
  );
}

function StatCard({
  icon,
  label,
  value,
}: {
  icon: ReactNode;
  label: string;
  value: string | number;
}) {
  return (
    <Card>
      <CardContent className="flex items-center gap-3 pt-6">
        <div className="bg-muted rounded-md p-2">{icon}</div>
        <div>
          <p className="text-muted-foreground text-xs">{label}</p>
          <p className="text-xl font-semibold tabular-nums">{value}</p>
        </div>
      </CardContent>
    </Card>
  );
}

function PendingLinkCard({
  link,
  onDone,
}: {
  link: FleetLink;
  onDone: () => void;
}) {
  const { t, format } = useLocale();
  const [rejectOpen, setRejectOpen] = useState(false);
  const accept = useMutation({
    mutationFn: () => portalApi.acceptFleetLink(link.uuid),
    onSuccess: () => {
      toast.success(t("portal.fleet.links.accepted"));
      onDone();
    },
    onError: () => toast.error(t("portal.fleet.links.failed")),
  });
  const reject = useMutation({
    mutationFn: () => portalApi.rejectFleetLink(link.uuid),
    onSuccess: () => {
      setRejectOpen(false);
      toast.success(t("portal.fleet.links.rejected"));
      onDone();
    },
    onError: () => toast.error(t("portal.fleet.links.failed")),
  });

  return (
    <Card data-testid="portal-fleet-pending-link">
      <CardContent className="flex flex-col gap-3 pt-6 sm:flex-row sm:items-center sm:justify-between">
        <div className="min-w-0">
          <p className="font-medium">{link.dealer_name}</p>
          <p className="text-muted-foreground text-xs">
            {t("portal.fleet.links.requested_at", {
              date: format.dateTime(link.created_at),
            })}
          </p>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            onClick={() => accept.mutate()}
            disabled={accept.isPending || reject.isPending}
            data-testid="portal-fleet-link-accept"
          >
            <LinkIcon className="size-4" />
            {t("portal.fleet.links.accept")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => setRejectOpen(true)}
            disabled={accept.isPending || reject.isPending}
            data-testid="portal-fleet-link-reject-open"
          >
            <X className="size-4" />
            {t("portal.fleet.links.reject")}
          </Button>
        </div>
      </CardContent>
      <Dialog open={rejectOpen} onOpenChange={setRejectOpen}>
        <DialogContent data-testid="portal-fleet-link-reject-dialog">
          <DialogHeader>
            <DialogTitle>{t("portal.fleet.links.reject_title")}</DialogTitle>
            <DialogDescription>
              {t("portal.fleet.links.reject_description", {
                dealer: link.dealer_name,
              })}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => setRejectOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="button"
              variant="destructive"
              onClick={() => reject.mutate()}
              disabled={reject.isPending}
              data-testid="portal-fleet-link-reject-confirm"
            >
              {t("portal.fleet.links.reject_confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

export function PortalFleetHome() {
  const { t, format } = useLocale();
  const overview = useQuery({
    queryKey: ["portal", "fleet", "overview"],
    queryFn: () => portalApi.getFleetOverview(),
  });
  const links = useQuery({
    queryKey: ["portal", "fleet", "links"],
    queryFn: () => portalApi.listFleetLinks(),
  });
  const pending = (links.data?.items ?? []).filter(
    (l) => l.status === "pending",
  );

  if (overview.isError) {
    return (
      <div className="mx-auto w-full max-w-5xl space-y-6 px-4 py-8">
        <FleetReadOnlyNotice />
        <ErrorState
          title={t("common.error_generic")}
          description={t("portal.fleet.feature_disabled")}
          onRetry={() => void overview.refetch()}
          retryLabel={t("common.retry")}
        />
        {pending.map((link) => (
          <PendingLinkCard
            key={link.uuid}
            link={link}
            onDone={() => {
              void links.refetch();
              void overview.refetch();
            }}
          />
        ))}
      </div>
    );
  }

  const data = overview.data;
  return (
    <div
      className="mx-auto w-full max-w-5xl space-y-6 px-4 py-8"
      data-testid="portal-fleet-home"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold">
            {data?.fleet.name ?? t("portal.fleet.title")}
          </h1>
          <FleetReadOnlyNotice />
        </div>
      </div>
      {overview.isLoading || !data ? (
        <p className="text-muted-foreground text-sm">
          {t("portal.vehicles.loading")}
        </p>
      ) : (
        <>
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <StatCard
              icon={<Car className="size-5" />}
              label={t("portal.fleet.stats.vehicles")}
              value={data.vehicle_count}
            />
            <StatCard
              icon={<Wrench className="size-5" />}
              label={t("portal.fleet.stats.services")}
              value={data.service_count}
            />
            <StatCard
              icon={<ShieldCheck className="size-5" />}
              label={t("portal.fleet.stats.warranties")}
              value={data.active_warranty_count}
            />
            <StatCard
              icon={<Building2 className="size-5" />}
              label={t("portal.fleet.stats.dealers")}
              value={data.dealers.length}
            />
          </div>
          {pending.length ? (
            <section className="space-y-3" aria-labelledby="pending-links">
              <h2 id="pending-links" className="text-base font-semibold">
                {t("portal.fleet.links.pending")}
              </h2>
              {pending.map((link) => (
                <PendingLinkCard
                  key={link.uuid}
                  link={link}
                  onDone={() => {
                    void links.refetch();
                    void overview.refetch();
                  }}
                />
              ))}
            </section>
          ) : null}
          <section className="grid gap-3 lg:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">
                  {t("portal.fleet.dealers")}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-2">
                {data.dealers.length ? (
                  data.dealers.map((dealer) => (
                    <div
                      key={dealer.uuid}
                      className="flex items-center justify-between gap-3 text-sm"
                    >
                      <span>{dealer.name}</span>
                      <Badge
                        variant={
                          dealer.link_status === "active"
                            ? "success"
                            : "secondary"
                        }
                      >
                        {t(`portal.fleet.link_status.${dealer.link_status}`)}
                      </Badge>
                    </div>
                  ))
                ) : (
                  <p className="text-muted-foreground text-sm">
                    {t("portal.fleet.no_dealers")}
                  </p>
                )}
              </CardContent>
            </Card>
            <Card>
              <CardHeader>
                <CardTitle className="text-base">
                  {t("portal.fleet.appointments")}
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-2">
                {data.upcoming_appointments.length ? (
                  data.upcoming_appointments.map((a) => (
                    <p key={a.uuid} className="text-sm">
                      <span className="font-medium">{a.dealer.name}</span>{" "}
                      <span className="text-muted-foreground">
                        {format.dateTime(a.starts_at)} · {a.plate ?? "—"}
                      </span>
                    </p>
                  ))
                ) : (
                  <p className="text-muted-foreground text-sm">
                    {t("portal.fleet.no_appointments")}
                  </p>
                )}
              </CardContent>
            </Card>
          </section>
        </>
      )}
    </div>
  );
}

export function PortalFleetVehicles() {
  const { t, format } = useLocale();
  const columns = useMemo<ColumnDef<FleetPortalVehicle, unknown>[]>(
    () => [
      {
        accessorKey: "plate",
        header: t("portal.fleet.col.plate"),
        cell: ({ row }) => (
          <span dir="ltr" className="font-mono">
            {row.original.plate ?? "—"}
          </span>
        ),
        meta: {
          labelKey: "portal.fleet.col.plate",
          sortParam: "plate",
          gridPrimary: true,
        },
      },
      {
        id: "vehicle",
        header: t("portal.fleet.col.vehicle"),
        accessorFn: (v) => portalVehicleTitle(v),
        cell: ({ row }) =>
          portalVehicleTitle(row.original) ||
          t("portal.vehicles.unknown_vehicle"),
        meta: { labelKey: "portal.fleet.col.vehicle", gridSecondary: true },
      },
      {
        id: "brand",
        header: t("portal.fleet.col.brand"),
        accessorFn: (v) => v.car_brand?.name ?? "",
        meta: {
          labelKey: "portal.fleet.col.brand",
          param: "brand",
          paramFormat: "csv",
          filterVariant: "faceted",
          filterOptions: [],
        },
      },
      {
        accessorKey: "vin",
        header: t("portal.fleet.col.vin"),
        cell: ({ row }) => (
          <span dir="ltr" className="font-mono">
            {row.original.vin ?? "—"}
          </span>
        ),
        meta: { labelKey: "portal.fleet.col.vin", defaultHidden: true },
      },
      {
        accessorKey: "active_warranty_count",
        header: t("portal.fleet.col.active_warranty"),
        cell: ({ row }) => row.original.active_warranty_count,
        meta: {
          labelKey: "portal.fleet.col.active_warranty",
          param: "has_active_warranty",
          paramFormat: "boolean",
          filterVariant: "boolean",
          sortParam: "warranty_until",
        },
      },
      {
        accessorKey: "last_service_at",
        header: t("portal.fleet.col.last_service"),
        cell: ({ row }) =>
          row.original.last_service_at
            ? format.date(row.original.last_service_at)
            : "—",
        meta: {
          labelKey: "portal.fleet.col.last_service",
          sortParam: "last_service_at",
        },
      },
    ],
    [format, t],
  );
  const state = useServerListState({
    columns,
    initialSort: "plate",
    initialPageSize: PAGE_SIZE,
    persistKey: VEHICLES_PERSIST_KEY,
  });
  const query = useQuery({
    queryKey: ["portal", "fleet", "vehicles", state.params],
    queryFn: () => portalApi.listFleetVehicles(state.params),
    placeholderData: keepPreviousData,
  });
  return (
    <div
      className="mx-auto w-full max-w-6xl space-y-4 px-4 py-8"
      data-testid="portal-fleet-vehicles"
    >
      <div>
        <h1 className="text-xl font-semibold">
          {t("portal.fleet.vehicles.title")}
        </h1>
        <FleetReadOnlyNotice />
      </div>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        rowCount={query.data?.total ?? 0}
        state={state.tableState}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("portal.fleet.vehicles.empty")}
        onRowClick={(row) => {
          window.location.href = routes.portal.fleet.vehicle(row.uuid);
        }}
        features={{ persistKey: VEHICLES_PERSIST_KEY, rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
    </div>
  );
}

export function PortalFleetVehicleDetail({ uuid }: { uuid: string }) {
  const { t, format } = useLocale();
  const detail = useQuery({
    queryKey: ["portal", "fleet", "vehicles", uuid],
    queryFn: () => portalApi.getFleetVehicle(uuid),
  });
  const v = detail.data;
  return (
    <div
      className="mx-auto w-full max-w-5xl space-y-6 px-4 py-8"
      data-testid="portal-fleet-vehicle-detail"
    >
      <Button asChild variant="outline" size="sm">
        <Link href={routes.portal.fleet.vehicles}>
          {t("portal.fleet.vehicles.back")}
        </Link>
      </Button>
      {detail.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : !v ? (
        <p className="text-muted-foreground text-sm">
          {t("portal.vehicles.loading")}
        </p>
      ) : (
        <>
          <Card>
            <CardContent className="space-y-4 pt-6">
              <h1 className="text-xl font-semibold">
                {portalVehicleTitle(v) || t("portal.vehicles.unknown_vehicle")}
              </h1>
              <FleetReadOnlyNotice />
              <dl className="grid gap-3 text-sm sm:grid-cols-3">
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {t("portal.vehicle.plate")}
                  </dt>
                  <dd className="font-mono" dir="ltr">
                    {v.plate ?? "—"}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {t("portal.vehicle.vin")}
                  </dt>
                  <dd className="font-mono break-all" dir="ltr">
                    {v.vin ?? "—"}
                  </dd>
                </div>
                <div>
                  <dt className="text-muted-foreground text-xs">
                    {t("portal.fleet.col.last_service")}
                  </dt>
                  <dd>
                    {v.last_service_at ? format.date(v.last_service_at) : "—"}
                  </dd>
                </div>
              </dl>
            </CardContent>
          </Card>
          <FleetVehicleSections vehicle={v} />
        </>
      )}
    </div>
  );
}

function FleetVehicleSections({
  vehicle,
}: {
  vehicle: FleetPortalVehicleDetail;
}) {
  const { t, format } = useLocale();
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t("portal.vehicle.history")}
          </CardTitle>
        </CardHeader>
        <CardContent>
          {vehicle.services.length ? (
            <ol
              className="divide-y"
              data-testid="portal-fleet-vehicle-services"
            >
              {vehicle.services.map((s) => (
                <li key={s.uuid} className="py-3">
                  <Link
                    href={routes.portal.service(s.uuid)}
                    className="hover:underline"
                  >
                    <span className="font-mono" dir="ltr">
                      {s.service_no}
                    </span>
                  </Link>
                  <p className="text-muted-foreground text-xs">
                    {s.dealer.name} ·{" "}
                    {format.date(s.completed_at ?? s.created_at)}
                  </p>
                </li>
              ))}
            </ol>
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("portal.vehicle.no_services")}
            </p>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle className="text-base">
            {t("portal.vehicle.active_warranties")}
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          {vehicle.warranties.length ? (
            vehicle.warranties.map((w) => (
              <div key={w.uuid} className="rounded-md border p-3">
                <p className="font-medium">{w.product_name}</p>
                <p className="text-muted-foreground text-xs">
                  {w.dealer.name} · {format.date(w.start_at)} -{" "}
                  {format.date(w.end_at)}
                </p>
              </div>
            ))
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("portal.vehicle.no_active_warranty")}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

const serviceStatuses = [
  "pending",
  "processing",
  "ready",
  "completed",
  "cancelled",
] as const;

function fleetServiceStatusTone(status: string) {
  if ((serviceStatuses as readonly string[]).includes(status)) {
    return serviceStatusTone(status as (typeof serviceStatuses)[number]);
  }
  return serviceStatusTone("pending");
}

export function PortalFleetServices() {
  const { t, format } = useLocale();
  const columns = useMemo<ColumnDef<FleetPortalService, unknown>[]>(
    () => [
      {
        accessorKey: "service_no",
        header: t("portal.fleet.col.service_no"),
        cell: ({ row }) => (
          <Link
            href={routes.portal.service(row.original.uuid)}
            className="font-mono hover:underline"
            dir="ltr"
          >
            {row.original.service_no}
          </Link>
        ),
        meta: {
          labelKey: "portal.fleet.col.service_no",
          sortParam: "service_no",
          gridPrimary: true,
        },
      },
      {
        accessorKey: "plate",
        header: t("portal.fleet.col.plate"),
        cell: ({ row }) => (
          <span className="font-mono" dir="ltr">
            {row.original.plate ?? "—"}
          </span>
        ),
        meta: { labelKey: "portal.fleet.col.plate", gridSecondary: true },
      },
      {
        id: "dealer",
        accessorFn: (s) => s.dealer.name,
        header: t("portal.fleet.col.dealer"),
        meta: {
          labelKey: "portal.fleet.col.dealer",
          param: "dealer",
          paramFormat: "csv",
          filterVariant: "faceted",
          filterOptions: [],
        },
      },
      {
        accessorKey: "status",
        header: t("portal.fleet.col.status"),
        cell: ({ row }) => (
          <StatusChip
            label={t(`services.status.${row.original.status}`)}
            tone={fleetServiceStatusTone(row.original.status)}
          />
        ),
        meta: {
          labelKey: "portal.fleet.col.status",
          param: "status",
          paramFormat: "csv",
          filterVariant: "faceted",
          filterOptions: serviceStatuses.map((s) => ({
            value: s,
            labelKey: `services.status.${s}`,
            label: s,
          })),
          sortParam: "status",
        },
      },
      {
        accessorKey: "created_at",
        header: t("portal.fleet.col.created_at"),
        cell: ({ row }) => format.date(row.original.created_at),
        meta: {
          labelKey: "portal.fleet.col.created_at",
          param: "date",
          paramFormat: "date-range",
          filterVariant: "date-range",
          sortParam: "created_at",
        },
      },
      {
        accessorKey: "completed_at",
        header: t("portal.fleet.col.completed_at"),
        cell: ({ row }) =>
          row.original.completed_at
            ? format.date(row.original.completed_at)
            : "—",
        meta: {
          labelKey: "portal.fleet.col.completed_at",
          sortParam: "completed_at",
        },
      },
    ],
    [format, t],
  );
  const state = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: PAGE_SIZE,
    persistKey: SERVICES_PERSIST_KEY,
  });
  const query = useQuery({
    queryKey: ["portal", "fleet", "services", state.params],
    queryFn: () => portalApi.listFleetServices(state.params),
    placeholderData: keepPreviousData,
  });
  return (
    <FleetListPage
      title={t("portal.fleet.services.title")}
      testId="portal-fleet-services"
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        rowCount={query.data?.total ?? 0}
        state={state.tableState}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("portal.fleet.services.empty")}
        features={{ persistKey: SERVICES_PERSIST_KEY, rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
    </FleetListPage>
  );
}

export function PortalFleetWarranties() {
  const { t, format } = useLocale();
  const columns = useMemo<ColumnDef<FleetPortalWarranty, unknown>[]>(
    () => [
      {
        accessorKey: "public_code",
        header: t("portal.fleet.col.warranty_code"),
        cell: ({ row }) => (
          <span className="font-mono" dir="ltr">
            {row.original.public_code}
          </span>
        ),
        meta: { labelKey: "portal.fleet.col.warranty_code", gridPrimary: true },
      },
      {
        accessorKey: "product_name",
        header: t("portal.fleet.col.product"),
        meta: { labelKey: "portal.fleet.col.product", gridSecondary: true },
      },
      {
        accessorKey: "plate",
        header: t("portal.fleet.col.plate"),
        cell: ({ row }) => (
          <Link
            href={routes.portal.fleet.vehicle(row.original.vehicle_uuid)}
            className="font-mono hover:underline"
            dir="ltr"
          >
            {row.original.plate ?? "—"}
          </Link>
        ),
        meta: { labelKey: "portal.fleet.col.plate" },
      },
      {
        accessorKey: "state",
        header: t("portal.fleet.col.status"),
        cell: ({ row }) => (
          <Badge
            variant={
              row.original.state === "active"
                ? "success"
                : row.original.state === "void"
                  ? "danger"
                  : "secondary"
            }
          >
            {t(`portal.fleet.warranty_state.${row.original.state}`)}
          </Badge>
        ),
        meta: {
          labelKey: "portal.fleet.col.status",
          param: "state",
          paramFormat: "csv",
          filterVariant: "faceted",
          filterOptions: ["active", "expired", "void"].map((s) => ({
            value: s,
            labelKey: `portal.fleet.warranty_state.${s}`,
            label: s,
          })),
        },
      },
      {
        accessorKey: "end_at",
        header: t("portal.fleet.col.end_at"),
        cell: ({ row }) => format.date(row.original.end_at),
        meta: { labelKey: "portal.fleet.col.end_at", sortParam: "end_at" },
      },
    ],
    [format, t],
  );
  const state = useServerListState({
    columns,
    initialSort: "end_at",
    initialPageSize: PAGE_SIZE,
    persistKey: WARRANTIES_PERSIST_KEY,
  });
  const query = useQuery({
    queryKey: ["portal", "fleet", "warranties", state.params],
    queryFn: () => portalApi.listFleetWarranties(state.params),
    placeholderData: keepPreviousData,
  });
  return (
    <FleetListPage
      title={t("portal.fleet.warranties.title")}
      testId="portal-fleet-warranties"
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        rowCount={query.data?.total ?? 0}
        state={state.tableState}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("portal.fleet.warranties.empty")}
        features={{ persistKey: WARRANTIES_PERSIST_KEY, rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
    </FleetListPage>
  );
}

function FleetListPage({
  title,
  testId,
  children,
}: {
  title: string;
  testId: string;
  children: ReactNode;
}) {
  return (
    <div
      className="mx-auto w-full max-w-6xl space-y-4 px-4 py-8"
      data-testid={testId}
    >
      <div>
        <h1 className="text-xl font-semibold">{title}</h1>
        <FleetReadOnlyNotice />
      </div>
      {children}
    </div>
  );
}

export function PortalFleetAccount() {
  const { t, format } = useLocale();
  const [dealer, setDealer] = useState("all");
  const query = useQuery({
    queryKey: ["portal", "fleet", "accounting"],
    queryFn: () => portalApi.getFleetAccounting(),
  });
  const dealers = query.data?.dealers ?? [];
  const shown =
    dealer === "all"
      ? dealers
      : dealers.filter((d) => d.dealer.uuid === dealer);
  const rows = shown.flatMap((d) =>
    d.lines.map((line) => ({
      ...line,
      dealerUuid: d.dealer.uuid,
      dealerName: d.dealer.name,
      currency: d.currency,
    })),
  );
  type AccountRow = FleetPortalAccountLine & {
    dealerUuid: string;
    dealerName: string;
    currency: string;
  };
  const columns = useMemo<ColumnDef<AccountRow, unknown>[]>(
    () => [
      {
        accessorKey: "dealerName",
        header: t("portal.fleet.col.dealer"),
        meta: { labelKey: "portal.fleet.col.dealer", gridPrimary: true },
      },
      {
        accessorKey: "date",
        header: t("portal.fleet.col.date"),
        cell: ({ row }) => format.date(row.original.date),
        meta: { labelKey: "portal.fleet.col.date" },
      },
      {
        accessorKey: "kind",
        header: t("portal.fleet.col.kind"),
        cell: ({ row }) => t(`portal.fleet.account_kind.${row.original.kind}`),
        meta: { labelKey: "portal.fleet.col.kind" },
      },
      {
        accessorKey: "debit",
        header: t("portal.fleet.col.debit"),
        cell: ({ row }) =>
          money(row.original.debit, row.original.currency, format),
        meta: { labelKey: "portal.fleet.col.debit" },
      },
      {
        accessorKey: "credit",
        header: t("portal.fleet.col.credit"),
        cell: ({ row }) =>
          money(row.original.credit, row.original.currency, format),
        meta: { labelKey: "portal.fleet.col.credit" },
      },
      {
        accessorKey: "balance",
        header: t("portal.fleet.col.balance"),
        cell: ({ row }) =>
          money(row.original.balance, row.original.currency, format),
        meta: { labelKey: "portal.fleet.col.balance" },
      },
    ],
    [format, t],
  );
  return (
    <FleetListPage
      title={t("portal.fleet.account.title")}
      testId="portal-fleet-account"
    >
      <div className="flex flex-wrap items-center gap-2">
        <label className="text-sm font-medium" htmlFor="fleet-account-dealer">
          {t("portal.fleet.account.dealer_filter")}
        </label>
        <Select value={dealer} onValueChange={setDealer}>
          <SelectTrigger
            id="fleet-account-dealer"
            data-testid="portal-fleet-account-dealer-filter"
            className="w-auto min-w-48"
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">
              {t("portal.fleet.account.all_dealers")}
            </SelectItem>
            {dealers.map((d) => (
              <SelectItem key={d.dealer.uuid} value={d.dealer.uuid}>
                {d.dealer.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        {shown.map((d) => (
          <Card key={d.dealer.uuid}>
            <CardContent className="space-y-2 pt-6 text-sm">
              <p className="font-medium">{d.dealer.name}</p>
              <p>
                {t("portal.fleet.account.closing")}:{" "}
                <span className="font-semibold">
                  {money(d.closing_balance, d.currency, format)}
                </span>
              </p>
              <p className="text-muted-foreground">
                {t("portal.fleet.account.service_income")}:{" "}
                {money(d.service_income_total, d.currency, format)} ·{" "}
                {t("portal.fleet.account.collections")}:{" "}
                {money(d.collection_total, d.currency, format)}
              </p>
            </CardContent>
          </Card>
        ))}
      </div>
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.uuid}
        rowCount={rows.length}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("portal.fleet.account.empty")}
        features={{
          rowSelection: false,
          persistKey: "portal-fleet-account-v1",
        }}
      />
    </FleetListPage>
  );
}

export function PortalFleetReports() {
  const { t, format } = useLocale();
  const columns = useMemo<ColumnDef<FleetPortalReport, unknown>[]>(
    () => [
      {
        accessorKey: "period_kind",
        header: t("portal.fleet.col.period_kind"),
        cell: ({ row }) =>
          t(`portal.fleet.report_kind.${row.original.period_kind}`),
        meta: {
          labelKey: "portal.fleet.col.period_kind",
          param: "period_kind",
          paramFormat: "csv",
          filterVariant: "faceted",
          filterOptions: ["monthly", "quarterly"].map((k) => ({
            value: k,
            labelKey: `portal.fleet.report_kind.${k}`,
            label: k,
          })),
        },
      },
      {
        accessorKey: "period_start",
        header: t("portal.fleet.col.period"),
        cell: ({ row }) =>
          `${format.date(row.original.period_start)} - ${format.date(row.original.period_end)}`,
        meta: {
          labelKey: "portal.fleet.col.period",
          sortParam: "period_start",
          gridPrimary: true,
        },
      },
      {
        accessorKey: "locale",
        header: t("portal.fleet.col.locale"),
        meta: { labelKey: "portal.fleet.col.locale" },
      },
      {
        id: "file",
        header: t("portal.fleet.col.file"),
        cell: ({ row }) => (
          <Button asChild size="sm" variant="outline">
            <a
              href={portalFleetReportUrl(row.original.uuid)}
              data-testid="portal-fleet-report-download"
            >
              <Download className="size-4" />
              {t("portal.fleet.reports.download")}
            </a>
          </Button>
        ),
        meta: { labelKey: "portal.fleet.col.file" },
      },
    ],
    [format, t],
  );
  const state = useServerListState({
    columns,
    initialSort: "-period_start",
    initialPageSize: PAGE_SIZE,
    persistKey: REPORTS_PERSIST_KEY,
  });
  const query = useQuery({
    queryKey: ["portal", "fleet", "reports", state.params],
    queryFn: () => portalApi.listFleetReports(state.params),
    placeholderData: keepPreviousData,
  });
  return (
    <FleetListPage
      title={t("portal.fleet.reports.title")}
      testId="portal-fleet-reports"
    >
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(row) => row.uuid}
        rowCount={query.data?.total ?? 0}
        state={state.tableState}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("portal.fleet.reports.empty")}
        features={{ persistKey: REPORTS_PERSIST_KEY, rowSelection: false }}
        toolbarExtra={
          <EntityToolbar
            onRefresh={() => void query.refetch()}
            refreshDisabled={query.isFetching}
          />
        }
      />
    </FleetListPage>
  );
}
