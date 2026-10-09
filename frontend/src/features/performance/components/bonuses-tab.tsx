"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { BadgeCheck, CheckCheck, XCircle } from "lucide-react";
import { useMemo, useState } from "react";

import { StatusChip } from "@/components/common/status-chip";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
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
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import {
  BulkActionMenu,
  SelectionBanner,
  useBulkSelection,
  type BulkActionDef,
} from "@/features/bulk-engine";
import { AchievementBar } from "@/features/performance/components/achievement-bar";
import { numberValue } from "@/features/performance/lib/performance";
import {
  BONUS_STATUSES,
  bonusApproval,
} from "@/features/performance/lib/targets";
import {
  performanceKeys,
  performanceService,
  type PerformanceBonus,
} from "@/features/performance/services/performance.service";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const BONUSES_PERSIST_KEY = "tenant-performance-bonuses-v1";

/**
 * `POST /v1/performance/bonuses/bulk` (TEC-497): approve the selected (or
 * every matching) calculated bonus with its calculated amount; each item
 * books the staff payment, so there is no undo. An adjusted amount needs a
 * note and stays a row action.
 */
export const BONUS_BULK_ACTIONS: BulkActionDef[] = [
  {
    id: "approve",
    label_key: "bulk.actions.performance_bonuses.approve",
    permission: permissions.performance.bonusManage,
    confirm_key: "bulk.confirm.performance_bonuses.approve",
    icon: CheckCheck,
  },
];

function statusTone(status: string | undefined) {
  switch (status) {
    case "posted":
    case "approved":
      return "success" as const;
    case "cancelled":
      return "danger" as const;
    default:
      return "warning" as const;
  }
}

function selectedValues(
  filters: { id: string; value: unknown }[],
  id: string,
): string[] {
  const value = filters.find((f) => f.id === id)?.value;
  if (Array.isArray(value)) return value.map(String);
  return value ? [String(value)] : [];
}

/**
 * Bonuses (TEC-497, dealer, performance.bonus.manage, dealer_accounting):
 * accruals of the picked month with status / staff facets; select rows
 * (or every matching one) for bulk approval, approve one with an adjusted
 * amount and a note, or cancel.
 */
export function BonusesTab({ period }: { period: string }) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const [approving, setApproving] = useState<PerformanceBonus | null>(null);
  const [items, setItems] = useState<PerformanceBonus[]>([]);
  const [userFilter, setUserFilter] = useState<string[]>([]);

  const userOptions = useMemo(() => {
    const users = new Map<string, string>();
    for (const b of items) {
      if (b.user_id) users.set(String(b.user_id), b.user_name ?? "");
    }
    for (const id of userFilter) if (!users.has(id)) users.set(id, `#${id}`);
    return [...users.entries()].map(([value, label]) => ({ value, label }));
  }, [items, userFilter]);

  const { mutate: cancelBonus } = useMutation({
    mutationFn: (uuid: string) => performanceService.cancelBonus(uuid),
    onSuccess: () => {
      appToast.success(t("performance.bonuses.cancelled"));
      void qc.invalidateQueries({ queryKey: ["performance", "bonuses"] });
    },
    onError: () => appToast.error(t("performance.bonuses.cancel_failed")),
  });

  const columns = useMemo(
    () =>
      [
        createSelectColumnDef<PerformanceBonus>(),
        createColumn<PerformanceBonus>({
          id: "user",
          accessorFn: (row) => String(row.user_id ?? ""),
          labelKey: "performance.bonuses.staff",
          enableSorting: false,
          enableHiding: false,
          gridPrimary: true,
          // GET /v1/performance/bonuses filters one user_id.
          filterVariant: "select",
          filterOptions: userOptions,
          param: "user_id",
          cell: ({ row }) => (
            <span className="font-medium">{row.original.user_name}</span>
          ),
        }),
        createColumn<PerformanceBonus>({
          accessorKey: "period",
          labelKey: "performance.bonuses.period",
          enableSorting: false,
          cell: ({ row }) => row.original.period,
        }),
        createColumn<PerformanceBonus>({
          accessorKey: "rule_name",
          labelKey: "performance.bonuses.rule",
          enableSorting: false,
          cell: ({ row }) => row.original.rule_name,
        }),
        createColumn<PerformanceBonus>({
          accessorKey: "achievement_pct",
          labelKey: "performance.bonuses.achievement",
          enableSorting: false,
          cell: ({ row }) => (
            <AchievementBar pct={row.original.achievement_pct} />
          ),
        }),
        createColumn<PerformanceBonus>({
          accessorKey: "amount",
          labelKey: "performance.bonuses.amount",
          enableSorting: false,
          meta: { cellClassName: "text-end tabular-nums" },
          cell: ({ row }) =>
            format.currency(
              numberValue(row.original.amount) ?? 0,
              row.original.currency ?? "TRY",
            ),
        }),
        createColumn<PerformanceBonus>({
          accessorKey: "status",
          labelKey: "performance.bonuses.status",
          enableSorting: false,
          filterVariant: "faceted",
          filterOptions: BONUS_STATUSES.map((value) => ({
            value,
            label: value,
            labelKey: `performance.bonuses.statuses.${value}`,
          })),
          param: "status",
          cell: ({ row }) => (
            <StatusChip
              label={t(`performance.bonuses.statuses.${row.original.status}`)}
              tone={statusTone(row.original.status)}
            />
          ),
        }),
        createColumn<PerformanceBonus>({
          accessorKey: "approved_at",
          labelKey: "performance.bonuses.approved_at",
          enableSorting: false,
          defaultHidden: true,
          cell: ({ row }) =>
            row.original.approved_at
              ? format.dateTime(row.original.approved_at)
              : "—",
        }),
        createColumn<PerformanceBonus>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          cell: ({ row }) => {
            const status = row.original.status;
            const actions: EntityRowAction[] = [
              {
                id: "approve",
                label: t("performance.bonuses.approve"),
                icon: BadgeCheck,
                disabled: status !== "calculated",
                onSelect: () => setApproving(row.original),
              },
              {
                id: "cancel",
                label: t("performance.bonuses.cancel"),
                icon: XCircle,
                variant: "destructive",
                disabled: status !== "calculated" && status !== "approved",
                onSelect: () =>
                  row.original.uuid && cancelBonus(row.original.uuid),
              },
            ];
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<PerformanceBonus, unknown>[],
    [cancelBonus, format, t, userOptions],
  );

  const listState = useServerListState({
    columns,
    initialSort: null,
    persistKey: BONUSES_PERSIST_KEY,
  });
  const selectedUsers = selectedValues(listState.columnFilters, "user");
  if (selectedUsers.join(",") !== userFilter.join(",")) {
    setUserFilter(selectedUsers);
  }
  const params = useMemo(
    () => ({ ...listState.params, period }),
    [listState.params, period],
  );
  const list = useQuery({
    queryKey: performanceKeys.bonuses(params),
    queryFn: () => performanceService.bonuses(params),
  });
  const total = list.data?.total ?? 0;
  const pageItems = list.data?.items;
  if (pageItems && pageItems !== items) setItems(pageItems);

  // "Select all matching" approves every calculated bonus of the month
  // (and staff filter); the backend only resolves calculated rows.
  const bulkQuery = useMemo(() => {
    const query: Record<string, string> = { period };
    for (const [key, value] of Object.entries(listState.filterParams)) {
      if (value && key !== "status") query[key] = String(value);
    }
    return query;
  }, [listState.filterParams, period]);
  const bulkSelection = useBulkSelection({
    listQueryKey: params,
    bulkQuery,
    total,
  });

  return (
    <div className="space-y-4" data-testid="performance-bonuses">
      <SelectionBanner
        selectedCount={bulkSelection.selectedCount}
        total={total}
        showSelectAll={bulkSelection.showSelectAllBanner}
        allMatchingSelected={bulkSelection.scope.mode === "all"}
        onSelectAllMatching={bulkSelection.selectAllMatching}
        onClearSelection={bulkSelection.clearSelection}
      />
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid ?? ""}
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("performance.bonuses.empty_title")}
        emptyDescription={t("performance.bonuses.empty_description")}
        rowCount={total}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: BONUSES_PERSIST_KEY,
          rowSelection: true,
          sorting: false,
          globalFilter: false,
        }}
        renderGridItem={(row) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <span className="font-medium">{row.user_name}</span>
              <StatusChip
                label={t(`performance.bonuses.statuses.${row.status}`)}
                tone={statusTone(row.status)}
              />
            </div>
            <p className="text-muted-foreground text-xs">
              {row.rule_name} ·{" "}
              {format.currency(
                numberValue(row.amount) ?? 0,
                row.currency ?? "TRY",
              )}
            </p>
            <AchievementBar pct={row.achievement_pct} />
          </div>
        )}
        toolbarExtra={
          <>
            <BulkActionMenu
              resource="performance.bonuses"
              actions={BONUS_BULK_ACTIONS}
              scope={bulkSelection.scope}
              selectedCount={bulkSelection.selectedCount}
              onComplete={() => {
                bulkSelection.clearSelection();
                void qc.invalidateQueries({ queryKey: performanceKeys.all });
              }}
            />
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
      {approving ? (
        <BonusApproveDialog
          key={approving.uuid}
          bonus={approving}
          onClose={() => setApproving(null)}
        />
      ) : null}
    </div>
  );
}

/**
 * Approve one bonus: the calculated amount, or an adjusted amount with a
 * required note (written into the staff payment description).
 */
export function BonusApproveDialog({
  bonus,
  onClose,
}: {
  bonus: PerformanceBonus;
  onClose: () => void;
}) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const [amount, setAmount] = useState(bonus.amount ?? "");
  const [note, setNote] = useState("");
  const result = bonusApproval(bonus.amount ?? "", amount, note);
  const error = "error" in result ? result.error : null;

  const approve = useMutation({
    mutationFn: () => {
      if ("error" in result) throw new Error(result.error);
      return performanceService.approveBonus(bonus.uuid ?? "", result.body);
    },
    onSuccess: () => {
      appToast.success(t("performance.bonuses.approved"));
      void qc.invalidateQueries({ queryKey: performanceKeys.all });
      onClose();
    },
    onError: () => appToast.error(t("performance.bonuses.approve_failed")),
  });

  return (
    <Dialog open onOpenChange={(open) => !open && onClose()}>
      <DialogContent data-testid="performance-bonus-approve">
        <DialogHeader>
          <DialogTitle>{t("performance.bonuses.approve_title")}</DialogTitle>
          <DialogDescription>
            {t("performance.bonuses.approve_description", {
              name: bonus.user_name ?? "",
              amount: format.currency(
                numberValue(bonus.amount) ?? 0,
                bonus.currency ?? "TRY",
              ),
            })}
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-4">
          <div className="grid gap-2">
            <Label htmlFor="performance-bonus-amount">
              {t("performance.bonuses.amount")} ({bonus.currency})
            </Label>
            <Input
              id="performance-bonus-amount"
              data-testid="performance-bonus-amount"
              inputMode="decimal"
              value={amount}
              aria-invalid={error === "amount_invalid"}
              onChange={(e) => setAmount(e.target.value)}
            />
          </div>
          <div className="grid gap-2">
            <Label htmlFor="performance-bonus-note">
              {t("performance.bonuses.note")}
            </Label>
            <Textarea
              id="performance-bonus-note"
              data-testid="performance-bonus-note"
              value={note}
              aria-invalid={error === "note_required"}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          {error ? (
            <p
              className="text-destructive text-xs"
              data-testid="performance-bonus-error"
            >
              {t(`performance.bonuses.errors.${error}`)}
            </p>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={Boolean(error) || approve.isPending}
            onClick={() => approve.mutate()}
            data-testid="performance-bonus-approve-submit"
          >
            {t("performance.bonuses.approve")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
