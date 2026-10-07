import { afterEach, describe, expect, it, vi } from "vitest";

import { measurementsService } from "@/features/measurements/services/measurements.service";
import { ApiError } from "@/lib/api";

afterEach(() => vi.unstubAllGlobals());

describe("measurement PDF download (TEC-299)", () => {
  it("polls the 202 render answer until the PDF streams", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ success: true, data: { status: "pending" } }),
          { status: 202, headers: { "Retry-After": "3" } },
        ),
      )
      .mockResolvedValueOnce(
        new Response("%PDF-1.7", {
          status: 200,
          headers: {
            "Content-Type": "application/pdf",
            "Content-Disposition": 'inline; filename="olcum.pdf"',
          },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);
    const wait = vi.fn(async () => undefined);

    const out = await measurementsService.downloadPdf("m-1", wait);

    expect(fetchMock).toHaveBeenCalledTimes(2);
    expect(String(fetchMock.mock.calls[0][0])).toMatch(
      /\/v1\/measurements\/m-1\/pdf$/,
    );
    expect(wait).toHaveBeenCalledWith(3000);
    expect(out.filename).toBe("olcum.pdf");
    expect(await out.blob.text()).toBe("%PDF-1.7");
  });

  it("throws the API error of a vin_pending measurement", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            success: false,
            error: { code: "MEASUREMENT_VIN_PENDING", message: "VIN needed" },
          }),
          { status: 422, headers: { "Content-Type": "application/json" } },
        ),
      ),
    );
    const err = await measurementsService
      .downloadPdf("m-2", async () => undefined)
      .catch((e: unknown) => e);
    expect(err).toBeInstanceOf(ApiError);
    expect((err as ApiError).code).toBe("MEASUREMENT_VIN_PENDING");
  });
});
