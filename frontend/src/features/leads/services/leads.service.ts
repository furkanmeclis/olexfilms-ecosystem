import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type Lead = Schemas["Lead"];
export type LeadEvent = Schemas["LeadEvent"];
export type LeadFollowUpCount = Schemas["LeadFollowUpCount"];
export type LeadCreateInput = Schemas["LeadCreateInput"];
export type LeadPatchInput = Schemas["LeadPatchInput"];
export type LeadStatusInput = Schemas["LeadStatusInput"];
export type LeadAssignInput = Schemas["LeadAssignInput"];
export type LeadTaskInput = Schemas["LeadTaskInput"];
export type LeadTargetType = Schemas["LeadTargetType"];
export type LeadSource = Schemas["LeadSource"];
export type LeadTemperature = Schemas["LeadTemperature"];
export type LeadStatus = Schemas["LeadStatus"];

/**
 * GET /v1/leads params (TEC-371): `status`, `target_type`, `source`,
 * `temperature` and `assignee_user_id` (user ids or `none`) are CSV,
 * `created_from` / `created_to` dates, `follow_up` overdue | today, `sort`
 * one of created_at, follow_up_date, status, temperature, name.
 */
export type LeadListQuery = {
  status?: string;
  target_type?: string;
  source?: string;
  temperature?: string;
  assignee_user_id?: string;
  created_from?: string;
  created_to?: string;
  follow_up?: "overdue" | "today";
  q?: string;
  sort?: string;
  limit: number;
  offset: number;
};

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

const enc = encodeURIComponent;

export const leadsService = {
  list(params: LeadListQuery) {
    return platformRequest<Page<Lead>>("GET", "/v1/leads", { query: params });
  },
  get(uuid: string) {
    return platformRequest<Lead>("GET", `/v1/leads/${enc(uuid)}`);
  },
  create(body: LeadCreateInput) {
    return platformRequest<Lead>("POST", "/v1/leads", { body });
  },
  patch(uuid: string, body: LeadPatchInput) {
    return platformRequest<Lead>("PATCH", `/v1/leads/${enc(uuid)}`, { body });
  },
  setStatus(uuid: string, body: LeadStatusInput) {
    return platformRequest<Lead>("POST", `/v1/leads/${enc(uuid)}/status`, {
      body,
    });
  },
  addNote(uuid: string, body: string) {
    return platformRequest<LeadEvent>("POST", `/v1/leads/${enc(uuid)}/notes`, {
      body: { body },
    });
  },
  assign(uuid: string, body: LeadAssignInput) {
    return platformRequest<Lead>("POST", `/v1/leads/${enc(uuid)}/assign`, {
      body,
    });
  },
  events(uuid: string) {
    return platformRequest<{ items: LeadEvent[]; total: number }>(
      "GET",
      `/v1/leads/${enc(uuid)}/events`,
      { query: { limit: 200, offset: 0 } },
    );
  },
  followUpCount() {
    return platformRequest<LeadFollowUpCount>(
      "GET",
      "/v1/leads/follow-up-count",
    );
  },
  createTask(uuid: string, body: LeadTaskInput) {
    return platformRequest<LeadEvent>("POST", `/v1/leads/${enc(uuid)}/task`, {
      body,
    });
  },
};

export const leadKeys = {
  all: ["leads"] as const,
  lists: ["leads", "list"] as const,
  list: (params: LeadListQuery) => ["leads", "list", params] as const,
  detail: (uuid: string) => ["leads", "detail", uuid] as const,
  events: (uuid: string) => ["leads", "events", uuid] as const,
  followUpCount: ["leads", "follow-up-count"] as const,
};
