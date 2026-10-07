import { beforeEach, describe, expect, it, vi } from "vitest";

const request = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/platform-request", () => ({ platformRequest: request }));

import { serviceSubscriptionsService } from "@/features/service-subscriptions/services/service-subscriptions.service";

beforeEach(() => {
  request.mockReset();
  request.mockResolvedValue({ items: [] });
});

describe("serviceSubscriptionsService (TEC-311)", () => {
  it("sends queue decisions to the approve / reject endpoints", async () => {
    await serviceSubscriptionsService.approveCancel("req-1");
    expect(request).toHaveBeenLastCalledWith(
      "POST",
      "/v1/service-subscriptions/cancel-requests/req-1/approve",
      { body: { note: null } },
    );
    await serviceSubscriptionsService.approveCancel("req-1", " ok ");
    expect(request).toHaveBeenLastCalledWith(
      "POST",
      "/v1/service-subscriptions/cancel-requests/req-1/approve",
      { body: { note: "ok" } },
    );
    await serviceSubscriptionsService.rejectCancel("req-2", " no budget ");
    expect(request).toHaveBeenLastCalledWith(
      "POST",
      "/v1/service-subscriptions/cancel-requests/req-2/reject",
      { body: { note: "no budget" } },
    );
  });

  it("lists the queue and requests cancellation on the subscription", async () => {
    await serviceSubscriptionsService.listCancelRequests({
      status: "pending",
      limit: 20,
    });
    expect(request).toHaveBeenLastCalledWith(
      "GET",
      "/v1/service-subscriptions/cancel-requests",
      { query: { status: "pending", limit: 20 } },
    );
    await serviceSubscriptionsService.requestCancel("sub-1", "reason");
    expect(request).toHaveBeenLastCalledWith(
      "POST",
      "/v1/service-subscriptions/sub-1/cancel-request",
      { body: { reason: "reason" } },
    );
    await serviceSubscriptionsService.previewPrice("item-1", "org-1");
    expect(request).toHaveBeenLastCalledWith(
      "GET",
      "/v1/service-subscriptions/price-preview",
      { query: { item_uuid: "item-1", organization_uuid: "org-1" } },
    );
  });
});
