import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type TenantMemberCreateRequest = Schemas["TenantMemberCreateRequest"];
export type TenantDealerCreateRequest = Schemas["TenantDealerCreateRequest"];
export type TenantMember = Schemas["TenantMember"];
export type TenantDealerResult = Schemas["TenantDealerResult"];

/**
 * Phone-first account opening of the active organization: staff with their
 * warehouses (members.write) and a dealer with its owner directly under the
 * center (organizations.write). The phone is the mobile OTP sign-in.
 */
export const accountsService = {
  createMember(body: TenantMemberCreateRequest) {
    return platformRequest<TenantMember>("POST", "/v1/tenant/members", {
      body,
    });
  },
  createDealer(body: TenantDealerCreateRequest) {
    return platformRequest<TenantDealerResult>("POST", "/v1/tenant/dealers", {
      body,
    });
  },
};
