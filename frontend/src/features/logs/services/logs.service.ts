import type { ResourceMeta } from "@/features/io/types";
import { platformRequest } from "@/lib/api/platform-request";
import type { ServerListParams } from "@/components/entity";

export type AppLog = {
  uuid: string;
  level: "debug" | "warn" | "error";
  message: string;
  source: string;
  attrs: Record<string, unknown>;
  request_id?: string;
  created_at: string;
};

export type LogStats = {
  debug: number;
  warn: number;
  error: number;
  total: number;
};

export type PurgeRule = {
  uuid: string;
  name: string;
  enabled: boolean;
  is_system: boolean;
  levels: string[];
  source?: string;
  message_contains?: string;
  older_than_hours: number;
  interval_minutes: number;
  last_run_at?: string | null;
  last_deleted_count: number;
  last_error?: string;
  created_at: string;
  updated_at: string;
};

export type LogListResult = {
  items: AppLog[];
  total: number;
  limit: number;
  offset: number;
};

export type ListLogsParams = ServerListParams & {
  level?: string;
  source?: string;
  created_from?: string;
  created_to?: string;
};

export type PurgeLogsInput = {
  uuids?: string[];
  dry_run?: boolean;
  levels?: string[];
  source?: string;
  q?: string;
  older_than_hours?: number;
};

export type PurgeLogsResult = {
  deleted: number;
  dry_run: boolean;
};

export type RuleInput = {
  name: string;
  enabled?: boolean;
  levels?: string[];
  source?: string;
  message_contains?: string;
  older_than_hours?: number;
  interval_minutes?: number;
};

export const logsService = {
  async meta() {
    return platformRequest<ResourceMeta>("GET", "/v1/platform/logs/meta");
  },

  async rulesMeta() {
    return platformRequest<ResourceMeta>("GET", "/v1/platform/log-rules/meta");
  },

  async stats() {
    return platformRequest<LogStats>("GET", "/v1/platform/logs/stats");
  },

  async sources() {
    return platformRequest<{ items: string[] }>(
      "GET",
      "/v1/platform/logs/sources",
    );
  },

  async list(params: ListLogsParams) {
    return platformRequest<LogListResult>("GET", "/v1/platform/logs", {
      query: {
        limit: params.limit,
        offset: params.offset,
        sort: params.sort,
        q: params.q,
        level: params.level,
        source: params.source,
        created_from: params.created_from,
        created_to: params.created_to,
      },
    });
  },

  async get(uuid: string) {
    return platformRequest<AppLog>("GET", `/v1/platform/logs/${uuid}`);
  },

  async remove(uuid: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/platform/logs/${uuid}`,
    );
  },

  async purge(body: PurgeLogsInput) {
    return platformRequest<PurgeLogsResult>("POST", "/v1/platform/logs/purge", {
      body,
    });
  },

  async listRules() {
    return platformRequest<LogListResult & { items: PurgeRule[] }>(
      "GET",
      "/v1/platform/log-rules",
    );
  },

  async createRule(body: RuleInput) {
    return platformRequest<PurgeRule>("POST", "/v1/platform/log-rules", {
      body,
    });
  },

  async updateRule(uuid: string, body: Partial<RuleInput>) {
    return platformRequest<PurgeRule>(
      "PATCH",
      `/v1/platform/log-rules/${uuid}`,
      { body },
    );
  },

  async deleteRule(uuid: string) {
    return platformRequest<{ deleted: boolean }>(
      "DELETE",
      `/v1/platform/log-rules/${uuid}`,
    );
  },

  async runRule(uuid: string) {
    return platformRequest<{ rule: PurgeRule; deleted: number }>(
      "POST",
      `/v1/platform/log-rules/${uuid}/run`,
    );
  },
};
