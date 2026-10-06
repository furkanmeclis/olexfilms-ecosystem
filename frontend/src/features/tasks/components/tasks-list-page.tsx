"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  CircleCheck,
  Eye,
  Flag,
  ListChecks,
  ListTodo,
  Play,
  Plus,
  RotateCcw,
  UserPlus,
} from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import {
  EntityRowActions,
  EntityTable,
  EntityToolbar,
  useServerListState,
  type EntityRowAction,
} from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  BulkActionMenu,
  SelectionBanner,
  useBulkSelection,
  type BulkActionDef,
} from "@/features/bulk-engine";
import {
  DUE_FILTERS,
  dueRange,
  isOverdue,
  TASK_PRIORITIES,
  TASK_STATUS_FILTERS,
  TASK_STATUSES,
  taskErrorMessage,
  taskPriorityTone,
  taskStatusTone,
  type DueFilter,
} from "@/features/tasks/lib/tasks";
import {
  taskKeys,
  tasksService,
  type Task,
  type TaskListQuery,
  type TaskPriority,
  type TaskStatus,
  type TaskUpdateInput,
} from "@/features/tasks/services/tasks.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

export const TASKS_PERSIST_KEY = "tenant-tasks-v1";

/**
 * `POST /v1/tasks/bulk` (TEC-212, TEC-379): assign to a center member,
 * set_status and set_priority; all three are undoable. Targets are the
 * selected ids or every task matching the list filters.
 */
export const TASK_BULK_ACTIONS: BulkActionDef[] = [
  {
    id: "assign",
    label_key: "bulk.actions.tasks.assign",
    permission: Permission.TasksWrite,
    reversible: true,
    params: [
      {
        key: "assignee_uuid",
        kind: "uuid",
        required: true,
        label_key: "bulk.params.assignee",
      },
    ],
    icon: UserPlus,
  },
  {
    id: "set_status",
    label_key: "bulk.actions.tasks.set_status",
    permission: Permission.TasksWrite,
    reversible: true,
    confirm_key: "bulk.confirm.tasks.set_status",
    params: [
      {
        key: "status",
        kind: "enum",
        required: true,
        label_key: "bulk.params.task_status",
        options: TASK_STATUSES,
      },
    ],
    icon: ListChecks,
  },
  {
    id: "set_priority",
    label_key: "bulk.actions.tasks.set_priority",
    permission: Permission.TasksWrite,
    reversible: true,
    params: [
      {
        key: "priority",
        kind: "enum",
        required: true,
        label_key: "bulk.params.task_priority",
        options: TASK_PRIORITIES,
      },
    ],
    icon: Flag,
  },
];

/** Status filter the list opens with: open + in progress. */
const INITIAL_FILTERS = [{ id: "status", value: ["active"] }];

/** Due preset of the toolbar, resolved once when it is picked. */
type DuePreset = { filter: DueFilter | ""; at: Date };

function enumOptions(values: readonly string[], prefix: string) {
  return values.map((value) => ({
    value,
    label: value,
    labelKey: `${prefix}.${value}`,
  }));
}

/**
 * Tenant > Tasks (TEC-221, TEC-380): server DataTable over GET /v1/tasks
 * with sort, `q`, status (default: open + in progress) / priority / subject
 * facets, an assignee select, due and created ranges, plus the due presets
 * in the toolbar. Writers change status / priority inline or from the row
 * menu and select rows (or every matching task) for the bulk actions.
 * Center roles with tasks.read only.
 */
export function TasksListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const qc = useQueryClient();
  const canRead = can(Permission.TasksRead);
  const canWrite = can(Permission.TasksWrite);
  const [preset, setPreset] = useState<DuePreset>(() => ({
    filter: "",
    at: new Date(),
  }));

  const assignees = useQuery({
    queryKey: taskKeys.assignees,
    queryFn: () => tasksService.assignees(),
    enabled: canRead,
    staleTime: 60_000,
  });
  const subjects = useQuery({
    queryKey: taskKeys.subjects,
    queryFn: () => tasksService.subjects(),
    enabled: canRead,
    staleTime: 60_000,
  });
  const assigneeOptions = useMemo(
    () => (assignees.data ?? []).map((u) => ({ value: u.uuid, label: u.name })),
    [assignees.data],
  );
  const subjectOptions = useMemo(
    () => (subjects.data ?? []).map((o) => ({ value: o.uuid, label: o.name })),
    [subjects.data],
  );

  const update = useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: TaskUpdateInput }) =>
      tasksService.update(uuid, body),
    onSuccess: (task, { body }) => {
      void qc.invalidateQueries({ queryKey: ["tasks", "list"] });
      void qc.invalidateQueries({ queryKey: taskKeys.detail(task.uuid) });
      toast.success(
        body.status
          ? t(`tasks.status_done.${body.status}`)
          : t("tasks.list.priority_done"),
      );
    },
    onError: (err) => {
      toast.error(taskErrorMessage(err, t, t("tasks.form.error")));
    },
  });
  const mutateTask = update.mutate;

  const baseColumns = useMemo(
    () =>
      [
        createColumn<Task>({
          accessorKey: "title",
          labelKey: "tasks.columns.title",
          enableSorting: true,
          enableHiding: false,
          gridPrimary: true,
          cell: ({ row }) => (
            <div data-testid="task-row" data-uuid={row.original.uuid}>
              <Link
                href={routes.tenant.tasks.detail(slug, row.original.uuid)}
                className="font-medium hover:underline"
                onClick={(event) => event.stopPropagation()}
              >
                {row.original.title}
              </Link>
              {row.original.comment_count > 0 ? (
                <div className="text-muted-foreground text-xs">
                  {t("tasks.list.comments", {
                    count: row.original.comment_count,
                  })}
                </div>
              ) : null}
            </div>
          ),
        }),
        createColumn<Task>({
          accessorKey: "status",
          labelKey: "tasks.columns.status",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: enumOptions(TASK_STATUS_FILTERS, "tasks.status"),
          param: "status",
          editVariant: "select",
          editOptions: TASK_STATUSES.map((s) => ({
            value: s,
            label: t(`tasks.status.${s}`),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`tasks.status.${row.original.status}`)}
              tone={taskStatusTone(row.original.status)}
            />
          ),
        }),
        createColumn<Task>({
          accessorKey: "priority",
          labelKey: "tasks.columns.priority",
          enableSorting: true,
          filterVariant: "faceted",
          filterOptions: enumOptions(TASK_PRIORITIES, "tasks.priority"),
          param: "priority",
          editVariant: "select",
          editOptions: TASK_PRIORITIES.map((p) => ({
            value: p,
            label: t(`tasks.priority.${p}`),
          })),
          cell: ({ row }) => (
            <StatusChip
              label={t(`tasks.priority.${row.original.priority}`)}
              tone={taskPriorityTone(row.original.priority)}
            />
          ),
        }),
        createColumn<Task>({
          id: "subject",
          accessorFn: (row) => row.subject_organization.name,
          labelKey: "tasks.columns.subject",
          enableSorting: true,
          gridSecondary: true,
          filterVariant: "faceted",
          filterOptions: subjectOptions,
          enableColumnFilter: subjectOptions.length > 0,
          param: "subject_organization_uuid",
          cell: ({ row }) => (
            <div>
              {row.original.subject_organization.name}
              <div className="text-muted-foreground text-xs">
                {t(`tasks.org_type.${row.original.subject_organization.type}`)}
              </div>
            </div>
          ),
        }),
        createColumn<Task>({
          id: "assignee",
          accessorFn: (row) => row.assignee?.name ?? "",
          labelKey: "tasks.columns.assignee",
          enableSorting: false,
          filterVariant: "select",
          filterOptions: assigneeOptions,
          enableColumnFilter: assigneeOptions.length > 0,
          param: "assignee_user_uuid",
          cell: ({ row }) =>
            row.original.assignee?.name ?? (
              <span className="text-muted-foreground">
                {t("tasks.form.unassigned")}
              </span>
            ),
        }),
        createColumn<Task>({
          accessorKey: "due_at",
          labelKey: "tasks.columns.due",
          enableSorting: true,
          filterVariant: "date-range",
          param: "due",
          cell: ({ row }) => {
            const late = isOverdue(row.original);
            return (
              <div
                className={cn(
                  "text-xs whitespace-nowrap",
                  late
                    ? "text-destructive font-medium"
                    : "text-muted-foreground",
                )}
                data-overdue={late ? "true" : undefined}
              >
                {row.original.due_at
                  ? format.dateTime(row.original.due_at)
                  : "—"}
                {late ? <div>{t("tasks.list.overdue")}</div> : null}
              </div>
            );
          },
        }),
        createColumn<Task>({
          accessorKey: "created_at",
          labelKey: "tasks.columns.created_at",
          enableSorting: true,
          filterVariant: "date-range",
          param: "created",
          cell: ({ row }) => (
            <span className="text-xs whitespace-nowrap">
              {format.dateTime(row.original.created_at)}
            </span>
          ),
        }),
        createColumn<Task>({
          accessorKey: "updated_at",
          labelKey: "tasks.columns.updated_at",
          enableSorting: true,
          enableColumnFilter: false,
          defaultHidden: true,
          cell: ({ row }) => (
            <span className="text-xs whitespace-nowrap">
              {format.dateTime(row.original.updated_at)}
            </span>
          ),
        }),
        createColumn<Task>({
          id: "actions",
          labelKey: "common.actions",
          enableSorting: false,
          enableHiding: false,
          enableResizing: false,
          enableColumnFilter: false,
          cell: ({ row }) => {
            const task = row.original;
            const closed =
              task.status === "done" || task.status === "cancelled";
            const actions: EntityRowAction[] = [
              {
                id: "open",
                label: t("common.open"),
                icon: Eye,
                onSelect: () =>
                  router.push(routes.tenant.tasks.detail(slug, task.uuid)),
              },
            ];
            if (canWrite && task.status === "open") {
              actions.push({
                id: "in_progress",
                label: t("tasks.action.in_progress"),
                icon: Play,
                onSelect: () =>
                  mutateTask({
                    uuid: task.uuid,
                    body: { status: "in_progress" },
                  }),
              });
            }
            if (canWrite && !closed) {
              actions.push({
                id: "done",
                label: t("tasks.action.done"),
                icon: CircleCheck,
                onSelect: () =>
                  mutateTask({ uuid: task.uuid, body: { status: "done" } }),
              });
            }
            if (canWrite && closed) {
              actions.push({
                id: "reopen",
                label: t("tasks.action.open"),
                icon: RotateCcw,
                onSelect: () =>
                  mutateTask({ uuid: task.uuid, body: { status: "open" } }),
              });
            }
            return <EntityRowActions actions={actions} />;
          },
        }),
      ] as ColumnDef<Task, unknown>[],
    [
      assigneeOptions,
      canWrite,
      format,
      mutateTask,
      router,
      slug,
      subjectOptions,
      t,
    ],
  );
  const columns = useMemo(
    () =>
      canWrite ? [createSelectColumnDef<Task>(), ...baseColumns] : baseColumns,
    [baseColumns, canWrite],
  );

  // Column meta drives the params: facets (CSV), assignee, due / created.
  const listState = useServerListState({
    columns,
    initialSort: "-created_at",
    persistKey: TASKS_PERSIST_KEY,
    initialColumnFilters: INITIAL_FILTERS,
  });

  // A due range typed in the column filter replaces the toolbar preset.
  const dueKey = JSON.stringify(
    listState.columnFilters.find((f) => f.id === "due_at")?.value ?? null,
  );
  const [seenDueKey, setSeenDueKey] = useState(dueKey);
  if (dueKey !== seenDueKey) {
    setSeenDueKey(dueKey);
    if (dueKey !== "null" && preset.filter) {
      setPreset({ filter: "", at: new Date() });
    }
  }
  const pickPreset = (filter: DueFilter | "") => {
    setPreset({ filter, at: new Date() });
    if (filter) {
      listState.onColumnFiltersChange((prev) =>
        prev.filter((f) => f.id !== "due_at"),
      );
    } else {
      listState.setPagination((p) => ({ ...p, pageIndex: 0 }));
    }
  };

  const dueParams = useMemo(
    () => (preset.filter ? dueRange(preset.filter, preset.at) : null),
    [preset],
  );
  const params = useMemo<TaskListQuery>(() => {
    if (!dueParams) return listState.params;
    const rest = { ...listState.params };
    delete rest.due_from;
    delete rest.due_to;
    return { ...rest, ...dueParams };
  }, [dueParams, listState.params]);

  const list = useQuery({
    queryKey: taskKeys.list(params),
    queryFn: () => tasksService.list(params),
    enabled: canRead,
  });
  const total = list.data?.total ?? 0;

  // "Select all matching" uses the list filters, search, due preset and
  // sort (GET /v1/tasks params; `mine` is not sent).
  const bulkQuery = useMemo(() => {
    const query: Record<string, string> = {};
    for (const [key, value] of Object.entries(params)) {
      if (key === "limit" || key === "offset") continue;
      if (value !== undefined && value !== "") query[key] = String(value);
    }
    return query;
  }, [params]);

  const bulkSelection = useBulkSelection({
    listQueryKey: params,
    bulkQuery,
    total,
  });

  const onCellEdit = ({
    row,
    columnId,
    value,
  }: {
    row: Task;
    columnId: string;
    value: unknown;
  }) => {
    const next = String(value ?? "");
    if (columnId === "status" && next && next !== row.status) {
      mutateTask({ uuid: row.uuid, body: { status: next as TaskStatus } });
    }
    if (columnId === "priority" && next && next !== row.priority) {
      mutateTask({
        uuid: row.uuid,
        body: { priority: next as TaskPriority },
      });
    }
  };

  const title = t("tasks.list.title");
  const header = (
    <PageHeader
      title={title}
      icon={<ListTodo className="size-6" />}
      description={t("tasks.list.description")}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: title },
      ]}
      actions={
        canWrite ? (
          <Button asChild>
            <Link
              href={routes.tenant.tasks.create(slug)}
              data-testid="task-new"
            >
              <Plus className="size-4" />
              {t("tasks.list.new")}
            </Link>
          </Button>
        ) : null
      }
    />
  );

  if (!canRead) {
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("tasks.list.forbidden")}
        />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {header}
      <div
        className="flex flex-wrap gap-2"
        role="tablist"
        aria-label={t("tasks.columns.due")}
      >
        {(["", ...DUE_FILTERS] as const).map((value) => (
          <Button
            key={value || "all"}
            type="button"
            role="tab"
            aria-selected={preset.filter === value}
            variant={preset.filter === value ? "default" : "outline"}
            size="sm"
            data-testid={`task-due-${value || "all"}`}
            onClick={() => pickPreset(value)}
          >
            {value ? t(`tasks.due.${value}`) : t("tasks.list.all")}
          </Button>
        ))}
      </div>
      {canWrite ? (
        <SelectionBanner
          selectedCount={bulkSelection.selectedCount}
          total={total}
          showSelectAll={bulkSelection.showSelectAllBanner}
          allMatchingSelected={bulkSelection.scope.mode === "all"}
          onSelectAllMatching={bulkSelection.selectAllMatching}
          onClearSelection={bulkSelection.clearSelection}
        />
      ) : null}
      <EntityTable
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(row) => row.uuid}
        onRowClick={(row) =>
          router.push(routes.tenant.tasks.detail(slug, row.uuid))
        }
        isLoading={list.isLoading}
        isError={list.isError}
        onRetry={() => void list.refetch()}
        emptyTitle={t("tasks.list.empty_title")}
        emptyDescription={t("tasks.list.empty_description")}
        rowCount={total}
        state={{
          ...listState.tableState,
          rowSelection: bulkSelection.rowSelection,
          onRowSelectionChange: bulkSelection.onRowSelectionChange,
        }}
        features={{
          persistKey: TASKS_PERSIST_KEY,
          rowSelection: canWrite,
          inlineEdit: canWrite,
        }}
        onCellEdit={canWrite ? onCellEdit : undefined}
        renderGridItem={(task) => (
          <div className="space-y-2">
            <div className="flex items-start justify-between gap-2">
              <Link
                href={routes.tenant.tasks.detail(slug, task.uuid)}
                className="font-medium hover:underline"
                onClick={(event) => event.stopPropagation()}
              >
                {task.title}
              </Link>
              <StatusChip
                label={t(`tasks.status.${task.status}`)}
                tone={taskStatusTone(task.status)}
              />
            </div>
            <p className="text-muted-foreground text-xs">
              {task.subject_organization.name} ·{" "}
              {t(`tasks.priority.${task.priority}`)}
              {task.assignee ? ` · ${task.assignee.name}` : ""}
            </p>
            {task.due_at ? (
              <p
                className={cn(
                  "text-xs",
                  isOverdue(task)
                    ? "text-destructive font-medium"
                    : "text-muted-foreground",
                )}
              >
                {t("tasks.columns.due")}: {format.dateTime(task.due_at)}
              </p>
            ) : null}
          </div>
        )}
        toolbarExtra={
          <>
            {canWrite ? (
              <BulkActionMenu
                resource="tasks"
                actions={TASK_BULK_ACTIONS}
                scope={bulkSelection.scope}
                selectedCount={bulkSelection.selectedCount}
                paramOptions={{ assignee_uuid: assigneeOptions }}
                onComplete={() => {
                  bulkSelection.clearSelection();
                  void qc.invalidateQueries({ queryKey: taskKeys.all });
                }}
              />
            ) : null}
            <EntityToolbar
              onRefresh={() => void list.refetch()}
              refreshDisabled={list.isFetching}
            />
          </>
        }
      />
    </div>
  );
}
