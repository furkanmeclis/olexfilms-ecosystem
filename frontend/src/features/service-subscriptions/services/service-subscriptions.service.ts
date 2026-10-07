import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type ServiceSubscription = Schemas["ServiceSubscription"];
export type ServiceSubscriptionStatus = Schemas["ServiceSubscriptionStatus"];
export type ServiceSubscriptionInput = Schemas["ServiceSubscriptionInput"];
export type ServiceSubscriptionCancelRequest =
  Schemas["ServiceSubscriptionCancelRequest"];
export type CancelQueueItem = Schemas["ServiceSubscriptionCancelQueueItem"];
export type CancelRequestStatus = ServiceSubscriptionCancelRequest["status"];
export type PricePreview = Schemas["ServiceSubscriptionPricePreview"];

export type TargetOrganization = {
  uuid: string;
  name: string;
  type: string;
};

export type Page<T> = {
  items: T[];
  total: number;
  limit: number;
  offset: number;
};

/** List query: limit/offset/sort/q plus the column filter params. */
export type SubscriptionListQuery = Record<string, string | number | undefined>;

const enc = encodeURIComponent;

export const SUBSCRIPTION_STATUSES = [
  "active",
  "cancel_requested",
  "cancelled",
  "expired",
] as const satisfies readonly ServiceSubscriptionStatus[];

export const CANCEL_REQUEST_STATUSES = [
  "pending",
  "approved",
  "rejected",
] as const satisfies readonly CancelRequestStatus[];

export const serviceSubscriptionKeys = {
  all: ["service-subscriptions"] as const,
  list: (params: SubscriptionListQuery) =>
    [...serviceSubscriptionKeys.all, "list", params] as const,
  detail: (uuid: string) =>
    [...serviceSubscriptionKeys.all, "detail", uuid] as const,
  cancelRequests: (params: SubscriptionListQuery) =>
    [...serviceSubscriptionKeys.all, "cancel-requests", params] as const,
  organizations: () =>
    [...serviceSubscriptionKeys.all, "organizations"] as const,
  items: () => [...serviceSubscriptionKeys.all, "items"] as const,
  preview: (item: string, org: string) =>
    [...serviceSubscriptionKeys.all, "preview", item, org] as const,
};

export const serviceSubscriptionsService = {
  list(params: SubscriptionListQuery) {
    return platformRequest<Page<ServiceSubscription>>(
      "GET",
      "/v1/service-subscriptions",
      { query: params },
    );
  },

  get(uuid: string) {
    return platformRequest<ServiceSubscription>(
      "GET",
      `/v1/service-subscriptions/${enc(uuid)}`,
    );
  },

  assign(body: ServiceSubscriptionInput) {
    return platformRequest<ServiceSubscription>(
      "POST",
      "/v1/service-subscriptions",
      { body },
    );
  },

  previewPrice(itemUuid: string, organizationUuid: string) {
    return platformRequest<PricePreview>(
      "GET",
      "/v1/service-subscriptions/price-preview",
      { query: { item_uuid: itemUuid, organization_uuid: organizationUuid } },
    );
  },

  requestCancel(uuid: string, reason: string) {
    return platformRequest<ServiceSubscriptionCancelRequest>(
      "POST",
      `/v1/service-subscriptions/${enc(uuid)}/cancel-request`,
      { body: { reason } },
    );
  },

  listCancelRequests(params: SubscriptionListQuery) {
    return platformRequest<Page<CancelQueueItem>>(
      "GET",
      "/v1/service-subscriptions/cancel-requests",
      { query: params },
    );
  },

  approveCancel(uuid: string, note?: string) {
    return platformRequest<ServiceSubscriptionCancelRequest>(
      "POST",
      `/v1/service-subscriptions/cancel-requests/${enc(uuid)}/approve`,
      { body: { note: note?.trim() || null } },
    );
  },

  rejectCancel(uuid: string, note: string) {
    return platformRequest<ServiceSubscriptionCancelRequest>(
      "POST",
      `/v1/service-subscriptions/cancel-requests/${enc(uuid)}/reject`,
      { body: { note: note.trim() } },
    );
  },

  /** Organizations in the organizations.read scope (filters, targets). */
  async listOrganizations(): Promise<TargetOrganization[]> {
    const data = await platformRequest<{
      items: { uuid: string; name: string; type: string }[];
    }>("GET", "/v1/tenant/organizations", { query: { limit: 200 } });
    return (data.items ?? []).map((o) => ({
      uuid: o.uuid,
      name: o.name,
      type: o.type,
    }));
  },
};
