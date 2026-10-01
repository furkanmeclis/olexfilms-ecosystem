import type { ServerListParams } from "@/components/entity";
import { apiConfig } from "@/config/api";
import { platformFormRequest } from "@/lib/api/platform-form-request";
import { apiClient, platformRequest, unwrap } from "@/lib/api";
import type { OrganizationSummary, OrganizationType } from "@/lib/auth/types";

export type { OrganizationSummary, OrganizationType };

/** pending/expired are legacy statuses kept for compatibility. */
export type OrganizationStatus =
  "pending" | "active" | "read_only" | "suspended" | "expired";

export type OrganizationParentRef = {
  uuid: string;
  slug?: string;
  name: string;
};

export type PublicOrganization = {
  uuid: string;
  slug: string;
  name: string;
  status: string;
  logo_url?: string | null;
  access_ok: boolean;
};

export type Organization = {
  uuid: string;
  slug: string;
  name: string;
  city: string;
  district: string;
  phone: string;
  address: string;
  status: OrganizationStatus;
  plan_code?: string | null;
  access_starts_at: string;
  access_ends_at?: string | null;
  logo_url?: string | null;
  type: OrganizationType;
  brand: { slug: string; name?: string };
  parent?: OrganizationParentRef | null;
  currency: string;
  locale: string;
  timezone: string;
  contract_valid_until?: string | null;
  settings: Record<string, unknown>;
  created_at: string;
  updated_at: string;
};

export type OrganizationMember = {
  uuid: string;
  email: string;
  name: string;
  surname: string;
  status: string;
  role: string;
  created_at?: string;
};

export type OrganizationDetail = {
  organization: Organization;
  members: OrganizationMember[];
};

export type OrganizationListResult = {
  items: Organization[];
  total: number;
  limit: number;
  offset: number;
};

export type ListOrganizationsParams = ServerListParams & {
  status?: string;
  type?: OrganizationType;
  parent_uuid?: string;
};

export type CreatePlatformOrganizationRequest = {
  name: string;
  city: string;
  district: string;
  phone: string;
  address: string;
  owner_user_uuid: string;
  type?: Exclude<OrganizationType, "center">;
  parent_uuid?: string | null;
  register_as_warehouse?: boolean;
  currency?: string;
  locale?: string;
  timezone?: string;
};

export type PatchPlatformOrganizationRequest = {
  name?: string;
  city?: string;
  district?: string;
  phone?: string;
  address?: string;
  status?: OrganizationStatus;
  plan_code?: string;
  access_ends_at?: string;
  clear_access_ends_at?: boolean;
  currency?: string;
  locale?: string;
  timezone?: string;
  /** Moves the organization in the tree (platform only). */
  parent_uuid?: string;
};

export type OrganizationRegisterInput = {
  name: string;
  surname: string;
  email: string;
  password: string;
  organization_name: string;
  city: string;
  district: string;
  phone: string;
  address: string;
};

export type OrganizationRegisterResult = {
  user: {
    uuid: string;
    email: string;
    name: string;
    surname: string;
  };
  organization: Organization;
  tokens: {
    access_token: string;
    refresh_token: string;
    expires_in: number;
  };
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
};

async function publicRequest<T>(method: string, path: string, body?: unknown) {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const response = await fetch(`${base}${path}`, {
    method,
    credentials: "include",
    headers: {
      Accept: "application/json",
      ...(body ? { "Content-Type": "application/json" } : {}),
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const payload = await response.json().catch(() => undefined);
  return unwrap<T>({ data: payload, response });
}

export const organizationsService = {
  async register(body: OrganizationRegisterInput) {
    return publicRequest<OrganizationRegisterResult>(
      "POST",
      "/v1/public/organizations/register",
      body,
    );
  },

  async getPublicBySlug(slug: string) {
    return publicRequest<PublicOrganization>(
      "GET",
      `/v1/public/organizations/by-slug/${encodeURIComponent(slug)}`,
    );
  },

  async list(params: ListOrganizationsParams) {
    return platformRequest<OrganizationListResult>(
      "GET",
      "/v1/platform/organizations",
      {
        query: {
          limit: params.limit,
          offset: params.offset,
          sort: params.sort,
          q: params.q,
          status: params.status,
          type: params.type,
          parent_uuid: params.parent_uuid,
        },
      },
    );
  },

  /** Caller's memberships within the domain's brand (org switcher). */
  async listMine() {
    const data = await unwrap<{ items: OrganizationSummary[] }>(
      await apiClient.GET("/v1/me/organizations"),
    );
    return data.items ?? [];
  },

  async children(uuid: string) {
    return platformRequest<{ items: Organization[] }>(
      "GET",
      `/v1/platform/organizations/${uuid}/children`,
    );
  },

  async meta() {
    return platformRequest<ResourceMeta>(
      "GET",
      "/v1/platform/organizations/meta",
    );
  },

  async get(uuid: string) {
    return platformRequest<OrganizationDetail>(
      "GET",
      `/v1/platform/organizations/${uuid}`,
    );
  },

  async create(body: CreatePlatformOrganizationRequest) {
    return platformRequest<Organization>("POST", "/v1/platform/organizations", {
      body,
    });
  },

  async update(uuid: string, body: PatchPlatformOrganizationRequest) {
    return platformRequest<Organization>(
      "PATCH",
      `/v1/platform/organizations/${uuid}`,
      { body },
    );
  },

  async uploadLogo(uuid: string, file: File) {
    const form = new FormData();
    form.append("logo", file);
    return platformFormRequest<Organization>(
      "PUT",
      `/v1/platform/organizations/${uuid}/logo`,
      form,
    );
  },

  async deleteLogo(uuid: string) {
    return platformRequest<Organization>(
      "DELETE",
      `/v1/platform/organizations/${uuid}/logo`,
    );
  },

  async addMember(uuid: string, body: { user_uuid: string; role?: string }) {
    return platformRequest<{ status: string }>(
      "POST",
      `/v1/platform/organizations/${uuid}/members`,
      { body },
    );
  },
};
