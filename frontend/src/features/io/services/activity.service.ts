import type { ActivityEvent, ResourceMeta } from "@/features/io/types";
import { platformRequest } from "@/lib/api/platform-request";
import type { ServerListParams } from "@/components/entity";

export type ActivityListResult = {
  items: ActivityEvent[];
  total: number;
  limit: number;
  offset: number;
};

/** `GET /v1/platform/activity`: resource/action CSV, actor = user uuid. */
export type ListActivityParams = ServerListParams & {
  resource?: string;
  action?: string;
  actor?: string;
  created_from?: string;
  created_to?: string;
};

export const activityService = {
  async meta() {
    return platformRequest<ResourceMeta>("GET", "/v1/platform/activity/meta");
  },

  async list(params: ListActivityParams) {
    return platformRequest<ActivityListResult>("GET", "/v1/platform/activity", {
      query: {
        limit: params.limit,
        offset: params.offset,
        sort: params.sort,
        q: params.q,
        resource: params.resource,
        action: params.action,
        actor: params.actor,
        created_from: params.created_from,
        created_to: params.created_to,
      },
    });
  },
};
