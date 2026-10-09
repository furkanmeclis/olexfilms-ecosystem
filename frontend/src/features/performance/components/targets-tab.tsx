"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";

import {
  EntityCreateButton,
  EntityDeleteDialog,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import {
  AsyncCombobox,
  type ComboboxOption,
} from "@/components/ui/async-combobox";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { AchievementBar } from "@/features/performance/components/achievement-bar";
import {
  numberValue,
  TARGET_METRICS,
  TARGET_PERIOD_KINDS,
  type TargetMetric,
} from "@/features/performance/lib/performance";
import { targetOrganizationOptions } from "@/features/performance/lib/targets";
import {
  performanceKeys,
  performanceService,
  type PerformanceTarget,
  type TargetOrganization,
} from "@/features/performance/services/performance.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

export const TARGETS_PERSIST_KEY = "tenant-performance-targets-v1";

type PeriodKind = (typeof TARGET_PERIOD_KINDS)[number];

type Formatters = ReturnType<typeof useLocale>["format"];

/** Target / actual in the target currency (order volume) or as a count. */
function targetAmount(
  format: Formatters,
  value: string | null | undefined,
  row: PerformanceTarget,
) {
  const n = numberValue(value) ?? 0;
  return row.currency
    ? format.currency(n, row.currency)
    : format.number(n, { maximumFractionDigits: 2 });
}

function enumOptions(values: readonly string[], prefix: string) {
  return values.map((value) => ({
    value,
    label: value,
    labelKey: `${prefix}.${value}`,
  }));
}

/**
 * Network targets (TEC-497, center / distributor): GET
 * /v1/performance/targets with sort, metric / period facets, the period
 * range and organization search; managers add, edit and delete targets.
 */
export function TargetsTab({
  orgType,
  orgUuid,
  period,
}: {
  orgType: string;
  orgUuid: string;
  period: string;
}) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canManage = can(permissions.performance.targetsManage);
  const [editing, setEditing] = useState<PerformanceTarget | null>(null);
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<PerformanceTarget | null>(null);

  const remove = useMutation({
    mutationFn: (uuid: string) => performanceService.deleteTarget(uuid),
    onSuccess: () => {
      appToast.success(t("performance.targets.deleted"));
      void qc.invalidateQueries({ queryKey: performanceKeys.all });
      setDeleting(null);
    },
    onError: () => appToast.error(t("performance.targets.delete_failed")),
  });

  const columns = useMemo(
    () =>
      [
        createColumn<PerformanceTarget>({
          accessorKey: "target_name",
          labelKey: "performance.targets.organization",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div>
              <div className="font-medium">{row.original.target_name}</div>
              <div className="text-muted-foreground text-xs">
                {row.original.target_type
                  ? t(`performance.org_type.${row.original.target_type}`)
                  : ""}
              </div>
            </div>
          ),
        }),
        createColumn<PerformanceTarget>({
          accessorKey: "metric",
          labelKey: "performance.target.metric",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: enumOptions(TARGET_METRICS, "performance.metrics"),
          param: "metric",
          cell: ({ row }) => t(`performance.metrics.${row.original.metric}`),
        }),
        createColumn<PerformanceTarget>({
          accessorKey: "period_kind",
          labelKey: "performance.target.period_kind",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: enumOptions(
            TARGET_PERIOD_KINDS,
            "performance.target.period_kinds",
          ),
          param: "period_kind",
          cell: ({ row }) =>
            t(`performance.target.period_kinds.${row.original.period_kind}`),
        }),
        createColumn<PerformanceTarget>({
          accessorKey: "period_start",
          labelKey: "performance.targets.period",
          enableSorting: true,
          filterVariant: "date-range",
          param: "period",
          cell: ({ row }) => (
            <span className="whitespace-nowrap">
              {format.date(row.original.period_start ?? "")} –{" "}
              {format.date(row.original.period_end ?? "")}
            </span>
          ),
        }),
        createColumn<PerformanceTarget>({
          accessorKey: "value",
          labelKey: "performance.target.value",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            targetAmount(format, row.original.value, row.original),
        }),
        createColumn<PerformanceTarget>({
          accessorKey: "actual",
          labelKey: "performance.targets.actual",
          enableSorting: false,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            row.original.actual == null
              ? "—"
              : targetAmount(format, row.original.actual, row.original),
        }),
        createColumn<PerformanceTarget>({
          accessorKey: "achievement_pct",
          labelKey: "performance.targets.achievement",
          enableSorting: true,
          cell: ({ row }) => (
            <AchievementBar pct={row.original.achievement_pct} />
          ),
        }),
        createColumn<PerformanceTarget>({
          accessorKey: "contract_ref",
          labelKey: "performance.targets.contract_ref",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.contract_ref
              ? format.date(row.original.contract_ref)
              : "—",
        }),
        createColumn<PerformanceTarget>({
          accessorKey: "note",
          labelKey: "performance.target.note",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) => row.original.note ?? "",
        }),
        createColumn<PerformanceTarget>({
          accessorKey: "created_at",
          labelKey: "performance.targets.created_at",
          enableSorting: true,
          defaultHidden: true,
          cell: ({ row }) => format.dateTime(row.original.created_at ?? ""),
        }),
        ...(canManage
          ? [
              createColumn<PerformanceTarget>({
                id: "actions",
                labelKey: "common.actions",
                enableSorting: false,
                enableHiding: false,
                enableResizing: false,
                cell: ({ row }) => {
                  const actions: EntityRowAction[] = [
                    {
                      id: "edit",
                      label: t("common.edit"),
                      icon: Pencil,
                      onSelect: () => setEditing(row.original),
                    },
                    {
                      id: "delete",
                      label: t("common.delete"),
                      icon: Trash2,
                      variant: "destructive",
                      onSelect: () => setDeleting(row.original),
                    },
                  ];
                  return <EntityRowActions actions={actions} />;
                },
              }),
            ]
          : []),
      ] as ColumnDef<PerformanceTarget, unknown>[],
    [canManage, format, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-period_start",
    persistKey: TARGETS_PERSIST_KEY,
  });
  const params = listState.params;
  const list = useQuery({
    queryKey: performanceKeys.targets(params),
    queryFn: () => performanceService.targets(params),
  });

  return (
    <div className="space-y-4" data-testid="performance-targets">
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid ?? ""}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("performance.targets.empty_title")}
        emptyDescription={t("performance.targets.empty_description")}
        rowCount={list.data?.total ?? 0}
        state={listState.tableState}
        features={{ persistKey: TARGETS_PERSIST_KEY, rowSelection: false }}
        renderGridItem={(row) => (
          <div className="space-y-2">
            <div className="font-medium">{row.target_name}</div>
            <p className="text-muted-foreground text-xs">
              {t(`performance.metrics.${row.metric}`)} ·{" "}
              {t(`performance.target.period_kinds.${row.period_kind}`)} ·{" "}
              {format.date(row.period_start ?? "")}
            </p>
            <AchievementBar pct={row.achievement_pct} />
          </div>
        )}
        toolbarExtra={
          <>
            {canManage ? (
              <EntityCreateButton
                label={t("performance.targets.new")}
                onClick={() => setCreating(true)}
              />
            ) : null}
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
      {creating || editing ? (
        <TargetFormDialog
          key={editing?.uuid ?? "new"}
          orgType={orgType}
          orgUuid={orgUuid}
          period={period}
          target={editing}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
        />
      ) : null}
      <EntityDeleteDialog
        open={Boolean(deleting)}
        entityLabel={deleting?.target_name ?? ""}
        softDelete={false}
        isPending={remove.isPending}
        onConfirm={() => deleting?.uuid && remove.mutate(deleting.uuid)}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}

function orgOption(org: TargetOrganization, t: (k: string) => string) {
  return {
    value: org.uuid,
    label: org.name,
    description: t(`performance.org_type.${org.type}`),
  };
}

/**
 * Create / edit a network target. The organization picker lists only the
 * organizations below the active one; "bind to contract period" stores
 * the organization's contract end date as contract_ref. An edit changes
 * value, contract binding and note (metric and period are fixed).
 */
export function TargetFormDialog({
  orgType,
  orgUuid,
  period,
  target,
  onClose,
}: {
  orgType: string;
  orgUuid: string;
  period: string;
  target: PerformanceTarget | null;
  onClose: () => void;
}) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const editing = Boolean(target);
  const [orgId, setOrgId] = useState(target?.target_organization_uuid ?? "");
  const [picked, setPicked] = useState<Record<string, TargetOrganization>>({});
  const [metric, setMetric] = useState<TargetMetric>(
    (target?.metric as TargetMetric) ?? "services_count",
  );
  const [periodKind, setPeriodKind] = useState<PeriodKind>(
    (target?.period_kind as PeriodKind) ?? "monthly",
  );
  const [month, setMonth] = useState(
    target?.period_start?.slice(0, 7) ?? period,
  );
  const [value, setValue] = useState(target?.value ?? "");
  const [note, setNote] = useState(target?.note ?? "");
  const [bindContract, setBindContract] = useState(
    Boolean(target?.contract_ref),
  );

  const initial = useQuery({
    queryKey: performanceKeys.targetOrganizations(""),
    queryFn: () => performanceService.targetOrganizations(""),
  });
  const allowed = useMemo(
    () => targetOrganizationOptions(initial.data ?? [], orgType, orgUuid),
    [initial.data, orgType, orgUuid],
  );
  const org: TargetOrganization | undefined =
    picked[orgId] ?? allowed.find((o) => o.uuid === orgId);
  const contractDate =
    org?.contract_valid_until?.slice(0, 10) ?? target?.contract_ref ?? null;

  const loadOptions = useCallback(
    async (q: string): Promise<ComboboxOption[]> => {
      const rows = targetOrganizationOptions(
        await performanceService.targetOrganizations(q),
        orgType,
        orgUuid,
      );
      setPicked((prev) => {
        const next = { ...prev };
        for (const r of rows) next[r.uuid] = r;
        return next;
      });
      return rows.map((r) => orgOption(r, t));
    },
    [orgType, orgUuid, t],
  );

  const currency =
    metric === "order_volume"
      ? (target?.currency ?? org?.currency ?? null)
      : null;
  const valid =
    Boolean(orgId) &&
    (numberValue(value) ?? 0) > 0 &&
    /^\d{4}-\d{2}$/.test(month) &&
    (!bindContract || Boolean(contractDate));

  const save = useMutation({
    mutationFn: () => {
      const body = {
        target_organization_uuid: orgId,
        metric,
        period_kind: periodKind,
        period_start: `${month}-01`,
        value: value.trim(),
        currency,
        contract_ref: bindContract ? contractDate : null,
        note: note.trim() || null,
      };
      return target?.uuid
        ? performanceService.updateTarget(target.uuid, body)
        : performanceService.createTarget(body);
    },
    onSuccess: () => {
      appToast.success(t("performance.target.saved"));
      void qc.invalidateQueries({ queryKey: performanceKeys.all });
      onClose();
    },
    onError: () => appToast.error(t("performance.target.failed")),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent data-testid="performance-target-form">
        <DialogHeader>
          <DialogTitle>
            {editing
              ? t("performance.targets.edit_title")
              : t("performance.targets.new")}
          </DialogTitle>
          <DialogDescription>
            {t("performance.targets.form_description")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label>{t("performance.targets.organization")}</Label>
            {editing ? (
              <Input value={target?.target_name ?? ""} disabled />
            ) : (
              <AsyncCombobox
                value={orgId}
                onValueChange={setOrgId}
                initialOptions={allowed.map((o) => orgOption(o, t))}
                options={allowed.map((o) => orgOption(o, t))}
                loadOptions={loadOptions}
                placeholder={t("performance.targets.organization_placeholder")}
                searchPlaceholder={t("common.search")}
                emptyText={t("performance.targets.organization_empty")}
              />
            )}
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label>{t("performance.target.metric")}</Label>
              <Select
                value={metric}
                disabled={editing}
                onValueChange={(v) => setMetric(v as TargetMetric)}
              >
                <SelectTrigger aria-label={t("performance.target.metric")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TARGET_METRICS.map((m) => (
                    <SelectItem key={m} value={m}>
                      {t(`performance.metrics.${m}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label>{t("performance.target.period_kind")}</Label>
              <Select
                value={periodKind}
                disabled={editing}
                onValueChange={(v) => setPeriodKind(v as PeriodKind)}
              >
                <SelectTrigger aria-label={t("performance.target.period_kind")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {TARGET_PERIOD_KINDS.map((k) => (
                    <SelectItem key={k} value={k}>
                      {t(`performance.target.period_kinds.${k}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label htmlFor="performance-target-month">
                {t("performance.targets.start_month")}
              </Label>
              <Input
                id="performance-target-month"
                type="month"
                value={month}
                disabled={editing}
                onChange={(e) => setMonth(e.target.value)}
              />
            </div>
            <div className="grid gap-2">
              <Label htmlFor="performance-target-form-value">
                {t("performance.target.value")}
                {currency ? ` (${currency})` : ""}
              </Label>
              <Input
                id="performance-target-form-value"
                inputMode="decimal"
                value={value}
                onChange={(e) => setValue(e.target.value)}
              />
            </div>
          </div>
          <div className="flex items-start gap-2">
            <Checkbox
              id="performance-target-contract"
              checked={bindContract}
              disabled={!contractDate}
              onCheckedChange={(v) => setBindContract(v === true)}
              data-testid="performance-target-contract"
            />
            <div className="grid gap-1">
              <Label htmlFor="performance-target-contract">
                {t("performance.targets.bind_contract")}
              </Label>
              <p className="text-muted-foreground text-xs">
                {contractDate
                  ? t("performance.targets.contract_until", {
                      date: format.date(contractDate),
                    })
                  : t("performance.targets.no_contract")}
              </p>
            </div>
          </div>
          <div className="grid gap-2">
            <Label htmlFor="performance-target-form-note">
              {t("performance.target.note")}
            </Label>
            <Textarea
              id="performance-target-form-note"
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={!valid || save.isPending}
            onClick={() => save.mutate()}
            data-testid="performance-target-save"
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
