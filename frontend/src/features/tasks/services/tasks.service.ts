import type { components } from "@/generated/api";
import type { ServerListQuery } from "@/components/entity";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Task = Schemas["Task"];
export type TaskStatus = Schemas["TaskStatus"];
export type TaskPriority = Schemas["TaskPriority"];
export type TaskUserRef = Schemas["TaskUserRef"];
export type TaskComment = Schemas["TaskComment"];
export type TaskCreateInput = Schemas["TaskCreateInput"];
export type TaskUpdateInput = Schemas["TaskUpdateInput"];

/** A distributor or dealer of the brand a task can be about. */
export type TaskSubjectOption = {
  uuid: string;
  name: string;
  type: "distributor" | "dealer";
};

/**
 * GET /v1/tasks query (TEC-214/221, list contract TEC-379): limit, offset,
 * sort (title, subject, status, priority, due_at, created_at, updated_at),
 * q (title / description), CSV status (incl. "active") / priority /
 * subject_organization_uuid, single assignee_user_uuid, due_from / due_to
 * and created_from / created_to.
 */
export type TaskListQuery = ServerListQuery;

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

const enc = encodeURIComponent;

/**
 * Center tasks (TEC-214/221) through the BFF with the active organization.
 * Only center roles hold tasks.*; the API answers 403 to anyone else.
 */
export const tasksService = {
  list(params: TaskListQuery) {
    return platformRequest<Page<Task>>("GET", "/v1/tasks", { query: params });
  },
  get(uuid: string) {
    return platformRequest<Task>("GET", `/v1/tasks/${enc(uuid)}`);
  },
  create(body: TaskCreateInput) {
    return platformRequest<Task>("POST", "/v1/tasks", { body });
  },
  update(uuid: string, body: TaskUpdateInput) {
    return platformRequest<Task>("PATCH", `/v1/tasks/${enc(uuid)}`, { body });
  },
  comments(uuid: string) {
    return platformRequest<Page<TaskComment>>(
      "GET",
      `/v1/tasks/${enc(uuid)}/comments`,
      { query: { limit: 200, offset: 0 } },
    );
  },
  addComment(uuid: string, body: string) {
    return platformRequest<TaskComment>(
      "POST",
      `/v1/tasks/${enc(uuid)}/comments`,
      { body: { body } },
    );
  },
  async assignees(): Promise<TaskUserRef[]> {
    const data = await platformRequest<{ items: TaskUserRef[] }>(
      "GET",
      "/v1/tasks/assignees",
    );
    return data.items ?? [];
  },
  /** Distributors and dealers of the brand (organizations.read scope). */
  async subjects(): Promise<TaskSubjectOption[]> {
    const data = await platformRequest<{
      items: { uuid: string; name: string; type: string }[];
    }>("GET", "/v1/tenant/organizations", { query: { limit: 200 } });
    return (data.items ?? [])
      .filter((o) => o.type === "distributor" || o.type === "dealer")
      .map((o) => ({
        uuid: o.uuid,
        name: o.name,
        type: o.type as TaskSubjectOption["type"],
      }));
  },
};

export const taskKeys = {
  all: ["tasks"] as const,
  list: (params: TaskListQuery) => ["tasks", "list", params] as const,
  detail: (uuid: string) => ["tasks", "detail", uuid] as const,
  comments: (uuid: string) => ["tasks", "comments", uuid] as const,
  assignees: ["tasks", "assignees"] as const,
  subjects: ["tasks", "subjects"] as const,
};
