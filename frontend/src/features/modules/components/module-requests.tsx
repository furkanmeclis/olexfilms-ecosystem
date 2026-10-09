"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { CheckCircle2, XCircle } from "lucide-react";
import { useMemo, useState } from "react";

import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
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
import { Textarea } from "@/components/ui/textarea";
import { modulesKeys } from "@/features/modules/hooks/use-features";
import { moduleName } from "@/features/modules/lib/labels";
import {
  MODULE_REQUEST_NOTE_MAX,
  requestNoteError,
} from "@/features/modules/lib/module-meta";
import {
  modulesService,
  type ListModuleRequestsParams,
  type ModuleRequestScope,
} from "@/features/modules/services/modules.service";
import type {
  ModuleRequestRow,
  ModuleRequestStatus,
} from "@/features/modules/types";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const MODULE_REQUEST_STATUSES: ModuleRequestStatus[] = [
  "pending",
  "approved",
  "rejected",
  "cancelled",
];

export const MODULE_REQUESTS_PERSIST_KEY: Record<ModuleRequestScope, string> = {
  tenant: "tenant-module-requests-v1",
  platform: "platform-module-requests-v1",
};

const STATUS_VARIANT: Record<
  ModuleRequestStatus,
  "warning" | "success" | "danger" | "outline"
> = {
  pending: "warning",
  approved: "success",
  rejected: "danger",
  cancelled: "outline",
};

export function ModuleRequestStatusBadge({
  status,
}: {
  status: ModuleRequestStatus;
}) {
  const { t } = useLocale();
  return (
    <Badge variant={STATUS_VARIANT[status]}>
      {t(`modules.requests.status.${status}`)}
    </Badge>
  );
}

/**
 * Note dialog of a module request and of its decision (≤ 1000 characters,
 * the backend cap). An over-long note shows an error and is not sent.
 */
export function ModuleNoteDialog({
  open,
  title,
  description,
  submitLabel,
  pending,
  onOpenChange,
  onSubmit,
}: {
  open: boolean;
  title: string;
  description?: string;
  submitLabel: string;
  pending?: boolean;
  onOpenChange: (open: boolean) => void;
  onSubmit: (note: string) => void;
}) {
  const { t } = useLocale();
  const [note, setNote] = useState("");
  const [error, setError] = useState<string | null>(null);

  function openChanged(next: boolean) {
    if (!next) {
      setNote("");
      setError(null);
    }
    onOpenChange(next);
  }

  function submit() {
    const err = requestNoteError(note);
    if (err) {
      setError(err);
      return;
    }
    onSubmit(note.trim());
  }

  return (
    <Dialog open={open} onOpenChange={openChanged}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description ? (
            <DialogDescription>{description}</DialogDescription>
          ) : null}
        </DialogHeader>
        <div className="space-y-1">
          <Textarea
            value={note}
            aria-label={t("modules.requests.note")}
            placeholder={t("modules.requests.note_placeholder")}
            aria-invalid={Boolean(error)}
            data-testid="module-request-note"
            onChange={(e) => {
              setNote(e.target.value);
              setError(requestNoteError(e.target.value));
            }}
          />
          <div className="flex items-start justify-between gap-2 text-xs">
            <span
              className="text-destructive"
              data-testid="module-request-note-error"
            >
              {error ? t(error, { max: MODULE_REQUEST_NOTE_MAX }) : null}
            </span>
            <span className="text-muted-foreground tabular-nums">
              {note.trim().length}/{MODULE_REQUEST_NOTE_MAX}
            </span>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => openChanged(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={pending || Boolean(error)}
            data-testid="module-request-submit"
            onClick={submit}
          >
            {submitLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Pending requests waiting for this queue (the tab counter). */
export function usePendingModuleRequests(
  scope: ModuleRequestScope,
  enabled = true,
) {
  return useQuery({
    queryKey: [...modulesKeys.requests(scope), "pending-count"],
    queryFn: () =>
      modulesService.requests(scope, { status: "pending", limit: 1 }),
    enabled,
    select: (page) => page.total,
  });
}

type Decision = { rows: ModuleRequestRow[]; approve: boolean };

/**
 * Module request queue (TEC-509): a distributor decides its dealers'
 * requests (tenant), the center those of distributors and of dealers
 * without a distributor (platform). Row approve / reject with a note, bulk
 * approve of the selected pending rows.
 */
export function ModuleRequestsTable({
  scope,
  canDecide,
  moduleKeys,
}: {
  scope: ModuleRequestScope;
  canDecide: boolean;
  /** Requestable (non-core) module keys for the module filter. */
  moduleKeys: string[];
}) {
  const { t, format } = useLocale();
  const queryClient = useQueryClient();
  const [rowSelection, setRowSelection] = useState<RowSelectionState>({});
  const [decision, setDecision] = useState<Decision | null>(null);
  const persistKey = MODULE_REQUESTS_PERSIST_KEY[scope];

  const keyOptions = useMemo(
    () => moduleKeys.map((key) => ({ value: key, label: moduleName(t, key) })),
    [moduleKeys, t],
  );

  const columns = useMemo<ColumnDef<ModuleRequestRow, unknown>[]>(
    () => [
      ...(canDecide ? [createSelectColumnDef<ModuleRequestRow>()] : []),
      createColumn<ModuleRequestRow>({
        accessorKey: "organization_name",
        labelKey: "modules.requests.columns.organization",
        enableSorting: true,
        gridPrimary: true,
        cell: ({ row }) => (
          <span className="font-medium">{row.original.organization_name}</span>
        ),
      }),
      createColumn<ModuleRequestRow>({
        accessorKey: "module_key",
        labelKey: "modules.columns.module",
        enableSorting: true,
        filterVariant: "faceted",
        filterOptions: keyOptions,
        param: "module_key",
        cell: ({ row }) => moduleName(t, row.original.module_key),
      }),
      createColumn<ModuleRequestRow>({
        accessorKey: "note",
        labelKey: "modules.requests.note",
        enableSorting: false,
        cell: ({ row }) => (
          <span className="line-clamp-2 max-w-md whitespace-pre-line">
            {row.original.note || "—"}
          </span>
        ),
      }),
      createColumn<ModuleRequestRow>({
        accessorKey: "requested_by_name",
        labelKey: "modules.requests.columns.requested_by",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => row.original.requested_by_name || "—",
      }),
      createColumn<ModuleRequestRow>({
        accessorKey: "created_at",
        labelKey: "modules.requests.columns.created_at",
        enableSorting: true,
        filterVariant: "date-range",
        param: "created",
        cell: ({ row }) => (
          <span className="whitespace-nowrap">
            {format.dateTime(row.original.created_at)}
          </span>
        ),
      }),
      createColumn<ModuleRequestRow>({
        accessorKey: "status",
        labelKey: "modules.columns.status",
        enableSorting: true,
        filterVariant: "faceted",
        param: "status",
        gridSecondary: true,
        filterOptions: MODULE_REQUEST_STATUSES.map((value) => ({
          value,
          labelKey: `modules.requests.status.${value}`,
          label: value,
        })),
        cell: ({ row }) => (
          <ModuleRequestStatusBadge status={row.original.status} />
        ),
      }),
      createColumn<ModuleRequestRow>({
        accessorKey: "decided_at",
        labelKey: "modules.requests.columns.decided_at",
        enableSorting: true,
        defaultHidden: true,
        cell: ({ row }) =>
          row.original.decided_at ? (
            <span className="whitespace-nowrap">
              {format.dateTime(row.original.decided_at)}
            </span>
          ) : (
            "—"
          ),
      }),
      createColumn<ModuleRequestRow>({
        id: "decision",
        accessorFn: (row) => row.decision_note,
        labelKey: "modules.requests.columns.decision",
        enableSorting: false,
        defaultHidden: true,
        cell: ({ row }) => {
          const r = row.original;
          if (r.status === "pending" || r.status === "cancelled") return "—";
          const by =
            r.decision_note === "auto"
              ? t("modules.requests.auto")
              : r.decided_by_name;
          const note = r.decision_note === "auto" ? "" : r.decision_note;
          return (
            <span className="text-muted-foreground">
              {[by, note].filter(Boolean).join(" · ") || "—"}
            </span>
          );
        },
      }),
      createColumn<ModuleRequestRow>({
        id: "actions",
        labelKey: "common.actions",
        enableSorting: false,
        enableHiding: false,
        enableResizing: false,
        cell: ({ row }) =>
          canDecide && row.original.status === "pending" ? (
            <EntityRowActions
              actions={[
                {
                  id: "approve",
                  label: t("modules.requests.approve"),
                  icon: CheckCircle2,
                  onSelect: () =>
                    setDecision({ rows: [row.original], approve: true }),
                },
                {
                  id: "reject",
                  label: t("modules.requests.reject"),
                  icon: XCircle,
                  variant: "destructive",
                  onSelect: () =>
                    setDecision({ rows: [row.original], approve: false }),
                },
              ]}
            />
          ) : null,
      }),
    ],
    [canDecide, format, keyOptions, t],
  );

  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    initialPageSize: 20,
    persistKey,
    initialColumnFilters: [{ id: "status", value: ["pending"] }],
  });
  const params: ListModuleRequestsParams = listState.params;
  const query = useQuery({
    queryKey: [...modulesKeys.requests(scope), params],
    queryFn: () => modulesService.requests(scope, params),
    placeholderData: (previous) => previous,
  });
  const rows = useMemo(() => query.data?.items ?? [], [query.data]);
  const selectedPending = useMemo(
    () => rows.filter((r) => rowSelection[r.uuid] && r.status === "pending"),
    [rowSelection, rows],
  );

  const decide = useMutation({
    mutationFn: async ({
      rows,
      approve,
      note,
    }: Decision & { note: string }) => {
      const results = await Promise.allSettled(
        rows.map((r) =>
          modulesService.decideRequest(
            scope,
            r.uuid,
            approve ? "approve" : "reject",
            note,
          ),
        ),
      );
      const failed = results.filter(
        (r): r is PromiseRejectedResult => r.status === "rejected",
      );
      return { done: results.length - failed.length, failed };
    },
    onSuccess: async ({ done, failed }, { approve }) => {
      setDecision(null);
      setRowSelection({});
      // Requests, the pending counter and the dealer matrix all change.
      await queryClient.invalidateQueries({ queryKey: modulesKeys.all });
      if (done) {
        appToast.success(
          t(
            approve
              ? "modules.requests.toast.approved"
              : "modules.requests.toast.rejected",
            { count: done },
          ),
        );
      }
      if (failed.length) {
        const first = failed[0].reason as unknown;
        appToast.error(
          isApiError(first)
            ? first.message
            : t("modules.requests.toast.failed", { count: failed.length }),
        );
      }
    },
    onError: () => appToast.error(t("modules.toast.failed")),
  });

  return (
    <>
      <EntityTable
        columns={columns}
        data={rows}
        getRowId={(row) => row.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorTitle={t("modules.requests.error")}
        errorDescription={t("modules.error.description")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("modules.requests.empty")}
        emptyDescription=""
        rowCount={query.data?.total ?? 0}
        state={{
          ...listState.tableState,
          rowSelection,
          onRowSelectionChange: setRowSelection,
        }}
        features={{ persistKey, rowSelection: canDecide }}
        toolbarExtra={
          <>
            {canDecide && selectedPending.length ? (
              <Button
                size="sm"
                data-testid="module-requests-bulk-approve"
                disabled={decide.isPending}
                onClick={() =>
                  setDecision({ rows: selectedPending, approve: true })
                }
              >
                {t("modules.requests.bulk_approve", {
                  count: selectedPending.length,
                })}
              </Button>
            ) : null}
            <EntityToolbar
              onRefresh={() => void query.refetch()}
              refreshDisabled={query.isFetching}
            />
          </>
        }
      />
      <ModuleNoteDialog
        open={Boolean(decision)}
        title={
          decision?.approve
            ? t("modules.requests.approve_title", {
                count: decision.rows.length,
              })
            : t("modules.requests.reject_title")
        }
        description={
          decision?.rows.length === 1
            ? `${decision.rows[0].organization_name} · ${moduleName(t, decision.rows[0].module_key)}`
            : undefined
        }
        submitLabel={
          decision?.approve
            ? t("modules.requests.approve")
            : t("modules.requests.reject")
        }
        pending={decide.isPending}
        onOpenChange={(open) => !open && setDecision(null)}
        onSubmit={(note) => decision && decide.mutate({ ...decision, note })}
      />
    </>
  );
}
