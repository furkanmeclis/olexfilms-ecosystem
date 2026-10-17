"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Lock, Pencil, Plus, Trash2, Truck } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
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
import { orderErrorMessage } from "@/features/orders/lib/errors";
import {
  deliveryMethodsService,
  orderKeys,
  type DeliveryMethod,
  type DeliveryMethodInput,
} from "@/features/orders/services/orders.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const DELIVERY_METHODS_PERSIST_KEY = "tenant-delivery-methods-v1";

/** Placeholder of a tracking URL template (backend TrackingPlaceholder). */
export const TRACKING_PLACEHOLDER = "{tracking_no}";

const CODE_RE = /^[a-z0-9_]{1,32}$/;

/** Client check of a tracking URL template: an http(s) URL. */
export function validTrackingTemplate(value: string): boolean {
  const v = value.trim();
  if (v === "") return true;
  if (/\s/.test(v)) return false;
  try {
    const u = new URL(v.replace(/\{[^{}]*\}/g, "placeholder"));
    return (u.protocol === "http:" || u.protocol === "https:") && u.host !== "";
  } catch {
    return false;
  }
}

type FormState = {
  code: string;
  name: string;
  template: string;
  active: boolean;
  sortOrder: string;
};

function toForm(m: DeliveryMethod | null): FormState {
  return {
    code: m?.code ?? "",
    name: m?.name ?? "",
    template: m?.tracking_url_template ?? "",
    active: m?.active ?? true,
    sortOrder: String(m?.sort_order ?? 0),
  };
}

function MethodDialog({
  method,
  open,
  onOpenChange,
}: {
  method: DeliveryMethod | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [form, setForm] = useState<FormState>(() => toForm(method));
  const sort = Number(form.sortOrder);
  const errors = {
    code: !CODE_RE.test(form.code.trim().toLowerCase()),
    name: form.name.trim() === "",
    template: !validTrackingTemplate(form.template),
    sort: !Number.isInteger(sort) || sort < 0 || sort > 100000,
  };
  const valid = !Object.values(errors).some(Boolean);
  const save = useMutation({
    mutationFn: () => {
      const body: DeliveryMethodInput = {
        code: form.code.trim().toLowerCase(),
        name: form.name.trim(),
        tracking_url_template: form.template.trim() || null,
        active: form.active,
        sort_order: sort,
      };
      return method
        ? deliveryMethodsService.update(method.uuid, body)
        : deliveryMethodsService.create(body);
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["orders", "delivery-methods"] });
      toast.success(t("orders.delivery_methods.saved"));
      onOpenChange(false);
    },
    onError: (err) =>
      toast.error(
        orderErrorMessage(err, t, t("orders.delivery_methods.save_failed")),
      ),
  });
  const set = <K extends keyof FormState>(k: K, v: FormState[K]) =>
    setForm((f) => ({ ...f, [k]: v }));
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="delivery-method-dialog">
        <form
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault();
            if (valid) save.mutate();
          }}
        >
          <DialogHeader>
            <DialogTitle>
              {method
                ? t("orders.delivery_methods.edit")
                : t("orders.delivery_methods.create")}
            </DialogTitle>
            <DialogDescription>
              {t("orders.delivery_methods.form_hint", {
                placeholder: TRACKING_PLACEHOLDER,
              })}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-1.5">
            <Label htmlFor="dm-name">{t("orders.delivery_methods.name")}</Label>
            <Input
              id="dm-name"
              name="name"
              value={form.name}
              maxLength={120}
              onChange={(e) => set("name", e.target.value)}
            />
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="dm-code">{t("orders.delivery_methods.code")}</Label>
            <Input
              id="dm-code"
              name="code"
              dir="ltr"
              value={form.code}
              maxLength={32}
              aria-invalid={form.code !== "" && errors.code}
              onChange={(e) => set("code", e.target.value)}
            />
            <p className="text-muted-foreground text-xs">
              {t("orders.delivery_methods.code_hint")}
            </p>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="dm-template">
              {t("orders.delivery_methods.template")}
            </Label>
            <Input
              id="dm-template"
              name="tracking_url_template"
              dir="ltr"
              value={form.template}
              maxLength={500}
              placeholder={`https://…?no=${TRACKING_PLACEHOLDER}`}
              aria-invalid={errors.template}
              onChange={(e) => set("template", e.target.value)}
            />
            {errors.template ? (
              <p className="text-destructive text-xs">
                {t("orders.delivery_methods.template_invalid")}
              </p>
            ) : null}
          </div>
          <div className="flex flex-wrap items-end gap-4">
            <div className="w-32 space-y-1.5">
              <Label htmlFor="dm-sort">
                {t("orders.delivery_methods.sort_order")}
              </Label>
              <Input
                id="dm-sort"
                name="sort_order"
                inputMode="numeric"
                dir="ltr"
                value={form.sortOrder}
                onChange={(e) => set("sortOrder", e.target.value)}
              />
            </div>
            <div className="flex items-center gap-2 pb-2">
              <Switch
                id="dm-active"
                checked={form.active}
                onCheckedChange={(v) => set("active", v)}
              />
              <Label htmlFor="dm-active">
                {t("orders.delivery_methods.active")}
              </Label>
            </div>
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("orders.actions.back")}
            </Button>
            <Button
              type="submit"
              disabled={!valid || save.isPending}
              data-testid="delivery-method-save"
            >
              {t("orders.delivery_methods.save")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Tenant > Orders > Delivery methods (TEC-768): the organization's carrier
 * catalog (name, code, tracking URL template, active, order). The system
 * method "Ambar teslim" is listed but read-only.
 */
export function DeliveryMethodsPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canRead = can(permissions.orders.read);
  const canManage = can(permissions.orders.ship);
  const [editing, setEditing] = useState<DeliveryMethod | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [dialogKey, setDialogKey] = useState(0);
  const openDialog = (m: DeliveryMethod | null) => {
    setEditing(m);
    setDialogKey((k) => k + 1);
    setDialogOpen(true);
  };
  const remove = useMutation({
    mutationFn: (m: DeliveryMethod) => deliveryMethodsService.remove(m.uuid),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["orders", "delivery-methods"] });
      toast.success(t("orders.delivery_methods.deleted"));
    },
    onError: (err) =>
      toast.error(
        orderErrorMessage(err, t, t("orders.delivery_methods.delete_failed")),
      ),
  });
  const removeMethod = remove.mutate;

  const columns = useMemo(
    () =>
      [
        createColumn<DeliveryMethod>({
          accessorKey: "name",
          labelKey: "orders.delivery_methods.name",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span
              className="flex items-center gap-2"
              data-testid="delivery-method-row"
              data-code={row.original.code}
            >
              {row.original.name}
              {row.original.is_system ? (
                <Badge variant="secondary">
                  <Lock className="size-3" />
                  {t("orders.delivery_methods.system")}
                </Badge>
              ) : null}
            </span>
          ),
        }),
        createColumn<DeliveryMethod>({
          accessorKey: "code",
          labelKey: "orders.delivery_methods.code",
          enableSorting: true,
          gridSecondary: true,
          cell: ({ row }) => (
            <span className="font-mono text-xs" dir="ltr">
              {row.original.code}
            </span>
          ),
        }),
        createColumn<DeliveryMethod>({
          accessorKey: "tracking_url_template",
          labelKey: "orders.delivery_methods.template",
          enableSorting: false,
          cell: ({ row }) =>
            row.original.tracking_url_template ? (
              <span className="font-mono text-xs break-all" dir="ltr">
                {row.original.tracking_url_template}
              </span>
            ) : (
              "—"
            ),
        }),
        createColumn<DeliveryMethod>({
          accessorKey: "active",
          labelKey: "orders.delivery_methods.active",
          enableSorting: false,
          filterVariant: "boolean",
          param: "active",
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.active
                  ? t("orders.delivery_methods.active")
                  : t("orders.delivery_methods.inactive")
              }
              tone={row.original.active ? "success" : "default"}
            />
          ),
        }),
        createColumn<DeliveryMethod>({
          accessorKey: "is_system",
          labelKey: "orders.delivery_methods.system",
          enableSorting: false,
          filterVariant: "boolean",
          param: "is_system",
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.is_system ? t("common.yes") : t("common.no"),
        }),
        createColumn<DeliveryMethod>({
          accessorKey: "sort_order",
          labelKey: "orders.delivery_methods.sort_order",
          enableSorting: true,
          cell: ({ row }) => (
            <span className="tabular-nums">{row.original.sort_order}</span>
          ),
        }),
        createColumn<DeliveryMethod>({
          accessorKey: "created_at",
          labelKey: "orders.delivery_methods.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.created_at),
        }),
        createColumn<DeliveryMethod>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const m = row.original;
            const locked = m.is_system || !canManage;
            return (
              <EntityRowActions
                actions={[
                  {
                    id: "edit",
                    label: t("orders.delivery_methods.edit"),
                    icon: Pencil,
                    disabled: locked,
                    onSelect: () => openDialog(m),
                  },
                  {
                    id: "delete",
                    label: t("orders.delivery_methods.delete"),
                    icon: Trash2,
                    variant: "destructive",
                    disabled: locked,
                    onSelect: () => removeMethod(m),
                  },
                ]}
              />
            );
          },
        }),
      ] as ColumnDef<DeliveryMethod, unknown>[],
    [canManage, format, removeMethod, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "sort_order",
    initialPageSize: 20,
    persistKey: DELIVERY_METHODS_PERSIST_KEY,
  });
  const params = listState.params;
  const list = useQuery({
    queryKey: orderKeys.deliveryMethods(params),
    queryFn: () => deliveryMethodsService.list(params),
    enabled: canRead,
  });

  const header = (
    <PageHeader
      title={t("orders.delivery_methods.title")}
      description={t("orders.delivery_methods.description")}
      icon={<Truck className="size-6" />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        {
          label: t("layout.nav_settings"),
          href: routes.tenant.settings.root(slug),
        },
        { label: t("orders.delivery_methods.title") },
      ]}
    />
  );
  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("orders.list.forbidden")}
        />
      </div>
    );
  }
  return (
    <div className="space-y-6" data-testid="delivery-methods-page">
      {header}
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("orders.delivery_methods.empty")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: DELIVERY_METHODS_PERSIST_KEY }}
        toolbarExtra={
          <>
            {canManage ? (
              <Button
                type="button"
                size="sm"
                data-testid="delivery-method-create"
                onClick={() => openDialog(null)}
              >
                <Plus className="size-4" />
                {t("orders.delivery_methods.create")}
              </Button>
            ) : null}
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
      <MethodDialog
        key={dialogKey}
        method={editing}
        open={dialogOpen}
        onOpenChange={setDialogOpen}
      />
    </div>
  );
}
