"use client";

import { useQuery } from "@tanstack/react-query";

import { AsyncCombobox } from "@/components/ui/async-combobox";
import { DateTimePicker } from "@/components/ui/date-time-picker";
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
        <AsyncCombobox
          id="task-subject"
          data-testid="task-subject"
          value={values.subjectUuid}
          disabled={disabled}
          clearable
          placeholder={t("tasks.form.subject_placeholder")}
          aria-invalid={errors.subject_organization_uuid ? true : undefined}
          onValueChange={(value) => onChange({ subjectUuid: value })}
          options={(subjects.data ?? []).map((o) => ({
            value: o.uuid,
            label: `${o.name} · ${t(`tasks.org_type.${o.type}`)}`,
          }))}
        />
        <FieldError
          id="task-subject-error"
          message={errors.subject_organization_uuid}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="task-assignee">{t("tasks.form.assignee")}</Label>
        <AsyncCombobox
          id="task-assignee"
          data-testid="task-assignee"
          value={values.assigneeUuid}
          disabled={disabled}
          clearable
          placeholder={t("tasks.form.unassigned")}
          aria-invalid={errors.assignee_user_uuid ? true : undefined}
          onValueChange={(value) => onChange({ assigneeUuid: value })}
          options={(assignees.data ?? []).map((u) => ({
            value: u.uuid,
            label: u.name,
          }))}
        />
        <FieldError
          id="task-assignee-error"
          message={errors.assignee_user_uuid}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="task-priority">{t("tasks.form.priority")}</Label>
        <Select
          value={values.priority}
          disabled={disabled}
          onValueChange={(value) =>
            onChange({ priority: value as TaskFormValues["priority"] })
          }
        >
          <SelectTrigger id="task-priority" data-testid="task-priority">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {TASK_PRIORITIES.map((p) => (
              <SelectItem key={p} value={p}>
                {t(`tasks.priority.${p}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="task-due">{t("tasks.form.due_at")}</Label>
        <DateTimePicker
          id="task-due"
          data-testid="task-due"
          value={values.dueLocal}
          disabled={disabled}
          aria-invalid={errors.due_at ? true : undefined}
          onChange={(value) => onChange({ dueLocal: value })}
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
