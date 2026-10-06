import { beforeEach, describe, expect, it, vi } from "vitest";

const request = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/platform-request", () => ({ platformRequest: request }));

import {
  ANNOUNCEMENT_READS_PAGE_SIZE,
  announcementsService,
} from "./announcements.service";

function reader(i: number) {
  return {
    user_uuid: `u-${i}`,
    email: `u${i}@example.com`,
    name: "U",
    surname: String(i),
    read_at: "2026-10-04T09:00:00Z",
  };
}

beforeEach(() => request.mockReset());

describe("announcementsService.readsAll", () => {
  it("pages through read_total with limit 500 (no 100 clamp)", async () => {
    const total = ANNOUNCEMENT_READS_PAGE_SIZE + 20;
    request.mockImplementation(
      async (_m: string, _p: string, opts?: { query: { offset: number } }) => {
        const offset = opts?.query.offset ?? 0;
        const size = Math.min(ANNOUNCEMENT_READS_PAGE_SIZE, total - offset);
        return {
          target_total: 900,
          read_total: total,
          read_rate: total / 900,
          limit: ANNOUNCEMENT_READS_PAGE_SIZE,
          offset,
          items: Array.from({ length: size }, (_, i) => reader(offset + i)),
        };
      },
    );

    const report = await announcementsService.readsAll("a-1");

    expect(request).toHaveBeenCalledTimes(2);
    expect(request.mock.calls[0]).toEqual([
      "GET",
      "/v1/announcements/a-1/reads",
      { query: { limit: 500, offset: 0 } },
    ]);
    expect(request.mock.calls[1][2]).toEqual({
      query: { limit: 500, offset: 500 },
    });
    expect(report.items).toHaveLength(total);
    expect(report.read_total).toBe(total);
  });

  it("sends the manage filters as given", async () => {
    request.mockResolvedValue({ items: [], total: 0, limit: 20, offset: 0 });
    await announcementsService.manage({
      limit: 20,
      offset: 0,
      sort: "title",
      status: "draft,archived",
      pinned: "true",
      publish_from: "2026-10-01",
    });
    expect(request).toHaveBeenCalledWith("GET", "/v1/announcements/manage", {
      query: {
        limit: 20,
        offset: 0,
        sort: "title",
        status: "draft,archived",
        pinned: "true",
        publish_from: "2026-10-01",
      },
    });
  });
});
