"use client";

import { keepPreviousData, useQuery } from "@tanstack/react-query";
import { ChevronLeft, ChevronRight, ListTodo, Plus } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { selectClass } from "@/features/tasks/components/task-fields";
import {
  DUE_FILTERS,
  isOverdue,
  listQuery,
  pageCount,
  TASK_PAGE_SIZE,
  TASK_PRIORITIES,
  TASK_STATUS_FILTERS,
  taskPriorityTone,
  taskStatusTone,
  type TaskListFilters as Filters,
} from "@/features/tasks/lib/tasks";
import {
  taskKeys,
  tasksService,
} from "@/features/tasks/services/tasks.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const ALL = "";

/**
 * Tenant > Tasks (TEC-221): center tasks of the brand with filters on
 * status (default: open + in progress), priority, assignee, subject
 * organization and due date. Center roles with tasks.read only.
 */
export function TasksListPage({ slug }: { slug: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const canRead = can(Permission.TasksRead);
  const canCreate = can(Permission.TasksWrite);
  const [filters, setFilters] = useState<Filters>({
    status: "active",
    priority: ALL,
    assignee: ALL,
    subject: ALL,
    due: ALL,
  });
  const [page, setPage] = useState(0);
  // The due presets are resolved when a filter changes, so the query key
  // stays stable between renders.
  const [anchor, setAnchor] = useState(() => new Date());

  const query = useMemo(
    () => listQuery(filters, page, anchor),
    [filters, page, anchor],
  );
  const list = useQuery({
    queryKey: taskKeys.list(query),
    queryFn: () => tasksService.list(query),
    enabled: canRead,
    placeholderData: keepPreviousData,
  });
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

  const patch = (p: Partial<Filters>) => {
    setFilters((f) => ({ ...f, ...p }));
    setPage(0);
    setAnchor(new Date());
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
        canCreate ? (
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

  const total = list.data?.total ?? 0;
  const pages = pageCount(total, TASK_PAGE_SIZE);
  const rows = list.data?.items ?? [];

  return (
    <div className="space-y-6">
      {header}
      <Card>
        <CardContent className="grid gap-4 pt-6 sm:grid-cols-2 lg:grid-cols-5">
          <div className="space-y-1.5">
            <Label htmlFor="task-filter-status">
              {t("tasks.columns.status")}
            </Label>
            <select
              id="task-filter-status"
              data-testid="task-filter-status"
              className={selectClass}
              value={filters.status}
              onChange={(e) =>
                patch({ status: e.target.value as Filters["status"] })
              }
            >
              <option value={ALL}>{t("tasks.list.all")}</option>
              {TASK_STATUS_FILTERS.map((s) => (
                <option key={s} value={s}>
                  {t(`tasks.status.${s}`)}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="task-filter-priority">
              {t("tasks.columns.priority")}
            </Label>
            <select
              id="task-filter-priority"
              data-testid="task-filter-priority"
              className={selectClass}
              value={filters.priority}
              onChange={(e) =>
                patch({ priority: e.target.value as Filters["priority"] })
              }
            >
              <option value={ALL}>{t("tasks.list.all")}</option>
              {TASK_PRIORITIES.map((p) => (
                <option key={p} value={p}>
                  {t(`tasks.priority.${p}`)}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="task-filter-assignee">
              {t("tasks.columns.assignee")}
            </Label>
            <select
              id="task-filter-assignee"
              data-testid="task-filter-assignee"
              className={selectClass}
              value={filters.assignee}
              onChange={(e) => patch({ assignee: e.target.value })}
            >
              <option value={ALL}>{t("tasks.list.all")}</option>
              {(assignees.data ?? []).map((u) => (
                <option key={u.uuid} value={u.uuid}>
                  {u.name}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="task-filter-subject">
              {t("tasks.columns.subject")}
            </Label>
            <select
              id="task-filter-subject"
              data-testid="task-filter-subject"
              className={selectClass}
              value={filters.subject}
              onChange={(e) => patch({ subject: e.target.value })}
            >
              <option value={ALL}>{t("tasks.list.all")}</option>
              {(subjects.data ?? []).map((o) => (
                <option key={o.uuid} value={o.uuid}>
                  {o.name}
                </option>
              ))}
            </select>
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="task-filter-due">{t("tasks.columns.due")}</Label>
            <select
              id="task-filter-due"
              data-testid="task-filter-due"
              className={selectClass}
              value={filters.due}
              onChange={(e) => patch({ due: e.target.value as Filters["due"] })}
            >
              <option value={ALL}>{t("tasks.list.all")}</option>
              {DUE_FILTERS.map((d) => (
                <option key={d} value={d}>
                  {t(`tasks.due.${d}`)}
                </option>
              ))}
            </select>
          </div>
        </CardContent>
      </Card>

      {list.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void list.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <Card>
          <CardContent className="pt-6">
            {list.isLoading ? (
              <p className="text-muted-foreground text-sm">
                {t("tasks.list.loading")}
              </p>
            ) : rows.length === 0 ? (
              <div className="py-8 text-center" data-testid="tasks-empty">
                <p className="font-medium">{t("tasks.list.empty_title")}</p>
                <p className="text-muted-foreground text-sm">
                  {t("tasks.list.empty_description")}
                </p>
              </div>
            ) : (
              <div className="overflow-x-auto">
                <table className="w-full text-sm" data-testid="tasks-table">
                  <thead>
                    <tr className="text-muted-foreground border-b text-xs">
                      <th className="p-2 text-start font-medium">
                        {t("tasks.columns.title")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("tasks.columns.status")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("tasks.columns.priority")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("tasks.columns.subject")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("tasks.columns.assignee")}
                      </th>
                      <th className="p-2 text-start font-medium">
                        {t("tasks.columns.due")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {rows.map((r) => {
                      const late = isOverdue(r);
                      return (
                        <tr
                          key={r.uuid}
                          className="hover:bg-accent/50 border-b align-top last:border-0"
                          data-testid="task-row"
                          data-uuid={r.uuid}
                        >
                          <td className="p-2">
                            <Link
                              href={routes.tenant.tasks.detail(slug, r.uuid)}
                              className="font-medium hover:underline"
                            >
                              {r.title}
                            </Link>
                            {r.comment_count > 0 ? (
                              <div className="text-muted-foreground text-xs">
                                {t("tasks.list.comments", {
                                  count: r.comment_count,
                                })}
                              </div>
                            ) : null}
                          </td>
                          <td className="p-2">
                            <StatusChip
                              label={t(`tasks.status.${r.status}`)}
                              tone={taskStatusTone(r.status)}
                            />
                          </td>
                          <td className="p-2">
                            <StatusChip
                              label={t(`tasks.priority.${r.priority}`)}
                              tone={taskPriorityTone(r.priority)}
                            />
                          </td>
                          <td className="p-2">
                            {r.subject_organization.name}
                            <div className="text-muted-foreground text-xs">
                              {t(
                                `tasks.org_type.${r.subject_organization.type}`,
                              )}
                            </div>
                          </td>
                          <td className="p-2">
                            {r.assignee?.name ?? (
                              <span className="text-muted-foreground">
                                {t("tasks.form.unassigned")}
                              </span>
                            )}
                          </td>
                          <td
                            className={cn(
                              "p-2 text-xs whitespace-nowrap",
                              late
                                ? "text-destructive font-medium"
                                : "text-muted-foreground",
                            )}
                            data-overdue={late ? "true" : undefined}
                          >
                            {r.due_at ? format.dateTime(r.due_at) : "—"}
                            {late ? <div>{t("tasks.list.overdue")}</div> : null}
                          </td>
                        </tr>
                      );
                    })}
                  </tbody>
                </table>
              </div>
            )}
            <div
              className={cn(
                "mt-4 flex flex-wrap items-center justify-between gap-2",
                rows.length === 0 && page === 0 && "hidden",
              )}
            >
              <p
                className="text-muted-foreground text-sm"
                data-testid="page-info"
              >
                {t("tasks.list.page", { page: page + 1, pages, total })}
              </p>
              <div className="flex gap-2">
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page === 0 || list.isFetching}
                  onClick={() => setPage((p) => Math.max(0, p - 1))}
                >
                  <ChevronLeft className="size-4 rtl:rotate-180" />
                  {t("tasks.list.prev")}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={page + 1 >= pages || list.isFetching}
                  onClick={() => setPage((p) => p + 1)}
                >
                  {t("tasks.list.next")}
                  <ChevronRight className="size-4 rtl:rotate-180" />
                </Button>
              </div>
            </div>
          </CardContent>
        </Card>
      )}
    </div>
  );
}
