import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("next-auth/react", () => ({ signIn: vi.fn() }));

import { fetchQRStatus, normalizeStatus } from "./qr-login-panel";

describe("QR sign-in status (TEC-91)", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("accepts only the known statuses", () => {
    expect(normalizeStatus("scanned")).toBe("scanned");
    expect(normalizeStatus("approved")).toBe("approved");
    expect(normalizeStatus("token")).toBe("error");
    expect(normalizeStatus(undefined)).toBe("error");
  });

  it("polls the panel BFF and maps 410 to expired", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ success: true, data: { status: "scanned" } }),
          { status: 200 },
        ),
      )
      .mockResolvedValueOnce(new Response("{}", { status: 410 }));
    vi.stubGlobal("fetch", fetchMock);

    expect(await fetchQRStatus("abc")).toBe("scanned");
    expect(await fetchQRStatus("abc")).toBe("expired");
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/auth/qr/abc/status");
  });
});
