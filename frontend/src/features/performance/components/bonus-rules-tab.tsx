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
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
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
  BONUS_KINDS,
  bonusRuleInput,
  STAFF_METRICS,
  type BonusKind,
  type BonusRuleForm,
  type StaffMetric,
} from "@/features/performance/lib/targets";
import {
  performanceKeys,
  performanceService,
  type PerformanceBonusRule,
} from "@/features/performance/services/performance.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const BONUS_RULES_PERSIST_KEY = "tenant-performance-bonus-rules-v1";

/**
 * Bonus rules (TEC-497, dealer, performance.bonus.manage, dealer_accounting):
 * the payout day of approved bonuses (QUESTIONS S20) and the dealer's
 * target-based rules (whole list, sorted / filtered in the browser).
 */
export function BonusRulesTab() {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const [editing, setEditing] = useState<PerformanceBonusRule | null>(null);
  const [creating, setCreating] = useState(false);
  const [deleting, setDeleting] = useState<PerformanceBonusRule | null>(null);

  const list = useQuery({
    queryKey: performanceKeys.bonusRules,
    queryFn: () => performanceService.bonusRules(),
  });
  const remove = useMutation({
    mutationFn: (uuid: string) => performanceService.deleteBonusRule(uuid),
    onSuccess: () => {
      appToast.success(t("performance.bonus_rules.deleted"));
      void qc.invalidateQueries({ queryKey: performanceKeys.bonusRules });
      setDeleting(null);
    },
    onError: () => appToast.error(t("performance.bonus_rules.delete_failed")),
  });

  const columns = useMemo(
    () =>
      [
        createColumn<PerformanceBonusRule>({
          accessorKey: "name",
          labelKey: "performance.bonus_rules.name",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <span className="font-medium">{row.original.name}</span>
          ),
        }),
        createColumn<PerformanceBonusRule>({
          accessorKey: "metric",
          labelKey: "performance.target.metric",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: STAFF_METRICS.map((value) => ({
            value,
            label: value,
            labelKey: `performance.team.metrics.${value}`,
          })),
          cell: ({ row }) =>
            t(`performance.team.metrics.${row.original.metric}`),
        }),
        createColumn<PerformanceBonusRule>({
          id: "threshold_pct",
          accessorFn: (row) => numberValue(row.threshold_pct) ?? 0,
          labelKey: "performance.bonus_rules.threshold",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) => `%${row.original.threshold_pct ?? ""}`,
        }),
        createColumn<PerformanceBonusRule>({
          accessorKey: "kind",
          labelKey: "performance.bonus_rules.kind",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: BONUS_KINDS.map((value) => ({
            value,
            label: value,
            labelKey: `performance.bonus_rules.kinds.${value}`,
          })),
          cell: ({ row }) =>
            t(`performance.bonus_rules.kinds.${row.original.kind}`),
        }),
        createColumn<PerformanceBonusRule>({
          id: "reward",
          accessorFn: (row) =>
            numberValue(row.kind === "fixed" ? row.amount : row.percent) ?? 0,
          labelKey: "performance.bonus_rules.reward",
          enableSorting: true,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            row.original.kind === "fixed"
              ? format.currency(
                  numberValue(row.original.amount) ?? 0,
                  row.original.currency ?? "TRY",
                )
              : t("performance.bonus_rules.percent_of_revenue_value", {
                  value: row.original.percent ?? "",
                }),
        }),
        createColumn<PerformanceBonusRule>({
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
        createColumn<PerformanceBonusRule>({
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
      ] as ColumnDef<PerformanceBonusRule, unknown>[],
    [format, t],
  );

  return (
    <div className="space-y-4" data-testid="performance-bonus-rules">
      <PayoutDayCard />
      <EntityTable
        columns={columns}
        data={list.data ?? []}
        getRowId={(row) => row.uuid ?? ""}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("performance.bonus_rules.empty_title")}
        emptyDescription={t("performance.bonus_rules.empty_description")}
        manual={CLIENT_SIDE_MANUAL}
        features={{ persistKey: BONUS_RULES_PERSIST_KEY, rowSelection: false }}
        renderGridItem={(row) => (
          <div className="space-y-1">
            <div className="font-medium">{row.name}</div>
            <p className="text-muted-foreground text-xs">
              {t(`performance.team.metrics.${row.metric}`)} · %
              {row.threshold_pct} ·{" "}
              {t(`performance.bonus_rules.kinds.${row.kind}`)}
            </p>
          </div>
        )}
        toolbarExtra={
          <>
            <EntityCreateButton
              label={t("performance.bonus_rules.new")}
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
        <BonusRuleFormDialog
          key={editing?.uuid ?? "new"}
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

/** Day of the month after the period approved bonuses are paid on. */
function PayoutDayCard() {
  const { t } = useLocale();
  const qc = useQueryClient();
  const settings = useQuery({
    queryKey: performanceKeys.bonusSettings,
    queryFn: () => performanceService.bonusSettings(),
  });
  const [draft, setDraft] = useState<string | null>(null);
  const value = draft ?? String(settings.data?.payout_day ?? "");
  const day = Number(value);
  const valid = Number.isInteger(day) && day >= 1 && day <= 28;
  const save = useMutation({
    mutationFn: () => performanceService.saveBonusSettings({ payout_day: day }),
    onSuccess: () => {
      appToast.success(t("performance.bonus_rules.payout_saved"));
      setDraft(null);
      void qc.invalidateQueries({ queryKey: performanceKeys.bonusSettings });
    },
    onError: () => appToast.error(t("performance.bonus_rules.payout_failed")),
  });
  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-base">
          {t("performance.bonus_rules.payout_title")}
        </CardTitle>
        <CardDescription>
          {t("performance.bonus_rules.payout_description")}
        </CardDescription>
      </CardHeader>
      <CardContent className="flex flex-wrap items-end gap-2">
        <div className="grid gap-2">
          <Label htmlFor="performance-payout-day">
            {t("performance.bonus_rules.payout_day")}
          </Label>
          <Input
            id="performance-payout-day"
            className="w-24"
            type="number"
            min={1}
            max={28}
            value={value}
            aria-invalid={!valid}
            onChange={(e) => setDraft(e.target.value)}
          />
        </div>
        <Button
          disabled={!valid || draft == null || save.isPending}
          onClick={() => save.mutate()}
        >
          {t("common.save")}
        </Button>
      </CardContent>
    </Card>
  );
}

export function BonusRuleFormDialog({
  rule,
  onClose,
}: {
  rule: PerformanceBonusRule | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [form, setForm] = useState<BonusRuleForm>({
    name: rule?.name ?? "",
    metric: (rule?.metric as StaffMetric) ?? "services_count",
    threshold_pct: rule?.threshold_pct ?? "100",
    kind: (rule?.kind as BonusKind) ?? "fixed",
    amount: rule?.amount ?? "",
    percent: rule?.percent ?? "",
    active: rule?.active ?? true,
  });
  const set = <K extends keyof BonusRuleForm>(key: K, v: BonusRuleForm[K]) =>
    setForm((prev) => ({ ...prev, [key]: v }));
  const body = bonusRuleInput(form);

  const save = useMutation({
    mutationFn: () => {
      if (!body) throw new Error("invalid");
      return rule?.uuid
        ? performanceService.updateBonusRule(rule.uuid, body)
        : performanceService.createBonusRule(body);
    },
    onSuccess: () => {
      appToast.success(t("performance.bonus_rules.saved"));
      void qc.invalidateQueries({ queryKey: performanceKeys.bonusRules });
      onClose();
    },
    onError: () => appToast.error(t("performance.bonus_rules.failed")),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent data-testid="performance-bonus-rule-form">
        <DialogHeader>
          <DialogTitle>
            {rule
              ? t("performance.bonus_rules.edit_title")
              : t("performance.bonus_rules.new")}
          </DialogTitle>
          <DialogDescription>
            {t("performance.bonus_rules.form_description")}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="performance-bonus-rule-name">
              {t("performance.bonus_rules.name")}
            </Label>
            <Input
              id="performance-bonus-rule-name"
              value={form.name}
              maxLength={200}
              onChange={(e) => set("name", e.target.value)}
            />
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label>{t("performance.target.metric")}</Label>
              <Select
                value={form.metric}
                onValueChange={(v) => set("metric", v as StaffMetric)}
              >
                <SelectTrigger aria-label={t("performance.target.metric")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {STAFF_METRICS.map((m) => (
                    <SelectItem key={m} value={m}>
                      {t(`performance.team.metrics.${m}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="grid gap-2">
              <Label htmlFor="performance-bonus-rule-threshold">
                {t("performance.bonus_rules.threshold")}
              </Label>
              <Input
                id="performance-bonus-rule-threshold"
                inputMode="decimal"
                value={form.threshold_pct}
                onChange={(e) => set("threshold_pct", e.target.value)}
              />
            </div>
          </div>
          <div className="grid gap-4 sm:grid-cols-2">
            <div className="grid gap-2">
              <Label>{t("performance.bonus_rules.kind")}</Label>
              <Select
                value={form.kind}
                onValueChange={(v) => set("kind", v as BonusKind)}
              >
                <SelectTrigger aria-label={t("performance.bonus_rules.kind")}>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {BONUS_KINDS.map((k) => (
                    <SelectItem key={k} value={k}>
                      {t(`performance.bonus_rules.kinds.${k}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            {form.kind === "fixed" ? (
              <div className="grid gap-2">
                <Label htmlFor="performance-bonus-rule-amount">
                  {t("performance.bonus_rules.amount")}
                </Label>
                <Input
                  id="performance-bonus-rule-amount"
                  inputMode="decimal"
                  value={form.amount}
                  onChange={(e) => set("amount", e.target.value)}
                />
              </div>
            ) : (
              <div className="grid gap-2">
                <Label htmlFor="performance-bonus-rule-percent">
                  {t("performance.bonus_rules.percent")}
                </Label>
                <Input
                  id="performance-bonus-rule-percent"
                  inputMode="decimal"
                  value={form.percent}
                  onChange={(e) => set("percent", e.target.value)}
                />
              </div>
            )}
          </div>
          <div className="flex items-center justify-between gap-2">
            <Label htmlFor="performance-bonus-rule-active">
              {t("performance.rules.active")}
            </Label>
            <Switch
              id="performance-bonus-rule-active"
              checked={form.active}
              onCheckedChange={(v) => set("active", v)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={!body || save.isPending}
            onClick={() => save.mutate()}
          >
            {t("common.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
