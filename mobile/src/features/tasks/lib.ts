export type TaskStatus = 'open' | 'in_progress' | 'done' | 'cancelled';
export type TaskPriority = 'low' | 'normal' | 'high' | 'urgent';
export type TaskTone = 'neutral' | 'progress' | 'done' | 'problem';

export const TASK_STATUSES: TaskStatus[] = ['open', 'in_progress', 'done', 'cancelled'];
export const TASK_PRIORITIES: TaskPriority[] = ['low', 'normal', 'high', 'urgent'];

export function statusTone(s: TaskStatus): TaskTone {
  if (s === 'done') return 'done';
  if (s === 'cancelled') return 'neutral';
  return 'progress';
}

export function priorityTone(p: TaskPriority): TaskTone {
  if (p === 'urgent') return 'problem';
  if (p === 'high') return 'progress';
  return 'neutral';
}

export function isClosed(s: TaskStatus): boolean {
  return s === 'done' || s === 'cancelled';
}

/** Overdue: still open and past its deadline. */
export function isOverdue(task: { status: TaskStatus; due_at: string | null }, now: Date = new Date()): boolean {
  if (!task.due_at || isClosed(task.status)) return false;
  const due = new Date(task.due_at).getTime();
  return !Number.isNaN(due) && due < now.getTime();
}

/** Status moves offered on the detail screen (everything but the current one). */
export function nextStatuses(current: TaskStatus): TaskStatus[] {
  if (current === 'open') return ['in_progress', 'done', 'cancelled'];
  if (current === 'in_progress') return ['done', 'open', 'cancelled'];
  return ['open', 'in_progress'];
}

/** Query value of the status filter chips (`active` = open + in progress). */
export type StatusFilter = 'active' | 'done' | 'cancelled' | 'all';

export function statusQuery(f: StatusFilter): string | undefined {
  return f === 'all' ? undefined : f;
}

export const QUICK_DUES = ['today', 'tomorrow', 'week'] as const;
export type QuickDue = (typeof QUICK_DUES)[number];

/** Due date of a quick choice in the date input's form (YYYY-MM-DD, local day). */
export function quickDueDate(kind: QuickDue, now: Date = new Date()): string {
  const days = kind === 'today' ? 0 : kind === 'tomorrow' ? 1 : 7;
  const d = new Date(now.getFullYear(), now.getMonth(), now.getDate() + days);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}

/** What still blocks creating a task, in screen order (empty = ready). */
export function taskFormMissing(v: { title: string; subject: boolean; due: string }): ('title' | 'subject' | 'due')[] {
  const out: ('title' | 'subject' | 'due')[] = [];
  if (!v.title.trim()) out.push('title');
  if (!v.subject) out.push('subject');
  if (dueDateToIso(v.due) === undefined) out.push('due');
  return out;
}

/** Date input (YYYY-MM-DD) to the RFC3339 instant the API wants; empty clears, invalid is undefined. */
export function dueDateToIso(input: string): string | null | undefined {
  const v = input.trim();
  if (!v) return null;
  if (!/^\d{4}-\d{2}-\d{2}$/.test(v)) return undefined;
  const d = new Date(`${v}T17:00:00`);
  return Number.isNaN(d.getTime()) ? undefined : d.toISOString();
}
