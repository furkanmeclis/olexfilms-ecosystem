"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ListTodo, MessageSquare } from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { TaskFields } from "@/features/tasks/components/task-fields";
import {
  buildUpdateBody,
  COMMENT_MAX,
  isOverdue,
  serverFieldErrors,
  statusTargets,
  taskErrorMessage,
  taskFormOf,
  taskPriorityTone,
  taskStatusTone,
  validateTaskForm,
  type TaskFormValues,
} from "@/features/tasks/lib/tasks";
import {
  taskKeys,
  tasksService,
  type Task,
  type TaskStatus,
  type TaskUpdateInput,
} from "@/features/tasks/services/tasks.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="space-y-0.5">
      <dt className="text-muted-foreground text-xs">{label}</dt>
      <dd className="text-sm">{children}</dd>
    </div>
  );
}

/** Edit card: the task form seeded from the loaded task. */
function EditCard({
  task,
  onSave,
  pending,
  serverErrors,
}: {
  task: Task;
  onSave: (body: TaskUpdateInput) => void;
  pending: boolean;
  serverErrors: Record<string, string>;
}) {
  const { t } = useLocale();
  const [values, setValues] = useState<TaskFormValues>(() => taskFormOf(task));
  const [errors, setErrors] = useState<Record<string, string>>({});
  const body = buildUpdateBody(task, values);
  const dirty = Object.keys(body).length > 0;

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    const local = validateTaskForm(values);
    if (Object.keys(local).length > 0) {
      setErrors(
        Object.fromEntries(Object.entries(local).map(([k, v]) => [k, t(v)])),
      );
      return;
    }
    setErrors({});
    if (dirty) onSave(body);
  };

  return (
    <form onSubmit={onSubmit} noValidate data-testid="task-edit-form">
      <Card>
        <CardHeader>
          <CardTitle>{t("tasks.detail.edit")}</CardTitle>
        </CardHeader>
        <CardContent className="space-y-6">
          <TaskFields
            values={values}
            errors={{ ...serverErrors, ...errors }}
            disabled={pending}
            onChange={(p) => setValues((v) => ({ ...v, ...p }))}
          />
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              type="button"
              variant="outline"
              disabled={!dirty || pending}
              onClick={() => {
                setValues(taskFormOf(task));
                setErrors({});
              }}
            >
              {t("tasks.form.reset")}
            </Button>
            <Button
              type="submit"
              data-testid="task-save"
              disabled={!dirty || pending}
            >
              {t("tasks.form.save")}
            </Button>
          </div>
        </CardContent>
      </Card>
    </form>
  );
}

/** Comment stream (oldest first) with the new comment box. */
function Comments({ uuid, canWrite }: { uuid: string; canWrite: boolean }) {
  const { t, format } = useLocale();
  const qc = useQueryClient();
  const [body, setBody] = useState("");
  const comments = useQuery({
    queryKey: taskKeys.comments(uuid),
    queryFn: () => tasksService.comments(uuid),
  });
  const add = useMutation({
    mutationFn: (text: string) => tasksService.addComment(uuid, text),
    onSuccess: () => {
      setBody("");
      void qc.invalidateQueries({ queryKey: taskKeys.comments(uuid) });
      void qc.invalidateQueries({ queryKey: taskKeys.detail(uuid) });
    },
    onError: (err) => {
      toast.error(taskErrorMessage(err, t, t("tasks.comments.error")));
    },
  });
  const items = comments.data?.items ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <MessageSquare className="size-4" />
          {t("tasks.comments.title")}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        {comments.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void comments.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : items.length === 0 ? (
          <p
            className="text-muted-foreground text-sm"
            data-testid="task-comments-empty"
          >
            {comments.isLoading
              ? t("tasks.list.loading")
              : t("tasks.comments.empty")}
          </p>
        ) : (
          <ol className="space-y-3" data-testid="task-comments">
            {items.map((c) => (
              <li
                key={c.uuid}
                className="border-s-2 ps-3"
                data-testid="task-comment"
              >
                <div className="text-muted-foreground text-xs">
                  <span className="text-foreground font-medium">
                    {c.author?.name ?? t("tasks.comments.system")}
                  </span>{" "}
                  · {format.dateTime(c.created_at)}
                </div>
                <p className="text-sm whitespace-pre-wrap">{c.body}</p>
              </li>
            ))}
          </ol>
        )}
        {canWrite ? (
          <form
            className="space-y-2"
            onSubmit={(e) => {
              e.preventDefault();
              const text = body.trim();
              if (text !== "") add.mutate(text);
            }}
          >
            <Label htmlFor="task-comment">{t("tasks.comments.new")}</Label>
            <Textarea
              id="task-comment"
              data-testid="task-comment-input"
              rows={3}
              maxLength={COMMENT_MAX}
              value={body}
              onChange={(e) => setBody(e.target.value)}
            />
            <div className="flex justify-end">
              <Button
                type="submit"
                size="sm"
                data-testid="task-comment-submit"
                disabled={body.trim() === "" || add.isPending}
              >
                {t("tasks.comments.send")}
              </Button>
            </div>
          </form>
        ) : null}
      </CardContent>
    </Card>
  );
}

/**
 * Tenant > Tasks > detail (TEC-221): summary, status buttons, the edit form
 * (tasks.write) and the comment stream.
 */
export function TaskDetailPage({ slug, uuid }: { slug: string; uuid: string }) {
  const { t, format } = useLocale();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canRead = can(Permission.TasksRead);
  const canWrite = can(Permission.TasksWrite);
  const [serverErrors, setServerErrors] = useState<Record<string, string>>({});
  // Bumped after a save so the edit form re-seeds from the saved task.
  const [version, setVersion] = useState(0);

  const detail = useQuery({
    queryKey: taskKeys.detail(uuid),
    queryFn: () => tasksService.get(uuid),
    enabled: canRead && uuid !== "",
    retry: (count, err) =>
      !(isApiError(err) && (err.status === 404 || err.status === 403)) &&
      count < 2,
  });
  const update = useMutation({
    mutationFn: (body: TaskUpdateInput) => tasksService.update(uuid, body),
    onSuccess: (updated: Task, body) => {
      qc.setQueryData(taskKeys.detail(uuid), updated);
      void qc.invalidateQueries({ queryKey: ["tasks", "list"] });
      setServerErrors({});
      setVersion((v) => v + 1);
      toast.success(
        body.status && Object.keys(body).length === 1
          ? t(`tasks.status_done.${updated.status}`)
          : t("tasks.form.saved"),
      );
    },
    onError: (err) => {
      setServerErrors(serverFieldErrors(err));
      toast.error(taskErrorMessage(err, t, t("tasks.form.error")));
    },
  });

  const r = detail.data;
  const listTitle = t("tasks.list.title");
  const header = (
    <PageHeader
      title={r ? r.title : t("tasks.detail.title")}
      icon={<ListTodo className="size-6" />}
      description={r ? r.subject_organization.name : undefined}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: listTitle, href: routes.tenant.tasks.list(slug) },
        { label: r ? r.title : t("tasks.detail.title") },
      ]}
      actions={
        r && canWrite ? (
          <div
            className="flex flex-wrap gap-2"
            data-testid="task-status-actions"
          >
            {statusTargets(r).map((status: TaskStatus) => (
              <Button
                key={status}
                type="button"
                size="sm"
                variant={status === "cancelled" ? "destructive" : "outline"}
                data-status={status}
                disabled={update.isPending}
                onClick={() => update.mutate({ status })}
              >
                {t(`tasks.action.${status}`)}
              </Button>
            ))}
          </div>
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
  if (detail.isError) {
    const notFound = isApiError(detail.error) && detail.error.status === 404;
    return (
      <div className="space-y-6">
        {header}
        <ErrorState
          title={
            notFound ? t("tasks.detail.not_found") : t("common.error_generic")
          }
          onRetry={notFound ? undefined : () => void detail.refetch()}
          retryLabel={t("common.retry")}
        />
      </div>
    );
  }
  if (!r) {
    return (
      <div className="space-y-6">
        {header}
        <p className="text-muted-foreground text-sm">
          {t("tasks.list.loading")}
        </p>
      </div>
    );
  }

  const when = (v: string | null | undefined) => (v ? format.dateTime(v) : "—");
  const late = isOverdue(r);

  return (
    <div className="space-y-6" data-testid="task-detail">
      {header}
      <Card>
        <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2">
          <CardTitle>{t("tasks.detail.summary")}</CardTitle>
          <div className="flex gap-2">
            <StatusChip
              label={t(`tasks.priority.${r.priority}`)}
              tone={taskPriorityTone(r.priority)}
            />
            <StatusChip
              label={t(`tasks.status.${r.status}`)}
              tone={taskStatusTone(r.status)}
            />
          </div>
        </CardHeader>
        <CardContent>
          <dl className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            <Field label={t("tasks.columns.subject")}>
              {r.subject_organization.name} ·{" "}
              {t(`tasks.org_type.${r.subject_organization.type}`)}
            </Field>
            <Field label={t("tasks.columns.assignee")}>
              {r.assignee?.name ?? t("tasks.form.unassigned")}
            </Field>
            <Field label={t("tasks.columns.due")}>
              <span
                className={late ? "text-destructive font-medium" : undefined}
                data-overdue={late ? "true" : undefined}
              >
                {when(r.due_at)}
                {late ? ` · ${t("tasks.list.overdue")}` : ""}
              </span>
            </Field>
            <Field label={t("tasks.detail.created_by")}>
              {r.created_by?.name ?? "—"}
            </Field>
            <Field label={t("tasks.detail.created_at")}>
              {when(r.created_at)}
            </Field>
            <Field label={t("tasks.detail.closed_at")}>
              {when(r.closed_at)}
            </Field>
          </dl>
          {r.description ? (
            <p className="mt-4 text-sm whitespace-pre-wrap">{r.description}</p>
          ) : null}
        </CardContent>
      </Card>
      {canWrite ? (
        <EditCard
          key={`${r.uuid}-${version}`}
          task={r}
          pending={update.isPending}
          serverErrors={serverErrors}
          onSave={(body) => update.mutate(body)}
        />
      ) : null}
      <Comments uuid={r.uuid} canWrite={canWrite} />
    </div>
  );
}
