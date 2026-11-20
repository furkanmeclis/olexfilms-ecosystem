import { describe, expect, it } from "vitest";

import {
  isIpAddress,
  parseIpList,
} from "@/features/integrations/glorian/components/glorian-connection-package";

describe("connection package IP list (TEC-1265)", () => {
  it("splits on commas, spaces and new lines", () => {
    expect(parseIpList(" 203.0.113.10,\n198.51.100.2  2001:db8::1 ")).toEqual([
      "203.0.113.10",
      "198.51.100.2",
      "2001:db8::1",
    ]);
    expect(parseIpList("")).toEqual([]);
  });

  it("flags addresses that are not IPs", () => {
    expect(isIpAddress("203.0.113.10")).toBe(true);
    expect(isIpAddress("2001:db8::1")).toBe(true);
    expect(isIpAddress("999.1.1.1")).toBe(false);
    expect(isIpAddress("glorian.app")).toBe(false);
  });
});
