"use client";

import { useQuery } from "@tanstack/react-query";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  DESCRIPTION_MAX,
  TASK_PRIORITIES,
  TITLE_MAX,
  type TaskFormValues,
} from "@/features/tasks/lib/tasks";
import {
  taskKeys,
  tasksService,
} from "@/features/tasks/services/tasks.service";
import { useLocale } from "@/providers/locale-provider";

export const selectClass =
  "border-input bg-background h-9 w-full rounded-md border px-2 text-sm";

function FieldError({ id, message }: { id: string; message?: string }) {
  if (!message) return null;
  return (
    <p id={id} className="text-destructive text-xs" role="alert">
      {message}
    </p>
  );
}

/**
 * Fields of the task form (create and the detail page's edit card): title,
 * subject (a distributor or dealer of the brand), assignee (a center
 * member), priority, due date and description. errors maps a field to a
 * translated message.
 */
export function TaskFields({
  values,
  onChange,
  errors,
  disabled,
}: {
  values: TaskFormValues;
  onChange: (patch: Partial<TaskFormValues>) => void;
  errors: Record<string, string>;
  disabled?: boolean;
}) {
  const { t } = useLocale();
  const subjects = useQuery({
    queryKey: taskKeys.subjects,
    queryFn: () => tasksService.subjects(),
    staleTime: 60_000,
  });
  const assignees = useQuery({
    queryKey: taskKeys.assignees,
    queryFn: () => tasksService.assignees(),
    staleTime: 60_000,
  });

  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <div className="space-y-1.5 sm:col-span-2">
        <Label htmlFor="task-title">
          {t("tasks.form.title")}
          <span className="text-destructive ms-1">*</span>
        </Label>
        <Input
          id="task-title"
          data-testid="task-title"
          maxLength={TITLE_MAX}
          value={values.title}
          disabled={disabled}
          aria-invalid={errors.title ? true : undefined}
          onChange={(e) => onChange({ title: e.target.value })}
        />
        <FieldError id="task-title-error" message={errors.title} />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="task-subject">
          {t("tasks.form.subject")}
          <span className="text-destructive ms-1">*</span>
        </Label>
        <select
          id="task-subject"
          data-testid="task-subject"
          className={selectClass}
          value={values.subjectUuid}
          disabled={disabled}
          aria-invalid={errors.subject_organization_uuid ? true : undefined}
          onChange={(e) => onChange({ subjectUuid: e.target.value })}
        >
          <option value="">{t("tasks.form.subject_placeholder")}</option>
          {(subjects.data ?? []).map((o) => (
            <option key={o.uuid} value={o.uuid}>
              {o.name} · {t(`tasks.org_type.${o.type}`)}
            </option>
          ))}
        </select>
        <FieldError
          id="task-subject-error"
          message={errors.subject_organization_uuid}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="task-assignee">{t("tasks.form.assignee")}</Label>
        <select
          id="task-assignee"
          data-testid="task-assignee"
          className={selectClass}
          value={values.assigneeUuid}
          disabled={disabled}
          aria-invalid={errors.assignee_user_uuid ? true : undefined}
          onChange={(e) => onChange({ assigneeUuid: e.target.value })}
        >
          <option value="">{t("tasks.form.unassigned")}</option>
          {(assignees.data ?? []).map((u) => (
            <option key={u.uuid} value={u.uuid}>
              {u.name}
            </option>
          ))}
        </select>
        <FieldError
          id="task-assignee-error"
          message={errors.assignee_user_uuid}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="task-priority">{t("tasks.form.priority")}</Label>
        <select
          id="task-priority"
          data-testid="task-priority"
          className={selectClass}
          value={values.priority}
          disabled={disabled}
          onChange={(e) =>
            onChange({
              priority: e.target.value as TaskFormValues["priority"],
            })
          }
        >
          {TASK_PRIORITIES.map((p) => (
            <option key={p} value={p}>
              {t(`tasks.priority.${p}`)}
            </option>
          ))}
        </select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="task-due">{t("tasks.form.due_at")}</Label>
        <Input
          id="task-due"
          data-testid="task-due"
          type="datetime-local"
          dir="ltr"
          value={values.dueLocal}
          disabled={disabled}
          aria-invalid={errors.due_at ? true : undefined}
          onChange={(e) => onChange({ dueLocal: e.target.value })}
        />
        <p className="text-muted-foreground text-xs">
          {t("tasks.form.due_hint")}
        </p>
        <FieldError id="task-due-error" message={errors.due_at} />
      </div>
      <div className="space-y-1.5 sm:col-span-2">
        <Label htmlFor="task-description">{t("tasks.form.description")}</Label>
        <Textarea
          id="task-description"
          data-testid="task-description"
          rows={5}
          maxLength={DESCRIPTION_MAX}
          value={values.description}
          disabled={disabled}
          aria-invalid={errors.description ? true : undefined}
          onChange={(e) => onChange({ description: e.target.value })}
        />
        <FieldError id="task-description-error" message={errors.description} />
      </div>
    </div>
  );
}
