"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, FilterFn } from "@tanstack/react-table";
import { Cpu, Pencil, Plus, Power, PowerOff } from "lucide-react";
import { useId, useMemo, useState, type FormEvent } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  CLIENT_SIDE_MANUAL,
  clientDateRangeFilter,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  type EntityRowAction,
} from "@/components/entity";
import { PageHeader } from "@/components/layout/page-header";
import { createColumn } from "@/components/tables";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  measurementKeys,
  measurementsService,
  type MeasurementDevice,
} from "@/features/measurements/services/measurements.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const MEASUREMENT_DEVICES_PERSIST_KEY = "tenant-measurement-devices-v1";

const dash = (v: string | null | undefined) => (v ? v : "—");
const orNull = (v: string) => (v.trim() === "" ? null : v.trim());

function DeviceDialog({
  device,
  onClose,
}: {
  device: MeasurementDevice | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const id = useId();
  const [serial, setSerial] = useState(device?.serial ?? "");
  const [label, setLabel] = useState(device?.label ?? "");
  const [model, setModel] = useState(device?.model ?? "");
  const [active, setActive] = useState(device?.is_active ?? true);
  const [error, setError] = useState<string | null>(null);

  const save = useMutation({
    mutationFn: () =>
      device
        ? measurementsService.updateDevice(device.uuid, {
            label: orNull(label),
            model: orNull(model),
            is_active: active,
          })
        : measurementsService.createDevice({
            serial: serial.trim(),
            label: orNull(label),
            model: orNull(model),
            is_active: active,
          }),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: measurementKeys.devices });
      appToast.success(t("measurements.devices.saved"));
      onClose();
    },
    onError: (err) => {
      if (isApiError(err) && err.code === "MEASUREMENT_DEVICE_SERIAL_EXISTS") {
        setError(t("measurements.devices.serial_exists"));
      } else if (isApiError(err) && err.code === "VALIDATION_ERROR") {
        setError(err.details[0]?.message ?? err.message);
      } else {
        setError(t("measurements.devices.failed"));
      }
    },
  });

  const onSubmit = (event: FormEvent) => {
    event.preventDefault();
    setError(null);
    if (!device && serial.trim() === "") {
      setError(t("measurements.devices.serial_required"));
      return;
    }
    save.mutate();
  };

  return (
    <Dialog open onOpenChange={(o) => (!o ? onClose() : undefined)}>
      <DialogContent>
        <form onSubmit={onSubmit} noValidate className="space-y-4">
          <DialogHeader>
            <DialogTitle>
              {device
                ? t("measurements.devices.edit_title")
                : t("measurements.devices.new_title")}
            </DialogTitle>
            <DialogDescription>
              {t("measurements.devices.form_description")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor={`${id}-serial`}>
              {t("measurements.devices.serial")}
            </Label>
            <Input
              id={`${id}-serial`}
              name="serial"
              dir="ltr"
              className="font-mono"
              maxLength={64}
              value={serial}
              disabled={Boolean(device)}
              onChange={(e) => setSerial(e.target.value)}
            />
            {device ? (
              <p className="text-muted-foreground text-xs">
                {t("measurements.devices.serial_immutable")}
              </p>
            ) : null}
          </div>
          <div className="space-y-2">
            <Label htmlFor={`${id}-label`}>
              {t("measurements.devices.label")}
            </Label>
            <Input
              id={`${id}-label`}
              name="label"
              maxLength={128}
              value={label}
              onChange={(e) => setLabel(e.target.value)}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor={`${id}-model`}>
              {t("measurements.devices.model")}
            </Label>
            <Input
              id={`${id}-model`}
              name="model"
              maxLength={64}
              value={model}
              onChange={(e) => setModel(e.target.value)}
            />
          </div>
          <div className="flex items-center gap-3">
            <Switch
              id={`${id}-active`}
              checked={active}
              onCheckedChange={setActive}
            />
            <Label htmlFor={`${id}-active`}>
              {t("measurements.devices.active")}
            </Label>
          </div>
          {error ? (
            <p className="text-destructive text-sm" role="alert">
              {error}
            </p>
          ) : null}
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={save.isPending}>
              {t("common.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Tenant > Measurements > Devices (TEC-299): the organization's NexPTG
 * devices (GET /v1/measurement-devices returns the full list, so the
 * table sorts and filters in the browser); add, edit, deactivate and
 * reactivate with measurement_devices.manage.
 */
export function MeasurementDevicesPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canManage = can(permissions.measurements.devicesManage);
  const [editing, setEditing] = useState<MeasurementDevice | "new" | null>(
    null,
  );

  const devices = useQuery({
    queryKey: measurementKeys.devices,
    queryFn: () => measurementsService.listDevices(),
    enabled: canManage,
  });

  const toggle = useMutation({
    mutationFn: (d: MeasurementDevice) =>
      measurementsService.updateDevice(d.uuid, { is_active: !d.is_active }),
    onSuccess: async (d) => {
      await qc.invalidateQueries({ queryKey: measurementKeys.devices });
      appToast.success(
        d.is_active
          ? t("measurements.devices.activated")
          : t("measurements.devices.deactivated"),
      );
    },
    onError: () => appToast.error(t("measurements.devices.failed")),
  });

  const columns = useMemo(
    () =>
      [
        createColumn<MeasurementDevice>({
          accessorKey: "serial",
          labelKey: "measurements.devices.serial",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-mono font-medium" dir="ltr">
              {row.original.serial}
            </span>
          ),
        }),
        createColumn<MeasurementDevice>({
          accessorKey: "label",
          labelKey: "measurements.devices.label",
          enableSorting: true,
          sortUndefined: "last",
          cell: ({ row }) => dash(row.original.label),
        }),
        createColumn<MeasurementDevice>({
          accessorKey: "model",
          labelKey: "measurements.devices.model",
          enableSorting: true,
          sortUndefined: "last",
          cell: ({ row }) => dash(row.original.model),
        }),
        createColumn<MeasurementDevice>({
          accessorKey: "is_active",
          labelKey: "measurements.devices.active",
          enableSorting: true,
          filterVariant: "boolean",
          gridSecondary: true,
          cell: ({ row }) => (
            <Badge variant={row.original.is_active ? "success" : "outline"}>
              {row.original.is_active
                ? t("measurements.devices.status_active")
                : t("measurements.devices.status_inactive")}
            </Badge>
          ),
        }),
        createColumn<MeasurementDevice>({
          accessorKey: "created_at",
          labelKey: "measurements.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          filterFn: clientDateRangeFilter as FilterFn<MeasurementDevice>,
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<MeasurementDevice>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const d = row.original;
            const items: EntityRowAction[] = [
              {
                id: "edit",
                label: t("measurements.devices.edit"),
                icon: Pencil,
                onSelect: () => setEditing(d),
              },
              d.is_active
                ? {
                    id: "deactivate",
                    label: t("measurements.devices.deactivate"),
                    icon: PowerOff,
                    onSelect: () => toggle.mutate(d),
                  }
                : {
                    id: "activate",
                    label: t("measurements.devices.activate"),
                    icon: Power,
                    onSelect: () => toggle.mutate(d),
                  },
            ];
            return <EntityRowActions actions={items} />;
          },
        }),
      ] as ColumnDef<MeasurementDevice, unknown>[],
    [format, t, toggle],
  );

  const title = t("measurements.devices.title");
  return (
    <div className="space-y-6">
      <PageHeader
        title={title}
        description={t("measurements.devices.description")}
        icon={<Cpu className="size-6" />}
        breadcrumbs={[
          {
            label: t("layout.breadcrumb_home"),
            href: routes.tenant.home(slug),
          },
          ...(can(permissions.measurements.read)
            ? [
                {
                  label: t("measurements.list.title"),
                  href: routes.tenant.measurements.list(slug),
                },
              ]
            : []),
          { label: title },
        ]}
        actions={
          canManage ? (
            <Button type="button" onClick={() => setEditing("new")}>
              <Plus className="size-4" />
              {t("measurements.devices.add")}
            </Button>
          ) : null
        }
      />
      {canManage ? (
        <EntityTable
          columns={columns}
          data={devices.data ?? []}
          getRowId={(row) => row.uuid}
          manual={CLIENT_SIDE_MANUAL}
          isLoading={devices.isLoading}
          isError={devices.isError}
          onRetry={() => void devices.refetch()}
          emptyTitle={t("measurements.devices.empty_title")}
          emptyDescription={t("measurements.devices.empty_description")}
          features={{ persistKey: MEASUREMENT_DEVICES_PERSIST_KEY }}
          toolbarExtra={
            <EntityToolbar
              onRefresh={() => void devices.refetch()}
              refreshDisabled={devices.isFetching}
            />
          }
        />
      ) : (
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("measurements.devices.forbidden")}
        />
      )}
      {editing ? (
        <DeviceDialog
          key={editing === "new" ? "new" : editing.uuid}
          device={editing === "new" ? null : editing}
          onClose={() => setEditing(null)}
        />
      ) : null}
    </div>
  );
}
