import { describe, expect, it } from "vitest";

import type {
  ServiceItem,
  ServiceStatusLog,
} from "@/features/services/services/service-wizard.service";

import {
  itemAmount,
  serviceStatusTone,
  sortedStatusLogs,
  warrantyTone,
} from "./detail";

const item = (over: Partial<ServiceItem>) =>
  ({ kind: "full", meters: null, quantity: null, ...over }) as ServiceItem;

describe("service detail helpers (TEC-183)", () => {
  it("formats the item amount", () => {
    expect(itemAmount(item({ kind: "partial", meters: "2.50" }))).toEqual({
      key: "services.stock.amount_meters",
      params: { meters: "2.50" },
    });
    expect(itemAmount(item({ quantity: 3 }))).toEqual({
      key: "services.stock.amount_pieces",
      params: { count: 3 },
    });
    expect(itemAmount(item({}))).toEqual({
      key: "services.stock.amount_whole",
    });
  });

  it("sorts the status history newest first without mutating", () => {
    const logs = [
      { to_status: "draft", created_at: "2026-10-01T08:00:00Z" },
      { to_status: "completed", created_at: "2026-10-01T10:00:00Z" },
      { to_status: "pending", created_at: "2026-10-01T09:00:00Z" },
    ] as ServiceStatusLog[];
    expect(sortedStatusLogs(logs).map((l) => l.to_status)).toEqual([
      "completed",
      "pending",
      "draft",
    ]);
    expect(logs[0].to_status).toBe("draft");
    expect(sortedStatusLogs(undefined)).toEqual([]);
  });

  it("maps tones", () => {
    expect(serviceStatusTone("cancelled")).toBe("danger");
    expect(serviceStatusTone("completed")).toBe("success");
    expect(warrantyTone("active")).toBe("success");
    expect(warrantyTone("void")).toBe("danger");
    expect(warrantyTone("expired")).toBe("default");
  });
});
