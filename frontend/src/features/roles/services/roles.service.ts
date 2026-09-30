import type { ServerListParams } from "@/components/entity";
import type { ResourceMeta } from "@/features/io/types";
import { platformRequest } from "@/lib/api/platform-request";

export type RoleSummary = {
  uuid: string;
  name: string;
  slug: string;
  description?: string | null;
  is_system?: boolean;
};

export type RoleDetail = RoleSummary & {
  permission_slugs: string[];
};

export type PermissionSummary = {
  uuid: string;
  name: string;
  slug: string;
};

export type RoleListResult = {
  items: RoleSummary[];
  total: number;
  limit: number;
  offset: number;
};

export type PermissionListResult = {
  items: PermissionSummary[];
  total: number;
  limit: number;
  offset: number;
};

export const rolesService = {
  async list(params: ServerListParams) {
    return platformRequest<RoleListResult>("GET", "/v1/platform/roles", {
      query: {
        limit: params.limit,
        offset: params.offset,
        q: params.q,
      },
    });
  },

  async meta() {
    return platformRequest<ResourceMeta>("GET", "/v1/platform/roles/meta");
  },

  async get(uuid: string) {
    return platformRequest<RoleDetail>("GET", `/v1/platform/roles/${uuid}`);
  },

  async create(body: {
    name: string;
    slug: string;
    description?: string;
    permission_slugs: string[];
  }) {
    return platformRequest<RoleDetail>("POST", "/v1/platform/roles", { body });
  },

  async update(
    uuid: string,
    body: {
      name?: string;
      description?: string;
      permission_slugs?: string[];
    },
  ) {
    return platformRequest<RoleDetail>("PATCH", `/v1/platform/roles/${uuid}`, {
      body,
    });
  },

  async remove(uuid: string) {
    return platformRequest<{ status: string }>(
      "DELETE",
      `/v1/platform/roles/${uuid}`,
    );
  },

  async listPermissions(params: ServerListParams) {
    return platformRequest<PermissionListResult>(
      "GET",
      "/v1/platform/permissions",
      {
        query: {
          limit: params.limit,
          offset: params.offset,
          q: params.q,
        },
      },
    );
  },
};
