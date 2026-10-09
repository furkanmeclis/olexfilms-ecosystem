"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Pencil, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  CLIENT_SIDE_MANUAL,
  EntityCreateButton,
  EntityDeleteDialog,
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn } from "@/components/tables";
import { AsyncCombobox } from "@/components/ui/async-combobox";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import { numberValue } from "@/features/performance/lib/performance";
import {
  canOpenTask,
  RULE_METRICS,
  RULE_OPERATORS,
  type RuleOperator,
} from "@/features/performance/lib/targets";
import {
  performanceKeys,
  performanceService,
  type PerformanceRule,
  type PerformanceRuleInput,
} from "@/features/performance/services/performance.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const RULES_PERSIST_KEY = "tenant-performance-weak-rules-v1";

function enumOptions(values: readonly string[], prefix: string) {
  return values.map((value) => ({
    value,
    label: value,
    labelKey: `${prefix}.${value}`,
  }));
}

/** Label of a weak dealer rule metric. */
export function ruleMetricKey(metric: string) {
  return metric === "target_achievement"
    ? "performance.rules.target_achievement"
    : `performance.metrics.${metric}`;
}

/**
 * Weak dealer rules (TEC-497, center / distributor, performance.rules.manage):
 * the organization's rules (GET /v1/performance/rules returns the whole
 * list, so the table sorts and filters in the browser) with create, edit
 * and delete (a rule that already fired is deactivated).
 */
export function WeakRulesTab({ orgType }: { orgType: string }) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [editing, setEditing] = useState<PerformanceRule | null>(null);
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<PerformanceRule | null>(null);
  const center = canOpenTask(orgType);

  const list = useQuery({
    queryKey: performanceKeys.rules,
    queryFn: () => performanceService.rules(),
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => performanceService.deleteRule(uuid),
    onSuccess: () => {
      appToast.success(t("performance.rules.deleted"));
      void qc.invalidateQueries({ queryKey: performanceKeys.rules });
      setDeleting(null);
    },
    onError: () => appToast.error(t("performance.rules.delete_failed")),
  });

  const columns = useMemo(
    () =>
      [
        createColumn<PerformanceRule>({
          accessorKey: "name",
          labelKey: "performance.rules.name",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<PerformanceRule>({
          accessorKey: "metric",
          labelKey: "performance.target.metric",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: RULE_METRICS.map((value) => ({
            value,
            label: value,
            labelKey: ruleMetricKey(value),
          })),
          cell: ({ row }) => t(ruleMetricKey(row.original.metric ?? "")),
        }),
        createColumn<PerformanceRule>({
          id: "condition",
          accessorFn: (row) => row.operator,
          labelKey: "performance.rules.condition",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: enumOptions(
            RULE_OPERATORS,
            "performance.rules.operators",
          ),
          cell: ({ row }) =>
            `${t(`performance.rules.operators.${row.original.operator}`)} ${row.original.threshold ?? ""}`,
        }),
        ...(center
          ? [
              createColumn<PerformanceRule>({
                id: "create_task",
                accessorFn: (row) => String(Boolean(row.create_task)),
                labelKey: "performance.rules.create_task",
                enableSorting: true,
                cell: ({ row }) =>
                  row.original.create_task ? t("common.yes") : t("common.no"),
              }),
              createColumn<PerformanceRule>({
                id: "assignee",
                accessorFn: (row) => row.assignee_name ?? "",
                labelKey: "performance.rules.assignee",
                enableSorting: true,
                cell: ({ row }) => row.original.assignee_name || "—",
              }),
            ]
          : []),
        createColumn<PerformanceRule>({
          id: "notify",
          accessorFn: (row) => String(Boolean(row.notify)),
          labelKey: "performance.rules.notify",
          enableSorting: true,
          cell: ({ row }) =>
            row.original.notify ? t("common.yes") : t("common.no"),
        }),
        createColumn<PerformanceRule>({
          id: "active",
          accessorFn: (row) => String(Boolean(row.active)),
          labelKey: "performance.rules.active",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: [
            { value: "true", label: "true", labelKey: "performance.rules.on" },
            {
              value: "false",
              label: "false",
              labelKey: "performance.rules.off",
            },
          ],
          cell: ({ row }) => (
            <StatusChip
              label={
                row.original.active
                  ? t("performance.rules.on")
                  : t("performance.rules.off")
              }
              tone={row.original.active ? "success" : "default"}
            />
          ),
        }),
        createColumn<PerformanceRule>({
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
      ] as ColumnDef<PerformanceRule, unknown>[],
    [center, t],
  );

  return (
    <div className="space-y-4" data-testid="performance-weak-rules">
      <p className="text-muted-foreground text-sm">
        {center
          ? t("performance.rules.center_hint")
          : t("performance.rules.distributor_hint")}
      </p>
      <EntityTable
        columns={columns}
        data={list.data ?? []}
        getRowId={(row) => row.uuid ?? ""}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("performance.rules.empty_title")}
        emptyDescription={t("performance.rules.empty_description")}
        manual={CLIENT_SIDE_MANUAL}
        features={{ persistKey: RULES_PERSIST_KEY, rowSelection: false }}
        renderGridItem={(row) => (
          <div className="space-y-1">
            <div className="font-medium">{row.name}</div>
            <p className="text-muted-foreground text-xs">
              {t(ruleMetricKey(row.metric ?? ""))}{" "}
              {t(`performance.rules.operators.${row.operator}`)} {row.threshold}
            </p>
          </div>
        )}
        toolbarExtra={
          <>
            <EntityCreateButton
              label={t("performance.rules.new")}
              onClick={() => setCreating(true)}
            />
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
      {creating || editing ? (
        <RuleFormDialog
          key={editing?.uuid ?? "new"}
          orgType={orgType}
          rule={editing}
          onClose={() => {
            setCreating(false);
            setEditing(null);
          }}
        />
      ) : null}
      <EntityDeleteDialog
        open={Boolean(deleting)}
        entityLabel={deleting?.name ?? ""}
        softDelete={false}
        isPending={remove.isPending}
        onConfirm={() => deleting?.uuid && remove.mutate(deleting.uuid)}
        onCancel={() => setDeleting(null)}
      />
    </div>
  );
}

/**
 * Weak dealer rule form: metric, operator, threshold, notify and (center
 * only, QUESTIONS S19) "open task" with the task assignee. A distributor
 * form has no task option at all.
 */
export function RuleFormDialog({
  orgType,
  rule,
  onClose,
}: {
  orgType: string;
  rule: PerformanceRule | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const center = canOpenTask(orgType);
  const [name, setName] = useState(rule?.name ?? "");
  const [metric, setMetric] = useState(rule?.metric ?? "target_achievement");
  const [operator, setOperator] = useState<RuleOperator>(
    (rule?.operator as RuleOperator) ?? "lt",
  );
  const [threshold, setThreshold] = useState(rule?.threshold ?? "");
  const [notify, setNotify] = useState(rule?.notify ?? true);
  const [createTask, setCreateTask] = useState(
    center && Boolean(rule?.create_task),
  );
  const [assignee, setAssignee] = useState(rule?.assignee_user_uuid ?? "");
  const [active, setActive] = useState(rule?.active ?? true);

  const members = useQuery({
    queryKey: performanceKeys.members,
    queryFn: () => performanceService.members(),
    enabled: center,
  });
  const memberOptions = (members.data ?? []).map((m) => ({
    value: m.uuid ?? "",
    label: m.name ?? "",
  }));

  const thresholdValue = numberValue(threshold);
  const medianOk =
    operator !== "below_median_pct" ||
    (thresholdValue != null && thresholdValue > 0 && thresholdValue <= 100);
  const task = center && createTask;
  const valid =
    Boolean(name.trim()) &&
    thresholdValue != null &&
    medianOk &&
    (notify || task);

  const save = useMutation({
    mutationFn: () => {
      const body: PerformanceRuleInput = {
        name: name.trim(),
        metric,
        operator,
        threshold: threshold.trim(),
        notify,
        create_task: task,
        assignee_user_uuid: task && assignee ? assignee : null,
        active,
      };
      return rule?.uuid
        ? performanceService.updateRule(rule.uuid, body)
        : performanceService.createRule(body);
    },
    onSuccess: () => {
      appToast.success(t("performance.rules.saved"));
      void qc.invalidateQueries({ queryKey: performanceKeys.rules });
      onClose();
    },
    onError: () => appToast.error(t("performance.rules.failed")),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent data-testid="performance-rule-form">
        <DialogHeader>
          <DialogTitle>
            {rule
              ? t("performance.rules.edit_title")
              : t("performance.rules.new")}
          </DialogTitle>
          <DialogDescription>
            {center
              ? t("performance.rules.center_hint")
              : t("performance.rules.distributor_hint")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="performance-rule-name">
              {t("performance.rules.name")}
            </Label>
            <Input
              id="performance-rule-name"
              value={name}
              maxLength={200}
              onChange={(e) => setName(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label>{t("performance.target.metric")}</Label>
            <Select value={metric} onValueChange={setMetric}>
              <SelectTrigger aria-label={t("performance.target.metric")}>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {RULE_METRICS.map((m) => (
                  <SelectItem key={m} value={m}>
                    {t(ruleMetricKey(m))}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label>{t("performance.rules.operator")}</Label>
              <Select
                value={operator}
                onValueChange={(v) => setOperator(v as RuleOperator)}
              >
                <SelectTrigger aria-label={t("performance.rules.operator")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {RULE_OPERATORS.map((op) => (
                    <SelectItem key={op} value={op}>
                      {t(`performance.rules.operators.${op}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="performance-rule-threshold">
                {t("performance.rules.threshold")}
              </Label>
              <Input
                id="performance-rule-threshold"
                inputMode="decimal"
                value={threshold}
                onChange={(e) => setThreshold(e.target.value)}
                aria-invalid={!medianOk}
              />
            </div>
          </div>
          {!medianOk ? (
            <p className="text-destructive text-xs">
              {t("performance.rules.median_range")}
            </p>
          ) : null}
          <div className="flex items-center justify-between gap-2">
            <Label htmlFor="performance-rule-notify">
              {t("performance.rules.notify")}
            </Label>
            <Switch
              id="performance-rule-notify"
              checked={notify}
              onCheckedChange={setNotify}
            />
          </div>
          {center ? (
            <>
              <div className="flex items-center justify-between gap-2">
                <Label htmlFor="performance-rule-create-task">
                  {t("performance.rules.create_task")}
                </Label>
                <Switch
                  id="performance-rule-create-task"
                  data-testid="performance-rule-create-task"
                  checked={createTask}
                  onCheckedChange={setCreateTask}
                />
              </div>
              {createTask ? (
                <div className="grid gap-2">
                  <Label>{t("performance.rules.assignee")}</Label>
                  <AsyncCombobox
                    value={assignee}
                    onValueChange={setAssignee}
                    options={memberOptions}
                    clearable
                    placeholder={t("performance.rules.assignee_placeholder")}
                    searchPlaceholder={t("common.search")}
                    emptyText={t("performance.rules.assignee_empty")}
                  />
                </div>
              ) : null}
            </>
          ) : null}
          <div className="flex items-center justify-between gap-2">
            <Label htmlFor="performance-rule-active">
              {t("performance.rules.active")}
            </Label>
            <Switch
              id="performance-rule-active"
              checked={active}
              onCheckedChange={setActive}
            />
          </div>
          {!notify && !task ? (
            <p className="text-destructive text-xs">
              {t("performance.rules.action_required")}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={!valid || save.isPending}
            onClick={() => save.mutate()}
            data-testid="performance-rule-save"
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
