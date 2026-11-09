import type { ServerListQuery } from "@/components/entity";
import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";

type Schemas = components["schemas"];

export type PendingRecipient = Schemas["PendingRecipient"];
export type PendingRecipientCreateRequest =
  Schemas["PendingRecipientCreateRequest"];
export type PendingRecipientConvertRequest =
  Schemas["PendingRecipientConvertRequest"];
export type PendingRecipientConvertResult =
  Schemas["PendingRecipientConvertResult"];
export type PendingRecipientKind = PendingRecipient["kind"];
export type PendingRecipientState = PendingRecipient["state"];

export const RECIPIENT_KINDS: PendingRecipientKind[] = [
  "dealer_candidate",
  "retail_point",
];
export const RECIPIENT_STATES: PendingRecipientState[] = [
  "pending",
  "retail",
  "converted",
];

export type RecipientPage = {
  items: PendingRecipient[];
  total: number;
  limit: number;
  offset: number;
};

export const pendingRecipientKeys = {
  all: ["accounts", "pending-recipients"] as const,
  list: (params: unknown) =>
    ["accounts", "pending-recipients", "list", params] as const,
};

/**
 * TEC-1062: the center's waiting pool of order recipients. A recipient is
 * opened from the order form without a dealer record (a dealer candidate or
 * a retail delivery point); the center later turns it into a dealer (with
 * its owner) or keeps it as a retail point.
 */
export const pendingRecipientsService = {
  list(params: ServerListQuery & { state?: string }) {
    return platformRequest<RecipientPage>(
      "GET",
      "/v1/tenant/pending-recipients",
      { query: params },
    );
  },
  create(body: PendingRecipientCreateRequest) {
    return platformRequest<PendingRecipient>(
      "POST",
      "/v1/tenant/pending-recipients",
      { body },
    );
  },
  convert(uuid: string, body: PendingRecipientConvertRequest) {
    return platformRequest<PendingRecipientConvertResult>(
      "POST",
      `/v1/tenant/pending-recipients/${encodeURIComponent(uuid)}/convert`,
      { body },
    );
  },
  keepRetail(uuid: string) {
    return platformRequest<PendingRecipient>(
      "POST",
      `/v1/tenant/pending-recipients/${encodeURIComponent(uuid)}/keep-retail`,
    );
  },
};
