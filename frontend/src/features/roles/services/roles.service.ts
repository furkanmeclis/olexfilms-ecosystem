import type { ServerListParams } from "@/components/entity";
import type { ResourceMeta } from "@/features/io/types";
import { platformRequest } from "@/lib/api/platform-request";

export type RoleSummary = {
  uuid: string;
  name: string;
  slug: string;
  description?: string | null;
  is_system?: boolean;
  org_type?: string;
};

export type RoleGrant = {
  permission: string;
  scope: string;
};

export type RoleDetail = RoleSummary & {
  permission_slugs: string[];
  grants?: RoleGrant[];
};

export type PermissionSummary = {
  uuid: string;
  name: string;
  slug: string;
  module?: string;
  scopes?: string[];
  is_sensitive?: boolean;
  super_admin_only?: boolean;
  description?: string | null;
};

/** Scopes of selected permissions only (the API grants every key it gets). */
export function pickGrants(
  slugs: string[],
  grants: Record<string, string> | undefined,
): Record<string, string> {
  const out: Record<string, string> = {};
  if (!grants) return out;
  for (const slug of slugs) {
    const scope = grants[slug];
    if (scope) out[slug] = scope;
  }
  return out;
}

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
    grants?: Record<string, string>;
  }) {
    return platformRequest<RoleDetail>("POST", "/v1/platform/roles", {
      body: { ...body, grants: pickGrants(body.permission_slugs, body.grants) },
    });
  },

  async update(
    uuid: string,
    body: {
      name?: string;
      description?: string;
      permission_slugs?: string[];
      grants?: Record<string, string>;
    },
  ) {
    const payload = body.permission_slugs
      ? { ...body, grants: pickGrants(body.permission_slugs, body.grants) }
      : body;
    return platformRequest<RoleDetail>("PATCH", `/v1/platform/roles/${uuid}`, {
      body: payload,
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
