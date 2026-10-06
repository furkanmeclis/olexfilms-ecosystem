import type { ServerListParams } from "@/components/entity";
import { platformRequest } from "@/lib/api/platform-request";
import type { RoleSummary } from "@/features/roles/services/roles.service";

export type UserAuthMethod = {
  kind: "password" | "passkey" | "oauth";
  provider?: string;
  label?: string | null;
  linked_at: string;
};

export type PublicUser = {
  uuid: string;
  email: string;
  name: string;
  surname: string;
  status: string;
  is_super_admin: boolean;
  email_verified: boolean;
  roles?: RoleSummary[];
  auth_methods?: UserAuthMethod[];
  created_at?: string;
};

export type PlatformUserDetail = PublicUser & {
  roles: RoleSummary[];
  auth_methods: UserAuthMethod[];
};

export type CreatePlatformUserRequest = {
  email: string;
  password: string;
  name: string;
  surname: string;
  status?: string;
  role_uuids?: string[];
};

export type PatchPlatformUserRequest = {
  name?: string;
  surname?: string;
  status?: string;
  role_uuids?: string[];
};

export type SetPlatformUserPasswordRequest = {
  password: string;
};

export type SessionSwitch = {
  user_uuid: string;
  email: string;
  impersonator_uuid?: string;
};

export type ImpersonationResult = {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
  refresh_expires_at: string;
  session: SessionSwitch;
};

export type ResourceMeta = {
  resource?: string;
  capabilities?: {
    create?: boolean;
    read?: boolean;
    update?: boolean;
    delete?: boolean;
    search?: boolean;
    filter?: boolean;
    sort?: boolean;
    export?: boolean;
    import?: boolean;
    bulk?: boolean;
  };
  sortable?: string[];
  filterable?: string[];
};

export type StatusPayload = {
  status: string;
};

export type UserListResult = {
  items: PublicUser[];
  total: number;
  limit: number;
  offset: number;
};

export type ListUsersParams = ServerListParams & {
  status?: string;
  role?: string;
};

export type UserStatus = "active" | "pending" | "disabled";

export const usersService = {
  async list(params: ListUsersParams) {
    return platformRequest<UserListResult>("GET", "/v1/platform/users", {
      query: {
        limit: params.limit,
        offset: params.offset,
        sort: params.sort,
        q: params.q,
        status: params.status,
        role: params.role,
      },
    });
  },

  async meta() {
    return platformRequest<ResourceMeta>("GET", "/v1/platform/users/meta");
  },

  async get(uuid: string) {
    return platformRequest<PlatformUserDetail>(
      "GET",
      `/v1/platform/users/${uuid}`,
    );
  },

  async create(body: CreatePlatformUserRequest) {
    return platformRequest<PublicUser>("POST", "/v1/platform/users", { body });
  },

  async update(uuid: string, body: PatchPlatformUserRequest) {
    return platformRequest<PublicUser>("PATCH", `/v1/platform/users/${uuid}`, {
      body,
    });
  },

  async setPassword(uuid: string, body: SetPlatformUserPasswordRequest) {
    return platformRequest<StatusPayload>(
      "POST",
      `/v1/platform/users/${uuid}/password`,
      { body },
    );
  },

  async impersonate(uuid: string) {
    return platformRequest<ImpersonationResult>(
      "POST",
      `/v1/platform/users/${uuid}/impersonate`,
    );
  },
};
