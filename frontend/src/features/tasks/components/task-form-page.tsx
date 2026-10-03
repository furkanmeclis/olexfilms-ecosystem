"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { ListTodo } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { TaskFields } from "@/features/tasks/components/task-fields";
import {
  buildCreateBody,
  emptyTaskForm,
  serverFieldErrors,
  taskErrorMessage,
  validateTaskForm,
  type TaskFormValues,
} from "@/features/tasks/lib/tasks";
import {
  taskKeys,
  tasksService,
  type Task,
} from "@/features/tasks/services/tasks.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * Tenant > Tasks > new (TEC-221): open a task about a distributor or dealer
 * of the brand, optionally assigned to a center member with a priority and
 * a due date. The assignee is notified (TASK_ASSIGNED).
 */
export function TaskFormPage({ slug }: { slug: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const router = useRouter();
  const qc = useQueryClient();
  const canCreate = can(Permission.TasksWrite);
  const [values, setValues] = useState<TaskFormValues>(emptyTaskForm);
  const [errors, setErrors] = useState<Record<string, string>>({});

  const create = useMutation({
    mutationFn: (v: TaskFormValues) => tasksService.create(buildCreateBody(v)),
    onSuccess: (task: Task) => {
      void qc.invalidateQueries({ queryKey: taskKeys.all });
      toast.success(t("tasks.form.created"));
      router.push(routes.tenant.tasks.detail(slug, task.uuid));
    },
    onError: (err) => {
      setErrors(serverFieldErrors(err));
      toast.error(taskErrorMessage(err, t, t("tasks.form.error")));
    },
  });

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
    create.mutate(values);
  };

  const listTitle = t("tasks.list.title");
  const title = t("tasks.form.new_title");
  const header = (
    <PageHeader
      title={title}
      icon={<ListTodo className="size-6" />}
      breadcrumbs={[
        { label: t("layout.breadcrumb_home"), href: routes.tenant.home(slug) },
        { label: listTitle, href: routes.tenant.tasks.list(slug) },
        { label: title },
      ]}
    />
  );

  if (!canCreate) {
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
      <form onSubmit={onSubmit} noValidate data-testid="task-create-form">
        <Card>
          <CardContent className="space-y-6 pt-6">
            <TaskFields
              values={values}
              errors={errors}
              disabled={create.isPending}
              onChange={(p) => setValues((v) => ({ ...v, ...p }))}
            />
            <div className="flex flex-wrap justify-end gap-2">
              <Button
                type="button"
                variant="outline"
                onClick={() => router.push(routes.tenant.tasks.list(slug))}
              >
                {t("tasks.form.cancel")}
              </Button>
              <Button
                type="submit"
                data-testid="task-submit"
                disabled={create.isPending}
              >
                {t("tasks.form.create")}
              </Button>
            </div>
          </CardContent>
        </Card>
      </form>
    </div>
  );
}
